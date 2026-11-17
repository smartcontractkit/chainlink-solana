//! Feed admin settings of data IDs (remaining accounts: their feed PDAs, in order). A feed that
//! doesn't exist yet is created first, the feed admin paying its rent: on Solana a feed is set by a
//! feed admin, as on Aptos and Stellar, since the forwarder passes no account that could pay for
//! it.

use std::mem::size_of;

use anchor_lang::{prelude::*, Discriminator};

use crate::{
    common::{
        ACCOUNT_VERSION, ANCHOR_DISCRIMINATOR, DF1_LINK_SEED, FEED_SEED, MAX_ACCOUNT_DATA_INCREASE,
        STORED_DECIMALS,
    },
    context::FeedAdminAccounts,
    data_id, df1,
    error::StoreError,
    event::*,
    logic::{check_max_stored_reports, MaxStoredReportsError},
    state::{feed_account_len, Df1Link, Feed, WorkflowMetadata},
    utils::{
        create_pda_account, df1_link_address, encode_description, feed_address, is_uninitialized,
        load_feed_into, next_account, store_feed, validate_data_id, validate_workflows,
        verify_feed_admin, EventEmitter,
    },
};

/// Check the admin, create the feeds that don't exist yet, then run `f` on each feed's header and
/// write it back. One header buffer and one event emitter serve the batch: the bump allocator never
/// frees. `FeedsCreated` comes first, so indexers applying events in order see a feed created
/// before its settings; the emitter is returned for events covering the whole call.
fn update_feeds_with_events<'info>(
    ctx: &Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: &[[u8; 16]],
    feeds: &[AccountInfo<'info>],
    mut f: impl FnMut(usize, &[u8; 16], &mut Feed, &EventEmitter<'info>) -> Result<()>,
) -> Result<EventEmitter<'info>> {
    let (max_stored_reports, state_key) = {
        let state = ctx.accounts.state.load()?;
        verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;
        (state.default_max_stored_reports, ctx.accounts.state.key())
    };
    require_eq!(data_ids.len(), feeds.len(), StoreError::ArrayLengthMismatch);
    let events = EventEmitter::new(
        ctx.accounts.event_authority.to_account_info(),
        ctx.bumps.event_authority,
    );
    let payer = ctx.accounts.feed_admin.to_account_info();
    let system_program = ctx.accounts.system_program.to_account_info();
    // one event for every feed created: each event is an instruction, and a transaction holds at
    // most 64 (a data ID listed twice is created once)
    let mut created: Vec<[u8; 16]> = Vec::new();
    for (id, info) in data_ids.iter().zip(feeds.iter()) {
        if is_uninitialized(info) && !created.contains(id) {
            created.push(*id);
        }
    }
    if !created.is_empty() {
        events.emit(&FeedsCreated {
            state: state_key,
            data_ids: created,
            max_stored_reports,
        })?;
    }
    let mut feed: Box<Feed> = bytemuck::zeroed_box();
    for (i, (id, info)) in data_ids.iter().zip(feeds.iter()).enumerate() {
        validate_data_id(id)?;
        if is_uninitialized(info) {
            let (address, bump) = feed_address(&state_key, id);
            require_keys_eq!(address, *info.key, StoreError::AccountMismatch);
            let seed = data_id::to_seed(id);
            create_pda_account(
                feed_account_len(max_stored_reports),
                &[FEED_SEED, state_key.as_ref(), &seed, &[bump]],
                &payer,
                info,
                &system_program,
            )?;
            bytemuck::bytes_of_mut(&mut *feed).fill(0);
            feed.version = ACCOUNT_VERSION;
            feed.min_decimals = STORED_DECIMALS;
            feed.data_id = seed;
            feed.state = state_key;
            feed.max_stored_reports = max_stored_reports;
        } else {
            load_feed_into(info, &state_key, id, &mut feed)?;
        }
        f(i, id, &mut feed, &events)?;
        store_feed(info, &feed)?;
    }
    Ok(events)
}

/// `update_feeds_with_events` for setters that emit only per-feed events.
fn update_feeds<'info>(
    ctx: &Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: &[[u8; 16]],
    feeds: &[AccountInfo<'info>],
    f: impl FnMut(usize, &[u8; 16], &mut Feed, &EventEmitter<'info>) -> Result<()>,
) -> Result<()> {
    update_feeds_with_events(ctx, data_ids, feeds, f).map(|_| ())
}

/// Replace a feed's own workflow list, zeroing the slots a shorter list leaves (the layout is
/// public, so removed workflows shouldn't linger in it).
fn set_specific(feed: &mut Feed, workflow_metadata: &[WorkflowMetadata]) {
    feed.specific_workflow_metadata = bytemuck::Zeroable::zeroed();
    feed.specific_workflow_metadata.extend(workflow_metadata);
}

/// One SpecificFeedMetadataSet for every data ID of the call, written from the borrowed lists.
fn emit_specific_set(
    events: &EventEmitter<'_>,
    state: &Pubkey,
    data_ids: &[[u8; 16]],
    workflow_metadata: &[WorkflowMetadata],
) -> Result<()> {
    events.emit_with(SpecificFeedMetadataSet::DISCRIMINATOR, |data| {
        state.serialize(data)?;
        data_ids.serialize(data)?;
        workflow_metadata.serialize(data)?;
        Ok(())
    })
}

/// FeedDescriptionSet, written from the borrowed description instead of a copy.
fn emit_description_set(
    events: &EventEmitter<'_>,
    state: &Pubkey,
    data_id: &[u8; 16],
    description: &str,
) -> Result<()> {
    events.emit_with(FeedDescriptionSet::DISCRIMINATOR, |data| {
        state.serialize(data)?;
        data_id.serialize(data)?;
        description.serialize(data)?;
        Ok(())
    })
}

/// Set feeds up, creating those that don't exist yet: their descriptions and, unless
/// `workflow_metadata` is empty, the workflows allowed to write them (the same list for every feed
/// of the call). With an empty list the feeds keep their feed-specific workflows, or the store's
/// default ones.
pub fn set_feeds<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
    descriptions: Vec<String>,
    workflow_metadata: Vec<WorkflowMetadata>,
) -> Result<()> {
    require_eq!(
        data_ids.len(),
        descriptions.len(),
        StoreError::ArrayLengthMismatch
    );
    validate_workflows(&workflow_metadata)?;
    let state = ctx.accounts.state.key();
    let events = update_feeds_with_events(
        &ctx,
        &data_ids,
        ctx.remaining_accounts,
        |i, id, feed, events| {
            feed.description = encode_description(&descriptions[i])?;
            emit_description_set(events, &state, id, &descriptions[i])?;
            if !workflow_metadata.is_empty() {
                set_specific(feed, &workflow_metadata);
            }
            Ok(())
        },
    )?;
    if !workflow_metadata.is_empty() {
        emit_specific_set(&events, &state, &data_ids, &workflow_metadata)?;
    }
    Ok(())
}

/// Set feed descriptions, e.g. "ETH / USD", an empty string clears one.
pub fn set_feed_descriptions<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
    descriptions: Vec<String>,
) -> Result<()> {
    require_eq!(
        data_ids.len(),
        descriptions.len(),
        StoreError::ArrayLengthMismatch
    );
    let state = ctx.accounts.state.key();
    update_feeds(
        &ctx,
        &data_ids,
        ctx.remaining_accounts,
        |i, id, feed, events| {
            feed.description = encode_description(&descriptions[i])?;
            emit_description_set(events, &state, id, &descriptions[i])
        },
    )
}

/// Set the workflows allowed to write the given data IDs, replacing any previous feed-specific
/// metadata. Data IDs without feed-specific metadata use the store's default metadata.
pub fn set_specific_feed_metadata<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
    workflow_metadata: Vec<WorkflowMetadata>,
) -> Result<()> {
    require!(
        !data_ids.is_empty() && !workflow_metadata.is_empty(),
        StoreError::EmptyConfig
    );
    validate_workflows(&workflow_metadata)?;
    let state = ctx.accounts.state.key();
    let events =
        update_feeds_with_events(&ctx, &data_ids, ctx.remaining_accounts, |_, _, feed, _| {
            set_specific(feed, &workflow_metadata);
            Ok(())
        })?;
    emit_specific_set(&events, &state, &data_ids, &workflow_metadata)
}

/// Remove feed-specific workflow metadata, the data IDs fall back to the default metadata.
pub fn remove_specific_feed_metadata<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
) -> Result<()> {
    let state = ctx.accounts.state.key();
    update_feeds(
        &ctx,
        &data_ids,
        ctx.remaining_accounts,
        |_, id, feed, events| {
            require!(
                !feed.specific_workflow_metadata.is_empty(),
                StoreError::FeedNotConfigured
            );
            feed.specific_workflow_metadata = bytemuck::Zeroable::zeroed();
            events.emit(&SpecificFeedMetadataRemoved {
                state,
                data_id: *id,
            })
        },
    )
}

/// Set the accepted answer range of feeds, the (i128::MIN, i128::MAX) pair clears the range.
pub fn set_min_max_answers<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
    min_answers: Vec<i128>,
    max_answers: Vec<i128>,
) -> Result<()> {
    require!(
        data_ids.len() == min_answers.len() && data_ids.len() == max_answers.len(),
        StoreError::ArrayLengthMismatch
    );
    let state = ctx.accounts.state.key();
    update_feeds(
        &ctx,
        &data_ids,
        ctx.remaining_accounts,
        |i, id, feed, events| {
            let (min_answer, max_answer) = (min_answers[i], max_answers[i]);
            if min_answer == i128::MIN && max_answer == i128::MAX {
                // unbounded pair, same as never configured
                feed.min_max_configured = 0;
                feed.min_answer = 0;
                feed.max_answer = 0;
            } else {
                require!(min_answer <= max_answer, StoreError::InvalidMinMaxRange);
                feed.min_max_configured = 1;
                feed.min_answer = min_answer;
                feed.max_answer = max_answer;
            }
            events.emit(&MinMaxAnswerSet {
                state,
                data_id: *id,
                min_answer,
                max_answer,
            })
        },
    )
}

/// Disable or enable writes of feeds: while disabled, every workflow's reports for the data ID are
/// skipped (default and feed-specific alike) and reads keep serving the last round.
pub fn set_feed_writes_disabled<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
    disabled: bool,
) -> Result<()> {
    let state = ctx.accounts.state.key();
    update_feeds(
        &ctx,
        &data_ids,
        ctx.remaining_accounts,
        |_, id, feed, events| {
            feed.writes_disabled = disabled as u8;
            events.emit(&FeedWritesDisabledSet {
                state,
                data_id: *id,
                disabled,
            })
        },
    )
}

/// Freeze or unfreeze round reads of feeds: frozen feeds refuse `latest_round` and `get_round`,
/// their description and decimals stay readable, and writes continue.
pub fn set_feed_frozen<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
    frozen: bool,
) -> Result<()> {
    let state = ctx.accounts.state.key();
    update_feeds(
        &ctx,
        &data_ids,
        ctx.remaining_accounts,
        |_, id, feed, events| {
            feed.frozen = frozen as u8;
            events.emit(&FeedFrozenSet {
                state,
                data_id: *id,
                frozen,
            })
        },
    )
}

/// Set the fewest decimals reads of feeds may ask for; new feeds start at the stored decimals.
pub fn set_min_decimals<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
    min_decimals: u8,
) -> Result<()> {
    require!(min_decimals <= STORED_DECIMALS, StoreError::InvalidDecimals);
    let state = ctx.accounts.state.key();
    update_feeds(
        &ctx,
        &data_ids,
        ctx.remaining_accounts,
        |_, id, feed, events| {
            feed.min_decimals = min_decimals;
            events.emit(&MinDecimalsSet {
                state,
                data_id: *id,
                min_decimals,
            })
        },
    )
}

/// Link data IDs to the DF1 feeds this store keeps written with their rounds (default pubkey
/// unlinks), and enable or disable that write. A DF1 feed is linked from at most one data ID: it
/// has a marker PDA `["df1_link", state, df1_feed]` holding the data ID that links it, created on
/// its first link (the feed admin paying, about 0.001 SOL) and cleared, not closed, when the link
/// is removed. Reports never read the marker. Moving a DF1 feed between data IDs in one call needs
/// the unlink first.
///
/// Remaining accounts: the feeds, then for each data ID in order: the marker of its current DF1
/// feed if the link changes away from it, then the new DF1 feed and its marker if one is linked.
pub fn set_df1_feeds<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
    df1_feeds: Vec<Pubkey>,
    write_disabled: Vec<bool>,
) -> Result<()> {
    require!(
        data_ids.len() == df1_feeds.len() && data_ids.len() == write_disabled.len(),
        StoreError::ArrayLengthMismatch
    );
    require_gte!(
        ctx.remaining_accounts.len(),
        data_ids.len(),
        StoreError::InvalidAccountCount
    );
    let (feeds, links) = ctx.remaining_accounts.split_at(data_ids.len());
    let mut links = links.iter();
    let df1_program = ctx.accounts.state.load()?.df1_program;
    let state = ctx.accounts.state.key();
    let payer = ctx.accounts.feed_admin.to_account_info();
    let system_program = ctx.accounts.system_program.to_account_info();
    update_feeds(&ctx, &data_ids, feeds, |i, id, feed, events| {
        let (old, new) = (feed.df1_feed, df1_feeds[i]);
        if old != Pubkey::default() && old != new {
            unlink_df1_feed(next_account(&mut links)?, &state, &old, id)?;
        }
        if new != Pubkey::default() {
            let info = next_account(&mut links)?;
            require_keys_eq!(*info.key, new, StoreError::AccountMismatch);
            require!(
                df1::check_layout(info, &df1_program).is_ok(),
                StoreError::InvalidDf1Feed
            );
            link_df1_feed(
                next_account(&mut links)?,
                &state,
                &new,
                id,
                &payer,
                &system_program,
            )?;
        }
        feed.df1_feed = new;
        feed.df1_write_disabled = write_disabled[i] as u8;
        events.emit(&Df1FeedSet {
            state,
            data_id: *id,
            df1_feed: new,
            write_disabled: write_disabled[i],
        })
    })?;
    require!(links.next().is_none(), StoreError::InvalidAccountCount);
    Ok(())
}

/// Create a DF1 feed's link marker for a data ID, or check it already belongs to that data ID.
fn link_df1_feed<'info>(
    marker: &AccountInfo<'info>,
    state: &Pubkey,
    df1_feed: &Pubkey,
    id: &[u8; 16],
    payer: &AccountInfo<'info>,
    system_program: &AccountInfo<'info>,
) -> Result<()> {
    let (address, bump) = df1_link_address(state, df1_feed);
    require_keys_eq!(*marker.key, address, StoreError::AccountMismatch);
    let seed = data_id::to_seed(id);
    if is_uninitialized(marker) {
        create_pda_account(
            ANCHOR_DISCRIMINATOR + size_of::<Df1Link>(),
            &[DF1_LINK_SEED, state.as_ref(), df1_feed.as_ref(), &[bump]],
            payer,
            marker,
            system_program,
        )?;
    } else {
        let linked = linked_data_id(marker)?;
        require!(
            linked == [0; 32] || linked == seed,
            StoreError::Df1FeedAlreadyLinked
        );
    }
    write_link(marker, seed)
}

fn write_link(marker: &AccountInfo, data_id: [u8; 32]) -> Result<()> {
    let link = Df1Link {
        version: ACCOUNT_VERSION,
        _padding0: [0; 7],
        data_id,
    };
    let mut data = marker.try_borrow_mut_data()?;
    data[..ANCHOR_DISCRIMINATOR].copy_from_slice(Df1Link::DISCRIMINATOR);
    data[ANCHOR_DISCRIMINATOR..].copy_from_slice(bytemuck::bytes_of(&link));
    Ok(())
}

/// Clear a DF1 feed's link marker held by a data ID, so another data ID can link the DF1 feed. The
/// marker is kept rather than closed: closing moves lamports by hand, which the runtime refuses
/// once a later call to another program follows in the same instruction.
fn unlink_df1_feed(
    marker: &AccountInfo,
    state: &Pubkey,
    df1_feed: &Pubkey,
    id: &[u8; 16],
) -> Result<()> {
    let (address, _) = df1_link_address(state, df1_feed);
    require_keys_eq!(*marker.key, address, StoreError::AccountMismatch);
    require!(
        linked_data_id(marker)? == data_id::to_seed(id),
        StoreError::AccountMismatch
    );
    write_link(marker, [0; 32])
}

/// The data ID (as its 32-byte seed) a link marker of this store holds.
fn linked_data_id(marker: &AccountInfo) -> Result<[u8; 32]> {
    require_keys_eq!(*marker.owner, crate::ID, StoreError::InvalidAccount);
    require!(marker.is_writable, StoreError::AccountNotWritable);
    let data = marker.try_borrow_data()?;
    require!(
        data.len() == ANCHOR_DISCRIMINATOR + size_of::<Df1Link>()
            && &data[..ANCHOR_DISCRIMINATOR] == Df1Link::DISCRIMINATOR,
        StoreError::InvalidAccount
    );
    let link: &Df1Link = bytemuck::from_bytes(&data[ANCHOR_DISCRIMINATOR..]);
    require_eq!(
        link.version,
        ACCOUNT_VERSION,
        StoreError::InvalidAccountVersion
    );
    Ok(link.data_id)
}

/// Grow the number of rounds feeds keep: only up, only before a feed's ring wraps, by at most 10 KiB
/// per feed per instruction counted from its length when the instruction started (320 rounds for an
/// existing feed; a feed created in the call holds at most 278). Called through another program,
/// as an MCMS batch does, the runtime counts from the outer instruction's start, so grow each feed
/// once per outer instruction. The feed admin tops up the rent.
pub fn set_max_stored_reports<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
    max_stored_reports: Vec<u64>,
) -> Result<()> {
    require_eq!(
        data_ids.len(),
        max_stored_reports.len(),
        StoreError::ArrayLengthMismatch
    );
    let state = ctx.accounts.state.key();
    let payer = ctx.accounts.feed_admin.to_account_info();
    let system_program = ctx.accounts.system_program.to_account_info();
    let rent = Rent::get()?;
    let feeds = ctx.remaining_accounts;
    // The runtime caps growth at 10 KiB against an account's length when the instruction started,
    // which is 0 for a feed created in this call and the same for a data ID listed twice.
    let entry_lens: Vec<usize> = feeds.iter().map(|info| info.data_len()).collect();
    update_feeds(&ctx, &data_ids, feeds, |i, id, feed, events| {
        let new_max = max_stored_reports[i];
        check_max_stored_reports(feed, new_max).map_err(|e| match e {
            MaxStoredReportsError::Invalid => StoreError::InvalidMaxStoredReports,
            MaxStoredReportsError::NotIncreased => StoreError::MaxStoredReportsNotIncreased,
            MaxStoredReportsError::Locked => StoreError::MaxStoredReportsLocked,
        })?;
        let info = &feeds[i];
        // before the rent top-up, which would otherwise fail with a less clear error
        require!(info.is_writable, StoreError::AccountNotWritable);
        let old_len = info.data_len();
        let new_len = feed_account_len(new_max);
        require!(
            new_len > old_len && new_len - entry_lens[i] <= MAX_ACCOUNT_DATA_INCREASE,
            StoreError::InvalidMaxStoredReports
        );
        let top_up = rent
            .minimum_balance(new_len)
            .saturating_sub(info.lamports());
        if top_up > 0 {
            anchor_lang::system_program::transfer(
                CpiContext::new(
                    system_program.clone(),
                    anchor_lang::system_program::Transfer {
                        from: payer.clone(),
                        to: info.clone(),
                    },
                ),
                top_up,
            )?;
        }
        // zero-fills the new bytes
        info.resize(new_len)?;
        feed.max_stored_reports = new_max;
        events.emit(&MaxStoredReportsSet {
            state,
            data_id: *id,
            max_stored_reports: new_max,
        })
    })
}

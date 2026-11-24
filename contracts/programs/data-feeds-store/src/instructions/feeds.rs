//! Feed admin instructions that change feeds. Their remaining accounts are the feed PDAs of the
//! data IDs, in the same order. A feed that doesn't exist yet is created first, and the feed admin
//! pays its rent. On Solana a feed admin creates feeds, as on Aptos and Stellar, because the
//! forwarder passes no account that could pay for a new one.

use std::mem::size_of;

use anchor_lang::{prelude::*, Discriminator};

use crate::{
    common::{
        ANCHOR_DISCRIMINATOR, DF1_LINK_SEED, DF1_LINK_VERSION, FEED_ACCOUNT_VERSION, FEED_SEED,
        MAX_ACCOUNT_DATA_INCREASE, STORED_DECIMALS,
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

/// Checks that the signer is a feed admin, creates the feeds that don't exist yet, then runs `f`
/// on each feed's header and writes the header back. One header buffer and one event emitter serve
/// the whole call, because a program's heap is never freed. `FeedsCreated` is emitted first, so
/// indexers that apply events in order see a feed created before its settings. The emitter is
/// returned for events that cover the whole call.
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
    // A single event lists every feed created, because each event is an inner instruction and
    // Solana allows at most 64 instructions per transaction. A data ID listed twice is created,
    // and listed, once.
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
            feed.version = FEED_ACCOUNT_VERSION;
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

/// Runs `update_feeds_with_events` for setters whose events are all emitted per feed.
fn update_feeds<'info>(
    ctx: &Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: &[[u8; 16]],
    feeds: &[AccountInfo<'info>],
    f: impl FnMut(usize, &[u8; 16], &mut Feed, &EventEmitter<'info>) -> Result<()>,
) -> Result<()> {
    update_feeds_with_events(ctx, data_ids, feeds, f).map(|_| ())
}

/// Replaces a feed's own workflow list. The entries a shorter list leaves unused are zeroed,
/// because the layout is public and removed workflows shouldn't stay visible in it.
fn set_specific(feed: &mut Feed, workflow_metadata: &[WorkflowMetadata]) {
    feed.specific_workflow_metadata = bytemuck::Zeroable::zeroed();
    feed.specific_workflow_metadata.extend(workflow_metadata);
}

/// Emits one `SpecificFeedMetadataSet` covering every data ID of the call, serialized straight
/// from the borrowed lists instead of from copies.
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

/// Emits `FeedDescriptionSet`, serialized straight from the borrowed description instead of from
/// a copy.
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

/// Sets feeds up, creating those that don't exist yet. It sets each feed's description and, if
/// `workflow_metadata` isn't empty, makes that list each feed's own workflow list. With an empty
/// list, each feed keeps its own list, or the store's default list if it has none.
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

/// Sets feed descriptions, for example "ETH / USD". An empty string clears a description.
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

/// Gives each feed its own workflow list, replacing any list it had. A feed without its own list
/// uses the store's default list.
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

/// Removes the feeds' own workflow lists, so the store's default list applies to them again.
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

/// Sets the range of answers each feed accepts. The pair `(i128::MIN, i128::MAX)` removes the
/// range.
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
                // The unbounded pair removes the range, as if it had never been set.
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

/// Disables or enables writes to feeds. While a feed's writes are disabled, every report for it
/// is skipped, whichever workflow sent it, and reads keep serving the last round.
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

/// Freezes or unfreezes round reads of feeds. While a feed is frozen, `latest_round` and
/// `get_round` fail, its description and decimals stay readable, and its reports are still
/// stored.
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

/// Sets the fewest decimals a read of each feed may ask for, at most 18. New feeds start at 18.
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

/// Links data IDs to the DF1 feeds that this store then writes their rounds to, or unlinks them
/// when given the default key, and enables or disables that write. A DF1 feed can be linked from
/// only one data ID. Its link marker, the PDA `["df1_link", state, df1_feed]`, holds the data ID
/// that links it. The marker is created on the DF1 feed's first link, with the feed admin paying
/// about 0.001 SOL, and it is cleared rather than closed when the link is removed. Reports never
/// read it. To move a DF1 feed between data IDs in one call, list the data ID that gives it up
/// first. Markers are passed writable; below, link(F) is DF1 feed F's marker.
///
/// ```text
/// data_ids        A        B        C        D
/// df1_feeds       X        none     W        V        none is the default key
/// linked before   none     Y        Z        V
///
/// remaining accounts, in this order:
///   feed A, feed B, feed C, feed D     the feeds, in data ID order
///   X, link(X)                         A links X (link(X) is created if it is new)
///   link(Y)                            B unlinks Y
///   link(Z), W, link(W)                C moves from Z to W
///   V, link(V)                         D keeps V, for example to change write_disabled
/// ```
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
            // Only the layout is checked: the DF1 feed can be linked before its owner makes this
            // store its writer. A new or changed link, and enabling an existing one, must be valid.
            // Disabling an existing link stays available if DF1's layout becomes unsupported.
            // Reports check the feed again before every `submit`.
            if old != new || !write_disabled[i] {
                require!(
                    df1::check_feed(info.owner, &info.try_borrow_data()?, &df1_program).is_ok(),
                    StoreError::InvalidDf1Feed
                );
            }
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

/// Creates a DF1 feed's link marker for a data ID, or checks that the existing marker is free or
/// already holds that data ID. It then writes the data ID into the marker.
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

/// Writes a link marker's tag, version and data ID.
fn write_link(marker: &AccountInfo, data_id: [u8; 32]) -> Result<()> {
    let link = Df1Link {
        version: DF1_LINK_VERSION,
        _padding0: [0; 7],
        data_id,
    };
    let mut data = marker.try_borrow_mut_data()?;
    data[..ANCHOR_DISCRIMINATOR].copy_from_slice(Df1Link::DISCRIMINATOR);
    data[ANCHOR_DISCRIMINATOR..].copy_from_slice(bytemuck::bytes_of(&link));
    Ok(())
}

/// Clears a DF1 feed's link marker held by a data ID, so that another data ID can link the DF1
/// feed. The marker is kept rather than closed. Closing would move its SOL by hand, and the
/// runtime's balance check then fails a later call in the same instruction, such as the system
/// program call that creates the next feed.
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

/// Returns the data ID, as its 32-byte seed, that a link marker of this program holds. It checks
/// that the marker is this program's, writable, of the right type and size, and at the current
/// version.
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
        DF1_LINK_VERSION,
        StoreError::InvalidAccountVersion
    );
    Ok(link.data_id)
}

/// Grows the number of rounds feeds keep. A ring can only grow, and only before it first wraps.
/// Each feed can grow by at most 10 KiB per instruction, counted from its length when the
/// instruction started: 213 rounds for an existing feed, while a feed created in the same call
/// holds at most 185. When this is called through another program, as an MCMS batch does, the
/// runtime counts from the start of the outer instruction, so grow each feed once per outer
/// instruction. The feed admin tops up the rent.
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
    // The runtime caps growth at 10 KiB against an account's length when the instruction started.
    // That length is 0 for a feed created in this call, and it stays the first length for a data
    // ID listed twice.
    let entry_lens: Vec<usize> = feeds.iter().map(|info| info.data_len()).collect();
    update_feeds(&ctx, &data_ids, feeds, |i, id, feed, events| {
        let new_max = max_stored_reports[i];
        check_max_stored_reports(feed, new_max).map_err(|e| match e {
            MaxStoredReportsError::Invalid => StoreError::InvalidMaxStoredReports,
            MaxStoredReportsError::NotIncreased => StoreError::MaxStoredReportsNotIncreased,
            MaxStoredReportsError::Locked => StoreError::MaxStoredReportsLocked,
        })?;
        let info = &feeds[i];
        // This is checked before the rent top-up, which would otherwise fail with a less clear
        // error.
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
        // `resize` zero-fills the new bytes, so the new ring indexes start empty.
        info.resize(new_len)?;
        feed.max_stored_reports = new_max;
        events.emit(&MaxStoredReportsSet {
            state,
            data_id: *id,
            max_stored_reports: new_max,
        })
    })
}

//! Feed admin settings of data IDs (remaining accounts: their feed PDAs, in order). A feed that
//! doesn't exist yet is created first, the feed admin paying its rent: on Solana a feed is set by a
//! feed admin, as on Aptos and Stellar, since the forwarder passes no account that could pay for it.

use anchor_lang::{prelude::*, Discriminator};

use crate::{
    common::{
        ACCOUNT_VERSION, FEED_SEED, MAX_ACCOUNT_DATA_INCREASE, MAX_WORKFLOW_METADATAS,
        STORED_DECIMALS,
    },
    context::FeedAdminAccounts,
    data_id, df1,
    error::StoreError,
    event::*,
    logic::{check_max_stored_reports, MaxStoredReportsError},
    state::{feed_account_len, Feed, WorkflowMetadata},
    utils::{
        create_pda_account, encode_description, feed_address, is_uninitialized, load_feed_into,
        store_feed, validate_data_id, validate_workflow_metadata, verify_feed_admin, EventEmitter,
    },
};

/// Check the admin, create the feeds that don't exist yet, then run `f` on each feed's header and
/// write it back. One header buffer and one event emitter serve the batch: the bump allocator never
/// frees.
fn update_feeds<'info>(
    ctx: &Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: &[[u8; 16]],
    feeds: &[AccountInfo<'info>],
    mut f: impl FnMut(usize, &[u8; 16], &mut Feed, &EventEmitter<'info>) -> Result<()>,
) -> Result<()> {
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
    let mut feed: Box<Feed> = bytemuck::zeroed_box();
    let mut created = Vec::new();
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
            created.push(*id);
        } else {
            load_feed_into(info, &state_key, id, &mut feed)?;
        }
        f(i, id, &mut feed, &events)?;
        store_feed(info, &feed)?;
    }
    if !created.is_empty() {
        events.emit(&FeedsCreated {
            state: state_key,
            data_ids: created,
            max_stored_reports,
        })?;
    }
    Ok(())
}

fn set_specific(
    state: &Pubkey,
    id: &[u8; 16],
    feed: &mut Feed,
    workflow_metadata: &[WorkflowMetadata],
    events: &EventEmitter,
) -> Result<()> {
    if !feed.specific_workflow_metadata.is_empty() {
        events.emit(&SpecificFeedMetadataRemoved {
            state: *state,
            data_id: *id,
        })?;
    }
    feed.specific_workflow_metadata.clear();
    feed.specific_workflow_metadata.extend(workflow_metadata);
    // SpecificFeedMetadataSet, written from the borrowed list instead of a copy per data ID
    events.emit_with(SpecificFeedMetadataSet::DISCRIMINATOR, |data| {
        state.serialize(data)?;
        id.serialize(data)?;
        workflow_metadata.serialize(data)?;
        Ok(())
    })
}

fn validate_workflows(workflow_metadata: &[WorkflowMetadata]) -> Result<()> {
    require!(
        workflow_metadata.len() <= MAX_WORKFLOW_METADATAS,
        StoreError::MaxEntriesExceeded
    );
    for metadata in workflow_metadata.iter() {
        validate_workflow_metadata(metadata)?;
    }
    Ok(())
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
    update_feeds(
        &ctx,
        &data_ids,
        ctx.remaining_accounts,
        |i, id, feed, events| {
            feed.description = encode_description(&descriptions[i])?;
            emit_description_set(events, &state, id, &descriptions[i])?;
            if !workflow_metadata.is_empty() {
                set_specific(&state, id, feed, &workflow_metadata, events)?;
            }
            Ok(())
        },
    )
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
    update_feeds(
        &ctx,
        &data_ids,
        ctx.remaining_accounts,
        |_, id, feed, events| set_specific(&state, id, feed, &workflow_metadata, events),
    )
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
            feed.specific_workflow_metadata.clear();
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

/// Freeze or unfreeze reads of feeds: frozen feeds refuse reads, writes continue.
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
/// unlinks), and enable or disable that write. Remaining accounts: the feeds, then each linked DF1
/// feed account in order, which must be a DF1 feed `submit` can write. A DF1 feed should be linked
/// from one data ID only; the deployment tooling checks that across feeds.
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
    let linked = df1_feeds
        .iter()
        .filter(|feed| **feed != Pubkey::default())
        .count();
    require_eq!(
        ctx.remaining_accounts.len(),
        data_ids.len() + linked,
        StoreError::InvalidAccountCount
    );
    let (feeds, df1_infos) = ctx.remaining_accounts.split_at(data_ids.len());
    let df1_program = ctx.accounts.state.load()?.df1_program;
    let mut df1_infos = df1_infos.iter();
    for df1_feed in df1_feeds.iter() {
        if *df1_feed != Pubkey::default() {
            let info = df1_infos.next().unwrap();
            require_keys_eq!(*info.key, *df1_feed, StoreError::AccountMismatch);
            require!(
                df1::check_layout(info, &df1_program).is_ok(),
                StoreError::InvalidDf1Feed
            );
        }
    }
    let state = ctx.accounts.state.key();
    update_feeds(&ctx, &data_ids, feeds, |i, id, feed, events| {
        feed.df1_feed = df1_feeds[i];
        feed.df1_write_disabled = write_disabled[i] as u8;
        events.emit(&Df1FeedSet {
            state,
            data_id: *id,
            df1_feed: df1_feeds[i],
            write_disabled: write_disabled[i],
        })
    })
}

/// Grow the number of rounds feeds keep: only up, only before a feed's ring wraps, by at most 10KiB
/// (320 rounds) per feed per instruction. The feed admin tops up the rent.
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
    update_feeds(&ctx, &data_ids, feeds, |i, id, feed, events| {
        let new_max = max_stored_reports[i];
        check_max_stored_reports(feed, new_max).map_err(|e| match e {
            MaxStoredReportsError::Invalid => StoreError::InvalidMaxStoredReports,
            MaxStoredReportsError::NotIncreased => StoreError::MaxStoredReportsNotIncreased,
            MaxStoredReportsError::Locked => StoreError::MaxStoredReportsLocked,
            MaxStoredReportsError::TooLow => StoreError::MaxStoredReportsTooLow,
        })?;
        let info = &feeds[i];
        let old_len = info.data_len();
        let new_len = feed_account_len(new_max);
        require!(
            new_len > old_len && new_len - old_len <= MAX_ACCOUNT_DATA_INCREASE,
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
        info.resize(new_len)?;
        info.try_borrow_mut_data()?[old_len..].fill(0);
        feed.max_stored_reports = new_max;
        events.emit(&MaxStoredReportsSet {
            state,
            data_id: *id,
            max_stored_reports: new_max,
        })
    })
}

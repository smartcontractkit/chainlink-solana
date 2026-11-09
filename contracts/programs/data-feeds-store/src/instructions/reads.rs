//! The reader interface: rounds, decimals and descriptions by data ID, returned as Borsh return
//! data. Proxies and other programs call it through CPI (see the
//! `chainlink-solana-data-feeds-store` crate), clients can simulate it offchain. A store version
//! that replaces this one keeps the same instructions and return types, so a proxy pointing at it
//! keeps working. Store accounts are also public, so reading them directly bypasses these
//! instructions.

use anchor_lang::prelude::*;

use crate::{
    common::{FEED_SEED, MAX_BATCH_READ, STORED_DECIMALS},
    context::{FeedRead, FeedsRead, PermissionRead},
    data_id,
    error::StoreError,
    logic::{self, LatestReport},
    state::{FeedData, ReaderRound, StoreState, WorkflowMetadata},
    utils::{
        decode_description, has_permission, is_uninitialized, load_boxed, load_feed,
        load_feed_into, load_settings, RingRef,
    },
};

fn to_round(report: LatestReport) -> ReaderRound {
    ReaderRound {
        round_id: report.round_id,
        answer: report.answer,
        observations_timestamp: report.observations_timestamp,
        recorded_timestamp: report.recorded_timestamp,
    }
}

fn check_data_id(id: &[u8; 16]) -> Result<()> {
    require!(data_id::is_valid_data_id(id), StoreError::InvalidDataId);
    Ok(())
}

/// The latest round of a data ID, answer with `STORED_DECIMALS`. Fails with `NoDataPresent` while
/// the feed has never received a report.
pub fn get_latest_round(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<ReaderRound> {
    check_data_id(&data_id)?;
    let feed = load_feed(
        &ctx.accounts.feed.to_account_info(),
        &ctx.accounts.state.key(),
        &data_id,
    )?;
    logic::latest_report(&feed)
        .map(to_round)
        .ok_or_else(|| StoreError::NoDataPresent.into())
}

/// Latest rounds of several data IDs, in order (remaining accounts: their feeds), as the EVM
/// store's `getLatestRoundDataBatch`. A feed without a report, or never created (its PDA passed but
/// empty), yields a zero round (round ID 0) instead of failing the read; an invalid data ID or a
/// feed account that does not match fails it. At most `MAX_BATCH_READ` data IDs, the return data
/// limit. One buffer serves every feed: the bump allocator never frees.
pub fn get_latest_rounds<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedsRead<'info>>,
    data_ids: Vec<[u8; 16]>,
) -> Result<Vec<ReaderRound>> {
    require!(data_ids.len() <= MAX_BATCH_READ, StoreError::TooManyDataIds);
    require_eq!(
        data_ids.len(),
        ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );
    let state_key = ctx.accounts.state.key();
    let mut feed: Box<FeedData> = bytemuck::zeroed_box();
    let mut rounds = Vec::with_capacity(data_ids.len());
    for (id, info) in data_ids.iter().zip(ctx.remaining_accounts.iter()) {
        check_data_id(id)?;
        if is_uninitialized(info) {
            // only the data ID's own feed address may stand for "never created"
            let (address, _) = Pubkey::find_program_address(
                &[FEED_SEED, state_key.as_ref(), &data_id::to_seed(id)],
                &crate::ID,
            );
            require_keys_eq!(address, *info.key, StoreError::AccountMismatch);
            rounds.push(ReaderRound::default());
            continue;
        }
        load_feed_into(info, &state_key, id, &mut feed)?;
        rounds.push(
            logic::latest_report(&feed)
                .map(to_round)
                .unwrap_or_default(),
        );
    }
    Ok(rounds)
}

/// A stored round of a data ID. Fails with `NoDataPresent` for a round that was never written or
/// has been overwritten in the ring.
pub fn get_round(ctx: Context<FeedRead>, data_id: [u8; 16], round_id: u64) -> Result<ReaderRound> {
    check_data_id(&data_id)?;
    let info = ctx.accounts.feed.to_account_info();
    let feed = load_feed(&info, &ctx.accounts.state.key(), &data_id)?;
    let data = info.try_borrow_data()?;
    logic::stored_report(round_id, &feed, &RingRef::new(&data[..]))
        .map(to_round)
        .ok_or_else(|| StoreError::NoDataPresent.into())
}

/// Decimals reports are stored with.
pub fn get_decimals(_ctx: Context<FeedsRead>, data_id: [u8; 16]) -> Result<u8> {
    check_data_id(&data_id)?;
    Ok(STORED_DECIMALS)
}

/// The description of a data ID's feed, empty when none is set.
pub fn get_description(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<String> {
    check_data_id(&data_id)?;
    let info = ctx.accounts.feed.to_account_info();
    let feed = load_feed(&info, &ctx.accounts.state.key(), &data_id)?;
    Ok(decode_description(&feed.description))
}

/// Whether a workflow may write a data ID, through its feed-specific metadata when configured and
/// the store's default metadata otherwise (also when no feed is passed).
pub fn check_feed_permission(
    ctx: Context<PermissionRead>,
    data_id: [u8; 16],
    workflow_metadata: WorkflowMetadata,
) -> Result<bool> {
    // a data ID the store doesn't serve can't be written, whatever the metadata says
    check_data_id(&data_id)?;
    let state_info = ctx.accounts.state.to_account_info();
    let state: Box<StoreState> = load_boxed(&state_info)?;
    let settings = match &ctx.accounts.feed {
        Some(feed) => Some(load_settings(
            &feed.to_account_info(),
            state_info.key,
            &data_id,
        )?),
        None => None,
    };
    let specific: &[WorkflowMetadata] = match &settings {
        Some(settings) => &settings.specific_workflow_metadata,
        None => &[],
    };
    Ok(has_permission(
        specific,
        &state.default_workflow_metadata,
        &workflow_metadata.allowed_sender,
        &workflow_metadata.allowed_workflow_owner,
        &workflow_metadata.allowed_workflow_name,
    ))
}

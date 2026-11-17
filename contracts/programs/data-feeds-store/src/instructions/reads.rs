//! Reads, as Stellar's proxy reads: the caller passes the data ID and the decimals it wants. Feed
//! accounts are public too, so readers can also decode them without a call (see the
//! `chainlink-solana-data-feeds-store` crate); these instructions are for those who'd rather call.

use anchor_lang::prelude::*;

use crate::{
    common::STORED_DECIMALS,
    context::{FeedRead, PermissionRead},
    data_id,
    error::StoreError,
    logic::{self, ScaleError, StoredRound},
    state::{Feed, Round, WorkflowMetadata},
    utils::{
        decode_description, feed_address, has_permission, is_uninitialized, load_feed_into, RingRef,
    },
};

impl From<ScaleError> for StoreError {
    fn from(e: ScaleError) -> Self {
        match e {
            ScaleError::InvalidDecimals => StoreError::InvalidDecimals,
            ScaleError::RoundsToZero => StoreError::RoundsToZero,
        }
    }
}

/// A round read at `decimals`, checked in Stellar's order: a frozen feed (`FeedFrozen`), decimals
/// outside the feed's minimum to 18 (`InvalidDecimals`), no such round (`NoDataPresent`), then a
/// non-zero answer that would read as zero (`RoundsToZero`).
pub fn to_round(feed: &Feed, round: Option<StoredRound>, decimals: u8) -> Result<Round> {
    require!(feed.frozen == 0, StoreError::FeedFrozen);
    logic::check_decimals(decimals, feed.min_decimals).map_err(StoreError::from)?;
    let round = round.ok_or(StoreError::NoDataPresent)?;
    Ok(Round {
        round_id: round.round_id,
        answer: logic::scale(round.answer, decimals).map_err(StoreError::from)?,
        observations_timestamp: round.observations_timestamp,
        recorded_timestamp: round.recorded_timestamp,
    })
}

fn load(ctx: &Context<FeedRead>, data_id: &[u8; 16]) -> Result<Box<Feed>> {
    let mut feed: Box<Feed> = bytemuck::zeroed_box();
    load_feed_into(
        &ctx.accounts.feed.to_account_info(),
        &ctx.accounts.state.key(),
        data_id,
        &mut feed,
    )?;
    Ok(feed)
}

/// The latest round of a data ID at `decimals`. Fails with `NoDataPresent` before the first round.
pub fn latest_round(ctx: Context<FeedRead>, data_id: [u8; 16], decimals: u8) -> Result<Round> {
    let feed = load(&ctx, &data_id)?;
    to_round(&feed, logic::latest_round(&feed), decimals)
}

/// A stored round of a data ID at `decimals`. Fails with `NoDataPresent` for a round that was
/// never written or has been overwritten in the ring.
pub fn get_round(
    ctx: Context<FeedRead>,
    data_id: [u8; 16],
    round_id: u64,
    decimals: u8,
) -> Result<Round> {
    let feed = load(&ctx, &data_id)?;
    let info = ctx.accounts.feed.to_account_info();
    let data = info.try_borrow_data()?;
    let round = logic::stored_round(round_id, &feed, &RingRef::new(&data[..]));
    to_round(&feed, round, decimals)
}

/// Decimals answers are stored with, the most a read can ask for.
pub fn decimals(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<u8> {
    load(&ctx, &data_id)?;
    Ok(STORED_DECIMALS)
}

/// The description of a data ID's feed, empty when none is set.
pub fn description(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<String> {
    Ok(decode_description(&load(&ctx, &data_id)?.description))
}

/// The fewest decimals reads of a data ID may ask for.
pub fn get_min_decimals(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<u8> {
    Ok(load(&ctx, &data_id)?.min_decimals)
}

/// Whether a workflow may write a data ID: false while its feed hasn't been set (reports for it are
/// skipped), otherwise whether the workflow is on the feed's own list, or the store's default list
/// when the feed has none. The feed account is required, so the answer always uses the list that
/// applies. As on EVM, the write freeze is not considered.
pub fn check_feed_permission(
    ctx: Context<PermissionRead>,
    data_id: [u8; 16],
    workflow_metadata: WorkflowMetadata,
) -> Result<bool> {
    // refuse, rather than answer for, a data ID the store doesn't serve
    require!(
        data_id::is_valid_data_id(&data_id),
        StoreError::InvalidDataId
    );
    let state_info = ctx.accounts.state.to_account_info();
    let feed_info = ctx.accounts.feed.to_account_info();
    // a feed that hasn't been set takes no reports, whatever the default workflows say
    if is_uninitialized(&feed_info) {
        let (address, _) = feed_address(state_info.key, &data_id);
        require_keys_eq!(address, *feed_info.key, StoreError::AccountMismatch);
        return Ok(false);
    }
    let state = ctx.accounts.state.load()?;
    let mut feed: Box<Feed> = bytemuck::zeroed_box();
    load_feed_into(&feed_info, state_info.key, &data_id, &mut feed)?;
    Ok(has_permission(
        &feed.specific_workflow_metadata,
        &state.default_workflow_metadata,
        &workflow_metadata.allowed_sender,
        &workflow_metadata.allowed_workflow_owner,
        &workflow_metadata.allowed_workflow_name,
    ))
}

//! Read instructions. The caller passes the store state and the data ID's feed account, the data
//! ID, and the decimals it wants. Feed accounts are public too, so readers can also
//! decode them without a call (see the `chainlink-solana-data-feeds-store` crate). These
//! instructions are for readers who would rather call.

use anchor_lang::prelude::*;

use crate::{
    common::STORED_DECIMALS,
    context::{FeedRead, PermissionRead},
    data_id,
    error::StoreError,
    logic::{self, ScaleError, StoredRound},
    state::{Feed, Round, WorkflowMetadata},
    utils::{
        decode_description, has_permission, is_uninitialized, load_feed_into,
        validate_feed_address, RingRef,
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

/// Turns a stored round into a read result at `decimals`. The checks run in this order:
/// decimals outside the feed's minimum to 18 (`InvalidDecimals`), a frozen feed (`FeedFrozen`), a
/// missing round (`NoDataPresent`), then a non-zero answer that would read as zero
/// (`RoundsToZero`). The client crate's decode path calls this too, so both read paths refuse the
/// same things.
pub fn to_round(feed: &Feed, round: Option<StoredRound>, decimals: u8) -> Result<Round> {
    logic::check_decimals(decimals, feed.min_decimals).map_err(StoreError::from)?;
    require!(feed.frozen == 0, StoreError::FeedFrozen);
    let round = round.ok_or(StoreError::NoDataPresent)?;
    Ok(Round {
        round_id: round.round_id,
        answer: logic::scale(round.answer, decimals).map_err(StoreError::from)?,
        observations_timestamp: round.observations_timestamp,
        recorded_timestamp: round.recorded_timestamp,
    })
}

/// Copies the header of the data ID's feed, checking that it belongs to this store and data ID.
/// A correctly derived but uninitialized feed returns `NoDataPresent`.
fn load(ctx: &Context<FeedRead>, data_id: &[u8; 16]) -> Result<Box<Feed>> {
    let feed_info = ctx.accounts.feed.to_account_info();
    if is_uninitialized(&feed_info) {
        validate_feed_address(&feed_info, &ctx.accounts.state.key(), data_id)?;
        return Err(StoreError::NoDataPresent.into());
    }
    let mut feed: Box<Feed> = bytemuck::zeroed_box();
    load_feed_into(&feed_info, &ctx.accounts.state.key(), data_id, &mut feed)?;
    Ok(feed)
}

/// Returns the latest round of a data ID at `decimals`. It fails with `NoDataPresent` before the
/// first round.
pub fn latest_round(ctx: Context<FeedRead>, data_id: [u8; 16], decimals: u8) -> Result<Round> {
    // Check a missing feed's address before validating decimals against the default minimum.
    let feed_info = ctx.accounts.feed.to_account_info();
    if is_uninitialized(&feed_info) {
        validate_feed_address(&feed_info, &ctx.accounts.state.key(), &data_id)?;
        logic::check_decimals(decimals, STORED_DECIMALS).map_err(StoreError::from)?;
    }
    let feed = load(&ctx, &data_id)?;
    to_round(&feed, logic::latest_round(&feed), decimals)
}

/// Returns a stored round of a data ID at `decimals`. It fails with `NoDataPresent` for a round
/// that was never stored or has been overwritten in the ring.
pub fn get_round(
    ctx: Context<FeedRead>,
    data_id: [u8; 16],
    round_id: u64,
    decimals: u8,
) -> Result<Round> {
    // Check a missing feed's address before validating decimals against the default minimum.
    let feed_info = ctx.accounts.feed.to_account_info();
    if is_uninitialized(&feed_info) {
        validate_feed_address(&feed_info, &ctx.accounts.state.key(), &data_id)?;
        logic::check_decimals(decimals, STORED_DECIMALS).map_err(StoreError::from)?;
    }
    let feed = load(&ctx, &data_id)?;
    let info = ctx.accounts.feed.to_account_info();
    let data = info.try_borrow_data()?;
    let round = logic::stored_round(round_id, &feed, &RingRef::new(&data[..]));
    to_round(&feed, round, decimals)
}

/// Returns the decimals answers are stored with (18), which is also the most a read can ask for.
/// It fails with `NoDataPresent` for an unset feed and `FeedFrozen` for a frozen one.
pub fn decimals(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<u8> {
    let feed = load(&ctx, &data_id)?;
    require!(feed.frozen == 0, StoreError::FeedFrozen);
    Ok(STORED_DECIMALS)
}

/// Returns the description of a data ID's feed, or an empty string when none is set. It fails with
/// `NoDataPresent` for an unset feed and `FeedFrozen` for a frozen one.
pub fn description(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<String> {
    let feed = load(&ctx, &data_id)?;
    require!(feed.frozen == 0, StoreError::FeedFrozen);
    Ok(decode_description(&feed.description))
}

/// Returns the fewest decimals a read of a data ID may ask for. It fails with `NoDataPresent` for
/// an unset feed but does not check the read freeze.
pub fn get_min_decimals(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<u8> {
    Ok(load(&ctx, &data_id)?.min_decimals)
}

/// Returns whether a workflow may write a data ID. It returns false while the feed hasn't been
/// set, because reports for it are skipped. Otherwise it returns whether the workflow is on the
/// feed's own list, or on the store's default list when the feed has none. The feed account is
/// required, so the answer always uses the list that applies. The write freeze is not considered;
/// this answer is about permission only.
pub fn check_feed_permission(
    ctx: Context<PermissionRead>,
    data_id: [u8; 16],
    workflow_metadata: WorkflowMetadata,
) -> Result<bool> {
    // A data ID the store doesn't serve is refused rather than answered for.
    require!(
        data_id::is_valid_data_id(&data_id),
        StoreError::InvalidDataId
    );
    let state_info = ctx.accounts.state.to_account_info();
    let feed_info = ctx.accounts.feed.to_account_info();
    // A feed that hasn't been set takes no reports, whatever the default workflows say, so the
    // answer is false once the address is confirmed to be the data ID's feed.
    if is_uninitialized(&feed_info) {
        validate_feed_address(&feed_info, state_info.key, &data_id)?;
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

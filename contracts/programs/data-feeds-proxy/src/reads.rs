//! Reads, as Stellar's proxy reader: the caller passes the data ID and the decimals it wants. The
//! feed accounts are public too, so readers can also decode them without a call (see the
//! `chainlink-solana-data-feeds-proxy` crate); these instructions are for those who'd rather call.

use anchor_lang::prelude::*;

use crate::{
    common::{data_id_seed, STORED_DECIMALS},
    context::{FeedRead, StateRead},
    error::ProxyError,
    logic::{self, ScaleError, StoredRound},
    state::{Feed, Round},
    utils::{decode_description, load_feed_into, RingRef},
};

impl From<ScaleError> for ProxyError {
    fn from(e: ScaleError) -> Self {
        match e {
            ScaleError::InvalidDecimals => ProxyError::InvalidDecimals,
            ScaleError::RoundsToZero => ProxyError::RoundsToZero,
        }
    }
}

/// A stored round at `decimals`, refusing a frozen feed and decimals below its minimum.
pub fn to_round(feed: &Feed, round: StoredRound, decimals: u8) -> Result<Round> {
    require!(feed.frozen == 0, ProxyError::FeedFrozen);
    logic::check_decimals(decimals, feed.min_decimals).map_err(ProxyError::from)?;
    Ok(Round {
        round_id: round.round_id,
        answer: logic::scale(round.answer, decimals).map_err(ProxyError::from)?,
        observations_timestamp: round.observations_timestamp,
        recorded_timestamp: round.recorded_timestamp,
    })
}

fn load(ctx: &Context<FeedRead>, data_id: &[u8; 16]) -> Result<Box<Feed>> {
    let mut feed: Box<Feed> = bytemuck::zeroed_box();
    load_feed_into(
        &ctx.accounts.feed.to_account_info(),
        &ctx.accounts.state.key(),
        &data_id_seed(data_id),
        &mut feed,
    )?;
    Ok(feed)
}

/// The latest round of a data ID at `decimals`. Fails with `NoDataPresent` before the first round.
pub fn latest_round(ctx: Context<FeedRead>, data_id: [u8; 16], decimals: u8) -> Result<Round> {
    let feed = load(&ctx, &data_id)?;
    let round = logic::latest_round(&feed).ok_or(ProxyError::NoDataPresent)?;
    to_round(&feed, round, decimals)
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
    let round = logic::stored_round(round_id, &feed, &RingRef::new(&data[..]))
        .ok_or(ProxyError::NoDataPresent)?;
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

/// The trusted store's signer.
pub fn get_store(ctx: Context<StateRead>) -> Result<Pubkey> {
    Ok(ctx.accounts.state.load()?.writer)
}

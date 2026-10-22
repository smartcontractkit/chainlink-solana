//! The reader interface: rounds, decimals and descriptions by data ID, returned as Borsh return
//! data. Proxies and other programs call it through CPI (see the
//! `chainlink-solana-data-feeds-store` crate), clients can simulate it offchain. A store version
//! that replaces this one keeps the same instructions and return types, so a proxy pointing at it
//! keeps working. Store accounts are also public, so reading them directly bypasses these
//! instructions.

use anchor_lang::prelude::*;

use crate::{
    common::STORED_DECIMALS,
    context::{DescriptionRead, FeedRead, PermissionRead},
    data_id,
    error::StoreError,
    logic::{self, LatestReport},
    state::{FeedDescription, ReaderRound, StoreState, WorkflowMetadata},
    utils::{decode_description, has_permission, load_boxed, load_feed, load_settings, RingRef},
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
pub fn get_decimals(_ctx: Context<DescriptionRead>, data_id: [u8; 16]) -> Result<u8> {
    check_data_id(&data_id)?;
    Ok(STORED_DECIMALS)
}

/// The description of the feed a data ID belongs to, empty when none is set. The description
/// account is optional so readers that do not need it do not pay for it.
pub fn get_description(ctx: Context<DescriptionRead>, data_id: [u8; 16]) -> Result<String> {
    check_data_id(&data_id)?;
    match ctx.accounts.feed_description.as_deref() {
        None => Ok(String::new()),
        Some(description) => {
            let FeedDescription {
                state,
                indexed_feed_id,
                description,
                ..
            } = description;
            require!(
                *state == ctx.accounts.state.key()
                    && *indexed_feed_id == data_id::to_indexed_feed_id(&data_id),
                StoreError::AccountMismatch
            );
            Ok(decode_description(description))
        }
    }
}

/// Whether a workflow may write a data ID, through its feed-specific metadata when configured and
/// the store's default metadata otherwise (also when no feed is passed).
pub fn check_feed_permission(
    ctx: Context<PermissionRead>,
    data_id: [u8; 16],
    workflow_metadata: WorkflowMetadata,
) -> Result<bool> {
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

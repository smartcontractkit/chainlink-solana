//! Aggregator-compatible reads, returned through return data. Readers call them through CPI (see
//! the `chainlink-solana-data-feeds-store` crate) or simulate them offchain. Store accounts are
//! also public, so reading them directly bypasses these instructions and the SVR selection rules.

use anchor_lang::prelude::*;

use crate::{
    common::{STORED_DECIMALS, STORE_VERSION},
    context::{DescriptionRead, DirectRead, PermissionRead, ProxyRead},
    data_id,
    error::StoreError,
    logic::{self, LatestReport, Now, SvrSlot},
    state::{
        FeedData, FeedDescription, RoundData, StoreState, StoredReport, SvrPathDescription,
        WorkflowMetadata,
    },
    utils::{
        combine_descriptions, decode_description, has_permission, load_boxed, load_data_id_config,
        load_feed, load_settings, RingRef,
    },
};

fn now() -> Result<Now> {
    let clock = Clock::get()?;
    Ok(Now {
        slot: clock.slot,
        timestamp: clock.unix_timestamp as u32,
    })
}

/// Load the feed history of a data ID and, for an SVR data ID, its path slot.
fn resolve(
    state: &Pubkey,
    id: &[u8; 16],
    feed_info: &AccountInfo,
    config_info: Option<&AccountInfo>,
) -> Result<(Box<FeedData>, Option<SvrSlot>)> {
    let feed = load_feed(feed_info, state, &data_id::to_non_svr(id))?;
    let svr = if data_id::is_svr(id) {
        let info = config_info.ok_or(StoreError::InvalidAccountCount)?;
        let config = load_data_id_config(info, state, id)?;
        Some(SvrSlot {
            disabled: config.svr_disabled != 0,
            round_id: config.svr_round_id,
            answer: config.svr_answer,
            observations_timestamp: config.svr_observations_timestamp,
            recorded_timestamp: config.svr_recorded_timestamp,
            recorded_slot: config.svr_recorded_slot,
        })
    } else {
        None
    };
    Ok((feed, svr))
}

fn latest(
    state: &Pubkey,
    id: &[u8; 16],
    feed_info: &AccountInfo,
    config_info: Option<&AccountInfo>,
    fallback_time: u32,
    max_sync_iterations: u32,
) -> Result<LatestReport> {
    let (feed, svr) = resolve(state, id, feed_info, config_info)?;
    let data = feed_info.try_borrow_data()?;
    Ok(logic::latest_report(
        &feed,
        &RingRef::new(&data[..]),
        svr.as_ref(),
        fallback_time,
        max_sync_iterations,
        now()?,
    ))
}

fn round_data(report: LatestReport, decimals: u8) -> RoundData {
    RoundData {
        round_id: report.round_id,
        answer: logic::downscale(report.answer, decimals),
        started_at: report.observations_timestamp.into(),
        updated_at: report.recorded_timestamp.into(),
        answered_in_round: report.round_id,
    }
}

fn description(
    state: &Pubkey,
    id: &[u8; 16],
    feed_description: Option<&FeedDescription>,
    svr_path_description: Option<&SvrPathDescription>,
) -> Result<String> {
    let feed = match feed_description {
        Some(d) => {
            require!(
                d.state == *state && d.indexed_feed_id == data_id::to_indexed_feed_id(id),
                StoreError::AccountMismatch
            );
            decode_description(&d.description)
        }
        None => String::new(),
    };
    let path = match svr_path_description {
        Some(d) if data_id::is_svr(id) => {
            require!(
                d.state == *state && d.svr_path_id == data_id::svr_path_id(id),
                StoreError::AccountMismatch
            );
            Some(decode_description(&d.description))
        }
        _ => None,
    };
    Ok(combine_descriptions(feed, path))
}

// ================================================================
// │                         Proxy reads                          │
// ================================================================

impl<'info> ProxyRead<'info> {
    fn config_info(&self) -> Option<AccountInfo<'info>> {
        self.data_id_config.as_ref().map(|a| a.to_account_info())
    }

    fn latest(&self) -> Result<LatestReport> {
        let proxy = &self.proxy;
        latest(
            &proxy.state,
            &proxy.data_id,
            &self.feed.to_account_info(),
            self.config_info().as_ref(),
            proxy.fallback_time,
            proxy.max_sync_iterations,
        )
    }

    fn stored(&self, round_id: u64) -> Result<StoredReport> {
        let proxy = &self.proxy;
        let feed_info = self.feed.to_account_info();
        let (feed, svr) = resolve(
            &proxy.state,
            &proxy.data_id,
            &feed_info,
            self.config_info().as_ref(),
        )?;
        let data = feed_info.try_borrow_data()?;
        Ok(logic::stored_report(
            round_id,
            &feed,
            &RingRef::new(&data[..]),
            svr.as_ref(),
            proxy.fallback_time,
            now()?,
        ))
    }
}

pub fn latest_round_data(ctx: Context<ProxyRead>) -> Result<RoundData> {
    Ok(round_data(
        ctx.accounts.latest()?,
        ctx.accounts.proxy.decimals,
    ))
}

pub fn latest_answer(ctx: Context<ProxyRead>) -> Result<i128> {
    Ok(logic::downscale(
        ctx.accounts.latest()?.answer,
        ctx.accounts.proxy.decimals,
    ))
}

pub fn latest_timestamp(ctx: Context<ProxyRead>) -> Result<u64> {
    Ok(ctx.accounts.latest()?.recorded_timestamp.into())
}

pub fn latest_round(ctx: Context<ProxyRead>) -> Result<u64> {
    Ok(ctx.accounts.latest()?.round_id)
}

/// Round data for a shared-history round, zero when the reader cannot be served it.
pub fn get_round_data(ctx: Context<ProxyRead>, round_id: u64) -> Result<RoundData> {
    let stored = ctx.accounts.stored(round_id)?;
    Ok(RoundData {
        round_id,
        answer: logic::downscale(stored.answer, ctx.accounts.proxy.decimals),
        started_at: stored.observations_timestamp.into(),
        updated_at: stored.recorded_timestamp.into(),
        answered_in_round: round_id,
    })
}

pub fn get_answer(ctx: Context<ProxyRead>, round_id: u64) -> Result<i128> {
    Ok(logic::downscale(
        ctx.accounts.stored(round_id)?.answer,
        ctx.accounts.proxy.decimals,
    ))
}

pub fn get_timestamp(ctx: Context<ProxyRead>, round_id: u64) -> Result<u64> {
    Ok(ctx.accounts.stored(round_id)?.recorded_timestamp.into())
}

pub fn decimals(ctx: Context<ProxyRead>) -> Result<u8> {
    Ok(ctx.accounts.proxy.decimals)
}

pub fn description_for_proxy(ctx: Context<ProxyRead>) -> Result<String> {
    let proxy = &ctx.accounts.proxy;
    description(
        &proxy.state,
        &proxy.data_id,
        ctx.accounts.feed_description.as_deref(),
        ctx.accounts.svr_path_description.as_deref(),
    )
}

pub fn version(_ctx: Context<ProxyRead>) -> Result<u64> {
    Ok(STORE_VERSION)
}

// ================================================================
// │                         Direct reads                         │
// ================================================================

fn check_direct(id: &[u8; 16], decimals: u8) -> Result<()> {
    require!(!data_id::is_zero(id), StoreError::InvalidDataId);
    require!(
        decimals <= STORED_DECIMALS,
        StoreError::DecimalsAboveStoredDecimals
    );
    Ok(())
}

impl DirectRead<'_> {
    fn latest(
        &self,
        id: &[u8; 16],
        fallback_time: u32,
        max_sync_iterations: u32,
    ) -> Result<LatestReport> {
        let config_info = self.data_id_config.as_ref().map(|a| a.to_account_info());
        latest(
            &self.state.key(),
            id,
            &self.feed.to_account_info(),
            config_info.as_ref(),
            fallback_time,
            max_sync_iterations,
        )
    }
}

/// Latest answer for any data ID, downscaled to `decimals`. Zero `fallback_time` and
/// `max_sync_iterations` serve an SVR path's selected report without syncing.
pub fn get_latest_answer(
    ctx: Context<DirectRead>,
    data_id: [u8; 16],
    decimals: u8,
    fallback_time: u32,
    max_sync_iterations: u32,
) -> Result<i128> {
    check_direct(&data_id, decimals)?;
    let report = ctx
        .accounts
        .latest(&data_id, fallback_time, max_sync_iterations)?;
    Ok(logic::downscale(report.answer, decimals))
}

pub fn get_latest_timestamp(
    ctx: Context<DirectRead>,
    data_id: [u8; 16],
    fallback_time: u32,
    max_sync_iterations: u32,
) -> Result<u64> {
    check_direct(&data_id, STORED_DECIMALS)?;
    let report = ctx
        .accounts
        .latest(&data_id, fallback_time, max_sync_iterations)?;
    Ok(report.recorded_timestamp.into())
}

pub fn get_latest_round_data(
    ctx: Context<DirectRead>,
    data_id: [u8; 16],
    decimals: u8,
    fallback_time: u32,
    max_sync_iterations: u32,
) -> Result<RoundData> {
    check_direct(&data_id, decimals)?;
    let report = ctx
        .accounts
        .latest(&data_id, fallback_time, max_sync_iterations)?;
    Ok(round_data(report, decimals))
}

/// Decimals reports are stored with, downscaling is a per-reader concern.
pub fn get_decimals(_ctx: Context<DescriptionRead>, data_id: [u8; 16]) -> Result<u8> {
    require!(!data_id::is_zero(&data_id), StoreError::InvalidDataId);
    Ok(STORED_DECIMALS)
}

pub fn get_description(ctx: Context<DescriptionRead>, data_id: [u8; 16]) -> Result<String> {
    require!(!data_id::is_zero(&data_id), StoreError::InvalidDataId);
    description(
        &ctx.accounts.state.key(),
        &data_id,
        ctx.accounts.feed_description.as_deref(),
        ctx.accounts.svr_path_description.as_deref(),
    )
}

/// Whether a workflow may write a data ID, through its feed-specific metadata when configured and
/// the store's default metadata otherwise (also when no settings account is passed).
pub fn check_feed_permission(
    ctx: Context<PermissionRead>,
    data_id: [u8; 16],
    workflow_metadata: WorkflowMetadata,
) -> Result<bool> {
    let state_info = ctx.accounts.state.to_account_info();
    let state: Box<StoreState> = load_boxed(&state_info)?;
    let settings = match &ctx.accounts.settings {
        Some(info) => Some(load_settings(info, state_info.key, &data_id)?),
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

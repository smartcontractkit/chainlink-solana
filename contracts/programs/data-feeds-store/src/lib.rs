#![allow(deprecated)] // anchor-lang 0.31 #[program] macro uses AccountInfo::realloc
//! Chainlink Data Feeds store for CRE: a Solana port of the EVM `DataFeedsStore`, SVR included.
//! Reports arrive from the keystone forwarder through `on_report`, are stored per non-SVR feed in a
//! circular buffer and served through proxy (reader config) and direct reads. Per data ID report
//! callbacks let other programs follow updates, the DF1 store uses one to keep its existing feed
//! accounts written.

pub mod common;
pub mod context;
pub mod data_id;
pub mod error;
pub mod event;
pub mod instructions;
pub mod logic;
pub mod state;
pub mod utils;

use anchor_lang::prelude::*;

use context::*;
use instructions::*;
use state::{CallbackConfigArgs, ProxyConfigArgs, RoundData, WorkflowMetadata};

declare_id!("9qCsUGTdKCanybkroJuPzAhuRU8P2SZN4NCexevSZAwu");

#[program]
pub mod data_feeds_store {
    use super::*;

    // ================================================================
    // │                         Owner                                │
    // ================================================================

    /// Create a store with a dedicated state account. `feed_admins` must be strictly increasing.
    pub fn initialize(
        ctx: Context<Initialize>,
        feed_admins: Vec<Pubkey>,
        default_max_stored_reports: u64,
        max_observation_forward_secs: u32,
    ) -> Result<()> {
        admin::initialize(
            ctx,
            feed_admins,
            default_max_stored_reports,
            max_observation_forward_secs,
        )
    }

    /// Step 1 of 2: propose a new owner.
    pub fn transfer_ownership(ctx: Context<OwnerOnly>, proposed_owner: Pubkey) -> Result<()> {
        admin::transfer_ownership(ctx, proposed_owner)
    }

    /// Step 2 of 2: the proposed owner accepts.
    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        admin::accept_ownership(ctx)
    }

    /// Grant or revoke feed admin permission for all feeds.
    pub fn set_feed_admin(
        ctx: Context<OwnerOnly>,
        feed_admin: Pubkey,
        is_admin: bool,
    ) -> Result<()> {
        admin::set_feed_admin(ctx, feed_admin, is_admin)
    }

    /// Point the store at another forwarder program.
    pub fn update_forwarder_id(ctx: Context<UpdateForwarder>) -> Result<()> {
        admin::update_forwarder_id(ctx)
    }

    /// Recover SPL tokens held by token accounts of the store authority PDA.
    pub fn recover_tokens(ctx: Context<RecoverTokens>, amount: u64) -> Result<()> {
        admin::recover_tokens(ctx, amount)
    }

    /// Recover native SOL sent to the store authority PDA.
    pub fn recover_native(ctx: Context<RecoverNative>, amount: u64) -> Result<()> {
        admin::recover_native(ctx, amount)
    }

    // ================================================================
    // │                       Feed admin                             │
    // ================================================================

    /// Set the workflows allowed to write any data ID without feed-specific metadata.
    pub fn set_default_feed_metadata(
        ctx: Context<FeedAdminState>,
        workflow_metadata: Vec<WorkflowMetadata>,
    ) -> Result<()> {
        admin::set_default_feed_metadata(ctx, workflow_metadata)
    }

    /// Set the max seconds a report's observation timestamp may lead the clock.
    pub fn set_max_observation_forward_duration(
        ctx: Context<FeedAdminState>,
        max_observation_forward_secs: u32,
    ) -> Result<()> {
        admin::set_max_observation_forward_duration(ctx, max_observation_forward_secs)
    }

    /// Create feed history accounts (remaining accounts: feed PDAs).
    pub fn init_feeds<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        non_svr_data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        feeds::init_feeds(ctx, non_svr_data_ids)
    }

    /// Close feed history accounts (remaining accounts: feed PDAs).
    pub fn close_feeds<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        non_svr_data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        feeds::close_feeds(ctx, non_svr_data_ids)
    }

    /// Set ring sizes (remaining accounts: feed PDAs).
    pub fn set_max_stored_reports<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        non_svr_data_ids: Vec<[u8; 16]>,
        max_stored_reports: Vec<u64>,
    ) -> Result<()> {
        feeds::set_max_stored_reports(ctx, non_svr_data_ids, max_stored_reports)
    }

    /// Set accepted answer ranges (remaining accounts: feed PDAs).
    pub fn set_min_max_answers<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        non_svr_data_ids: Vec<[u8; 16]>,
        min_answers: Vec<i128>,
        max_answers: Vec<i128>,
    ) -> Result<()> {
        feeds::set_min_max_answers(ctx, non_svr_data_ids, min_answers, max_answers)
    }

    /// Create SVR path configs (remaining accounts: data ID config PDAs).
    pub fn init_data_ids<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        data_ids::init_data_ids(ctx, data_ids)
    }

    /// Close SVR path configs (remaining accounts: data ID config PDAs).
    pub fn close_data_ids<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        data_ids::close_data_ids(ctx, data_ids)
    }

    /// Set feed-specific workflow metadata (remaining accounts: per data ID its feed PDA when
    /// non-SVR, its data ID config PDA when an SVR path).
    pub fn set_specific_feed_metadata<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        workflow_metadata: Vec<WorkflowMetadata>,
    ) -> Result<()> {
        data_ids::set_specific_feed_metadata(ctx, data_ids, workflow_metadata)
    }

    /// Remove feed-specific workflow metadata (remaining accounts: per data ID its feed PDA when
    /// non-SVR, its data ID config PDA when an SVR path).
    pub fn remove_specific_feed_metadata<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        data_ids::remove_specific_feed_metadata(ctx, data_ids)
    }

    /// Enable or disable SVR paths (remaining accounts: data ID config PDAs).
    pub fn set_svr_disabled<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        svr_data_ids: Vec<[u8; 16]>,
        is_disabled: Vec<bool>,
    ) -> Result<()> {
        data_ids::set_svr_disabled(ctx, svr_data_ids, is_disabled)
    }

    /// Configure report callbacks (remaining accounts: per data ID its feed PDA when non-SVR, its
    /// data ID config PDA when an SVR path, then the program
    /// account of every enabled callback).
    pub fn set_report_callback_configs<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        callbacks: Vec<CallbackConfigArgs>,
    ) -> Result<()> {
        data_ids::set_report_callback_configs(ctx, data_ids, callbacks)
    }

    /// Create or update proxy configs (remaining accounts: per proxy, its PDA then the non-SVR
    /// feed PDA of its data ID).
    pub fn update_configs_for_proxies<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        proxy_ids: Vec<[u8; 32]>,
        configs: Vec<ProxyConfigArgs>,
    ) -> Result<()> {
        proxies::update_configs_for_proxies(ctx, proxy_ids, configs)
    }

    /// Remove proxy configs (remaining accounts: proxy PDAs).
    pub fn remove_configs_for_proxies<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        proxy_ids: Vec<[u8; 32]>,
    ) -> Result<()> {
        proxies::remove_configs_for_proxies(ctx, proxy_ids)
    }

    /// Set feed descriptions (remaining accounts: feed description PDAs).
    pub fn set_feed_descriptions<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        indexed_feed_ids: Vec<[u8; 16]>,
        descriptions: Vec<String>,
    ) -> Result<()> {
        descriptions::set_feed_descriptions(ctx, indexed_feed_ids, descriptions)
    }

    /// Set SVR path descriptions (remaining accounts: SVR path description PDAs).
    pub fn set_svr_path_descriptions<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        svr_path_ids: Vec<[u8; 2]>,
        descriptions: Vec<String>,
    ) -> Result<()> {
        descriptions::set_svr_path_descriptions(ctx, svr_path_ids, descriptions)
    }

    // ================================================================
    // │                         Reports                              │
    // ================================================================

    /// Forwarder entrypoint. `report` is Borsh `Vec<ReceivedReport>`, remaining accounts are one
    /// group per report: the data ID config for an SVR path, the non-SVR feed, then the callback
    /// program and accounts when a callback is configured.
    pub fn on_report<'info>(
        ctx: Context<'_, '_, 'info, 'info, OnReport<'info>>,
        metadata: Vec<u8>,
        report: Vec<u8>,
    ) -> Result<()> {
        on_report::handler(ctx, metadata, report)
    }

    // ================================================================
    // │                       Proxy reads                            │
    // ================================================================

    pub fn latest_round_data(ctx: Context<ProxyRead>) -> Result<RoundData> {
        reads::latest_round_data(ctx)
    }

    pub fn latest_answer(ctx: Context<ProxyRead>) -> Result<i128> {
        reads::latest_answer(ctx)
    }

    pub fn latest_timestamp(ctx: Context<ProxyRead>) -> Result<u64> {
        reads::latest_timestamp(ctx)
    }

    pub fn latest_round(ctx: Context<ProxyRead>) -> Result<u64> {
        reads::latest_round(ctx)
    }

    pub fn get_round_data(ctx: Context<ProxyRead>, round_id: u64) -> Result<RoundData> {
        reads::get_round_data(ctx, round_id)
    }

    pub fn get_answer(ctx: Context<ProxyRead>, round_id: u64) -> Result<i128> {
        reads::get_answer(ctx, round_id)
    }

    pub fn get_timestamp(ctx: Context<ProxyRead>, round_id: u64) -> Result<u64> {
        reads::get_timestamp(ctx, round_id)
    }

    pub fn decimals(ctx: Context<ProxyRead>) -> Result<u8> {
        reads::decimals(ctx)
    }

    pub fn description(ctx: Context<ProxyRead>) -> Result<String> {
        reads::description_for_proxy(ctx)
    }

    pub fn version(ctx: Context<ProxyRead>) -> Result<u64> {
        reads::version(ctx)
    }

    // ================================================================
    // │                       Direct reads                           │
    // ================================================================

    pub fn get_latest_answer(
        ctx: Context<DirectRead>,
        data_id: [u8; 16],
        decimals: u8,
        fallback_time: u32,
        max_sync_iterations: u32,
    ) -> Result<i128> {
        reads::get_latest_answer(ctx, data_id, decimals, fallback_time, max_sync_iterations)
    }

    pub fn get_latest_timestamp(
        ctx: Context<DirectRead>,
        data_id: [u8; 16],
        fallback_time: u32,
        max_sync_iterations: u32,
    ) -> Result<u64> {
        reads::get_latest_timestamp(ctx, data_id, fallback_time, max_sync_iterations)
    }

    pub fn get_latest_round_data(
        ctx: Context<DirectRead>,
        data_id: [u8; 16],
        decimals: u8,
        fallback_time: u32,
        max_sync_iterations: u32,
    ) -> Result<RoundData> {
        reads::get_latest_round_data(ctx, data_id, decimals, fallback_time, max_sync_iterations)
    }

    pub fn get_decimals(ctx: Context<DescriptionRead>, data_id: [u8; 16]) -> Result<u8> {
        reads::get_decimals(ctx, data_id)
    }

    pub fn get_description(ctx: Context<DescriptionRead>, data_id: [u8; 16]) -> Result<String> {
        reads::get_description(ctx, data_id)
    }

    pub fn check_feed_permission(
        ctx: Context<PermissionRead>,
        data_id: [u8; 16],
        workflow_metadata: WorkflowMetadata,
    ) -> Result<bool> {
        reads::check_feed_permission(ctx, data_id, workflow_metadata)
    }
}

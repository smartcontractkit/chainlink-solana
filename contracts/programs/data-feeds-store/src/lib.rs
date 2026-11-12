#![allow(deprecated)] // anchor-lang 0.31 #[program] macro uses AccountInfo::realloc
//! Chainlink Data Feeds store for CRE: the write side of a Solana port of the EVM `DataFeedsStore`,
//! without SVR. Reports arrive from the keystone forwarder through `on_report`, are checked against
//! the workflows permitted for their data ID, their ordering and their answer range, and are written
//! into the data feeds proxy's feed accounts, which readers integrate with. The store is the proxy's
//! one trusted writer (its `feed_writer` PDA).
//!
//! As on EVM, a feed is created by its first report from a workflow on the store's default list;
//! here the rent comes from a vault the store holds. A data ID can also be linked to an existing DF1
//! feed account, which the store then keeps written through DF1's `submit`, checking first everything
//! that could make `submit` fail. SVR data IDs are reserved in the data ID layout, not supported yet.
//! Events are CPI events.

pub mod common;
pub mod context;
pub mod data_id;
pub mod df1;
pub mod error;
pub mod event;
pub mod instructions;
pub mod state;
pub mod utils;

use anchor_lang::prelude::*;

use context::*;
pub use instructions::admin::StoreConfig;
use instructions::*;
use state::WorkflowMetadata;

declare_id!("9qCsUGTdKCanybkroJuPzAhuRU8P2SZN4NCexevSZAwu");

#[program]
pub mod data_feeds_store {
    use super::*;

    // ================================================================
    // │                         Owner                                │
    // ================================================================

    /// Create a store in a pre-created state account, signed by the program's upgrade authority,
    /// which becomes the store's owner.
    pub fn initialize(ctx: Context<Initialize>, config: StoreConfig) -> Result<()> {
        admin::initialize(ctx, config)
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

    /// Recover SPL tokens held by token accounts of the store authority PDA.
    pub fn recover_tokens(ctx: Context<RecoverTokens>, amount: u64) -> Result<()> {
        admin::recover_tokens(ctx, amount)
    }

    /// Recover native SOL from the vault (the store authority PDA).
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

    /// Set feed-specific workflow metadata (remaining accounts: feed configs).
    pub fn set_specific_feed_metadata<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminConfigs<'info>>,
        data_ids: Vec<[u8; 16]>,
        workflow_metadata: Vec<WorkflowMetadata>,
    ) -> Result<()> {
        settings::set_specific_feed_metadata(ctx, data_ids, workflow_metadata)
    }

    /// Remove feed-specific workflow metadata (remaining accounts: feed configs).
    pub fn remove_specific_feed_metadata<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminConfigs<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        settings::remove_specific_feed_metadata(ctx, data_ids)
    }

    /// Set accepted answer ranges (remaining accounts: feed configs).
    pub fn set_min_max_answers<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminConfigs<'info>>,
        data_ids: Vec<[u8; 16]>,
        min_answers: Vec<i128>,
        max_answers: Vec<i128>,
    ) -> Result<()> {
        settings::set_min_max_answers(ctx, data_ids, min_answers, max_answers)
    }

    /// Disable or enable writes of feeds for every workflow (remaining accounts: feed configs).
    pub fn set_feed_writes_disabled<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminConfigs<'info>>,
        data_ids: Vec<[u8; 16]>,
        disabled: bool,
    ) -> Result<()> {
        settings::set_feed_writes_disabled(ctx, data_ids, disabled)
    }

    /// Link data IDs to the DF1 feeds this store keeps written (remaining accounts: feed configs,
    /// then each linked DF1 feed).
    pub fn set_df1_feeds<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminConfigs<'info>>,
        data_ids: Vec<[u8; 16]>,
        df1_feeds: Vec<Pubkey>,
        write_disabled: Vec<bool>,
    ) -> Result<()> {
        settings::set_df1_feeds(ctx, data_ids, df1_feeds, write_disabled)
    }

    /// Set feed descriptions, empty to clear (remaining accounts: proxy feeds).
    pub fn set_feed_descriptions<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminProxy<'info>>,
        data_ids: Vec<[u8; 16]>,
        descriptions: Vec<String>,
    ) -> Result<()> {
        proxy_settings::set_feed_descriptions(ctx, data_ids, descriptions)
    }

    /// Freeze or unfreeze reads of feeds (remaining accounts: proxy feeds).
    pub fn set_feed_frozen<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminProxy<'info>>,
        data_ids: Vec<[u8; 16]>,
        frozen: bool,
    ) -> Result<()> {
        proxy_settings::set_feed_frozen(ctx, data_ids, frozen)
    }

    /// Grow the number of rounds feeds keep (remaining accounts: proxy feeds).
    pub fn set_max_stored_reports<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminProxy<'info>>,
        data_ids: Vec<[u8; 16]>,
        max_stored_reports: Vec<u64>,
    ) -> Result<()> {
        proxy_settings::set_max_stored_reports(ctx, data_ids, max_stored_reports)
    }

    // ================================================================
    // │                         Reports                              │
    // ================================================================

    /// Forwarder entrypoint. `report` is Borsh `Vec<ReceivedReport>`. Remaining accounts are one
    /// group per report: the data ID's proxy feed, its feed config, then the linked DF1 feed when
    /// the feed config has one.
    pub fn on_report<'info>(
        ctx: Context<'_, '_, 'info, 'info, OnReport<'info>>,
        metadata: Vec<u8>,
        report: Vec<u8>,
    ) -> Result<()> {
        on_report::handler(ctx, metadata, report)
    }

    // ================================================================
    // │                          Reads                               │
    // ================================================================

    /// Whether a workflow may write a data ID.
    pub fn check_feed_permission(
        ctx: Context<PermissionRead>,
        data_id: [u8; 16],
        workflow_metadata: WorkflowMetadata,
    ) -> Result<bool> {
        reads::check_feed_permission(ctx, data_id, workflow_metadata)
    }
}

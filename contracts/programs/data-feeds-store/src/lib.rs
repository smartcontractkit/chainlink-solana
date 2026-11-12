#![allow(deprecated)] // anchor-lang 0.31 #[program] macro uses AccountInfo::realloc
//! Chainlink Data Feeds store for CRE: a Solana port of the EVM `DataFeedsStore` without SVR, and
//! the program readers integrate with. Each data ID has a feed account holding its latest round, a
//! circular buffer of its history, its description, how it is read (minimum decimals, a read freeze)
//! and how it is written (its own workflows, answer bounds, a write freeze, a DF1 link). Readers pass
//! that one account and either call a read here, as on Stellar's proxy, or decode it themselves.
//!
//! Reports arrive from the keystone forwarder through `on_report` and are checked against the
//! workflows permitted for their data ID (the feed's own, or the store's default ones), their
//! ordering and their answer range. Feeds are set by a feed admin (`set_feeds` or any per-feed
//! setter, the admin paying their rent): unlike on EVM, where a feed's storage is paid in gas on its
//! first report, the forwarder passes no account that could pay for one. A data ID can also be
//! linked to an existing DF1 feed account, which the store then keeps written through DF1's
//! `submit`, checking first everything that could make `submit` fail. SVR data IDs are reserved in
//! the data ID layout, not supported yet. Events are CPI events.

pub mod common;
pub mod context;
pub mod data_id;
pub mod df1;
pub mod error;
pub mod event;
pub mod instructions;
pub mod logic;
pub mod state;
pub mod utils;

use anchor_lang::prelude::*;

use context::*;
use instructions::*;
use state::{Round, StoreConfig, WorkflowMetadata};

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

    /// Create feeds that don't exist yet and set their descriptions and, unless empty, their
    /// workflows (remaining accounts: feeds). Every per-feed setter below also creates missing
    /// feeds, the feed admin paying their rent.
    pub fn set_feeds<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        descriptions: Vec<String>,
        workflow_metadata: Vec<WorkflowMetadata>,
    ) -> Result<()> {
        feeds::set_feeds(ctx, data_ids, descriptions, workflow_metadata)
    }

    /// Set feed descriptions, empty to clear (remaining accounts: feeds).
    pub fn set_feed_descriptions<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        descriptions: Vec<String>,
    ) -> Result<()> {
        feeds::set_feed_descriptions(ctx, data_ids, descriptions)
    }

    /// Set feed-specific workflow metadata (remaining accounts: feeds).
    pub fn set_specific_feed_metadata<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        workflow_metadata: Vec<WorkflowMetadata>,
    ) -> Result<()> {
        feeds::set_specific_feed_metadata(ctx, data_ids, workflow_metadata)
    }

    /// Remove feed-specific workflow metadata (remaining accounts: feeds).
    pub fn remove_specific_feed_metadata<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        feeds::remove_specific_feed_metadata(ctx, data_ids)
    }

    /// Set accepted answer ranges (remaining accounts: feeds).
    pub fn set_min_max_answers<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        min_answers: Vec<i128>,
        max_answers: Vec<i128>,
    ) -> Result<()> {
        feeds::set_min_max_answers(ctx, data_ids, min_answers, max_answers)
    }

    /// Disable or enable writes of feeds for every workflow (remaining accounts: feeds).
    pub fn set_feed_writes_disabled<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        disabled: bool,
    ) -> Result<()> {
        feeds::set_feed_writes_disabled(ctx, data_ids, disabled)
    }

    /// Freeze or unfreeze reads of feeds (remaining accounts: feeds).
    pub fn set_feed_frozen<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        frozen: bool,
    ) -> Result<()> {
        feeds::set_feed_frozen(ctx, data_ids, frozen)
    }

    /// Set the fewest decimals reads of feeds may ask for (remaining accounts: feeds).
    pub fn set_min_decimals<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        min_decimals: u8,
    ) -> Result<()> {
        feeds::set_min_decimals(ctx, data_ids, min_decimals)
    }

    /// Link data IDs to the DF1 feeds this store keeps written (remaining accounts: feeds, then
    /// each linked DF1 feed).
    pub fn set_df1_feeds<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        df1_feeds: Vec<Pubkey>,
        write_disabled: Vec<bool>,
    ) -> Result<()> {
        feeds::set_df1_feeds(ctx, data_ids, df1_feeds, write_disabled)
    }

    /// Grow the number of rounds feeds keep (remaining accounts: feeds).
    pub fn set_max_stored_reports<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        max_stored_reports: Vec<u64>,
    ) -> Result<()> {
        feeds::set_max_stored_reports(ctx, data_ids, max_stored_reports)
    }

    // ================================================================
    // │                         Reports                              │
    // ================================================================

    /// Forwarder entrypoint. `report` is Borsh `Vec<ReceivedReport>`. Remaining accounts are one
    /// group per report: the data ID's feed, then the linked DF1 feed when the feed has one.
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

    /// The latest round of a data ID with the answer at `decimals`: at least the feed's minimum,
    /// at most 18. Fails for a frozen feed, or if a non-zero answer would round to zero.
    pub fn latest_round(ctx: Context<FeedRead>, data_id: [u8; 16], decimals: u8) -> Result<Round> {
        reads::latest_round(ctx, data_id, decimals)
    }

    /// A stored round of a data ID, as `latest_round`.
    pub fn get_round(
        ctx: Context<FeedRead>,
        data_id: [u8; 16],
        round_id: u64,
        decimals: u8,
    ) -> Result<Round> {
        reads::get_round(ctx, data_id, round_id, decimals)
    }

    /// Decimals answers are stored with (18), the most a read can ask for.
    pub fn decimals(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<u8> {
        reads::decimals(ctx, data_id)
    }

    /// Description of a data ID's feed, empty when none is set.
    pub fn description(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<String> {
        reads::description(ctx, data_id)
    }

    /// The fewest decimals reads of a data ID may ask for.
    pub fn get_min_decimals(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<u8> {
        reads::get_min_decimals(ctx, data_id)
    }

    /// Whether a workflow may write a data ID.
    pub fn check_feed_permission(
        ctx: Context<PermissionRead>,
        data_id: [u8; 16],
        workflow_metadata: WorkflowMetadata,
    ) -> Result<bool> {
        reads::check_feed_permission(ctx, data_id, workflow_metadata)
    }
}

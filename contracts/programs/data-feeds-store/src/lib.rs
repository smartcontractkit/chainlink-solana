#![allow(deprecated)] // anchor-lang 0.31 #[program] macro uses AccountInfo::realloc
//! Chainlink Data Feeds store for CRE: a Solana port of the EVM `DataFeedsStore` without SVR.
//! Reports arrive from the keystone forwarder through `on_report`, are stored per feed in a
//! circular buffer and served through a small reader interface that proxies call. Per data ID
//! report callbacks let other programs follow updates, the DF1 store uses one to keep its existing
//! feed accounts written. SVR data IDs are reserved in the data ID layout, not supported yet.
//! Events are CPI events (`emit_cpi!`).

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
use state::{CallbackConfigArgs, ReaderRound, WorkflowMetadata};

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

    /// Allow or disallow a program as a report callback. Its allowlist entry PDA
    /// `[CALLBACK_PROGRAM_SEED, state, callback_program]` is created on first use.
    pub fn set_callback_program_allowed(
        ctx: Context<SetCallbackProgramAllowed>,
        callback_program: Pubkey,
        allowed: bool,
    ) -> Result<()> {
        admin::set_callback_program_allowed(ctx, callback_program, allowed)
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

    /// Create feed accounts (remaining accounts: feed PDAs).
    pub fn init_feeds<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        feeds::init_feeds(ctx, data_ids)
    }

    /// Set ring sizes (remaining accounts: feed PDAs).
    pub fn set_max_stored_reports<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        max_stored_reports: Vec<u64>,
    ) -> Result<()> {
        feeds::set_max_stored_reports(ctx, data_ids, max_stored_reports)
    }

    /// Set accepted answer ranges (remaining accounts: feed PDAs).
    pub fn set_min_max_answers<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        min_answers: Vec<i128>,
        max_answers: Vec<i128>,
    ) -> Result<()> {
        feeds::set_min_max_answers(ctx, data_ids, min_answers, max_answers)
    }

    /// Set feed-specific workflow metadata (remaining accounts: feed PDAs).
    pub fn set_specific_feed_metadata<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        workflow_metadata: Vec<WorkflowMetadata>,
    ) -> Result<()> {
        settings::set_specific_feed_metadata(ctx, data_ids, workflow_metadata)
    }

    /// Remove feed-specific workflow metadata (remaining accounts: feed PDAs).
    pub fn remove_specific_feed_metadata<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        settings::remove_specific_feed_metadata(ctx, data_ids)
    }

    /// Configure report callbacks (remaining accounts: feed PDAs, then the program account and
    /// allowlist entry of every enabled callback).
    pub fn set_report_callback_configs<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        callbacks: Vec<CallbackConfigArgs>,
    ) -> Result<()> {
        settings::set_report_callback_configs(ctx, data_ids, callbacks)
    }

    /// Set feed descriptions (remaining accounts: feed description PDAs).
    pub fn set_feed_descriptions<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        indexed_feed_ids: Vec<[u8; 16]>,
        descriptions: Vec<String>,
    ) -> Result<()> {
        descriptions::set_feed_descriptions(ctx, indexed_feed_ids, descriptions)
    }

    // ================================================================
    // │                         Reports                              │
    // ================================================================

    /// Forwarder entrypoint. `report` is Borsh `Vec<ReceivedReport>`, remaining accounts are one
    /// group per report: the feed of its data ID, then the callback program, its callback authority
    /// PDA and the configured accounts when a callback is configured.
    pub fn on_report<'info>(
        ctx: Context<'_, '_, 'info, 'info, OnReport<'info>>,
        metadata: Vec<u8>,
        report: Vec<u8>,
    ) -> Result<()> {
        on_report::handler(ctx, metadata, report)
    }

    // ================================================================
    // │                    Reader interface                          │
    // ================================================================

    /// Latest round of a data ID, answer with 18 decimals.
    pub fn get_latest_round(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<ReaderRound> {
        reads::get_latest_round(ctx, data_id)
    }

    /// Latest rounds of up to 31 data IDs (remaining accounts: their feeds), answers with 18
    /// decimals. A feed without a report yields round ID 0.
    pub fn get_latest_rounds<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedsRead<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<Vec<ReaderRound>> {
        reads::get_latest_rounds(ctx, data_ids)
    }

    /// A stored round of a data ID, answer with 18 decimals.
    pub fn get_round(
        ctx: Context<FeedRead>,
        data_id: [u8; 16],
        round_id: u64,
    ) -> Result<ReaderRound> {
        reads::get_round(ctx, data_id, round_id)
    }

    /// Decimals reports are stored with.
    pub fn get_decimals(ctx: Context<DescriptionRead>, data_id: [u8; 16]) -> Result<u8> {
        reads::get_decimals(ctx, data_id)
    }

    /// Description of the feed of a data ID, empty when none is set.
    pub fn get_description(ctx: Context<DescriptionRead>, data_id: [u8; 16]) -> Result<String> {
        reads::get_description(ctx, data_id)
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

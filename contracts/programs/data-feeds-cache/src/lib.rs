pub mod instructions;
pub mod utils;

pub use instructions::*;

use anchor_lang::prelude::*;
use anchor_lang::solana_program::program::invoke_signed;

declare_id!("3kX63udXtYcsdj2737Wi2KGd2PhqiKPgAFAxstrjtRUa");

mod common;
mod context;
mod error;
mod event;
pub mod state;

use common::ZERO_DATA_ID;
use context::*;
use state::{CacheTransmission, DecimalReport, FeedConfig, WorkflowMetadata};

/// Data Feed Cache receives report updates from an pre-authorized Forwarder authority (sender)
/// only for pre-authorized data ids and workflows.
/// Feed Admins are highly priveleged roles who can unilaterally edit the configuration of feeds
/// and the authorization of workflows.
#[program]
pub mod data_feeds_cache {
    use super::*;

    /// Creates a new data cache instance with a dedicated state account.
    /// Sets the initial feed admins and state configuration.
    pub fn initialize(ctx: Context<Initialize>, feed_admins: Vec<Pubkey>) -> Result<()> {
        initialize::handler(ctx, feed_admins)
    }

    /// Not to be used in normal circumstances unless the original forwarder
    /// undergoes maintenance.
    pub fn update_forwarder_id(ctx: Context<UpdateForwarder>) -> Result<()> {
        update_forwarder_id::handler(ctx)
    }

    /// Add or remove a feed admin.
    /// Feed Admins are highly priveleged roles who can unilaterally edit the configuration of feeds
    /// and the authorization of workflows.
    pub fn set_feed_admin(ctx: Context<SetFeedAdmin>, admin: Pubkey, is_admin: bool) -> Result<()> {
        set_feed_admin::handler(ctx, admin, is_admin)
    }

    /// Step 1 of 2-step ownership process: propose a new owner
    pub fn transfer_ownership(
        ctx: Context<TransferOwnership>,
        proposed_owner: Pubkey,
    ) -> Result<()> {
        transfer_ownership::handler(ctx, proposed_owner)
    }

    /// Step 2 of 2-step ownership process: accept ownership
    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        accept_ownership::handler(ctx)
    }

    /// Closes the data id's associated decimal report account and feed config account.
    /// The feed config must have an empty workflow metadata list, by calling
    /// `set_decimal_feeds_config` prior to clear out the workflow metadata and
    /// close write permission flag accounts
    pub fn close_decimal_report(ctx: Context<CloseDecimalReport>, data_id: [u8; 16]) -> Result<()> {
        close_decimal_report::handler(ctx, data_id)
    }

    /// Create decimal report accounts, where report data lives (i.e answer, timestamp, etc.)
    /// Recommended limit of N = 20 data ids can be initialized at once.
    /// The decimal report account and feed config account must exist before reports can
    /// be received successfully in `on_report`
    pub fn init_decimal_reports<'info>(
        ctx: Context<'_, '_, 'info, 'info, InitDecimalReports<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        init_decimal_reports::handler(ctx, data_ids)
    }

    /// Initializes the legacy feeds config account, which stores the legacy store program
    /// and data id to legacy feed account mappings for double writing if enabled.
    /// Write disabled flags are set to 0 by default (writes enabled), however if
    /// optional legacy accounts are omitted in `on_report` context no legacy writes will occur
    /// (see `on_report`).
    /// Recommended limit of 15 legacy feeds can be initialized at once.
    /// Instruction does not verify internally if data id and associated legacy feed account
    /// are correct pairs or if aforementioned legacy feed account is owned by the legacy store.
    pub fn init_legacy_feeds_config(
        ctx: Context<InitLegacyFeedsConfig>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        init_legacy_feeds_config::handler(ctx, data_ids)
    }

    /// Updates legacy feed config.
    /// Recommended limit of 15 legacy feeds can be updated at once.
    /// Instruction does not verify internally if data id and associated legacy feed account
    /// are correct pairs or if aforementioned legacy feed account is owned by the legacy store.
    pub fn update_legacy_feeds_config(
        ctx: Context<UpdateLegacyFeedsConfig>,
        data_ids: Vec<[u8; 16]>,
        write_disabled: Vec<bool>,
    ) -> Result<()> {
        update_legacy_feeds_config::handler(ctx, data_ids, write_disabled)
    }

    /// Closes the legacy feeds config. Only to be used once all legacy feeds are
    /// no longer used.
    pub fn close_legacy_feeds_config(_ctx: Context<CloseLegacyFeedsConfig>) -> Result<()> {
        Ok(())
    }

    /// An instruction which is only meant to be simulated off-chain. This instruction
    /// does nothing beyond returning a list of permission accounts to be closed when
    /// calling `set_decimal_feed_configs`. No account state changes in this function.
    /// You should call this function before calling `set_decimal_feed_configs` in order
    /// to know the write permission accounts that must be passed into the `set_decimal_feed_configs`
    /// context to be closed.
    pub fn preview_decimal_feed_configs<'info>(
        ctx: Context<'_, '_, 'info, 'info, PreviewDecimalFeedConfigs<'info>>,
        data_ids: Vec<[u8; 16]>,
        descriptions: Vec<[u8; 32]>,
        workflow_metadatas: Vec<WorkflowMetadata>,
    ) -> Result<Vec<Pubkey>> {
        preview_decimal_feed_configs::handler(ctx, data_ids, descriptions, workflow_metadatas)
    }

    /// Given N data ids and M workflows, configures N feed config accounts and N*M write permission accounts.
    /// Creates feed config and permission accounts where they do not exist.
    /// All feed config accounts and permission accounts are passed as ctx.remaining_accounts (see SetDecimalFeedConfigs context).
    /// If you'd like to prepare the feed config account for closing by calling `close_decimal_report` you need to
    /// set the data id's feed config to an empty workflow metadata and [0; 32] (empty) description.
    /// Because there are two variables N and M which directly influence the size, the table in
    /// docs/data-feeds-cache/README.md#L297 shows safe ranges for N and M as guidelines.
    pub fn set_decimal_feed_configs<'info>(
        ctx: Context<'_, '_, 'info, 'info, SetDecimalFeedConfigs<'info>>,
        data_ids: Vec<[u8; 16]>,
        descriptions: Vec<[u8; 32]>,
        workflow_metadatas: Vec<WorkflowMetadata>,
    ) -> Result<()> {
        set_decimal_feed_configs::handler(ctx, data_ids, descriptions, workflow_metadatas)
    }

    /// Receives the a forwarder report that contains a list of [`ReceivedDecimalReport`]
    /// Maximum amount of 6 ReceivedDecimalReports can be included in the report before
    /// the transaction limit will be exceeded.
    /// For calculation look to ../../docs/data-feeds-cache/README.md#L579
    /// There are three optional accounts, all related to legacy feed writing.
    /// If you omit any of these or have write_disabled = 1 for all feeds
    /// then we guarentee no legacy feeds will be written to.
    pub fn on_report<'info>(
        ctx: Context<'_, '_, '_, 'info, OnReport<'info>>,
        metadata: Vec<u8>,
        report: Vec<u8>,
    ) -> Result<()> {
        on_report::handler(ctx, metadata, report)
    }

    /// Returns a feed's workflow metadata. Chunks return
    /// values by `max_count`.
    /// Can be used on-chain to verify a feed's configuration.
    /// If `start_index` is out of bounds the function will return an
    /// empty array.
    /// If `max_count = 0` then function will return the entire
    /// workflow metadata list.
    pub fn query_feed_metadata(
        ctx: Context<QueryFeedMetadata>,
        _data_id: [u8; 16],
        start_index: u8,
        max_count: u8,
    ) -> Result<Vec<WorkflowMetadata>> {
        query_feed_metadata::handler(ctx, _data_id, start_index, max_count)
    }

    /// The data ids passed in must match the ordering of the decimal report accounts
    /// passed into the remaining account context.
    pub fn query_values<'info>(
        ctx: Context<'_, '_, 'info, 'info, QueryValues<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<Vec<DecimalReport>> {
        query_values::handler(ctx, data_ids)
    }
}

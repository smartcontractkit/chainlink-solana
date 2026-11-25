//! Chainlink Data Feeds store for CRE. It receives reports, keeps each data ID's rounds in its
//! own feed account, and is the program that readers integrate with. The EVM and Stellar
//! implementations informed its design.
//!
//! Each data ID has one feed account. It holds the latest round, a circular buffer of earlier
//! rounds, a description, the settings that control reads (minimum decimals and a read freeze) and
//! the settings that control writes (the feed's own workflows, answer bounds, a write freeze and an
//! optional DF1 link). Readers pass the feed account and the store state to a read instruction, or
//! decode the feed account themselves with the `chainlink-solana-data-feeds-store` crate.
//!
//! Reports arrive from the keystone forwarder through `on_report`. Each report is checked against
//! the workflows allowed for its data ID (the feed's own list, or else the store's default list),
//! against the feed's latest round and against the feed's answer range.
//!
//! A feed admin creates feeds, with `set_feeds` or any per-feed setter, and pays their rent. The
//! forwarder passes no account that could pay for a new account.
//!
//! A data ID can also be linked to an existing DF1 feed account. The store then writes each new
//! round to that account through DF1's `submit`, after first checking everything that could make
//! `submit` fail. SVR data IDs are reserved in the data ID layout but not supported yet. Events are
//! CPI events (see the `event` module).
//!
//! Solana terms used in these docs:
//! - Account: a byte buffer at an address, owned by one program, which alone can change its data.
//! - PDA (program derived address): an address computed from seeds and a program ID. No private
//!   key exists for it, so only that program can sign as it. Its bump is the extra seed byte that
//!   keeps the address off the curve that normal keys lie on.
//! - Rent: the SOL deposit an account must hold for its size, paid by whoever creates it.
//! - CPI (cross-program invocation): one program calling another inside the same transaction. If
//!   the called program fails, the whole transaction fails.
//! - Remaining accounts: accounts an instruction takes after its fixed list, in an order the
//!   instruction defines.
//! - Slot: the chain's clock tick, about 400 ms.
//! - Zero-copy account: an account read in place as a fixed-layout struct, with no decoding step.

// anchor-lang 0.31's `#[program]` expansion calls the deprecated `AccountInfo::realloc`, and
// anchor-lang re-exports the upgradeable loader, whose ProgramData account `initialize` checks,
// only from a deprecated module.
#![allow(deprecated)]

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

    /// Creates a store. It creates the store's state account at the PDA `["state", store_id]`,
    /// so each store ID can hold one store. The program's upgrade authority (the key allowed to
    /// redeploy the program) must sign; it becomes the store's owner and pays the account's rent.
    pub fn initialize(ctx: Context<Initialize>, store_id: u64, config: StoreConfig) -> Result<()> {
        admin::initialize(ctx, store_id, config)
    }

    /// Proposes a new owner, step 1 of 2. The owner only changes when the proposed owner accepts,
    /// so proposing the default key cancels a pending proposal.
    pub fn transfer_ownership(ctx: Context<OwnerOnly>, proposed_owner: Pubkey) -> Result<()> {
        admin::transfer_ownership(ctx, proposed_owner)
    }

    /// Makes the proposed owner the owner, step 2 of 2. The proposed owner must sign.
    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        admin::accept_ownership(ctx)
    }

    /// Grants or revokes a key's feed admin permission, which covers every feed of the store.
    pub fn set_feed_admin(
        ctx: Context<OwnerOnly>,
        feed_admin: Pubkey,
        is_admin: bool,
    ) -> Result<()> {
        admin::set_feed_admin(ctx, feed_admin, is_admin)
    }

    /// Moves SPL tokens out of a token account owned by the store authority PDA. Only the owner
    /// can call it, to recover tokens sent to the store by mistake.
    pub fn recover_tokens(ctx: Context<RecoverTokens>, amount: u64) -> Result<()> {
        admin::recover_tokens(ctx, amount)
    }

    /// Moves SOL out of the store authority PDA. Only the owner can call it, to recover SOL sent
    /// to the store by mistake. It must take all of the SOL or leave at least the rent-exempt
    /// minimum.
    pub fn recover_native(ctx: Context<RecoverNative>, amount: u64) -> Result<()> {
        admin::recover_native(ctx, amount)
    }

    // ================================================================
    // │                       Feed admin                             │
    // ================================================================

    /// Replaces the store's default workflow list, which applies to every feed that has no
    /// workflow list of its own. An empty list leaves only feeds with their own list writable.
    pub fn set_default_feed_metadata(
        ctx: Context<FeedAdminState>,
        workflow_metadata: Vec<WorkflowMetadata>,
    ) -> Result<()> {
        admin::set_default_feed_metadata(ctx, workflow_metadata)
    }

    /// Sets how many seconds ahead of the chain's clock a report's observation timestamp may be.
    /// A report stamped further ahead is skipped with `FutureReportIgnored`.
    pub fn set_max_observation_forward_duration(
        ctx: Context<FeedAdminState>,
        max_observation_forward_secs: u32,
    ) -> Result<()> {
        admin::set_max_observation_forward_duration(ctx, max_observation_forward_secs)
    }

    /// Creates the feeds that don't exist yet and sets each feed's description. If
    /// `workflow_metadata` isn't empty, it also becomes each listed feed's own workflow list. The
    /// remaining accounts are the feeds, in data ID order. Every per-feed setter below except
    /// `remove_specific_feed_metadata` also creates missing feeds, and the feed admin pays their
    /// rent.
    pub fn set_feeds<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        descriptions: Vec<String>,
        workflow_metadata: Vec<WorkflowMetadata>,
    ) -> Result<()> {
        feeds::set_feeds(ctx, data_ids, descriptions, workflow_metadata)
    }

    /// Sets each feed's description. An empty string clears it. The remaining accounts are the
    /// feeds, in data ID order.
    pub fn set_feed_descriptions<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        descriptions: Vec<String>,
    ) -> Result<()> {
        feeds::set_feed_descriptions(ctx, data_ids, descriptions)
    }

    /// Gives each feed its own workflow list, replacing any list it had. The remaining accounts
    /// are the feeds, in data ID order.
    pub fn set_specific_feed_metadata<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        workflow_metadata: Vec<WorkflowMetadata>,
    ) -> Result<()> {
        feeds::set_specific_feed_metadata(ctx, data_ids, workflow_metadata)
    }

    /// Removes each feed's own workflow list, so the store's default list applies to it again. It
    /// fails for a feed with no list of its own. The remaining accounts are the feeds, in data ID
    /// order.
    pub fn remove_specific_feed_metadata<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        feeds::remove_specific_feed_metadata(ctx, data_ids)
    }

    /// Sets the range of answers each feed accepts. Passing `i128::MIN` and `i128::MAX` removes
    /// the range. The remaining accounts are the feeds, in data ID order.
    pub fn set_min_max_answers<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        min_answers: Vec<i128>,
        max_answers: Vec<i128>,
    ) -> Result<()> {
        feeds::set_min_max_answers(ctx, data_ids, min_answers, max_answers)
    }

    /// Disables or enables writes to each feed. While writes are disabled, every report for the
    /// feed is skipped, whichever workflow sent it. The remaining accounts are the feeds, in data
    /// ID order.
    pub fn set_feed_writes_disabled<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        disabled: bool,
    ) -> Result<()> {
        feeds::set_feed_writes_disabled(ctx, data_ids, disabled)
    }

    /// Freezes or unfreezes reads of each feed. While a feed is frozen, `latest_round`, `get_round`,
    /// `decimals` and `description` fail, `get_min_decimals` stays readable, and reports are still
    /// stored. The remaining accounts are the feeds, in data ID order.
    pub fn set_feed_frozen<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        frozen: bool,
    ) -> Result<()> {
        feeds::set_feed_frozen(ctx, data_ids, frozen)
    }

    /// Sets the fewest decimals a read of each feed may ask for, at most 18. The remaining
    /// accounts are the feeds, in data ID order.
    pub fn set_min_decimals<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        min_decimals: u8,
    ) -> Result<()> {
        feeds::set_min_decimals(ctx, data_ids, min_decimals)
    }

    /// Links each data ID to a DF1 feed account that the store then writes every new round to, or
    /// unlinks it when given the default key. It also enables or disables that write. A DF1 feed
    /// can be linked from only one data ID. The remaining accounts are the feeds, then for each
    /// data ID in order: the link marker of its current DF1 feed if the link changes away from it,
    /// then, if its new DF1 feed isn't the default key (even when it is unchanged), that DF1 feed
    /// and its link marker. `instructions::feeds::set_df1_feeds` has a worked example.
    pub fn set_df1_feeds<'info>(
        ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
        data_ids: Vec<[u8; 16]>,
        df1_feeds: Vec<Pubkey>,
        write_disabled: Vec<bool>,
    ) -> Result<()> {
        feeds::set_df1_feeds(ctx, data_ids, df1_feeds, write_disabled)
    }

    /// Grows the number of rounds each feed keeps. A ring can only grow, and only before it first
    /// wraps. The remaining accounts are the feeds, in data ID order.
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

    /// Receives reports from the keystone forwarder. The store takes the workflow name and owner
    /// from `metadata`, and `report` is a Borsh-encoded `Vec<ReceivedReport>`. The remaining
    /// accounts hold one group per report: the feed of its data ID, then the linked DF1 feed if
    /// that feed has one (see `context::OnReport`).
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

    /// Returns the latest round of a data ID, with the answer at `decimals`, which must be at
    /// least the feed's minimum and at most 18. It fails if the feed is frozen, if it has no round
    /// yet, or if a non-zero answer would round to zero.
    pub fn latest_round(ctx: Context<FeedRead>, data_id: [u8; 16], decimals: u8) -> Result<Round> {
        reads::latest_round(ctx, data_id, decimals)
    }

    /// Returns a stored round of a data ID, with the answer at `decimals`. It fails as
    /// `latest_round` does, and also when the round was never stored or has been overwritten in
    /// the ring.
    pub fn get_round(
        ctx: Context<FeedRead>,
        data_id: [u8; 16],
        round_id: u64,
        decimals: u8,
    ) -> Result<Round> {
        reads::get_round(ctx, data_id, round_id, decimals)
    }

    /// Returns the number of decimals answers are stored with, which is 18. This is also the most
    /// decimals a read can ask for. It fails with `NoDataPresent` for an unset feed and
    /// `FeedFrozen` for a frozen one.
    pub fn decimals(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<u8> {
        reads::decimals(ctx, data_id)
    }

    /// Returns the description of a data ID's feed, or an empty string if none is set. It fails
    /// with `NoDataPresent` for an unset feed and `FeedFrozen` for a frozen one.
    pub fn description(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<String> {
        reads::description(ctx, data_id)
    }

    /// Returns the fewest decimals a read of a data ID may ask for. It fails with `NoDataPresent`
    /// for an unset feed but does not check the read freeze.
    pub fn get_min_decimals(ctx: Context<FeedRead>, data_id: [u8; 16]) -> Result<u8> {
        reads::get_min_decimals(ctx, data_id)
    }

    /// Returns whether a workflow may write a data ID. It returns false if the feed hasn't been
    /// set, since reports for it are skipped. Otherwise it checks the feed's own workflow list, or
    /// the store's default list if the feed has none. It takes the data ID's feed account and
    /// fails with `InvalidDataId` for a zero or SVR data ID.
    pub fn check_feed_permission(
        ctx: Context<PermissionRead>,
        data_id: [u8; 16],
        workflow_metadata: WorkflowMetadata,
    ) -> Result<bool> {
        reads::check_feed_permission(ctx, data_id, workflow_metadata)
    }
}

#![allow(deprecated)] // anchor-lang 0.31 #[program] macro uses AccountInfo::realloc
//! Chainlink Data Feeds proxy: the program readers integrate with. One proxy owns every feed
//! account, as DF1's store does: a feed's latest round, a circular buffer of its history, its
//! description, the fewest decimals it may be read at and a frozen flag. Readers pass that one
//! account and either call a read here or decode it themselves.
//!
//! The proxy holds no write logic. It trusts one store (its signer, set by the owner like Stellar's
//! `set_cache`), which checks reports and writes them here. Because the round counter lives in the
//! feed account, round IDs keep increasing when the store is replaced, so there are no phase IDs.

pub mod admin;
pub mod common;
pub mod context;
pub mod error;
pub mod event;
pub mod logic;
pub mod reads;
pub mod state;
pub mod utils;
pub mod writer;

use anchor_lang::prelude::*;

use context::*;
use state::{NewRound, Round};

declare_id!("CeRGs3KJ4HXk4S6eyj8Zs2yp4mZdT4zSLz3qrQVRnfuT");

#[program]
pub mod data_feeds_proxy {
    use super::*;

    // ================================================================
    // │                         Owner                                │
    // ================================================================

    /// Create a proxy trusting `writer` (the store's signer), in a pre-created state account,
    /// signed by the program's upgrade authority, which becomes the proxy's owner.
    pub fn initialize(ctx: Context<Initialize>, writer: Pubkey) -> Result<()> {
        admin::initialize(ctx, writer)
    }

    /// Step 1 of 2: propose a new owner.
    pub fn transfer_ownership(ctx: Context<OwnerOnly>, proposed_owner: Pubkey) -> Result<()> {
        admin::transfer_ownership(ctx, proposed_owner)
    }

    /// Step 2 of 2: the proposed owner accepts.
    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        admin::accept_ownership(ctx)
    }

    /// Replace the trusted store's signer. Feeds, history and round IDs carry on.
    pub fn set_store(ctx: Context<OwnerOnly>, writer: Pubkey) -> Result<()> {
        admin::set_store(ctx, writer)
    }

    /// Set the fewest decimals reads of a data ID may ask for, the stored decimals by default.
    pub fn set_min_decimals(
        ctx: Context<SetMinDecimals>,
        data_id: [u8; 16],
        min_decimals: u8,
    ) -> Result<()> {
        admin::set_min_decimals(ctx, data_id, min_decimals)
    }

    // ================================================================
    // │                     Trusted store                            │
    // ================================================================

    /// Create feeds (remaining accounts: their PDAs), existing ones are left as they are.
    pub fn create_feeds<'info>(
        ctx: Context<'_, '_, 'info, 'info, WriterPayer<'info>>,
        data_ids: Vec<[u8; 16]>,
        bumps: Vec<u8>,
        max_stored_reports: u64,
    ) -> Result<()> {
        writer::create_feeds(ctx, data_ids, bumps, max_stored_reports)
    }

    /// Set feed descriptions, empty to clear (remaining accounts: the feeds).
    pub fn set_feed_descriptions<'info>(
        ctx: Context<'_, '_, 'info, 'info, WriterOnly<'info>>,
        data_ids: Vec<[u8; 16]>,
        descriptions: Vec<String>,
    ) -> Result<()> {
        writer::set_feed_descriptions(ctx, data_ids, descriptions)
    }

    /// Freeze or unfreeze reads of feeds (remaining accounts: the feeds).
    pub fn set_feed_frozen<'info>(
        ctx: Context<'_, '_, 'info, 'info, WriterOnly<'info>>,
        data_ids: Vec<[u8; 16]>,
        frozen: bool,
    ) -> Result<()> {
        writer::set_feed_frozen(ctx, data_ids, frozen)
    }

    /// Grow feed rings (remaining accounts: the feeds).
    pub fn set_max_stored_reports<'info>(
        ctx: Context<'_, '_, 'info, 'info, WriterPayer<'info>>,
        data_ids: Vec<[u8; 16]>,
        max_stored_reports: Vec<u64>,
    ) -> Result<()> {
        writer::set_max_stored_reports(ctx, data_ids, max_stored_reports)
    }

    /// Append one round per feed (remaining accounts: the feeds, in order).
    pub fn write_rounds<'info>(
        ctx: Context<'_, '_, 'info, 'info, WriterOnly<'info>>,
        rounds: Vec<NewRound>,
    ) -> Result<()> {
        writer::write_rounds(ctx, rounds)
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

    /// The trusted store's signer.
    pub fn get_store(ctx: Context<StateRead>) -> Result<Pubkey> {
        reads::get_store(ctx)
    }
}

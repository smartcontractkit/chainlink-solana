#![allow(deprecated)] // anchor-lang 0.31 #[program] macro uses AccountInfo::realloc
//! Chainlink Data Feeds proxy: a stable address readers integrate with, in front of a data feeds
//! store that can change. Like Stellar's data feeds proxy, one proxy serves every feed: the reader
//! passes the data ID, and optionally the decimals it wants.
//!
//! Reads call the configured store's reader interface (`get_latest_round`, `get_round`), so any
//! store version or program that implements that interface can be pointed at with `set_store`
//! without readers changing the proxy they call. They do pass the accounts of the store behind it,
//! which they can resolve from the proxy state.

pub mod admin;
pub mod common;
pub mod context;
pub mod error;
pub mod event;
pub mod reads;
pub mod state;

use anchor_lang::prelude::*;

use context::*;
use state::Round;

declare_id!("CeRGs3KJ4HXk4S6eyj8Zs2yp4mZdT4zSLz3qrQVRnfuT");

#[program]
pub mod data_feeds_proxy {
    use super::*;

    // ================================================================
    // │                         Owner                                │
    // ================================================================

    /// Create a proxy serving reads from a store. `feed_admins` must be strictly increasing.
    pub fn initialize(ctx: Context<Initialize>, feed_admins: Vec<Pubkey>) -> Result<()> {
        admin::initialize(ctx, feed_admins)
    }

    /// Step 1 of 2: propose a new owner.
    pub fn transfer_ownership(ctx: Context<OwnerOnly>, proposed_owner: Pubkey) -> Result<()> {
        admin::transfer_ownership(ctx, proposed_owner)
    }

    /// Step 2 of 2: the proposed owner accepts.
    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        admin::accept_ownership(ctx)
    }

    /// Grant or revoke the permission to set minimum decimals.
    pub fn set_feed_admin(
        ctx: Context<OwnerOnly>,
        feed_admin: Pubkey,
        is_admin: bool,
    ) -> Result<()> {
        admin::set_feed_admin(ctx, feed_admin, is_admin)
    }

    /// Point the proxy at another store, readers keep the proxy address.
    pub fn set_store(ctx: Context<SetStore>) -> Result<()> {
        admin::set_store(ctx)
    }

    // ================================================================
    // │                        Feed admin                            │
    // ================================================================

    /// Set the fewest decimals `read_scaled` serves a data ID at, the stored decimals by default.
    pub fn set_min_decimals(
        ctx: Context<SetMinDecimals>,
        data_id: [u8; 16],
        min_decimals: u8,
    ) -> Result<()> {
        admin::set_min_decimals(ctx, data_id, min_decimals)
    }

    // ================================================================
    // │                          Reads                               │
    // ================================================================

    /// The fewest decimals `read_scaled` serves a data ID at.
    pub fn get_min_decimals(ctx: Context<GetMinDecimals>, data_id: [u8; 16]) -> Result<u8> {
        admin::get_min_decimals(ctx, data_id)
    }

    /// The latest round of a data ID, or the round with `round_id`, with the store's 18 decimals.
    pub fn read(ctx: Context<Read>, data_id: [u8; 16], round_id: Option<u64>) -> Result<Round> {
        reads::read(ctx, data_id, round_id)
    }

    /// Like `read`, with the answer at `decimals`: at least the data ID's minimum and at most 18.
    /// Fails if a non-zero answer would round to zero.
    pub fn read_scaled(
        ctx: Context<ReadScaled>,
        data_id: [u8; 16],
        decimals: u8,
        round_id: Option<u64>,
    ) -> Result<Round> {
        reads::read_scaled(ctx, data_id, decimals, round_id)
    }
}

use std::mem::size_of;

use anchor_lang::prelude::*;
use arrayvec::arrayvec;
use chainlink_solana_data_feeds_store::ReaderRound;
use static_assertions::const_assert_eq;

use crate::common::MAX_FEED_ADMINS;

/// Proxy instance: ownership, admins and the store reads are served from. The address of this
/// account and the program id are what readers integrate with, the store behind them can change.
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct ProxyState {
    pub version: u8,
    pub _padding0: [u8; 7],
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    /// Program of the store reads are served from
    pub store_program: Pubkey,
    /// State account of that store
    pub store_state: Pubkey,
    /// Admins allowed to set per-feed minimum decimals
    pub feed_admins: AccountList,
    /// Room for later settings, e.g. serving some data IDs from another program
    pub _reserved: [u8; 128],
}

/// Fixed size list of public keys
#[zero_copy]
#[derive(InitSpace)]
pub struct AccountList {
    pub xs: [Pubkey; MAX_FEED_ADMINS],
    pub len: u64,
}
arrayvec!(AccountList, Pubkey, u64);

const_assert_eq!(size_of::<ProxyState>(), 784);

/// Per data ID settings. A data ID without one has the strictest minimum, the stored decimals.
#[account]
#[derive(InitSpace)]
pub struct FeedConfig {
    pub version: u8,
    pub state: Pubkey,
    /// Data ID zero padded to 32 bytes
    pub data_id: [u8; 32],
    /// Fewest decimals `read_scaled` serves this data ID at, at most the stored decimals
    pub min_decimals: u8,
    /// Room for later per-feed read settings, e.g. the EVM store's SVR fallback time and sync
    /// iterations if SVR is added
    pub _reserved: [u8; 32],
}

/// A round as returned by the proxy. `read` returns the store's answer as is, with the store's
/// 18 decimals, `read_scaled` returns it with the requested decimals.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, Default, PartialEq)]
pub struct Round {
    pub round_id: u64,
    pub answer: i128,
    /// Offchain observations timestamp
    pub observations_timestamp: u32,
    /// Onchain recorded timestamp
    pub recorded_timestamp: u32,
}

impl From<ReaderRound> for Round {
    fn from(round: ReaderRound) -> Self {
        Self {
            round_id: round.round_id,
            answer: round.answer,
            observations_timestamp: round.observations_timestamp,
            recorded_timestamp: round.recorded_timestamp,
        }
    }
}

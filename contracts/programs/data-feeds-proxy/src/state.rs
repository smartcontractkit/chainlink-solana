use std::mem::size_of;

use anchor_lang::prelude::*;
use static_assertions::const_assert_eq;

use crate::common::{ANCHOR_DISCRIMINATOR, DESCRIPTION_LEN};

/// Proxy instance: its owner and the one store trusted to write its feeds. The program ID, this
/// account and the feed accounts under it are what readers integrate with.
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct ProxyState {
    pub version: u8,
    pub _padding0: [u8; 7],
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    /// The store's signer, the only key that can create, configure and write feeds
    pub writer: Pubkey,
    pub _reserved: [u8; 128],
}

/// One feed: its latest round and settings, followed in the account by a circular buffer of
/// `max_stored_reports` [`StoredReport`]s, round `r` at slot `r % max_stored_reports`. This is the
/// account readers pass, and its layout is public: fields only move with a version bump.
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct Feed {
    pub version: u8,
    /// Reads refuse the feed while set, writes continue
    pub frozen: u8,
    /// Fewest decimals a read may ask for
    pub min_decimals: u8,
    pub _padding0: u8,
    pub latest_observations_timestamp: u32,
    pub latest_recorded_timestamp: u32,
    pub _padding1: [u8; 4],
    /// Data ID zero padded to 32 bytes
    pub data_id: [u8; 32],
    /// Proxy state the feed belongs to
    pub state: Pubkey,
    /// Latest round ID, 0 before the first round
    pub round_id: u64,
    /// Kept free for SVR: the EVM store tracks the newest round released to non-SVR readers here
    /// (`visibleRoundId`), so SVR can be added without moving the fields below.
    pub _reserved0: [u8; 8],
    pub max_stored_reports: u64,
    pub latest_recorded_slot: u64,
    /// Latest answer with `STORED_DECIMALS`
    pub latest_answer: i128,
    /// UTF-8 description, e.g. "ETH / USD", zero padded, empty when none is set
    pub description: [u8; DESCRIPTION_LEN],
    pub _reserved: [u8; 64],
}

/// Offset of the circular buffer in a feed account.
pub const FEED_RING_OFFSET: usize = ANCHOR_DISCRIMINATOR + size_of::<Feed>();

/// Length of a feed account with a ring of `max_stored_reports`, saturating: an absurd ring size
/// gives an absurd length that every bound check rejects, instead of an overflow panic.
pub fn feed_account_len(max_stored_reports: u64) -> usize {
    (max_stored_reports as usize)
        .saturating_mul(size_of::<StoredReport>())
        .saturating_add(FEED_RING_OFFSET)
}

/// One stored round of a feed's history.
#[zero_copy]
#[derive(Default, Debug, PartialEq, Eq, InitSpace)]
pub struct StoredReport {
    /// Answer with `STORED_DECIMALS`
    pub answer: i128,
    /// When the observations were made offchain
    pub observations_timestamp: u32,
    /// When the round was written onchain
    pub recorded_timestamp: u32,
    /// Slot the round was written in
    pub recorded_slot: u64,
}

const_assert_eq!(size_of::<ProxyState>(), 232);
const_assert_eq!(size_of::<Feed>(), 256);
const_assert_eq!(size_of::<StoredReport>(), 32);
// Offsets readers decode feed accounts by (add 8 for the discriminator); `state` is the
// getProgramAccounts filter for a proxy's feeds.
const_assert_eq!(std::mem::offset_of!(Feed, frozen), 1);
const_assert_eq!(std::mem::offset_of!(Feed, min_decimals), 2);
const_assert_eq!(std::mem::offset_of!(Feed, latest_observations_timestamp), 4);
const_assert_eq!(std::mem::offset_of!(Feed, data_id), 16);
const_assert_eq!(std::mem::offset_of!(Feed, state), 48);
const_assert_eq!(std::mem::offset_of!(Feed, round_id), 80);
const_assert_eq!(std::mem::offset_of!(Feed, max_stored_reports), 96);
const_assert_eq!(std::mem::offset_of!(Feed, latest_answer), 112);
const_assert_eq!(std::mem::offset_of!(Feed, description), 128);

// ================================================================
// │                Instruction arguments and returns             │
// ================================================================

/// A round to append, written by the store.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Debug, PartialEq)]
pub struct NewRound {
    /// Offchain observations timestamp, must be newer than the feed's latest
    pub observations_timestamp: u32,
    /// Answer with `STORED_DECIMALS`
    pub answer: i128,
}

/// A round as returned by reads, with the answer at the decimals the reader asked for.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, Default, PartialEq)]
pub struct Round {
    pub round_id: u64,
    pub answer: i128,
    /// Offchain observations timestamp
    pub observations_timestamp: u32,
    /// Onchain recorded timestamp
    pub recorded_timestamp: u32,
}

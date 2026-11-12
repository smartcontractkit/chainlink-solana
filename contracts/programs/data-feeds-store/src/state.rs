use std::mem::size_of;

use anchor_lang::prelude::{
    borsh::{BorshDeserialize, BorshSerialize},
    *,
};
use arrayvec::arrayvec;
use static_assertions::const_assert_eq;

use crate::common::{
    ANCHOR_DISCRIMINATOR, DESCRIPTION_LEN, MAX_FEED_ADMINS, MAX_WORKFLOW_METADATAS,
};

/// Store instance: ownership, admins, the DF1 program it keeps written and the defaults applied to
/// every feed.
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct StoreState {
    pub version: u8,
    /// Bump of the store authority PDA, which only holds recoverable tokens and SOL
    pub store_authority_bump: u8,
    /// Bump of the `df1_writer` PDA, the writer of DF1 feeds this store keeps written
    pub df1_writer_bump: u8,
    pub _padding0: u8,
    /// Maximum seconds a report's observation timestamp may lead the clock before it is ignored
    pub max_observation_forward_secs: u32,
    /// Ring size of the feeds this store creates
    pub default_max_stored_reports: u64,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    /// DF1 store program whose feeds this store can keep written
    pub df1_program: Pubkey,
    pub feed_admins: AccountList,
    /// Workflows allowed to write any data ID without feed-specific metadata
    pub default_workflow_metadata: WorkflowMetadataList,
    pub _reserved: [u8; 64],
}

/// One feed: its latest round, how it is read and how it is written, followed in the account by a
/// circular buffer of `max_stored_reports` [`StoredReport`]s, round `r` at slot `r % max_stored_reports`.
/// This is the account readers pass, and its layout is public: fields only move with a version bump.
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct Feed {
    pub version: u8,
    /// Reads refuse the feed while set, writes continue
    pub frozen: u8,
    /// Fewest decimals a read may ask for
    pub min_decimals: u8,
    /// Reports are skipped for every workflow while set, reads continue
    pub writes_disabled: u8,
    pub latest_observations_timestamp: u32,
    pub latest_recorded_timestamp: u32,
    /// Whether an accepted answer range is set, unbounded otherwise
    pub min_max_configured: u8,
    /// The DF1 write-through is skipped while set, the round is still stored
    pub df1_write_disabled: u8,
    pub _padding0: [u8; 2],
    /// Data ID zero padded to 32 bytes, see [`crate::data_id::DataIdSeed`]
    pub data_id: [u8; 32],
    /// Store state the feed belongs to
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
    /// DF1 feed account kept written with this data ID's rounds, default for none
    pub df1_feed: Pubkey,
    pub min_answer: i128,
    pub max_answer: i128,
    /// Workflows allowed to write this data ID, the store's default workflows when empty
    pub specific_workflow_metadata: WorkflowMetadataList,
    pub _reserved: [u8; 72],
}

impl Feed {
    pub fn has_df1_feed(&self) -> bool {
        self.df1_feed != Pubkey::default()
    }
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

/// Fixed size list of public keys
#[zero_copy]
#[derive(InitSpace)]
pub struct AccountList {
    pub xs: [Pubkey; MAX_FEED_ADMINS],
    pub len: u64,
}
arrayvec!(AccountList, Pubkey, u64);

/// Fixed size list of workflow metadata
#[zero_copy]
#[derive(InitSpace)]
pub struct WorkflowMetadataList {
    pub xs: [WorkflowMetadata; MAX_WORKFLOW_METADATAS],
    pub len: u64,
}
arrayvec!(WorkflowMetadataList, WorkflowMetadata, u64);

/// A workflow authorized to write feeds
#[zero_copy]
#[derive(InitSpace, BorshSerialize, BorshDeserialize, PartialEq, Eq, Debug)]
pub struct WorkflowMetadata {
    /// Address allowed to send new reports, the forwarder authority PDA of this store
    pub allowed_sender: Pubkey,
    pub allowed_workflow_owner: [u8; 20],
    pub allowed_workflow_name: [u8; 10],
}

const_assert_eq!(size_of::<StoreState>(), 3232);
const_assert_eq!(size_of::<Feed>(), 1328);
const_assert_eq!(size_of::<StoredReport>(), 32);
// Offsets readers decode feed accounts by (add 8 for the discriminator); `state` is the
// getProgramAccounts filter for a store's feeds.
const_assert_eq!(std::mem::offset_of!(Feed, frozen), 1);
const_assert_eq!(std::mem::offset_of!(Feed, min_decimals), 2);
const_assert_eq!(std::mem::offset_of!(Feed, latest_observations_timestamp), 4);
const_assert_eq!(std::mem::offset_of!(Feed, data_id), 16);
const_assert_eq!(std::mem::offset_of!(Feed, state), 48);
const_assert_eq!(std::mem::offset_of!(Feed, round_id), 80);
const_assert_eq!(std::mem::offset_of!(Feed, max_stored_reports), 96);
const_assert_eq!(std::mem::offset_of!(Feed, latest_answer), 112);
const_assert_eq!(std::mem::offset_of!(Feed, description), 128);
const_assert_eq!(std::mem::offset_of!(Feed, df1_feed), 192);
const_assert_eq!(std::mem::offset_of!(Feed, specific_workflow_metadata), 256);

// ================================================================
// │                Instruction arguments and returns             │
// ================================================================

/// Report entry received from the forwarder, Borsh `Vec<ReceivedReport>` is the report payload.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, PartialEq)]
pub struct ReceivedReport {
    pub data_id: [u8; 16],
    /// Offchain observations timestamp
    pub timestamp: u32,
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

/// What a store is created with.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, PartialEq)]
pub struct StoreConfig {
    /// Strictly increasing
    pub feed_admins: Vec<Pubkey>,
    /// Ring size of the feeds the store creates
    pub default_max_stored_reports: u64,
    pub max_observation_forward_secs: u32,
    /// DF1 store program whose feeds this store can keep written
    pub df1_program: Pubkey,
}

/// Why a round was stored without writing its DF1 feed.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Debug, PartialEq, Eq)]
pub enum Df1SkipReason {
    /// The feed's DF1 write-through is disabled
    Disabled,
    /// The transaction did not pass the DF1 program and writer
    NotPassed,
    /// Not a DF1 feed account of the configured program, or a layout `submit` can't write
    InvalidAccount,
    /// Not the feed layout version `submit` writes
    InvalidVersion,
    /// The DF1 feed's writer is not this store
    NotWriter,
    /// The DF1 feed already has a round as new or newer
    Stale,
    /// The answer doesn't fit the DF1 feed's decimals
    AnswerOverflow,
    /// The DF1 feed's round counter is exhausted
    RoundIdOverflow,
    /// The DF1 feed account was not passed writable
    NotWritable,
}

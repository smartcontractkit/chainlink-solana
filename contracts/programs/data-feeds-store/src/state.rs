use std::mem::size_of;

use anchor_lang::prelude::{
    borsh::{BorshDeserialize, BorshSerialize},
    *,
};
use arrayvec::arrayvec;
use static_assertions::const_assert_eq;

use crate::common::{MAX_FEED_ADMINS, MAX_WORKFLOW_METADATAS};

/// Store instance: ownership, admins, the proxy it writes, the DF1 program it keeps written and the
/// defaults applied to every feed.
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct StoreState {
    pub version: u8,
    /// Bump of the store authority PDA, the vault paying for feeds created by their first report
    pub store_authority_bump: u8,
    /// Bump of the `feed_writer` PDA, the proxy's trusted writer
    pub feed_writer_bump: u8,
    /// Bump of the `df1_writer` PDA, the writer of DF1 feeds this store keeps written
    pub df1_writer_bump: u8,
    /// Maximum seconds a report's observation timestamp may lead the clock before it is ignored
    pub max_observation_forward_secs: u32,
    /// Ring size of the feeds this store creates
    pub default_max_stored_reports: u64,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    /// Proxy program owning the feed accounts
    pub proxy_program: Pubkey,
    /// Proxy state trusting this store's `feed_writer`
    pub proxy_state: Pubkey,
    /// DF1 store program whose feeds this store can keep written
    pub df1_program: Pubkey,
    pub feed_admins: AccountList,
    /// Workflows allowed to write any data ID without feed-specific metadata
    pub default_workflow_metadata: WorkflowMetadataList,
    pub _reserved: [u8; 64],
}

/// Write settings of one data ID. A data ID without one is written through the default workflow
/// metadata only, and gets one when its feed is created or a feed admin first configures it.
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct FeedConfig {
    pub version: u8,
    /// Reports for the data ID are skipped for every workflow while set, reads continue
    pub writes_disabled: u8,
    /// Whether an accepted answer range is set, unbounded otherwise
    pub min_max_configured: u8,
    /// The DF1 write-through is skipped while set, the round is still stored
    pub df1_write_disabled: u8,
    pub _padding0: [u8; 4],
    /// Data ID zero padded to 32 bytes, see [`crate::data_id::DataIdSeed`]
    pub data_id: [u8; 32],
    pub state: Pubkey,
    /// DF1 feed account kept written with this data ID's rounds, default for none
    pub df1_feed: Pubkey,
    pub _padding1: [u8; 8],
    pub min_answer: i128,
    pub max_answer: i128,
    /// Workflows allowed to write this data ID, the store's default workflows when empty
    pub specific_workflow_metadata: WorkflowMetadataList,
    pub _reserved: [u8; 56],
}

impl FeedConfig {
    pub fn has_df1_feed(&self) -> bool {
        self.df1_feed != Pubkey::default()
    }
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

const_assert_eq!(size_of::<StoreState>(), 3296);
const_assert_eq!(size_of::<FeedConfig>(), 1200);
const_assert_eq!(std::mem::offset_of!(FeedConfig, data_id), 8);
const_assert_eq!(std::mem::offset_of!(FeedConfig, state), 40);
const_assert_eq!(std::mem::offset_of!(FeedConfig, df1_feed), 72);
const_assert_eq!(std::mem::offset_of!(FeedConfig, min_answer), 112);
const_assert_eq!(
    std::mem::offset_of!(FeedConfig, specific_workflow_metadata),
    144
);

/// Report entry received from the forwarder, Borsh `Vec<ReceivedReport>` is the report payload.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, PartialEq)]
pub struct ReceivedReport {
    pub data_id: [u8; 16],
    /// Offchain observations timestamp
    pub timestamp: u32,
    /// Answer with `STORED_DECIMALS`
    pub answer: i128,
}

/// Why a feed was not created by its first report.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Debug, PartialEq, Eq)]
pub enum FeedCreationSkipReason {
    /// The transaction did not pass the vault and system program
    VaultNotPassed,
    /// The vault can't pay the rent and stay rent exempt
    InsufficientVaultBalance,
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

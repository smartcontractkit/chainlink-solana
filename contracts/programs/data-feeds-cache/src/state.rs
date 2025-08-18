use std::mem;

use anchor_lang::prelude::{
    borsh::{BorshDeserialize, BorshSerialize},
    *,
};
use arrayvec::arrayvec;
use static_assertions::const_assert;

use crate::common::MAX_WORKFLOW_METADATAS;

/// Cache State account contains owners and admin
/// information in addition to the bump/nonce for the
/// PDA which writes to legacy data feeds
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct CacheState {
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub feed_admins: AccountList,
    pub forwarder_id: Pubkey,
    pub legacy_writer_bump: u8, // pda writing to the legacy feeds
    pub _padding: [u8; 7],
}

/// Decimal report received by the cache from the forwarder
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, PartialEq)]
pub struct ReceivedDecimalReport {
    pub timestamp: u32,
    pub answer: u128,
    pub data_id: [u8; 16],
}

/// Report sent to legacy feed
#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct CacheTransmission {
    pub timestamp: u32,
    pub answer: u128,
}

/// Decimal Report stored
#[account]
#[derive(InitSpace)]
pub struct DecimalReport {
    pub timestamp: u32,
    pub answer: u128,
}

/// Contains feed information such as description
/// and the authorized workflows permitted to
/// report on the data id. Account key is derived by the data id.
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct FeedConfig {
    // UTF-bytes encoded
    pub description: [u8; 32],
    pub workflow_metadata: WorkflowMetadataList,
}

/// Fixed size struct which stores list of public keys
#[zero_copy]
#[derive(InitSpace)]
pub struct AccountList {
    pub xs: [Pubkey; MAX_ENTRIES],
    pub len: u64,
}
arrayvec!(AccountList, Pubkey, u64);
const_assert!(
    mem::size_of::<AccountList>() == mem::size_of::<u64>() + mem::size_of::<Pubkey>() * MAX_ENTRIES
);

/// Fixed size struct which stores list of workflow metadatas
#[zero_copy]
#[derive(InitSpace)]
pub struct WorkflowMetadataList {
    pub xs: [WorkflowMetadata; MAX_WORKFLOW_METADATAS],
    pub len: u64,
}
arrayvec!(WorkflowMetadataList, WorkflowMetadata, u64);
const_assert!(
    mem::size_of::<WorkflowMetadataList>()
        == mem::size_of::<u64>()
            + (mem::size_of::<Pubkey>() + mem::size_of::<[u8; 20]>() + mem::size_of::<[u8; 10]>())
                * MAX_WORKFLOW_METADATAS
);

/// Represents information about a workflow which can be used to authorize it
/// for the reporting of a feed
#[zero_copy]
#[derive(InitSpace, BorshSerialize, BorshDeserialize)]
pub struct WorkflowMetadata {
    pub allowed_sender: Pubkey, // Address of the sender allowed to send new reports (forwarder)
    pub allowed_workflow_owner: [u8; 20], // ─╮ Address of the workflow owner
    pub allowed_workflow_name: [u8; 10], // ──╯ Name of the workflow UTF-bytes encoded
}

/// The existence of this account means that a data id can be reported by a workflow
#[account]
#[derive(Default)]
pub struct WritePermissionFlag {}

/// Contains config information of a legacy feed
#[zero_copy]
#[derive(InitSpace)]
pub struct LegacyFeedEntry {
    pub data_id: [u8; 16],
    pub legacy_feed: Pubkey,
    // functions mainly as a killswitch in case of emergencies
    // under normal operations, this is expected to be 0
    // 0 = enabled. 1 = disabled
    // regardless of what this flag is, if legacy_store or legacy_feed_config is not passed into report, writes cannot occur
    pub write_disabled: u8,
}

// in reality, there are only ~14 legacy feeds at the time of writing, but we provide a healthy buffer
const MAX_ENTRIES: usize = 64;

/// Fixed size struct which stores list of legacy feed entries.
#[zero_copy]
#[derive(InitSpace)]
pub struct LegacyFeedList {
    // entries are sorted by data_id for quick lookup during on_report
    pub xs: [LegacyFeedEntry; MAX_ENTRIES],
    pub len: u64,
}
arrayvec!(LegacyFeedList, LegacyFeedEntry, u64);
const_assert!(
    mem::size_of::<LegacyFeedList>()
        == mem::size_of::<u64>()
            + (mem::size_of::<[u8; 16]>() + mem::size_of::<Pubkey>() + mem::size_of::<u8>())
                * MAX_ENTRIES
);

/// Stores data ids which are flagged to have their reports written to
/// the legacy store program as well.
/// We can assume there's only going to be a limited amount of legacy feeds to write to
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct LegacyFeedsConfig {
    pub id_to_feed: LegacyFeedList,
    pub legacy_store: Pubkey,
}

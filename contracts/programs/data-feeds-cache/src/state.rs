use std::mem;

use anchor_lang::prelude::{
    borsh::{BorshDeserialize, BorshSerialize},
    *,
};
use arrayvec::arrayvec;
use static_assertions::const_assert;

#[account(zero_copy)]
#[derive(InitSpace)]
pub struct CacheState {
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub feed_admins: AccountList,
    pub legacy_writer_nonce: u8, // pda writing to the legacy feeds
    pub _padding: [u8; 7],
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, PartialEq)]
pub struct ReceivedDecimalReport {
    pub timestamp: u32,
    pub answer: u128,
    pub data_id: [u8; 16],
}
// 36 *2 + 4
// 16 + 20 + 4

#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct CacheTransmission {
    pub timestamp: u32,
    pub answer: u128,
}

#[account]
#[derive(InitSpace)]
pub struct DecimalReport {
    pub timestamp: u32,
    pub answer: u128,
}

// account also derived by the dataId
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct FeedConfig {
    // UTF-bytes encoded
    pub description: [u8; 32],
    pub workflow_metadata: WorkflowMetadataList,
}

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

#[zero_copy]
#[derive(InitSpace)]
pub struct WorkflowMetadataList {
    pub xs: [WorkflowMetadata; MAX_ENTRIES],
    pub len: u64,
}
arrayvec!(WorkflowMetadataList, WorkflowMetadata, u64);
const_assert!(
    mem::size_of::<WorkflowMetadataList>()
        == mem::size_of::<u64>()
            + (mem::size_of::<Pubkey>() + mem::size_of::<[u8; 20]>() + mem::size_of::<[u8; 10]>())
                * MAX_ENTRIES
);

// #[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, PartialEq, InitSpace)]
#[zero_copy]
#[derive(InitSpace, BorshSerialize, BorshDeserialize)]
pub struct WorkflowMetadata {
    pub allowed_sender: Pubkey, // Address of the sender allowed to send new reports (forwarder)
    pub allowed_workflow_owner: [u8; 20], // ─╮ Address of the workflow owner
    pub allowed_workflow_name: [u8; 10], // ──╯ Name of the workflow UTF-bytes encoded
}

#[account]
#[derive(Default)]
pub struct WritePermissionFlag {}

// 16 + 32 = 48 bytes
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

// in reality, there are only ~14 legacy feeds, but we provide a healthy buffer
const MAX_ENTRIES: usize = 64;

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

// 3080 + 32 = 3112 (can use init)
// flagged feeds need to be written to the legacy store
// we can assume there's only going to be a limited amount of legacy feeds
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct LegacyFeedsConfig {
    pub id_to_feed: LegacyFeedList,
    pub legacy_store: Pubkey,
}

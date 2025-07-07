use anchor_lang::prelude::{borsh::BorshSerialize, *};

use crate::state::WorkflowMetadata;

#[event]
pub struct DecimalFeedConfigSet {
    pub data_id: [u8; 16],
    pub decimals: u8,
    pub description: [u8; 32],
    pub workflow_metadatas: Vec<WorkflowMetadata>,
}

// todo: should be per
#[event]
pub struct LegacyFeedsReported {
    pub feeds_skipped: Vec<[u8; 16]>,
    pub feeds_written: Vec<[u8; 16]>,
}

#[event]
pub struct InvalidUpdatePermission {
    pub data_id: [u8; 16],
    pub sender: Pubkey,
    pub workflow_owner: [u8; 20],
    pub workflow_name: [u8; 10],
}

#[event]
pub struct StaleDecimalReport {
    pub data_id: [u8; 16],
    pub received_timestamp: u32,
    pub latest_timestamp: u32,
}

#[event]
pub struct DecimalReportUpdate {
    pub data_id: [u8; 16],
    pub timestamp: u32,
    pub answer: u128,
}

#[event]
pub struct FeedAdminUpdated {
    pub admin: Pubkey,
    pub is_admin: bool,
}

#[event]
pub struct OwnershipTransfer {
    pub current_owner: Pubkey,
    pub proposed_owner: Pubkey,
}

#[event]
pub struct OwnershipAcceptance {
    pub previous_owner: Pubkey,
    pub new_owner: Pubkey,
}

#[event]
pub struct DecimalReportInitialized {
    pub data_id: [u8; 16],
}

#[event]
pub struct DecimalReportClosed {
    pub data_id: [u8; 16],
}

#[event]
pub struct LegacyFeedsConfigInitialized {
    pub config: Pubkey,
}

#[event]
pub struct LegacyFeedsConfigUpdated {
    pub config: Pubkey,
}

#[event]
pub struct CacheInitialized {
    pub state: Pubkey,
}

use anchor_lang::prelude::{borsh::BorshSerialize, *};

use crate::state::WorkflowMetadata;

#[event]
pub struct DecimalFeedConfigSet {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub decimals: u8,
    pub description: [u8; 32],
    pub workflow_metadatas: Vec<WorkflowMetadata>,
}

#[event]
pub struct LegacyFeedsReported {
    pub state: Pubkey,
    pub feeds_skipped: Vec<[u8; 16]>,
    pub feeds_written: Vec<[u8; 16]>,
}

#[event]
pub struct InvalidUpdatePermission {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub sender: Pubkey,
    pub workflow_owner: [u8; 20],
    pub workflow_name: [u8; 10],
}

#[event]
pub struct StaleDecimalReport {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub received_timestamp: u32,
    pub latest_timestamp: u32,
}

#[event]
pub struct DecimalReportUpdated {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub timestamp: u32,
    pub answer: u128,
}

#[event]
pub struct FeedAdminUpdated {
    pub state: Pubkey,
    pub admin: Pubkey,
    pub is_admin: bool,
}

#[event]
pub struct OwnershipTransfer {
    pub state: Pubkey,
    pub current_owner: Pubkey,
    pub proposed_owner: Pubkey,
}

#[event]
pub struct OwnershipAcceptance {
    pub state: Pubkey,
    pub previous_owner: Pubkey,
    pub new_owner: Pubkey,
}

#[event]
pub struct DecimalReportInitialized {
    pub state: Pubkey,
    pub data_id: [u8; 16],
}

#[event]
pub struct DecimalReportClosed {
    pub state: Pubkey,
    pub data_id: [u8; 16],
}

#[event]
pub struct LegacyFeedsConfigInitialized {
    pub state: Pubkey,
    pub config: Pubkey,
}

#[event]
pub struct LegacyFeedsConfigUpdated {
    pub state: Pubkey,
    pub config: Pubkey,
}

#[event]
pub struct CacheInitialized {
    pub state: Pubkey,
    pub forwarder_id: Pubkey,
    pub legacy_writer_bump: u8,
}

#[event]
pub struct ForwarderUpdated {
    pub previous_forwarder: Pubkey,
    pub new_forwarder: Pubkey,
}

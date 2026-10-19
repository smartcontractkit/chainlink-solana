//! Events mirror the EVM `DataFeedsStore`, each also carries the store state account.

use anchor_lang::prelude::*;

use crate::state::{CallbackAccount, WorkflowMetadata};

#[event]
pub struct StoreInitialized {
    pub state: Pubkey,
    pub forwarder_id: Pubkey,
    pub default_max_stored_reports: u64,
    pub max_observation_forward_secs: u32,
}

#[event]
pub struct OwnershipTransferRequested {
    pub state: Pubkey,
    pub from: Pubkey,
    pub to: Pubkey,
}

#[event]
pub struct OwnershipTransferred {
    pub state: Pubkey,
    pub from: Pubkey,
    pub to: Pubkey,
}

#[event]
pub struct ForwarderUpdated {
    pub state: Pubkey,
    pub previous_forwarder: Pubkey,
    pub new_forwarder: Pubkey,
}

#[event]
pub struct FeedAdminSet {
    pub state: Pubkey,
    pub feed_admin: Pubkey,
    pub is_admin: bool,
}

#[event]
pub struct DefaultFeedMetadataSet {
    pub state: Pubkey,
    pub workflow_metadata: Vec<WorkflowMetadata>,
}

#[event]
pub struct SpecificFeedMetadataSet {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub workflow_metadata: Vec<WorkflowMetadata>,
}

#[event]
pub struct SpecificFeedMetadataRemoved {
    pub state: Pubkey,
    pub data_id: [u8; 16],
}

#[event]
pub struct MaxObservationForwardDurationSecondsUpdated {
    pub state: Pubkey,
    pub new_duration: u32,
}

#[event]
pub struct FeedInitialized {
    pub state: Pubkey,
    pub non_svr_data_id: [u8; 16],
    pub max_stored_reports: u64,
}

#[event]
pub struct FeedClosed {
    pub state: Pubkey,
    pub non_svr_data_id: [u8; 16],
}

#[event]
pub struct DataIdInitialized {
    pub state: Pubkey,
    pub data_id: [u8; 16],
}

#[event]
pub struct DataIdClosed {
    pub state: Pubkey,
    pub data_id: [u8; 16],
}

#[event]
pub struct MaxStoredReportsSet {
    pub state: Pubkey,
    pub non_svr_data_id: [u8; 16],
    pub max_stored_reports: u64,
}

#[event]
pub struct MinMaxAnswerSet {
    pub state: Pubkey,
    pub non_svr_data_id: [u8; 16],
    pub min_answer: i128,
    pub max_answer: i128,
}

#[event]
pub struct SvrFeedDisabled {
    pub state: Pubkey,
    pub svr_data_id: [u8; 16],
    pub is_disabled: bool,
}

#[event]
pub struct ProxyConfigUpdated {
    pub state: Pubkey,
    pub proxy: Pubkey,
    pub proxy_id: [u8; 32],
    pub data_id: [u8; 16],
    pub decimals: u8,
    pub fallback_time: u32,
    pub max_sync_iterations: u32,
}

#[event]
pub struct ProxyConfigRemoved {
    pub state: Pubkey,
    pub proxy: Pubkey,
    pub proxy_id: [u8; 32],
    pub data_id: [u8; 16],
}

#[event]
pub struct FeedDescriptionSet {
    pub state: Pubkey,
    pub indexed_feed_id: [u8; 16],
    pub description: String,
}

#[event]
pub struct SvrPathDescriptionSet {
    pub state: Pubkey,
    pub svr_path_id: [u8; 2],
    pub description: String,
}

#[event]
pub struct ReportCallbackConfigSet {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub previous_program: Pubkey,
    pub current_program: Pubkey,
    pub accounts: Vec<CallbackAccount>,
    pub revert_on_failure: bool,
}

#[event]
pub struct TokenRecovered {
    pub state: Pubkey,
    /// Default pubkey for native SOL
    pub mint: Pubkey,
    pub to: Pubkey,
    pub amount: u64,
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
pub struct FutureReportIgnored {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub observations_timestamp: u32,
    pub recorded_timestamp: u32,
}

#[event]
pub struct StaleReport {
    pub state: Pubkey,
    pub non_svr_data_id: [u8; 16],
    pub observations_timestamp: u32,
    pub latest_observations_timestamp: u32,
}

#[event]
pub struct StaleSvrReport {
    pub state: Pubkey,
    pub svr_data_id: [u8; 16],
    pub observations_timestamp: u32,
    pub latest_observations_timestamp: u32,
}

#[event]
pub struct AnswerOutOfMinMaxRange {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub min_answer: i128,
    pub max_answer: i128,
    pub answer: i128,
}

#[event]
pub struct FeedUpdated {
    pub state: Pubkey,
    pub non_svr_data_id: [u8; 16],
    pub round_id: u64,
    pub observations_timestamp: u32,
    pub answer: i128,
}

#[event]
pub struct SvrFeedUpdated {
    pub state: Pubkey,
    pub svr_data_id: [u8; 16],
    pub round_id: u64,
    pub observations_timestamp: u32,
    pub answer: i128,
}

#[event]
pub struct SvrReportSynced {
    pub state: Pubkey,
    pub svr_data_id: [u8; 16],
    pub round_id: u64,
    pub observations_timestamp: u32,
    pub answer: i128,
}

#[event]
pub struct ReportCallbackRejected {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub round_id: u64,
    pub answer: i128,
}

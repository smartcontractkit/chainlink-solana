use std::mem::size_of;

use anchor_lang::prelude::{
    borsh::{BorshDeserialize, BorshSerialize},
    *,
};
use arrayvec::arrayvec;
use static_assertions::const_assert_eq;

use crate::common::{
    ANCHOR_DISCRIMINATOR, DESCRIPTION_LEN, MAX_CALLBACK_ACCOUNTS, MAX_FEED_ADMINS,
    MAX_WORKFLOW_METADATAS,
};

/// Store instance: ownership, admins, forwarder and defaults applied to every feed.
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct StoreState {
    pub version: u8,
    /// Bump of the store authority PDA, which signs report callbacks
    pub store_authority_bump: u8,
    pub _padding0: [u8; 2],
    /// Maximum seconds a report's observation timestamp may lead the clock before it is ignored
    pub max_observation_forward_secs: u32,
    /// Ring size of feeds created without an explicit one
    pub default_max_stored_reports: u64,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    /// Program id of the keystone forwarder whose authority PDA may call `on_report`
    pub forwarder_id: Pubkey,
    pub feed_admins: AccountList,
    /// Workflows allowed to write any data ID without feed-specific metadata
    pub default_workflow_metadata: WorkflowMetadataList,
    pub _reserved: [u8; 64],
}

/// Non-SVR feed history and the non-SVR data ID's settings, one per non-SVR data ID. The account
/// holds this header followed by a circular buffer of `max_stored_reports` [`StoredReport`]s, round
/// `r` lives at slot `r % max_stored_reports`. Keeping the settings here rather than in a separate
/// account saves a non-SVR report one account in the transaction.
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct FeedData {
    pub version: u8,
    /// Whether a feed admin has set the ring size
    pub max_stored_reports_configured: u8,
    /// Whether an accepted answer range is set, unbounded otherwise
    pub min_max_configured: u8,
    pub _padding0: u8,
    pub latest_observations_timestamp: u32,
    pub latest_recorded_timestamp: u32,
    pub _padding1: [u8; 4],
    pub data_id: [u8; 16],
    pub state: Pubkey,
    /// Latest round ID, SVR-authored rounds included
    pub round_id: u64,
    /// Newest round released to non-SVR readers, behind `round_id` while an SVR-authored round is
    /// exclusive to its path for the slot it landed in
    pub visible_round_id: u64,
    pub max_stored_reports: u64,
    pub latest_recorded_slot: u64,
    pub latest_answer: i128,
    pub min_answer: i128,
    pub max_answer: i128,
    pub settings: DataIdSettings,
    pub _reserved: [u8; 64],
}

/// Offset of the circular buffer in a feed account.
pub const FEED_RING_OFFSET: usize = ANCHOR_DISCRIMINATOR + size_of::<FeedData>();

pub fn feed_account_len(max_stored_reports: u64) -> usize {
    FEED_RING_OFFSET + max_stored_reports as usize * size_of::<StoredReport>()
}

/// One stored round of a feed's history.
#[zero_copy]
#[derive(Default, Debug, PartialEq, Eq, InitSpace)]
pub struct StoredReport {
    pub answer: i128,
    /// When were observations made offchain
    pub observations_timestamp: u32,
    /// When was the report received onchain
    pub recorded_timestamp: u32,
    /// Slot the report was received in
    pub recorded_slot: u64,
}

/// Permissions and report callback of a data ID: part of the feed account for a non-SVR data ID,
/// of the [`DataIdConfig`] for an SVR path.
#[zero_copy]
#[derive(InitSpace)]
pub struct DataIdSettings {
    /// Whether a callback returning false drops the report
    pub callback_revert_on_failure: u8,
    pub callback_accounts_len: u8,
    /// Bit i set when `callback_accounts[i]` is passed writable
    pub callback_writable_mask: u8,
    pub _padding0: [u8; 5],
    /// Program called with every report applied for this data ID, default when disabled
    pub callback_program: Pubkey,
    pub callback_accounts: [Pubkey; MAX_CALLBACK_ACCOUNTS],
    /// Workflows allowed to write this data ID, the store's default workflows when empty
    pub specific_workflow_metadata: WorkflowMetadataList,
}

impl DataIdSettings {
    pub fn has_callback(&self) -> bool {
        self.callback_program != Pubkey::default()
    }

    pub fn callback_accounts(&self) -> &[Pubkey] {
        &self.callback_accounts[..self.callback_accounts_len as usize]
    }

    pub fn callback_account_writable(&self, i: usize) -> bool {
        self.callback_writable_mask & (1 << i) != 0
    }
}

/// SVR path state: its settings and the report it selected. Non-SVR data IDs keep their settings
/// in their feed account and have no config.
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct DataIdConfig {
    pub version: u8,
    /// Readers fall back to the non-SVR feed when set
    pub svr_disabled: u8,
    pub _padding0: [u8; 2],
    pub svr_observations_timestamp: u32,
    pub svr_recorded_timestamp: u32,
    pub _padding1: [u8; 4],
    pub data_id: [u8; 16],
    pub state: Pubkey,
    /// Shared-history round the SVR path selected, 0 until it selects one
    pub svr_round_id: u64,
    pub svr_recorded_slot: u64,
    pub svr_answer: i128,
    pub settings: DataIdSettings,
    pub _reserved: [u8; 64],
}

/// Reader configuration, the Solana counterpart of an EVM aggregator proxy. Its address is the
/// address readers pass to the proxy read instructions.
#[account]
#[derive(InitSpace)]
pub struct ProxyConfig {
    pub version: u8,
    pub state: Pubkey,
    pub proxy_id: [u8; 32],
    /// Full data ID, non-SVR or SVR path
    pub data_id: [u8; 16],
    /// Decimals answers are downscaled to, at most the stored decimals
    pub decimals: u8,
    /// Seconds an SVR report stays valid before readers fall back to non-SVR rounds
    pub fallback_time: u32,
    /// Max non-SVR rounds scanned when syncing a stale SVR path
    pub max_sync_iterations: u32,
    pub _reserved: [u8; 32],
}

/// Description shared by every instance and SVR path of a feed, keyed by indexed feed ID.
#[account]
#[derive(InitSpace)]
pub struct FeedDescription {
    pub version: u8,
    pub state: Pubkey,
    pub indexed_feed_id: [u8; 16],
    /// UTF-8, zero padded
    pub description: [u8; DESCRIPTION_LEN],
}

#[account]
#[derive(InitSpace)]
pub struct SvrPathDescription {
    pub version: u8,
    pub state: Pubkey,
    pub svr_path_id: [u8; 2],
    /// UTF-8, zero padded
    pub description: [u8; DESCRIPTION_LEN],
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
const_assert_eq!(size_of::<DataIdSettings>(), 1168);
const_assert_eq!(size_of::<FeedData>(), 1376);
const_assert_eq!(size_of::<StoredReport>(), 32);
const_assert_eq!(size_of::<DataIdConfig>(), 1328);

// ================================================================
// │                     Instruction arguments                    │
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

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, PartialEq)]
pub struct ProxyConfigArgs {
    pub data_id: [u8; 16],
    pub decimals: u8,
    pub fallback_time: u32,
    pub max_sync_iterations: u32,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, PartialEq)]
pub struct CallbackAccount {
    pub pubkey: Pubkey,
    pub is_writable: bool,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, PartialEq)]
pub struct CallbackConfigArgs {
    /// Default pubkey disables the callback
    pub program: Pubkey,
    /// Accounts passed to the callback after the store authority signer
    pub accounts: Vec<CallbackAccount>,
    /// Whether a callback returning false drops the report
    pub revert_on_failure: bool,
}

/// Arguments of the `on_report_callback` instruction a callback program implements. It is called
/// with the store authority PDA as the first (signer) account followed by the configured accounts,
/// and must set Borsh `bool` return data, true to accept the report.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, PartialEq)]
pub struct OnReportCallbackArgs {
    /// Data ID the round is stored under
    pub data_id: [u8; 16],
    /// SVR path the report arrived through, zero for the non-SVR path
    pub svr_path_id: [u8; 2],
    pub round_id: u64,
    pub answer: i128,
    /// Decimals of `answer`, always `STORED_DECIMALS`
    pub decimals: u8,
    pub observations_timestamp: u32,
    pub recorded_timestamp: u32,
    /// Whether the report is appended to the shared history, false when an SVR path only adopted
    /// a round that was already stored
    pub is_freshest: bool,
}

/// Aggregator-compatible round data.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, Default, PartialEq)]
pub struct RoundData {
    pub round_id: u64,
    pub answer: i128,
    /// Offchain observations timestamp
    pub started_at: u64,
    /// Onchain recorded timestamp
    pub updated_at: u64,
    pub answered_in_round: u64,
}

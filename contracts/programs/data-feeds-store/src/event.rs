//! Events mirror the EVM `DataFeedsStore`, each also carries the store state account. They are
//! emitted as CPI events (`emit_cpi!`): a self-invoke the event authority PDA signs, carried in the
//! transaction's inner instructions, so they cannot be lost to log truncation when one `on_report`
//! applies many reports. `InvalidDataIdIgnored`, `FeedNotInitializedIgnored` and
//! `CallbackProgramAllowedSet` have no EVM counterpart: the EVM store creates feed storage on first
//! write and needs no callback allowlist.

use anchor_lang::prelude::*;

use crate::state::{CallbackAccount, WorkflowMetadata};

#[event]
pub struct StoreInitialized {
    pub state: Pubkey,
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
    pub data_id: [u8; 16],
    pub max_stored_reports: u64,
}

#[event]
pub struct MaxStoredReportsSet {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub max_stored_reports: u64,
}

#[event]
pub struct MinMaxAnswerSet {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub min_answer: i128,
    pub max_answer: i128,
}

#[event]
pub struct FeedDescriptionSet {
    pub state: Pubkey,
    pub indexed_feed_id: [u8; 16],
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
    pub data_id: [u8; 16],
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
    pub data_id: [u8; 16],
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

/// The skip events carry the sender: they are emitted before the permission check, so anyone can
/// cause them, and indexers must not take them as coming from a forwarder.
#[event]
pub struct InvalidDataIdIgnored {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub sender: Pubkey,
}

#[event]
pub struct FeedNotInitializedIgnored {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub sender: Pubkey,
}

#[event]
pub struct CallbackProgramAllowedSet {
    pub state: Pubkey,
    pub program: Pubkey,
    pub allowed: bool,
}

#[cfg(test)]
mod tests {
    use anchor_lang::{Discriminator, Event};

    use super::*;

    /// `EventEmitter::emit_with` writes some events from borrowed fields; the bytes must be the
    /// event struct's own encoding, or indexers decoding the IDL would misread them.
    #[test]
    fn borrowed_field_encodings_match_the_event_structs() {
        let state = Pubkey::new_unique();
        let id = [7u8; 16];
        let metadata = vec![WorkflowMetadata {
            allowed_sender: Pubkey::new_unique(),
            allowed_workflow_owner: [1; 20],
            allowed_workflow_name: [2; 10],
        }];
        let mut manual = SpecificFeedMetadataSet::DISCRIMINATOR.to_vec();
        state.serialize(&mut manual).unwrap();
        id.serialize(&mut manual).unwrap();
        metadata.as_slice().serialize(&mut manual).unwrap();
        let event = SpecificFeedMetadataSet {
            state,
            data_id: id,
            workflow_metadata: metadata,
        };
        assert_eq!(manual, event.data());

        let description = "ETH / USD";
        let mut manual = FeedDescriptionSet::DISCRIMINATOR.to_vec();
        state.serialize(&mut manual).unwrap();
        id.serialize(&mut manual).unwrap();
        description.serialize(&mut manual).unwrap();
        let event = FeedDescriptionSet {
            state,
            indexed_feed_id: id,
            description: description.to_string(),
        };
        assert_eq!(manual, event.data());

        let accounts = vec![CallbackAccount {
            pubkey: Pubkey::new_unique(),
            is_writable: true,
        }];
        let (previous, current) = (Pubkey::new_unique(), Pubkey::new_unique());
        let mut manual = ReportCallbackConfigSet::DISCRIMINATOR.to_vec();
        state.serialize(&mut manual).unwrap();
        id.serialize(&mut manual).unwrap();
        previous.serialize(&mut manual).unwrap();
        current.serialize(&mut manual).unwrap();
        accounts.as_slice().serialize(&mut manual).unwrap();
        true.serialize(&mut manual).unwrap();
        let event = ReportCallbackConfigSet {
            state,
            data_id: id,
            previous_program: previous,
            current_program: current,
            accounts,
            revert_on_failure: true,
        };
        assert_eq!(manual, event.data());
    }
}

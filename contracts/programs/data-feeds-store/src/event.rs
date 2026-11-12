//! Events mirror the EVM `DataFeedsStore` where it has them, each also carries the store state
//! account. They are emitted as CPI events: a self-invoke the event authority PDA signs, carried in
//! the transaction's inner instructions, so they cannot be lost to log truncation when one
//! `on_report` applies many reports.

use anchor_lang::prelude::*;

use crate::state::{Df1SkipReason, WorkflowMetadata};

#[event]
pub struct StoreInitialized {
    pub state: Pubkey,
    pub default_max_stored_reports: u64,
    pub max_observation_forward_secs: u32,
    pub df1_program: Pubkey,
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

/// Feeds were created by a feed admin setting, the feed admin paying their rent.
#[event]
pub struct FeedsCreated {
    pub state: Pubkey,
    pub data_ids: Vec<[u8; 16]>,
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
    pub data_id: [u8; 16],
    pub description: String,
}

#[event]
pub struct MinDecimalsSet {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub min_decimals: u8,
}

/// Reads of the feed were frozen or unfrozen.
#[event]
pub struct FeedFrozenSet {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub frozen: bool,
}

/// Writes of the feed were disabled or enabled.
#[event]
pub struct FeedWritesDisabledSet {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub disabled: bool,
}

/// The DF1 feed kept written with a data ID's rounds, default for none.
#[event]
pub struct Df1FeedSet {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub df1_feed: Pubkey,
    pub write_disabled: bool,
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

/// A round was stored without writing the data ID's DF1 feed.
#[event]
pub struct Df1WriteSkipped {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub df1_feed: Pubkey,
    pub reason: Df1SkipReason,
}

/// The skip events below carry the sender: they are emitted before the permission check, so anyone
/// can cause them, and indexers must not take them as coming from a forwarder.
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
pub struct FeedWritesDisabledIgnored {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub sender: Pubkey,
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
            data_id: id,
            description: description.to_string(),
        };
        assert_eq!(manual, event.data());
    }
}

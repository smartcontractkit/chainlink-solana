//! Events each carry the store state account. They are emitted as CPI events: the program calls
//! itself, signed by its event authority PDA, with the event as the call's data. The event is then
//! recorded in the transaction's inner instructions, so it can't be lost to log truncation when
//! one `on_report` applies many reports.

use anchor_lang::prelude::*;

use crate::state::{Df1SkipReason, WorkflowMetadata};

/// A store was created with its owner, settings and first feed admins. The admins are listed in
/// this one event rather than in one `FeedAdminSet` each, because every event is an inner
/// instruction and Solana allows at most 64 instructions per transaction, inner ones included.
#[event]
pub struct StoreInitialized {
    /// The new store's state account.
    pub state: Pubkey,
    /// The store's first owner, the program's upgrade authority.
    pub owner: Pubkey,
    /// The ring size the store's feeds are created with.
    pub default_max_stored_reports: u64,
    /// How many seconds ahead of the chain's clock a report's observation timestamp may be.
    pub max_observation_forward_secs: u32,
    /// The DF1 program whose feed accounts the store may write.
    pub df1_program: Pubkey,
    /// The first feed admins, in increasing order.
    pub feed_admins: Vec<Pubkey>,
}

/// The owner proposed a new owner, who still has to accept.
#[event]
pub struct OwnershipTransferRequested {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The current owner, who made the proposal.
    pub from: Pubkey,
    /// The proposed owner. The default key means a pending proposal was cancelled.
    pub to: Pubkey,
}

/// The proposed owner accepted and is now the owner.
#[event]
pub struct OwnershipTransferred {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The previous owner.
    pub from: Pubkey,
    /// The new owner.
    pub to: Pubkey,
}

/// The owner granted or revoked a key's feed admin permission. The event is emitted even when the
/// key already had the requested status.
#[event]
pub struct FeedAdminSet {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The key whose permission was set.
    pub feed_admin: Pubkey,
    /// True if the key is now a feed admin, false if it no longer is one.
    pub is_admin: bool,
}

/// A feed admin replaced the store's default workflow list.
#[event]
pub struct DefaultFeedMetadataSet {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The new default workflow list, which may be empty.
    pub workflow_metadata: Vec<WorkflowMetadata>,
}

/// Feeds were given their own workflow list, replacing any list they had. One event per call
/// lists every data ID given the list, because each event is an inner instruction and copies its
/// data onto the heap.
#[event]
pub struct SpecificFeedMetadataSet {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data IDs whose feeds were given the list, in call order. A data ID listed twice in the
    /// call appears twice here.
    pub data_ids: Vec<[u8; 16]>,
    /// The workflow list each of those feeds now has as its own.
    pub workflow_metadata: Vec<WorkflowMetadata>,
}

/// A feed's own workflow list was removed, so the store's default list applies to it again.
#[event]
pub struct SpecificFeedMetadataRemoved {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID of the feed concerned.
    pub data_id: [u8; 16],
}

/// The forward window changed: how many seconds ahead of the chain's clock a report's observation
/// timestamp may be.
#[event]
pub struct MaxObservationForwardDurationSecondsUpdated {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The new forward window, in seconds.
    pub new_duration: u32,
}

/// A feed admin's call created feeds and paid their rent.
#[event]
pub struct FeedsCreated {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data IDs whose feeds were created, each listed once.
    pub data_ids: Vec<[u8; 16]>,
    /// The ring size the new feeds were created with, which is the store's default.
    pub max_stored_reports: u64,
}

/// A feed's ring was grown.
#[event]
pub struct MaxStoredReportsSet {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID of the feed concerned.
    pub data_id: [u8; 16],
    /// The feed's new ring size.
    pub max_stored_reports: u64,
}

/// A feed's accepted answer range was set. A minimum of `i128::MIN` with a maximum of `i128::MAX`
/// means the range was removed.
#[event]
pub struct MinMaxAnswerSet {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID of the feed concerned.
    pub data_id: [u8; 16],
    /// The lowest accepted answer, with 18 decimals.
    pub min_answer: i128,
    /// The highest accepted answer, with 18 decimals.
    pub max_answer: i128,
}

/// A feed's description was set.
#[event]
pub struct FeedDescriptionSet {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID of the feed concerned.
    pub data_id: [u8; 16],
    /// The new description. An empty description means it was cleared.
    pub description: String,
}

/// The fewest decimals a read of a feed may ask for changed.
#[event]
pub struct MinDecimalsSet {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID of the feed concerned.
    pub data_id: [u8; 16],
    /// The fewest decimals a read of the feed may now ask for.
    pub min_decimals: u8,
}

/// Round, decimals and description reads of a feed were frozen or unfrozen.
#[event]
pub struct FeedFrozenSet {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID of the feed concerned.
    pub data_id: [u8; 16],
    /// True if round, decimals and description reads are now frozen, false if they were unfrozen.
    pub frozen: bool,
}

/// Writes to a feed were disabled or enabled.
#[event]
pub struct FeedWritesDisabledSet {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID of the feed concerned.
    pub data_id: [u8; 16],
    /// True if writes are now disabled, false if they were enabled.
    pub disabled: bool,
}

/// A data ID's DF1 link was set.
#[event]
pub struct Df1FeedSet {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID of the feed concerned.
    pub data_id: [u8; 16],
    /// The DF1 feed that the data ID's rounds are now written to, or the default key if the data
    /// ID was unlinked.
    pub df1_feed: Pubkey,
    /// True if the DF1 write is disabled, so rounds are stored in this store only.
    pub write_disabled: bool,
}

/// The owner recovered tokens or SOL from the store authority PDA.
#[event]
pub struct TokenRecovered {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The mint of the recovered tokens, or the default key for SOL.
    pub mint: Pubkey,
    /// The account the tokens or SOL were sent to.
    pub to: Pubkey,
    /// The amount moved, in the token's smallest unit or in lamports.
    pub amount: u64,
}

/// A report was skipped because its workflow is not allowed to write the data ID.
#[event]
pub struct InvalidUpdatePermission {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID from the report.
    pub data_id: [u8; 16],
    /// The account that signed as the report's sender.
    pub sender: Pubkey,
    /// The workflow owner from the report's metadata.
    pub workflow_owner: [u8; 20],
    /// The workflow name from the report's metadata.
    pub workflow_name: [u8; 10],
}

/// A report was skipped because its observation timestamp is further ahead of the chain's clock
/// than the forward window allows.
#[event]
pub struct FutureReportIgnored {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID from the report.
    pub data_id: [u8; 16],
    /// The report's observation timestamp, in Unix seconds.
    pub observations_timestamp: u64,
    /// The chain's clock when the report arrived, in Unix seconds.
    pub recorded_timestamp: u64,
}

/// A report was skipped because it is no newer than the feed's latest round.
#[event]
pub struct StaleReport {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID from the report.
    pub data_id: [u8; 16],
    /// The report's observation timestamp, in Unix seconds.
    pub observations_timestamp: u64,
    /// The observation timestamp of the feed's latest round, which the report had to exceed.
    pub latest_observations_timestamp: u64,
}

/// A report was skipped because its answer is outside the feed's accepted range.
#[event]
pub struct AnswerOutOfMinMaxRange {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID from the report.
    pub data_id: [u8; 16],
    /// The feed's lowest accepted answer, with 18 decimals.
    pub min_answer: i128,
    /// The feed's highest accepted answer, with 18 decimals.
    pub max_answer: i128,
    /// The report's answer, with 18 decimals.
    pub answer: i128,
}

/// A round was stored.
#[event]
pub struct FeedUpdated {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID of the feed concerned.
    pub data_id: [u8; 16],
    /// The ID the new round was stored as.
    pub round_id: u64,
    /// The round's observation timestamp, in Unix seconds.
    pub observations_timestamp: u64,
    /// The round's answer, with 18 decimals.
    pub answer: i128,
}

/// A round was stored without being written to the data ID's linked DF1 feed.
#[event]
pub struct Df1WriteSkipped {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID of the feed concerned.
    pub data_id: [u8; 16],
    /// The linked DF1 feed that was not written.
    pub df1_feed: Pubkey,
    /// Why the DF1 write was skipped.
    pub reason: Df1SkipReason,
}

/// A report was skipped because its data ID is zero or has an SVR path ID. This check comes before
/// the permission check, so anyone can cause this event, and indexers must not assume it came from
/// a forwarder.
#[event]
pub struct InvalidDataIdIgnored {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID from the report.
    pub data_id: [u8; 16],
    /// The account that signed as the report's sender. It has not been checked against any
    /// workflow, so it may not be a forwarder.
    pub sender: Pubkey,
}

/// A report was skipped because the data ID's feed has not been set. This check comes before the
/// permission check, so anyone can cause this event, and indexers must not assume it came from a
/// forwarder.
#[event]
pub struct FeedNotInitializedIgnored {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID from the report.
    pub data_id: [u8; 16],
    /// The account that signed as the report's sender. It has not been checked against any
    /// workflow, so it may not be a forwarder.
    pub sender: Pubkey,
}

/// A report was skipped because the feed's writes are disabled. This check comes before the
/// permission check, so anyone can cause this event, and indexers must not assume it came from a
/// forwarder.
#[event]
pub struct FeedWritesDisabledIgnored {
    /// The store state account the event is about.
    pub state: Pubkey,
    /// The data ID from the report.
    pub data_id: [u8; 16],
    /// The account that signed as the report's sender. It has not been checked against any
    /// workflow, so it may not be a forwarder.
    pub sender: Pubkey,
}

#[cfg(test)]
mod tests {
    use anchor_lang::{Discriminator, Event};

    use super::*;

    /// `EventEmitter::emit_with` writes some events from borrowed fields. Those bytes must be the
    /// event struct's own encoding, or indexers decoding events with the IDL would misread them.
    #[test]
    fn borrowed_field_encodings_match_the_event_structs() {
        let state = Pubkey::new_unique();
        let id = [7u8; 16];
        let metadata = vec![WorkflowMetadata {
            allowed_sender: Pubkey::new_unique(),
            allowed_workflow_owner: [1; 20],
            allowed_workflow_name: [2; 10],
        }];
        let ids = [id, [8u8; 16]];
        let mut manual = SpecificFeedMetadataSet::DISCRIMINATOR.to_vec();
        state.serialize(&mut manual).unwrap();
        ids.as_slice().serialize(&mut manual).unwrap();
        metadata.as_slice().serialize(&mut manual).unwrap();
        let event = SpecificFeedMetadataSet {
            state,
            data_ids: ids.to_vec(),
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

use std::mem::size_of;

use anchor_lang::prelude::{
    borsh::{BorshDeserialize, BorshSerialize},
    *,
};
use arrayvec::arrayvec;
use static_assertions::const_assert_eq;

use crate::common::{
    ANCHOR_DISCRIMINATOR, DESCRIPTION_LEN, MAX_FEED_ADMINS, MAX_WORKFLOW_METADATAS,
};

/// The state account of one store. It holds the owner, the feed admins, the DF1 program the store
/// may write to, and the settings that apply to every feed of the store. One program can hold many
/// stores, each with its own state account and its own feeds.
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct StoreState {
    /// The layout version of this account, set to `STORE_STATE_VERSION`.
    pub version: u8,
    /// The bump of the store authority PDA. A PDA's address is found by trying bump values from
    /// 255 downwards until the address is one no private key exists for, and signing as the PDA
    /// needs that bump. `initialize` finds it once and stores it here, so later instructions don't
    /// repeat the search. The store authority only holds tokens and SOL sent to the store, which
    /// the owner can recover.
    pub store_authority_bump: u8,
    /// The bump of the `df1_writer` PDA, stored for the same reason. The store signs its `submit`
    /// calls to DF1 as this PDA, and each linked DF1 feed must name it as its writer.
    pub df1_writer_bump: u8,
    /// Always zero. This byte fills the gap before `max_observation_forward_secs`, because a
    /// zero-copy account is read straight from its bytes and so cannot have hidden padding.
    pub _padding0: u8,
    /// How many seconds ahead of the chain's clock a report's observation timestamp may be. A
    /// report stamped further ahead is skipped with `FutureReportIgnored`. If it were stored, it
    /// would become the feed's latest round, and every later report carrying the correct time would
    /// be skipped as stale until the clock caught up. The limit is a window rather than "not after
    /// now" because the DON's clocks and the chain's clock drift slightly apart. EVM has the same
    /// setting.
    pub max_observation_forward_secs: u32,
    /// How many rounds each feed this store creates keeps, which is the size of its ring.
    pub default_max_stored_reports: u64,
    /// The owner. It manages the feed admins and ownership, and can recover tokens and SOL sent
    /// to the store.
    pub owner: Pubkey,
    /// The owner proposed by `transfer_ownership`, until it accepts. It is the default key when no
    /// transfer is pending.
    pub proposed_owner: Pubkey,
    /// The DF1 program whose feed accounts this store may write.
    pub df1_program: Pubkey,
    /// The keys allowed to manage feeds and the store-wide feed settings. They are kept sorted so
    /// that a lookup can use binary search.
    pub feed_admins: AccountList,
    /// The workflows allowed to write any feed that has no workflow list of its own. The list may
    /// be empty, which leaves only feeds with their own list writable.
    pub default_workflow_metadata: WorkflowMetadataList,
    /// Always zero. These bytes are kept free so that fields can be added later without moving
    /// the layout.
    pub _reserved: [u8; 64],
}

/// One feed's account. It holds the feed's latest round, the settings that control how the feed
/// is read and written, and a ring (a circular buffer) of its last `max_stored_reports` rounds.
/// Readers pass this account, and its layout is public, so fields only move with a version bump.
///
/// ```text
/// byte 0     8                           1352    1400                    1352 + 48 * N
///      +-----+---------------------------+-------+-------+-----+---------+
///      | tag | Feed header (1344 bytes)  | idx 0 | idx 1 | ... | idx N-1 |
///      +-----+---------------------------+-------+-------+-----+---------+
///                                          one 48-byte StoredReport per index
///
/// N is max_stored_reports, and round r is stored at index r % N. Round IDs start at 1, so
/// index 0 is first written by round N. With N = 4, after 5 rounds:
///
///      idx 0     idx 1     idx 2     idx 3
///      round 4   round 5   round 2   round 3      (round 5 overwrote round 1)
///
/// Only the last N rounds can be read, here rounds 2 to 5. The header repeats the latest round.
/// Some header fields by account byte (header offset + 8): state 72, round_id 104,
/// max_stored_reports 120, latest_answer 136, description 152.
/// ```
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct Feed {
    /// The layout version of this account, set to `FEED_ACCOUNT_VERSION`.
    pub version: u8,
    /// Non-zero while round reads are frozen. `latest_round`, `get_round` and the client crate's
    /// reads then fail, while the description and decimals stay readable and new reports are
    /// still stored.
    pub frozen: u8,
    /// The fewest decimals a read may ask for. New feeds start at 18.
    pub min_decimals: u8,
    /// Non-zero while writes are disabled. Every report for the feed is then skipped, whichever
    /// workflow sent it, and reads keep working.
    pub writes_disabled: u8,
    /// Always zero. These bytes align the timestamps. A zero-copy account is read straight from
    /// its bytes and cannot have hidden padding.
    pub _padding0: [u8; 4],
    /// The time, in Unix seconds, at which the latest round was observed offchain, taken from its
    /// report. A new report must be newer than this to be stored.
    pub latest_observations_timestamp: u64,
    /// The time, in Unix seconds, at which the latest round was written onchain.
    pub latest_recorded_timestamp: u64,
    /// Non-zero when `min_answer` and `max_answer` apply. Otherwise any answer is accepted.
    pub min_max_configured: u8,
    /// Non-zero while the DF1 write is disabled. Rounds are then still stored here, but they are
    /// not written to the linked DF1 feed.
    pub df1_write_disabled: u8,
    /// Always zero. These bytes fill the gap before `data_id`.
    pub _padding1: [u8; 6],
    /// The data ID, zero padded to 32 bytes (see [`crate::data_id::DataIdSeed`]).
    pub data_id: [u8; 32],
    /// The store state account this feed belongs to.
    pub state: Pubkey,
    /// The latest round ID. It is 0 until the first round is stored.
    pub round_id: u64,
    /// Always zero. These bytes are kept free for SVR: the EVM store keeps the newest round
    /// released to non-SVR readers here (`visibleRoundId`), so SVR can be added without moving the
    /// fields below.
    pub _reserved0: [u8; 8],
    /// How many rounds the ring after this header holds.
    pub max_stored_reports: u64,
    /// The slot (the chain's clock tick) in which the latest round was written.
    pub latest_recorded_slot: u64,
    /// The latest answer, with 18 decimals (`STORED_DECIMALS`).
    pub latest_answer: i128,
    /// The description as UTF-8, for example "ETH / USD", zero padded. It is all zeros when no
    /// description is set.
    pub description: [u8; DESCRIPTION_LEN],
    /// The DF1 feed account that each new round is also written to, or the default key when the
    /// feed has no DF1 link.
    pub df1_feed: Pubkey,
    /// The lowest answer accepted, with 18 decimals. It only applies when `min_max_configured` is
    /// set.
    pub min_answer: i128,
    /// The highest answer accepted, with 18 decimals. It only applies when `min_max_configured` is
    /// set.
    pub max_answer: i128,
    /// The workflows allowed to write this feed. When the list is empty, the store's default list
    /// applies instead.
    pub specific_workflow_metadata: WorkflowMetadataList,
    /// Always zero. These bytes are kept free so that fields can be added later without moving
    /// the layout.
    pub _reserved: [u8; 72],
}

impl Feed {
    /// Returns whether the feed is linked to a DF1 feed.
    pub fn has_df1_feed(&self) -> bool {
        self.df1_feed != Pubkey::default()
    }
}

/// The byte offset at which the ring starts in a feed account: after Anchor's 8-byte tag and the
/// 1,344-byte header, at byte 1,352.
pub const FEED_RING_OFFSET: usize = ANCHOR_DISCRIMINATOR + size_of::<Feed>();

/// Returns the length of a feed account whose ring holds `max_stored_reports` rounds. The
/// arithmetic saturates, so an absurd ring size gives an absurd length that every bound check
/// rejects, rather than an overflow panic.
pub fn feed_account_len(max_stored_reports: u64) -> usize {
    (max_stored_reports as usize)
        .saturating_mul(size_of::<StoredReport>())
        .saturating_add(FEED_RING_OFFSET)
}

/// A marker account that records which data ID links a DF1 feed, so that two data IDs can never
/// both write it. Its address is the PDA `["df1_link", state, df1_feed]`. It is created on the
/// DF1 feed's first link and cleared, by zeroing its data ID, when the link is removed. Reports
/// never read it.
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct Df1Link {
    /// The layout version of this account, set to `DF1_LINK_VERSION`.
    pub version: u8,
    /// Always zero. These bytes keep `data_id` at offset 8.
    pub _padding0: [u8; 7],
    /// The data ID, zero padded to 32 bytes, whose feed links the DF1 feed. It is all zeros while
    /// no data ID links the DF1 feed.
    pub data_id: [u8; 32],
}

/// One round stored in a feed's ring.
#[zero_copy]
#[derive(Default, Debug, PartialEq, Eq, InitSpace)]
pub struct StoredReport {
    /// The answer, with 18 decimals.
    pub answer: i128,
    /// The time, in Unix seconds, at which the round was observed offchain.
    pub observations_timestamp: u64,
    /// The time, in Unix seconds, at which the round was written onchain.
    pub recorded_timestamp: u64,
    /// The slot (the chain's clock tick) in which the round was written.
    pub recorded_slot: u64,
    /// Always zero. These bytes make the struct's size a multiple of its 16-byte alignment.
    pub _padding0: [u8; 8],
}

/// A fixed-size list of public keys. The first `len` entries are in use and the rest are zero.
/// A zero-copy account has a fixed layout, so it cannot hold a `Vec`.
#[zero_copy]
#[derive(InitSpace)]
pub struct AccountList {
    /// The entries. Those at index `len` and after are zero.
    pub xs: [Pubkey; MAX_FEED_ADMINS],
    /// The number of entries in use.
    pub len: u64,
}
arrayvec!(AccountList, Pubkey, u64);

/// A fixed-size list of workflows. The first `len` entries are in use and the rest are zero.
#[zero_copy]
#[derive(InitSpace)]
pub struct WorkflowMetadataList {
    /// The entries. Those at index `len` and after are zero.
    pub xs: [WorkflowMetadata; MAX_WORKFLOW_METADATAS],
    /// The number of entries in use.
    pub len: u64,
}
arrayvec!(WorkflowMetadataList, WorkflowMetadata, u64);

/// A workflow allowed to write feeds. A report is accepted only when its sender, workflow owner
/// and workflow name all match one entry.
#[zero_copy]
#[derive(InitSpace, BorshSerialize, BorshDeserialize, PartialEq, Eq, Debug)]
pub struct WorkflowMetadata {
    /// The address that must send the reports. It is the authority PDA that a forwarder program
    /// derives for this program, `["forwarder", forwarder_state, this program's ID]`, so it is the
    /// same for every store state of this program.
    pub allowed_sender: Pubkey,
    /// The workflow owner, 20 bytes, as the forwarder's metadata carries it.
    pub allowed_workflow_owner: [u8; 20],
    /// The workflow name, 10 bytes, as the forwarder's metadata carries it.
    pub allowed_workflow_name: [u8; 10],
}

const_assert_eq!(size_of::<StoreState>(), 3232);
const_assert_eq!(size_of::<Feed>(), 1344);
const_assert_eq!(size_of::<StoredReport>(), 48);
const_assert_eq!(size_of::<Df1Link>(), 40);
// These are the header offsets that readers decode feed accounts by; add 8 for Anchor's tag.
// `state` (account bytes 72 to 104) is the filter for listing a store's feeds with
// `getProgramAccounts`, the RPC call that lists a program's accounts.
const_assert_eq!(std::mem::offset_of!(Feed, frozen), 1);
const_assert_eq!(std::mem::offset_of!(Feed, min_decimals), 2);
const_assert_eq!(std::mem::offset_of!(Feed, latest_observations_timestamp), 8);
const_assert_eq!(std::mem::offset_of!(Feed, data_id), 32);
const_assert_eq!(std::mem::offset_of!(Feed, state), 64);
const_assert_eq!(std::mem::offset_of!(Feed, round_id), 96);
const_assert_eq!(std::mem::offset_of!(Feed, max_stored_reports), 112);
const_assert_eq!(std::mem::offset_of!(Feed, latest_answer), 128);
const_assert_eq!(std::mem::offset_of!(Feed, description), 144);
const_assert_eq!(std::mem::offset_of!(Feed, df1_feed), 208);
const_assert_eq!(std::mem::offset_of!(Feed, specific_workflow_metadata), 272);

// ================================================================
// │                Instruction arguments and returns             │
// ================================================================

/// One entry of a report. The forwarder's report payload is a Borsh-encoded
/// `Vec<ReceivedReport>`.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, PartialEq)]
pub struct ReceivedReport {
    /// The data ID the round is for.
    pub data_id: [u8; 16],
    /// The time, in Unix seconds, at which the observations were made offchain.
    pub timestamp: u64,
    /// The answer, with 18 decimals.
    pub answer: i128,
}

/// A round as reads return it, with the answer at the decimals the reader asked for.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, Default, PartialEq)]
pub struct Round {
    /// The round's ID. Round IDs start at 1 and increase by one per stored round.
    pub round_id: u64,
    /// The answer, at the decimals the reader asked for.
    pub answer: i128,
    /// The time, in Unix seconds, at which the round was observed offchain.
    pub observations_timestamp: u64,
    /// The time, in Unix seconds, at which the round was written onchain.
    pub recorded_timestamp: u64,
}

/// The settings `initialize` creates a store with: its first feed admins, the ring size new feeds
/// get, the forward window for report timestamps, and the DF1 program whose feeds it may write.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug, PartialEq)]
pub struct StoreConfig {
    /// The first feed admins. There can be at most 64 (otherwise `FeedAdminsFull`), and they must
    /// be in strictly increasing order (otherwise `InvalidAddress`), which also refuses the
    /// default key.
    pub feed_admins: Vec<Pubkey>,
    /// The ring size of the feeds the store creates. It must be from 2 to 185, because a feed
    /// account created by the program can be at most 10 KiB.
    pub default_max_stored_reports: u64,
    /// How many seconds ahead of the chain's clock a report's observation timestamp may be (see
    /// `StoreState::max_observation_forward_secs`).
    pub max_observation_forward_secs: u32,
    /// The DF1 program whose feed accounts the store may write. It must not be the default key or
    /// this program.
    pub df1_program: Pubkey,
}

/// The reason a round was stored without being written to its linked DF1 feed. The
/// `Df1WriteSkipped` event carries it.
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Debug, PartialEq, Eq)]
pub enum Df1SkipReason {
    /// A feed admin has disabled this feed's DF1 write with `set_df1_feeds`, so the round is
    /// stored in this store only.
    Disabled,
    /// The transaction left out the DF1 program or the `df1_writer` PDA, passing this program's
    /// ID in its place, so the store cannot call `submit`.
    NotPassed,
    /// The account is not a DF1 feed that `submit` can write. The configured DF1 program does not
    /// own it, it is shorter than DF1's header, it is not tagged as a feed, or its ring lengths and
    /// cursors are out of range.
    InvalidAccount,
    /// The DF1 feed's layout version is not 2, the only version `submit` accepts.
    InvalidVersion,
    /// The DF1 feed names a writer other than this store's `df1_writer` PDA, so `submit` would
    /// refuse the store's signature. This is expected until the DF1 feed's owner switches its
    /// writer from OCR2 to this store with DF1's `set_writer`.
    NotWriter,
    /// The DF1 feed's latest round was observed at the same time as this round or later, for
    /// example a round OCR2 wrote shortly before the switch. `submit` does not check order, so
    /// writing this round would put the DF1 feed's rounds out of order.
    Stale,
    /// The answer, rescaled from 18 decimals to the DF1 feed's decimals, does not fit in an i128.
    /// This can only happen when the DF1 feed has more than 18 decimals.
    AnswerOverflow,
    /// The DF1 feed's round counter is already at `u32::MAX`, so `submit` would fail when it adds
    /// one.
    RoundIdOverflow,
    /// The answer is not zero, but rescaling it from 18 decimals to the DF1 feed's decimals would
    /// make it zero. The store keeps its exact round and skips only the DF1 write rather than
    /// presenting a non-zero answer as zero to DF1 readers.
    RoundsToZero,
    /// DF1 stores its transmission timestamp as u32, so it cannot store this round's observation
    /// timestamp without wrapping it. The store keeps its exact round and skips only the DF1 write.
    TimestampOverflow,
}

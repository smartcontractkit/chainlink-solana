use anchor_lang::error_code;

// The program has one error enum, so every code is unique and appears in the IDL. Anchor numbers
// each `#[error_code]` block from 6000, and the IDL keeps only one block. New errors are appended
// at the end, so existing errors keep their codes.
#[error_code]
pub enum StoreError {
    /// An instruction's argument lists, or the feed accounts passed to it, don't have one entry
    /// per data ID.
    #[msg("Each list, and the feed accounts passed, must have one entry per data ID")]
    ArrayLengthMismatch,
    /// `set_specific_feed_metadata` was called with no data IDs or no workflows. To remove a
    /// feed's own workflow list, use `remove_specific_feed_metadata`.
    #[msg("At least one data ID and one workflow are required")]
    EmptyConfig,
    /// The data ID is all zeros or has a non-zero SVR path ID, so the store doesn't serve it.
    #[msg("The data ID must not be zero, and its SVR path ID must be zero")]
    InvalidDataId,
    /// An address that must be set is the default key or all zeros: a feed admin, a workflow's
    /// sender or owner, or the DF1 program, which also must not be this program. It is also
    /// returned when the first feed admins are not in strictly increasing order.
    #[msg("An address is unset, not allowed here, or out of order")]
    InvalidAddress,
    /// A workflow entry's name is all zeros.
    #[msg("The workflow name must not be all zeros")]
    InvalidWorkflowName,
    /// A workflow list has more than `MAX_WORKFLOW_METADATAS` (16) entries.
    #[msg("A workflow list can hold at most 16 workflows")]
    MaxEntriesExceeded,
    /// An account is not the one expected. Its address is not the PDA derived for it, it belongs
    /// to another store state or data ID, its ring is shorter than its header says, it is not the
    /// configured DF1 feed or DF1 program, or a link marker holds another data ID.
    #[msg("An account doesn't match the expected address or contents")]
    AccountMismatch,
    /// The instruction was passed too few remaining accounts for its data IDs or reports, or
    /// accounts were left over after the last one it needed.
    #[msg("Too few or too many remaining accounts were passed")]
    InvalidAccountCount,
    /// An account the instruction must write was passed read-only: a feed being changed or grown,
    /// a link marker, the feed of a report that is stored, or a DF1 feed about to be written.
    #[msg("This account must be passed writable")]
    AccountNotWritable,
    /// An account is not of the expected kind: this program doesn't own it, its tag is not the
    /// expected type's, or it is too short for that type.
    #[msg("The account isn't one of this program's accounts of the expected type")]
    InvalidAccount,
    /// `remove_specific_feed_metadata` was called for a feed that has no workflow list of its own,
    /// including a feed that doesn't exist yet.
    #[msg("The feed has no workflow list of its own to remove")]
    FeedNotConfigured,
    /// A ring size is below 2, or would grow a feed account by more than 10 KiB in one
    /// instruction. For `initialize`, the default ring size would make a new feed account larger
    /// than 10 KiB, which is more than 278 rounds.
    #[msg("The ring size must be at least 2 and grow a feed by at most 10 KiB per instruction")]
    InvalidMaxStoredReports,
    /// The new ring size is not larger than the feed's current one. A ring can only grow.
    #[msg("The ring size can only increase")]
    MaxStoredReportsNotIncreased,
    /// The feed has stored at least as many rounds as its ring holds, so the ring has wrapped and
    /// can no longer grow.
    #[msg("The ring size can't change after the ring has wrapped")]
    MaxStoredReportsLocked,
    /// No longer returned: growing a ring before it wraps always makes it larger than the latest
    /// round ID. The variant is kept so that the errors after it keep their codes.
    #[msg("max stored reports must exceed the latest round id")]
    MaxStoredReportsTooLow,
    /// `set_min_max_answers` was given a minimum answer larger than the maximum.
    #[msg("The minimum answer must not exceed the maximum")]
    InvalidMinMaxRange,
    /// The requested round doesn't exist. The feed has no round yet, the round ID is 0 or later
    /// than the latest, or the round has been overwritten in the ring.
    #[msg("The round doesn't exist or is no longer stored")]
    NoDataPresent,
    /// A description is longer than 64 UTF-8 bytes.
    #[msg("A description can be at most 64 bytes")]
    DescriptionTooLong,
    /// A description contains a zero byte, which would cut it short when it is read back.
    #[msg("A description must not contain zero bytes")]
    InvalidDescription,
    /// The forwarder's metadata is shorter than 62 bytes, or the report is not a valid
    /// Borsh-encoded `Vec<ReceivedReport>`.
    #[msg("The report or its metadata can't be decoded")]
    MalformedReport,
    /// The store already has `MAX_FEED_ADMINS` (64) feed admins, or `initialize` was given more
    /// than that.
    #[msg("A store can have at most 64 feed admins")]
    FeedAdminsFull,
    /// `transfer_ownership` proposed the current owner.
    #[msg("The proposed owner must differ from the current owner")]
    InvalidProposedOwner,
    /// A recovery asked for more than the account holds. For SOL, it is also returned when the
    /// amount would leave the store authority with some SOL but less than the rent-exempt minimum.
    #[msg("The amount exceeds the balance or would leave less than the rent-exempt minimum")]
    InsufficientBalance,
    /// An account's layout version is not `ACCOUNT_VERSION`, the only one this program reads.
    #[msg("The account has a layout version this program doesn't support")]
    InvalidAccountVersion,
    /// The signer is not allowed to call this instruction. It is not the owner, the proposed
    /// owner, a feed admin, or, for `initialize`, the program's upgrade authority.
    #[msg("The signer isn't allowed to call this instruction")]
    Unauthorized,
    /// `set_df1_feeds` was given an account that is not a DF1 feed `submit` can write. The
    /// configured DF1 program doesn't own it, it isn't tagged as a feed, its version isn't 2, or
    /// its ring lengths and cursors are out of range.
    #[msg("The account isn't a DF1 feed this store can write")]
    InvalidDf1Feed,
    /// A read asked for fewer decimals than the feed's minimum or more than 18, or
    /// `set_min_decimals` was given more than 18.
    #[msg("The decimals must be between the feed's minimum and 18")]
    InvalidDecimals,
    /// The answer is not zero but truncates to zero at the requested decimals. Asking for more
    /// decimals reads it.
    #[msg("A non-zero answer rounds to zero at the requested decimals")]
    RoundsToZero,
    /// A feed admin has frozen round reads of the feed.
    #[msg("Round reads of this feed are frozen")]
    FeedFrozen,
    /// The DF1 feed is already linked from another data ID. Unlink it there first, in an earlier
    /// call or earlier in the same call.
    #[msg("The DF1 feed is already linked from another data ID")]
    Df1FeedAlreadyLinked,
    /// `initialize` was called for a store ID whose state account already exists.
    #[msg("A store already exists for this store ID")]
    StoreAlreadyInitialized,
}

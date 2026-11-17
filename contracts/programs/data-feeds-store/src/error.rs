use anchor_lang::error_code;

// One error enum, so every code is unique and in the IDL (Anchor numbers each `#[error_code]`
// block from 6000 and the IDL keeps only one block). New errors are appended so existing codes
// keep their numbers.
#[error_code]
pub enum StoreError {
    #[msg("array lengths must match")]
    ArrayLengthMismatch,
    #[msg("config is empty")]
    EmptyConfig,
    #[msg("invalid data id, it must not be zero and its svr path id must be zero")]
    InvalidDataId,
    #[msg("invalid address")]
    InvalidAddress,
    #[msg("invalid workflow name")]
    InvalidWorkflowName,
    #[msg("too many entries")]
    MaxEntriesExceeded,
    #[msg("account does not match the expected address or data")]
    AccountMismatch,
    #[msg("invalid number of accounts")]
    InvalidAccountCount,
    #[msg("account must be writable")]
    AccountNotWritable,
    #[msg("account is not initialized or has an unexpected layout")]
    InvalidAccount,
    #[msg("feed has no specific workflow metadata")]
    FeedNotConfigured,
    #[msg(
        "max stored reports must be at least 2 and grow a feed by at most 10 KiB per instruction"
    )]
    InvalidMaxStoredReports,
    #[msg("max stored reports can only increase")]
    MaxStoredReportsNotIncreased,
    #[msg("max stored reports can only change before the ring wraps")]
    MaxStoredReportsLocked,
    #[msg("max stored reports must exceed the latest round id")]
    /// No longer returned: growing the ring before it wraps always exceeds the latest round.
    /// Kept so the error codes after it don't change.
    MaxStoredReportsTooLow,
    #[msg("min answer exceeds max answer")]
    InvalidMinMaxRange,
    #[msg("no data present")]
    NoDataPresent,
    #[msg("description is too long")]
    DescriptionTooLong,
    #[msg("description must not contain NUL bytes")]
    InvalidDescription,
    #[msg("malformed report")]
    MalformedReport,
    #[msg("feed admin list is full")]
    FeedAdminsFull,
    #[msg("invalid proposed owner")]
    InvalidProposedOwner,
    #[msg("insufficient balance")]
    InsufficientBalance,
    #[msg("account has an unsupported version")]
    InvalidAccountVersion,
    #[msg("the signer is unauthorized")]
    Unauthorized,
    #[msg("not a DF1 feed this store can write")]
    InvalidDf1Feed,
    #[msg("decimals are below the feed's minimum or above the stored decimals")]
    InvalidDecimals,
    #[msg("a non-zero answer rounds to zero at the requested decimals")]
    RoundsToZero,
    #[msg("feed is frozen")]
    FeedFrozen,
    #[msg("DF1 feed is already linked from another data ID")]
    Df1FeedAlreadyLinked,
}

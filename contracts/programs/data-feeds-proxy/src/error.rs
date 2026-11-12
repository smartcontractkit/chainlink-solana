use anchor_lang::error_code;

// One error enum, so every code is unique and in the IDL (Anchor numbers each `#[error_code]`
// block from 6000 and the IDL keeps only one block). New errors are appended so existing codes
// keep their numbers.
#[error_code]
pub enum ProxyError {
    #[msg("decimals are below the feed's minimum or above the stored decimals")]
    InvalidDecimals,
    #[msg("a non-zero answer rounds to zero at the requested decimals")]
    RoundsToZero,
    #[msg("invalid address")]
    InvalidAddress,
    #[msg("invalid proposed owner")]
    InvalidProposedOwner,
    #[msg("account has an unsupported version")]
    InvalidAccountVersion,
    #[msg("The signer is unauthorized")]
    Unauthorized,
    #[msg("account is not initialized or has an unexpected layout")]
    InvalidAccount,
    #[msg("account does not match the expected address or data")]
    AccountMismatch,
    #[msg("array lengths must match")]
    ArrayLengthMismatch,
    #[msg("no data present")]
    NoDataPresent,
    #[msg("feed is frozen")]
    FeedFrozen,
    #[msg("round is not newer than the feed's latest round")]
    StaleRound,
    #[msg("description is too long")]
    DescriptionTooLong,
    #[msg("description must not contain NUL bytes")]
    InvalidDescription,
    #[msg("max stored reports must be at least 2 and fit in one account")]
    InvalidMaxStoredReports,
    #[msg("max stored reports can only increase once configured")]
    MaxStoredReportsNotIncreased,
    #[msg("max stored reports can only change before the ring wraps")]
    MaxStoredReportsLocked,
    #[msg("max stored reports must exceed the latest round id")]
    MaxStoredReportsTooLow,
    #[msg("account must be writable")]
    AccountNotWritable,
}

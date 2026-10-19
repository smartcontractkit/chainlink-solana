use anchor_lang::error_code;

// Kept separate from `StoreError`: anchor-go only generates Go types for error blocks after the
// first one (see data-feeds-cache/src/error.rs).
#[error_code]
pub enum AuthError {
    #[msg("The signer is unauthorized")]
    Unauthorized,
}

#[error_code]
pub enum StoreError {
    #[msg("array lengths must match")]
    ArrayLengthMismatch,
    #[msg("config is empty")]
    EmptyConfig,
    #[msg("invalid data id")]
    InvalidDataId,
    #[msg("svr path id must not be zero")]
    ZeroSvrPathId,
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
    #[msg("max stored reports must be at least 2 and fit in one account")]
    InvalidMaxStoredReports,
    #[msg("max stored reports can only increase once configured")]
    MaxStoredReportsNotIncreased,
    #[msg("max stored reports can only change before the ring wraps")]
    MaxStoredReportsLocked,
    #[msg("max stored reports must exceed the latest round id")]
    MaxStoredReportsTooLow,
    #[msg("min answer exceeds max answer")]
    InvalidMinMaxRange,
    #[msg("fallback time exceeds max stored reports")]
    FallbackTimeAboveMaxStoredReports,
    #[msg("svr data ids need max sync iterations")]
    InvalidMaxSyncIterations,
    #[msg("decimals exceed the stored decimals")]
    DecimalsAboveStoredDecimals,
    #[msg("report callback program is not executable")]
    ReportCallbackNoContract,
    #[msg("description is too long")]
    DescriptionTooLong,
    #[msg("malformed report")]
    MalformedReport,
    #[msg("feed admin list is full")]
    FeedAdminsFull,
    #[msg("invalid proposed owner")]
    InvalidProposedOwner,
    #[msg("insufficient balance")]
    InsufficientBalance,
}

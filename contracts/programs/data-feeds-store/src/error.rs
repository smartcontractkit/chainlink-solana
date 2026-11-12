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
    #[msg("max stored reports must be at least 2 and fit in one account")]
    InvalidMaxStoredReports,
    #[msg("min answer exceeds max answer")]
    InvalidMinMaxRange,
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
    #[msg("The signer is unauthorized")]
    Unauthorized,
    #[msg("not a DF1 feed this store can write")]
    InvalidDf1Feed,
}

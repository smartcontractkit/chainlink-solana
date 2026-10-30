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
    #[msg("feed admin list is full")]
    FeedAdminsFull,
    #[msg("feed admins must be strictly increasing")]
    FeedAdminsNotSorted,
    #[msg("invalid proposed owner")]
    InvalidProposedOwner,
    #[msg("account has an unsupported version")]
    InvalidAccountVersion,
    #[msg("The signer is unauthorized")]
    Unauthorized,
}

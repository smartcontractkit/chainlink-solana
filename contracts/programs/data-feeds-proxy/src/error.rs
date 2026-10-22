use anchor_lang::error_code;

// Kept separate from `ProxyError`: anchor-go only generates Go types for error blocks after the
// first one.
#[error_code]
pub enum AuthError {
    #[msg("The signer is unauthorized")]
    Unauthorized,
}

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
}

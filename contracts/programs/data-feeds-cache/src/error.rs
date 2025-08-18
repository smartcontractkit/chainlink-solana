use anchor_lang::error_code;

// customizable error range
// note: custom numeric error codes start from 6000 unless specified like #[error_code(offset = 1000)]
// https://github.com/coral-xyz/anchor/blob/c25bd7b7ebbcaf12f6b8cbd3e6f34ae4e2833cb2/lang/syn/src/codegen/error.rs#L72
// Anchor built-in errors: https://anchor.so/errors
//
// [0:100]   Global errors
// [100:N]   Function errors

// this "AuthError" is separated from the "DataCacheError" for error type generation from "anchor-go" tool
// Known issue: only the first error_code block is included in idl.errors field, and go bindings for this first errors not generated.
// anchor-go generates types for error from the second error_code block onwards.
// This might be a bug in anchor-go, should be revisited once program functionality is stable.
// Workaround: keep errors that not likely to change during development in the first error_code block(keeping hardcoded error types for this),
// and other errors in the second block.
#[error_code]
pub enum AuthError {
    #[msg("The signer is unauthorized")]
    Unauthorized,
}

#[error_code]
pub enum DataCacheError {
    #[msg("config is empty")]
    EmptyConfig,

    #[msg("array length should be equal")]
    ArrayLengthMismatch,

    #[msg("invalid data id")]
    InvalidDataId,

    #[msg("invalid number of accounts")]
    InvalidAccountCount,

    #[msg("account mismatch")]
    AccountMismatch,

    #[msg("invalid address")]
    InvalidAddress,

    #[msg("invalid workflow name")]
    InvalidWorkflowName,

    #[msg("exceeded max amount allowed")]
    MaxWorkflowsExceeded,

    #[msg("addresses must strictly increase")]
    AddressesMustStrictlyIncrease,

    #[msg("out of bounds")]
    OutOfBounds,

    #[msg("ids must strictly increase")]
    IdsMustStrictlyIncrease,

    #[msg("missing legacy feed account")]
    MissingLegacyFeedAccount,

    #[msg("feed not configured")]
    FeedNotConfigured,

    #[msg("invalid length")]
    InvalidLength,

    #[msg("malformed report")]
    MalformedReport,

    #[msg("failed legacy write")]
    FailedLegacyWrite,

    #[msg("Invalid proposed owner")]
    InvalidProposedOwner,

    #[msg("feed config workflow list not empty")]
    FeedConfigListNotEmpty,

    #[msg("empty description enforced")]
    EmptyDescriptionEnforced,

    #[msg("invalid description")]
    InvalidDescription,
}

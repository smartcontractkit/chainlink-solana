use anchor_lang::error_code;

// customizable error range
// note: custom numeric error codes start from 6000 unless specified like #[error_code(offset = 1000)]
// https://github.com/coral-xyz/anchor/blob/c25bd7b7ebbcaf12f6b8cbd3e6f34ae4e2833cb2/lang/syn/src/codegen/error.rs#L72
// Anchor built-in errors: https://anchor.so/errors
//
// [0:100]   Global errors
// [100:N]   Function errors

// this "AuthError" is separated from the "ForwarderError" for error type generation from "anchor-go" tool
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
pub enum ForwarderError {
    #[msg("Invalid proposed owner")]
    InvalidProposedOwner,

    #[msg("Signers exceed max limit")]
    ExcessSigners,

    #[msg("Signer addresses must strictly increase")]
    SignersNotSortedInIncreasingOrder,

    #[msg("Report does not meet minimum length")]
    InvalidReport,

    #[msg("Invalid signature count")]
    InvalidSignatureCount,

    #[msg("Invalid signature")]
    InvalidSignature,

    #[msg("Unauthorized signer")]
    UnauthorizedSigner,

    #[msg("Duplicate signatures")]
    DuplicateSignatures,

    #[msg("Execution already succeded")]
    ExecutionAlreadySucceded,

    #[msg("Fault tolerance must be positive")]
    FaultToleranceMustBePositive,

    #[msg("Insufficient Signers")]
    InsufficientSigners,

    #[msg("Forwarder Report Expected")]
    ForwarderReportExpected,

    #[msg("Invalid Account Hash")]
    InvalidAccountHash,
}

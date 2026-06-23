use anchor_lang::error_code;

// First block kept for anchor-go binding-generation quirk (see keystone-forwarder/src/error.rs).
#[error_code]
pub enum AuthError {
    #[msg("The signer is unauthorized")]
    Unauthorized,
}

#[error_code]
pub enum ForwarderError {
    #[msg("Invalid proposed owner")]
    InvalidProposedOwner,

    #[msg("Report does not meet minimum length")]
    InvalidReport,

    #[msg("Execution already succeded")]
    ExecutionAlreadySucceded,

    #[msg("Forwarder Report Expected")]
    ForwarderReportExpected,

    #[msg("Invalid Account Hash")]
    InvalidAccountHash,
}

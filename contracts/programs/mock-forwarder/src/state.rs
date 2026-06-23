use anchor_lang::prelude::*;

/// Account which represents a distinct instance of a mock forwarder.
#[account]
#[derive(Default, InitSpace)]
pub struct ForwarderState {
    pub version: u8,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
}

/// Per-transmission account. Persists so repeat submissions for the same
/// `transmission_id` abort — mirrors prod replay-protection semantics.
#[account]
#[derive(Default, InitSpace)]
pub struct ExecutionState {
    pub transmitter: Pubkey,
    pub transmission_id: [u8; 32],
    pub success: bool,
}

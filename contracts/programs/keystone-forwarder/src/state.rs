use anchor_lang::prelude::*;

// combination of solidity's OracleSet and the configId mapping
#[account]
pub struct OraclesConfig {
    pub config_id: u64,
    pub f: u8,
    pub signer_addresses: Vec<[u8; 20]>,
}

impl OraclesConfig {
    pub const INIT_SPACE: usize = 8 + 1 + 4;

    pub fn space_with_signers(num_signers: usize) -> usize {
        Self::INIT_SPACE + (num_signers * 20)
    }
}

#[account]
#[derive(Default, InitSpace)]
pub struct ForwarderState {
    pub version: u8,
    pub authority_nonce: u8, // bump
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
}

#[account]
#[derive(Default, InitSpace)]
pub struct ExecutionState {
    pub transmitter: Pubkey,
    pub transmission_id: [u8; 32],
    // until failure states are reported by the write target, success will always be true
    pub success: bool,
}

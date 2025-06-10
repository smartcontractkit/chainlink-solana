use crate::common::MAX_ACCTS;
use anchor_lang::prelude::*;
use arrayvec::arrayvec;

#[zero_copy]
#[derive(InitSpace)]
pub struct SignerAddresses {
    pub xs: [[u8; 20]; MAX_ACCTS], // Fixed array of 64 addresses (20 bytes each)
    pub len: u8,                   // Length fits in u8 since MAX_ACCTS is 64
    pub _padding: [u8; 7],         // Padding for proper alignment
}

impl Default for SignerAddresses {
    fn default() -> Self {
        Self {
            xs: [[0u8; 20]; MAX_ACCTS],
            len: 0,
            _padding: [0; 7],
        }
    }
}

// Apply the arrayvec macro to get all the utility methods
arrayvec!(SignerAddresses, [u8; 20], u8);

// combination of solidity's OracleSet and the configId mapping
#[account(zero_copy)]
#[derive(InitSpace)]
pub struct OraclesConfig {
    pub config_id: u64, // 4 bytes
    pub f: u8,
    pub _padding: [u8; 7],                 // 7 bytes padding for alignment
    pub signer_addresses: SignerAddresses, // 64*20 + 1 + 7 = 1288 bytes
}

impl OraclesConfig {
    // 8 + 8 + 1 + 7 + (64*20 + 1 + 7) = 8 + 8 + 1 + 7 + 1288 = 1312 bytes
    pub const INIT_SPACE: usize = 8 + 8 + 1 + 7 + (MAX_ACCTS * 20 + 1 + 7);
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

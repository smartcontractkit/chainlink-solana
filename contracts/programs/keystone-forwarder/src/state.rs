use crate::common::MAX_ACCTS;
use anchor_lang::prelude::*;
use arrayvec::arrayvec;

#[zero_copy]
#[derive(InitSpace, AnchorSerialize, AnchorDeserialize)]
pub struct SignerAddresses {
    pub xs: [[u8; 20]; MAX_ACCTS], // Fixed array of 64 addresses (20 bytes each)
    pub len: u64
}

// Apply the arrayvec macro to get all the utility methods
arrayvec!(SignerAddresses, [u8; 20], u64);

// combination of solidity's OracleSet and the configId mapping
#[account(zero_copy)]
#[derive(InitSpace, AnchorSerialize, AnchorDeserialize)]
pub struct OraclesConfig {
    pub config_id: u64, // 8 bytes
    pub f: u8,          // 1 byte
    pub _padding: [u8; 7], // 7 bytes to align to 8 bytes     
    pub signer_addresses: SignerAddresses, // 64*20 + 1 + 7 = 1288 bytes
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

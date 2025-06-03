use anchor_lang::prelude::*;
use crate::common::MAX_ACCTS;

// combination of solidity's OracleSet and the configId mapping
#[account(zero_copy)]
#[derive(bytemuck::Zeroable, bytemuck::Pod)]
#[repr(C)]
pub struct OraclesConfig {
    pub config_id: u64,                              // 4 bytes
    pub f: u8,
    pub _padding: [u8; 7],                           // 7 bytes padding for alignment
    pub signer_addresses: [[u8; 20]; MAX_ACCTS],    // Fixed size: 20 * MAX_ACCTS + 8 bytes
}

impl OraclesConfig {
    // discriminator + config_id + f + padding + (address_size * max_addresses) + arrayvec_overhead
    pub const INIT_SPACE: usize = 8 + 4 + 1 + 7 + (20 * 64) + 8; 
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

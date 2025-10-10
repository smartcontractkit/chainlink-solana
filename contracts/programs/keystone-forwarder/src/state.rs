use crate::common::MAX_ACCTS;
use anchor_lang::prelude::*;
use arrayvec::arrayvec;
use static_assertions::const_assert;
use std::mem;

#[zero_copy]
#[derive(InitSpace, AnchorSerialize, AnchorDeserialize)]
pub struct SignerAddressList {
    pub xs: [[u8; 20]; MAX_ACCTS], // Fixed array of 32 addresses (20 bytes each)
    pub len: u64,
}

// Apply the arrayvec macro to get all the utility methods
arrayvec!(SignerAddressList, [u8; 20], u64);
const_assert!(
    mem::size_of::<SignerAddressList>()
        == mem::size_of::<u64>() + mem::size_of::<[u8; 20]>() * MAX_ACCTS
);

/// Account which represent a set of oracles expected to sign a forwarder report.
#[account(zero_copy)]
#[derive(InitSpace, AnchorSerialize, AnchorDeserialize)]
pub struct OraclesConfig {
    pub config_id: u64,                      // 8 bytes
    pub f: u8,                               // 1 byte
    pub _padding: [u8; 7],                   // 7 bytes to align to 8 bytes
    pub signer_addresses: SignerAddressList, // 32*20 + 8 = 648 bytes
}

/// Account which represents a distinct instance of a forwarder.
#[account]
#[derive(Default, InitSpace)]
pub struct ForwarderState {
    pub version: u8,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
}

/// Account which stores status of a transmission.
/// This account will never be closed because it provides persistent proof if a transmission was received on-chain.
#[account]
#[derive(Default, InitSpace)]
pub struct ExecutionState {
    pub transmitter: Pubkey,
    pub transmission_id: [u8; 32],
    // until failure states are reported by the write target, success will always be true
    pub success: bool,
}

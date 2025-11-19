use crate::common::MAX_ORACLES;
use anchor_lang::prelude::*;
use arrayvec::arrayvec;
use static_assertions::const_assert;
use std::mem;
use std::{
    fmt::{self, Display, Formatter},
    str::FromStr,
};

#[zero_copy]
#[derive(InitSpace, AnchorSerialize, AnchorDeserialize)]
pub struct SignerAddressList {
    pub xs: [[u8; 20]; MAX_ORACLES], // Fixed array of 16 addresses (20 bytes each)
    pub len: u64,
}

// Apply the arrayvec macro to get all the utility methods
arrayvec!(SignerAddressList, [u8; 20], u64);
const_assert!(
    mem::size_of::<SignerAddressList>()
        == mem::size_of::<u64>() + mem::size_of::<[u8; 20]>() * MAX_ORACLES
);

#[zero_copy]
#[derive(InitSpace, AnchorSerialize, AnchorDeserialize)]
pub struct TransmitterAddressList {
    pub xs: [[u8; 32]; MAX_ORACLES], // Fixed array of 16 addresses (32 bytes each - Solana public keys)
    pub len: u64,
}

// Apply the arrayvec macro to get all the utility methods
arrayvec!(TransmitterAddressList, [u8; 32], u64);
const_assert!(
    mem::size_of::<TransmitterAddressList>()
        == mem::size_of::<u64>() + mem::size_of::<[u8; 32]>() * MAX_ORACLES
);

/// Account which represent a set of oracles expected to sign a forwarder report.
#[account(zero_copy)]
#[derive(InitSpace, AnchorSerialize, AnchorDeserialize)]
pub struct OraclesConfig {
    pub config_id: u64,                                // 8 bytes
    pub f: u8,                                         // 1 byte
    pub _padding: [u8; 7],                             // 7 bytes to align to 8 bytes
    pub signer_addresses: SignerAddressList,           // 16*20 + 8 = 328 bytes
    pub transmitter_addresses: TransmitterAddressList, // 16*32 + 8 = 528 bytes
}

/// Account which represents a distinct instance of a forwarder.
#[account]
#[derive(Default, InitSpace)]
pub struct ForwarderState {
    pub version: u8,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
}

/// Status enum for execution state
#[derive(
    Clone, AnchorSerialize, AnchorDeserialize, InitSpace, Debug, PartialEq, Eq, Copy, Default,
)]
pub enum Status {
    #[default]
    NotReported, // 0
    Success, // 1
    Failure, // 2
}

impl Display for Status {
    fn fmt(&self, f: &mut Formatter<'_>) -> fmt::Result {
        match self {
            Status::NotReported => write!(f, "NotReported"),
            Status::Success => write!(f, "Success"),
            Status::Failure => write!(f, "Failure"),
        }
    }
}

impl FromStr for Status {
    type Err = ();
    fn from_str(s: &str) -> std::result::Result<Self, Self::Err> {
        match s {
            "NotReported" => Ok(Status::NotReported),
            "Success" => Ok(Status::Success),
            "Failure" => Ok(Status::Failure),
            _ => Err(()),
        }
    }
}

/// Account which stores status of a transmission.
/// This account will never be closed because it provides persistent proof if a transmission was received on-chain.
#[account]
#[derive(Default, InitSpace)]
pub struct ExecutionState {
    pub transmitter: Pubkey,
    pub transmission_id: [u8; 32],
    pub status: Status,
}

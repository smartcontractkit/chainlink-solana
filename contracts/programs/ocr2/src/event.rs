use anchor_lang::prelude::*;

// use crate::state::MAX_ORACLES;

// #[index]
#[event]
pub struct SetConfig {
    pub config_digest: [u8; 32],
    pub f: u8,
    pub signers: Vec<[u8; 20]>,
}

#[event]
pub struct SetBilling {
    pub observation_payment_gjuels: u32,
    pub transmission_payment_gjuels: u32,
}

#[event]
pub struct RoundRequested {
    pub config_digest: [u8; 32],
    pub requester: Pubkey,
    pub epoch: u32,
    pub round: u8,
}

#[event]
pub struct NewTransmission {
    #[index]
    pub round_id: u32,
    pub config_digest: [u8; 32],
    pub answer: i128,
    pub transmitter: u8,
    pub observations_timestamp: u32,
    pub observer_count: u8,
    pub observers: [u8; 19], // Can't use MAX_ORACLES because of IDL parsing issues
    pub juels_per_lamport: u64,
    pub reimbursement: u64,
}

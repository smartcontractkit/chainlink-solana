use anchor_lang::prelude::*;

#[event]
pub struct ReportProcessed {
    pub receiver: Pubkey,
    pub transmission_id: [u8; 32],
    pub result: bool,
}

#[event]
pub struct ConfigSet {
    pub don_id: u32,
    pub config_version: u32,
    pub f: u8,
    pub signers: Vec<[u8; 20]>,
}

#[event]
pub struct InitializeEmit {
    pub owner: Pubkey,
    pub authority_nonce: u8,
}

#[event]
pub struct OwnershipTransfer {
    pub current_owner: Pubkey,
    pub proposed_owner: Pubkey,
}

#[event]
pub struct OwnershipAcceptance {
    pub previous_owner: Pubkey,
    pub new_owner: Pubkey,
}

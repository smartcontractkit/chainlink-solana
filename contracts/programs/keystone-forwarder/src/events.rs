use anchor_lang::prelude::*;

#[event]
pub struct ReportProcessedEvent {
    pub receiver: Pubkey,
    pub transmission_id: [u8; 32],
    pub result: bool,
}

#[event]
pub struct ConfigSetEvent {
    pub don_id: u32,
    pub config_version: u32,
    pub f: u8,
    pub signers: Vec<[u8; 20]>,
}

#[event]
pub struct InitializeEmitEvent {
  pub owner: Pubkey,
  pub authority_nonce: u8,
  pub timestamp: i64,
}

#[event]
pub struct TransferOwnershipEvent {
  pub new_owner: Pubkey
}

#[event]
pub struct AcceptOwnershipEvent {
  pub owner: Pubkey
}
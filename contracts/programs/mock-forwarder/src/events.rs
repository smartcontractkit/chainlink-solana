use anchor_lang::prelude::*;

#[event]
pub struct ReportProcessed {
    pub state: Pubkey,
    pub receiver: Pubkey,
    pub transmission_id: [u8; 32],
}

#[event]
pub struct ReportInProgress {
    pub state: Pubkey,
    pub transmission_id: [u8; 32],
}

#[event]
pub struct ForwarderInitialize {
    pub state: Pubkey,
    pub owner: Pubkey,
}

#[event]
pub struct OwnershipTransfer {
    pub state: Pubkey,
    pub current_owner: Pubkey,
    pub proposed_owner: Pubkey,
}

#[event]
pub struct OwnershipAcceptance {
    pub state: Pubkey,
    pub previous_owner: Pubkey,
    pub new_owner: Pubkey,
}

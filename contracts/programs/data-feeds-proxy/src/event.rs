use anchor_lang::prelude::*;

#[event]
pub struct ProxyInitialized {
    pub state: Pubkey,
    pub store_program: Pubkey,
    pub store_state: Pubkey,
}

#[event]
pub struct OwnershipTransferRequested {
    pub state: Pubkey,
    pub from: Pubkey,
    pub to: Pubkey,
}

#[event]
pub struct OwnershipTransferred {
    pub state: Pubkey,
    pub from: Pubkey,
    pub to: Pubkey,
}

#[event]
pub struct FeedAdminSet {
    pub state: Pubkey,
    pub feed_admin: Pubkey,
    pub is_admin: bool,
}

#[event]
pub struct StoreSet {
    pub state: Pubkey,
    pub previous_program: Pubkey,
    pub previous_state: Pubkey,
    pub new_program: Pubkey,
    pub new_state: Pubkey,
}

#[event]
pub struct MinDecimalsSet {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub min_decimals: u8,
}

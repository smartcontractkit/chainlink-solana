//! Events of the proxy's owner actions. Feed changes are made by the store, which emits their
//! events (the proxy's writer-signed instructions emit none, so writes need no event accounts).

use anchor_lang::prelude::*;

#[event]
pub struct ProxyInitialized {
    pub state: Pubkey,
    pub writer: Pubkey,
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
pub struct StoreSet {
    pub state: Pubkey,
    pub previous_writer: Pubkey,
    pub new_writer: Pubkey,
}

#[event]
pub struct MinDecimalsSet {
    pub state: Pubkey,
    pub data_id: [u8; 16],
    pub min_decimals: u8,
}

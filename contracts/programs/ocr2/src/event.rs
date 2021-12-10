use anchor_lang::prelude::*;

#[event]
pub struct SetConfig {
    // #[index]
    pub previous_config_block_number: u64,
    pub latest_config_digest: [u8; 32],
}

#[event]
pub struct RoundRequested {
    // #[index]
    pub requester: Pubkey,
    pub config_digest: [u8; 32],
    pub round: u8,
    pub epoch: u32,
}

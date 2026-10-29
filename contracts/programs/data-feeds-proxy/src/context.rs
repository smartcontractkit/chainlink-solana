use anchor_lang::prelude::*;

use crate::common::{data_id_seed, ACCOUNT_VERSION, ANCHOR_DISCRIMINATOR, FEED_CONFIG_SEED};
use crate::error::{AuthError, ProxyError};

// Contexts that change state emit CPI events (`#[event_cpi]` adds the event authority PDA and this
// program as their last accounts). Every context checks the state's layout version.
use crate::state::{FeedConfig, ProxyState};

#[event_cpi]
#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(mut)]
    pub owner: Signer<'info>,

    #[account(init, payer = owner, space = ANCHOR_DISCRIMINATOR + ProxyState::INIT_SPACE)]
    pub state: AccountLoader<'info, ProxyState>,

    /// CHECK: the store program, executable is enough
    #[account(executable)]
    pub store_program: UncheckedAccount<'info>,

    /// CHECK: state account of that store, it must be owned by the store program
    #[account(owner = store_program.key())]
    pub store_state: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct OwnerOnly<'info> {
    #[account(address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(mut, constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(address = state.load()?.proposed_owner @ AuthError::Unauthorized)]
    pub new_owner: Signer<'info>,

    #[account(mut, constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct SetStore<'info> {
    #[account(address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(mut, constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,

    /// CHECK: the new store program, executable is enough
    #[account(executable)]
    pub store_program: UncheckedAccount<'info>,

    /// CHECK: state account of the new store, it must be owned by the new store program
    #[account(owner = store_program.key())]
    pub store_state: UncheckedAccount<'info>,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(data_id: [u8; 16])]
pub struct SetMinDecimals<'info> {
    /// The owner or a feed admin, checked in the handler
    #[account(mut)]
    pub admin: Signer<'info>,

    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,

    #[account(
        init_if_needed,
        payer = admin,
        space = ANCHOR_DISCRIMINATOR + FeedConfig::INIT_SPACE,
        seeds = [FEED_CONFIG_SEED, state.key().as_ref(), &data_id_seed(&data_id)],
        bump,
    )]
    pub feed_config: Account<'info, FeedConfig>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(data_id: [u8; 16])]
pub struct GetMinDecimals<'info> {
    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,

    /// Settings of the data ID, the stored decimals apply when omitted
    #[account(seeds = [FEED_CONFIG_SEED, state.key().as_ref(), &data_id_seed(&data_id)], bump)]
    pub feed_config: Option<Account<'info, FeedConfig>>,
}

/// Read a round of a data ID at the store's decimals.
#[derive(Accounts)]
pub struct Read<'info> {
    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,

    /// CHECK: the configured store program
    #[account(executable, address = state.load()?.store_program)]
    pub store_program: UncheckedAccount<'info>,

    /// CHECK: the configured store state
    #[account(address = state.load()?.store_state)]
    pub store_state: UncheckedAccount<'info>,

    /// CHECK: feed account of the data ID in that store, the store validates it
    pub feed: UncheckedAccount<'info>,
}

/// Read a round of a data ID at the caller's decimals.
#[derive(Accounts)]
#[instruction(data_id: [u8; 16])]
pub struct ReadScaled<'info> {
    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,

    /// CHECK: the configured store program
    #[account(executable, address = state.load()?.store_program)]
    pub store_program: UncheckedAccount<'info>,

    /// CHECK: the configured store state
    #[account(address = state.load()?.store_state)]
    pub store_state: UncheckedAccount<'info>,

    /// CHECK: feed account of the data ID in that store, the store validates it
    pub feed: UncheckedAccount<'info>,

    /// Settings of the data ID, the stored decimals are the minimum when omitted
    #[account(seeds = [FEED_CONFIG_SEED, state.key().as_ref(), &data_id_seed(&data_id)], bump)]
    pub feed_config: Option<Account<'info, FeedConfig>>,
}

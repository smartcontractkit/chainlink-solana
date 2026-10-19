use anchor_lang::prelude::*;
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};

use crate::common::{ANCHOR_DISCRIMINATOR, STORE_AUTHORITY_SEED};
use crate::error::AuthError;
use crate::state::{
    DataIdConfig, FeedData, FeedDescription, ProxyConfig, StoreState, SvrPathDescription,
};

#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(mut)]
    pub owner: Signer<'info>,

    #[account(init, payer = owner, space = ANCHOR_DISCRIMINATOR + StoreState::INIT_SPACE)]
    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: the forwarder program id can differ per environment, executable is enough
    #[account(executable)]
    pub forwarder_program: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct OwnerOnly<'info> {
    #[account(address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(mut)]
    pub state: AccountLoader<'info, StoreState>,
}

#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(address = state.load()?.proposed_owner @ AuthError::Unauthorized)]
    pub new_owner: Signer<'info>,

    #[account(mut)]
    pub state: AccountLoader<'info, StoreState>,
}

#[derive(Accounts)]
pub struct UpdateForwarder<'info> {
    #[account(address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(mut)]
    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: the forwarder program id can differ per environment, executable is enough
    #[account(executable)]
    pub forwarder_program: UncheckedAccount<'info>,
}

/// Feed admin changes to the store state itself.
#[derive(Accounts)]
pub struct FeedAdminState<'info> {
    pub feed_admin: Signer<'info>,

    #[account(mut)]
    pub state: AccountLoader<'info, StoreState>,
}

/// Feed admin changes to per-feed accounts passed as remaining accounts. The feed admin pays for
/// accounts that are created or grown and receives the rent of closed ones.
#[derive(Accounts)]
pub struct FeedAdminAccounts<'info> {
    #[account(mut)]
    pub feed_admin: Signer<'info>,

    pub state: AccountLoader<'info, StoreState>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct OnReport<'info> {
    /// CHECK: the forwarder authority below can only be signed by the forwarder program for this
    /// forwarder state and this receiver, which the forwarder only does for verified reports
    pub forwarder_state: UncheckedAccount<'info>,

    #[account(
        seeds = [b"forwarder", forwarder_state.key().as_ref(), crate::ID.as_ref()],
        bump,
        seeds::program = state.load()?.forwarder_id,
    )]
    pub forwarder_authority: Signer<'info>,

    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: PDA signing report callbacks
    #[account(
        seeds = [STORE_AUTHORITY_SEED, state.key().as_ref()],
        bump = state.load()?.store_authority_bump,
    )]
    pub store_authority: UncheckedAccount<'info>,
    // remaining accounts, one group per report in report order:
    //   SVR paths only: data id config (writable)
    //   non-SVR feed (writable)
    //   callback program + configured callback accounts, when a callback is configured
}

/// Proxy (reader config) reads.
#[derive(Accounts)]
pub struct ProxyRead<'info> {
    pub proxy: Account<'info, ProxyConfig>,

    /// Non-SVR feed of the proxy's data ID, matched in the handler
    pub feed: AccountLoader<'info, FeedData>,

    /// Data ID config of the proxy's data ID, required for SVR data IDs
    pub data_id_config: Option<AccountLoader<'info, DataIdConfig>>,

    pub feed_description: Option<Account<'info, FeedDescription>>,

    pub svr_path_description: Option<Account<'info, SvrPathDescription>>,
}

/// Reads by data ID, bypassing proxy configuration.
#[derive(Accounts)]
pub struct DirectRead<'info> {
    pub state: AccountLoader<'info, StoreState>,

    /// Non-SVR feed of the data ID, matched in the handler
    pub feed: AccountLoader<'info, FeedData>,

    /// Data ID config, required for SVR data IDs
    pub data_id_config: Option<AccountLoader<'info, DataIdConfig>>,
}

#[derive(Accounts)]
pub struct DescriptionRead<'info> {
    pub state: AccountLoader<'info, StoreState>,

    pub feed_description: Option<Account<'info, FeedDescription>>,

    pub svr_path_description: Option<Account<'info, SvrPathDescription>>,
}

#[derive(Accounts)]
pub struct PermissionRead<'info> {
    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: settings of the data ID, its feed (non-SVR) or data ID config (SVR path), checked in
    /// the handler. The default metadata applies when omitted.
    pub settings: Option<UncheckedAccount<'info>>,
}

#[derive(Accounts)]
pub struct RecoverTokens<'info> {
    #[account(address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: PDA owning the store's token accounts
    #[account(
        seeds = [STORE_AUTHORITY_SEED, state.key().as_ref()],
        bump = state.load()?.store_authority_bump,
    )]
    pub store_authority: UncheckedAccount<'info>,

    #[account(mut, token::authority = store_authority, token::mint = mint, token::token_program = token_program)]
    pub from: InterfaceAccount<'info, TokenAccount>,

    #[account(mut, token::mint = mint, token::token_program = token_program)]
    pub to: InterfaceAccount<'info, TokenAccount>,

    pub mint: InterfaceAccount<'info, Mint>,

    pub token_program: Interface<'info, TokenInterface>,
}

#[derive(Accounts)]
pub struct RecoverNative<'info> {
    #[account(address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: PDA holding native SOL sent to the store
    #[account(
        mut,
        seeds = [STORE_AUTHORITY_SEED, state.key().as_ref()],
        bump = state.load()?.store_authority_bump,
    )]
    pub store_authority: UncheckedAccount<'info>,

    /// CHECK: any account can receive SOL
    #[account(mut)]
    pub to: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,
}

use anchor_lang::{prelude::*, solana_program::bpf_loader_upgradeable};
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};

use crate::common::{
    ACCOUNT_VERSION, ANCHOR_DISCRIMINATOR, DF1_WRITER_SEED, FEED_WRITER_SEED, STORE_AUTHORITY_SEED,
};
use crate::error::StoreError;
use crate::state::{FeedConfig, StoreState};

// Every context that changes state emits CPI events (`#[event_cpi]` adds the event authority PDA
// and this program as its last accounts). Every context checks the state's layout version.

/// Only the program's upgrade authority creates stores, so every store under this program ID is
/// ours. The state account is created beforehand (system `create_account`, owned by this program)
/// so that `initialize` needs no other signer, e.g. when the authority is an MCMS timelock.
#[event_cpi]
#[derive(Accounts)]
pub struct Initialize<'info> {
    /// The program's upgrade authority, the store's first owner
    pub owner: Signer<'info>,

    #[account(
        zero,
        constraint = state.to_account_info().data_len() == ANCHOR_DISCRIMINATOR + StoreState::INIT_SPACE
            @ StoreError::InvalidAccount
    )]
    pub state: AccountLoader<'info, StoreState>,

    #[account(
        seeds = [crate::ID.as_ref()],
        bump,
        seeds::program = bpf_loader_upgradeable::ID,
        constraint = program_data.upgrade_authority_address == Some(owner.key()) @ StoreError::Unauthorized
    )]
    pub program_data: Account<'info, ProgramData>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct OwnerOnly<'info> {
    #[account(address = state.load()?.owner @ StoreError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(mut, constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(address = state.load()?.proposed_owner @ StoreError::Unauthorized)]
    pub new_owner: Signer<'info>,

    #[account(mut, constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,
}

/// Feed admin changes to the store state itself.
#[event_cpi]
#[derive(Accounts)]
pub struct FeedAdminState<'info> {
    pub feed_admin: Signer<'info>,

    #[account(mut, constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,
}

/// Feed admin changes to feed configs passed as remaining accounts. The feed admin pays for configs
/// that are created.
#[event_cpi]
#[derive(Accounts)]
pub struct FeedAdminConfigs<'info> {
    #[account(mut)]
    pub feed_admin: Signer<'info>,

    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

    pub system_program: Program<'info, System>,
}

/// Feed admin changes to proxy feeds passed as remaining accounts, made by the store's
/// `feed_writer` through the proxy. The feed admin pays for feeds that are created or grown.
#[event_cpi]
#[derive(Accounts)]
pub struct FeedAdminProxy<'info> {
    #[account(mut)]
    pub feed_admin: Signer<'info>,

    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: the proxy program this store writes
    #[account(executable, address = state.load()?.proxy_program @ StoreError::AccountMismatch)]
    pub proxy_program: UncheckedAccount<'info>,

    /// CHECK: the proxy state trusting this store, checked by the proxy
    #[account(address = state.load()?.proxy_state @ StoreError::AccountMismatch)]
    pub proxy_state: UncheckedAccount<'info>,

    /// CHECK: the store's signer towards the proxy
    #[account(seeds = [FEED_WRITER_SEED, state.key().as_ref()], bump = state.load()?.feed_writer_bump)]
    pub feed_writer: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct OnReport<'info> {
    /// CHECK: passed by the forwarder, not read
    pub forwarder_state: UncheckedAccount<'info>,

    /// The sender: reports are only applied when it is the allowed sender of a workflow permitted
    /// for the data ID, a forwarder authority PDA that its forwarder only signs with for reports it
    /// verified. Any number of forwarders, e.g. one per zone, is just more workflow metadata.
    pub forwarder_authority: Signer<'info>,

    /// Version checked in the handler, which loads it once for every report
    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: the proxy program this store writes
    #[account(address = state.load()?.proxy_program @ StoreError::AccountMismatch)]
    pub proxy_program: UncheckedAccount<'info>,

    /// CHECK: the proxy state trusting this store, checked by the proxy
    #[account(address = state.load()?.proxy_state @ StoreError::AccountMismatch)]
    pub proxy_state: UncheckedAccount<'info>,

    /// CHECK: the store's signer towards the proxy
    #[account(seeds = [FEED_WRITER_SEED, state.key().as_ref()], bump = state.load()?.feed_writer_bump)]
    pub feed_writer: UncheckedAccount<'info>,

    /// CHECK: the DF1 program, when a report's feed is kept written in DF1
    #[account(address = state.load()?.df1_program @ StoreError::AccountMismatch)]
    pub df1_program: Option<UncheckedAccount<'info>>,

    /// CHECK: the store's signer towards DF1, when a report's feed is kept written in DF1
    #[account(seeds = [DF1_WRITER_SEED, state.key().as_ref()], bump = state.load()?.df1_writer_bump)]
    pub df1_writer: Option<UncheckedAccount<'info>>,

    /// CHECK: the vault paying for feeds created by their first report, when one is created
    #[account(mut, seeds = [STORE_AUTHORITY_SEED, state.key().as_ref()], bump = state.load()?.store_authority_bump)]
    pub vault: Option<UncheckedAccount<'info>>,

    /// When a feed is created
    pub system_program: Option<Program<'info, System>>,
    // event authority and program (`#[event_cpi]`), then the remaining accounts, one group per
    // report in report order:
    //   the data ID's proxy feed (writable)
    //   the data ID's feed config (writable when it doesn't exist yet)
    //   the DF1 feed (writable), when the feed config links one
}

/// Whether a workflow may write a data ID.
#[derive(Accounts)]
pub struct PermissionRead<'info> {
    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

    /// Config of the data ID, the default metadata applies when omitted
    pub feed_config: Option<AccountLoader<'info, FeedConfig>>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct RecoverTokens<'info> {
    #[account(address = state.load()?.owner @ StoreError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
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

#[event_cpi]
#[derive(Accounts)]
pub struct RecoverNative<'info> {
    #[account(address = state.load()?.owner @ StoreError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: the vault, holding SOL for feed creation and SOL sent to the store
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

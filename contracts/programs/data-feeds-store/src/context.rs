use anchor_lang::prelude::*;
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};

use crate::common::{ACCOUNT_VERSION, ANCHOR_DISCRIMINATOR, STORE_AUTHORITY_SEED};
use crate::error::StoreError;
use crate::state::{FeedData, FeedDescription, StoreState};

// Every context that changes state emits CPI events (`#[event_cpi]` adds the event authority PDA
// and this program as its last accounts). Every context checks the state's layout version.

#[event_cpi]
#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(mut)]
    pub owner: Signer<'info>,

    #[account(init, payer = owner, space = ANCHOR_DISCRIMINATOR + StoreState::INIT_SPACE)]
    pub state: AccountLoader<'info, StoreState>,

    pub system_program: Program<'info, System>,
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

/// Feed admin changes to per-feed accounts passed as remaining accounts. The feed admin pays for
/// accounts that are created or grown.
#[event_cpi]
#[derive(Accounts)]
pub struct FeedAdminAccounts<'info> {
    #[account(mut)]
    pub feed_admin: Signer<'info>,

    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

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
    // event authority and program (`#[event_cpi]`), then the remaining accounts, one group per
    // report in report order:
    //   feed of the data id (writable)
    //   callback program, its callback authority PDA, then the configured callback accounts, when
    //   a callback is configured
}

/// Owner allows or disallows a program as a report callback.
#[event_cpi]
#[derive(Accounts)]
pub struct SetCallbackProgramAllowed<'info> {
    #[account(mut, address = state.load()?.owner @ StoreError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: allowlist entry PDA `[CALLBACK_PROGRAM_SEED, state, callback program]`, created in
    /// the handler
    #[account(mut)]
    pub allowlist_entry: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,
}

/// Round reads by data ID: the reader interface proxies and other programs call.
#[derive(Accounts)]
pub struct FeedRead<'info> {
    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

    /// Feed of the data ID, matched against the state and the data ID in the handler
    pub feed: AccountLoader<'info, FeedData>,
}

/// Latest rounds of several data IDs (remaining accounts: their feeds, in order).
#[derive(Accounts)]
pub struct FeedsRead<'info> {
    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,
}

#[derive(Accounts)]
pub struct DescriptionRead<'info> {
    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

    pub feed_description: Option<Account<'info, FeedDescription>>,
}

#[derive(Accounts)]
pub struct PermissionRead<'info> {
    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

    /// Feed of the data ID, the default metadata applies when omitted
    pub feed: Option<AccountLoader<'info, FeedData>>,
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

use anchor_lang::prelude::*;
use anchor_lang::solana_program::sysvar;
use anchor_spl::associated_token::AssociatedToken;
use anchor_spl::token::{Mint, Token, TokenAccount, Transfer};

use crate::state::{Proposal, State};

use access_controller::AccessController;
use store::{Store, Transmissions};

// NOTE: (has_one = name) is equivalent to a custom access_control

#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(zero)]
    pub state: AccountLoader<'info, State>,
    pub feed: Account<'info, Transmissions>,
    #[account(mut)]
    pub payer: Signer<'info>,
    pub owner: Signer<'info>,

    pub token_mint: Account<'info, Mint>,
    #[account(
        init,
        payer = payer,
        associated_token::mint = token_mint,
        associated_token::authority = vault_authority,
    )]
    pub token_vault: Account<'info, TokenAccount>,
    /// CHECK: this is a PDA
    #[account(seeds = [b"vault", state.key().as_ref()], bump)]
    pub vault_authority: AccountInfo<'info>,

    pub requester_access_controller: AccountLoader<'info, AccessController>,
    pub billing_access_controller: AccountLoader<'info, AccessController>,

    #[account(address = sysvar::rent::ID)]
    pub rent: Sysvar<'info, Rent>,

    pub system_program: Program<'info, System>,
    pub token_program: Program<'info, Token>,
    pub associated_token_program: Program<'info, AssociatedToken>,
}

#[derive(Accounts)]
pub struct Close<'info> {
    #[account(mut, close = receiver)]
    pub state: AccountLoader<'info, State>,
    #[account(mut)]
    pub receiver: SystemAccount<'info>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct TransferOwnership<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct CreateProposal<'info> {
    #[account(zero)]
    pub proposal: AccountLoader<'info, Proposal>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct CloseProposal<'info> {
    #[account(mut, close = receiver)]
    pub proposal: AccountLoader<'info, Proposal>,
    #[account(mut)]
    pub receiver: SystemAccount<'info>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct ProposeConfig<'info> {
    #[account(mut)]
    pub proposal: AccountLoader<'info, Proposal>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct AcceptProposal<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    #[account(mut, close = receiver)]
    pub proposal: AccountLoader<'info, Proposal>,
    #[account(mut)]
    pub receiver: SystemAccount<'info>,
    pub authority: Signer<'info>,

    #[account(mut, address = state.load()?.config.token_vault)]
    pub token_vault: Account<'info, TokenAccount>,
    /// CHECK: This is a PDA
    #[account(seeds = [b"vault", state.key().as_ref()], bump = state.load()?.vault_nonce)]
    pub vault_authority: AccountInfo<'info>,

    pub token_program: Program<'info, Token>,
}

#[derive(Accounts)]
pub struct Transmit<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub transmitter: Signer<'info>,
    #[account(mut, address = state.load()?.feed)]
    pub feed: Account<'info, Transmissions>,

    pub store_program: Program<'info, Store>,
    /// CHECK: PDA from the aggregator state, validated by the store program
    pub store_authority: AccountInfo<'info>,
}

#[derive(Accounts)]
pub struct SetAccessController<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct RequestNewRound<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct SetBilling<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct WithdrawFunds<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,
    #[account(mut, address = state.load()?.config.token_vault)]
    pub token_vault: Account<'info, TokenAccount>,
    /// CHECK: This is a PDA
    #[account(seeds = [b"vault", state.key().as_ref()], bump = state.load()?.vault_nonce)]
    pub vault_authority: AccountInfo<'info>,
    #[account(mut)]
    pub recipient: Account<'info, TokenAccount>,

    pub token_program: Program<'info, Token>,
}

impl<'info> WithdrawFunds<'info> {
    pub fn transfer_ctx(&self) -> CpiContext<'_, '_, '_, 'info, Transfer<'info>> {
        CpiContext::new(
            self.token_program.to_account_info(),
            Transfer {
                from: self.token_vault.to_account_info(),
                to: self.recipient.to_account_info(),
                authority: self.vault_authority.to_account_info(),
            },
        )
    }
}

#[derive(Accounts)]
pub struct WithdrawPayment<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    #[account(mut, address = state.load()?.config.token_vault)]
    pub token_vault: Account<'info, TokenAccount>,
    /// CHECK: This is a PDA
    #[account(seeds = [b"vault", state.key().as_ref()], bump = state.load()?.vault_nonce)]
    pub vault_authority: AccountInfo<'info>,
    #[account(mut)]
    pub payee: Account<'info, TokenAccount>,

    pub token_program: Program<'info, Token>,
}

impl<'info> WithdrawPayment<'info> {
    pub fn transfer_ctx(&self) -> CpiContext<'_, '_, '_, 'info, Transfer<'info>> {
        CpiContext::new(
            self.token_program.to_account_info(),
            Transfer {
                from: self.token_vault.to_account_info(),
                to: self.payee.to_account_info(),
                authority: self.vault_authority.to_account_info(),
            },
        )
    }
}

/// Expects all the payees listed in matching order to state.oracles as remaining_accounts
#[derive(Accounts)]
pub struct PayOracles<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,

    #[account(mut, address = state.load()?.config.token_vault)]
    pub token_vault: Account<'info, TokenAccount>,
    /// CHECK: This is a PDA
    #[account(seeds = [b"vault", state.key().as_ref()], bump = state.load()?.vault_nonce)]
    pub vault_authority: AccountInfo<'info>,

    pub token_program: Program<'info, Token>,
}

#[derive(Accounts)]
pub struct TransferPayeeship<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    // Matches one of the oracle's transmitter keys
    pub transmitter: SystemAccount<'info>,
    pub payee: Account<'info, TokenAccount>,
    pub proposed_payee: Account<'info, TokenAccount>,
}

#[derive(Accounts)]
pub struct AcceptPayeeship<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    // Matches one of the oracle's transmitter keys
    pub transmitter: SystemAccount<'info>,
    pub proposed_payee: Account<'info, TokenAccount>,
}

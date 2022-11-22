use anchor_lang::prelude::*;
use anchor_spl::token::{Mint, Token, TokenAccount, Transfer};

use crate::state::{Proposal, State};
use crate::ErrorCode;

use access_controller::AccessController;
use store::{Store, Transmissions};

// NOTE: (has_one = name) is equivalent to a custom access_control

#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(zero)]
    pub state: AccountLoader<'info, State>,
    pub feed: Account<'info, Transmissions>,
    pub owner: Signer<'info>,

    pub token_mint: Account<'info, Mint>,
    #[account(
        associated_token::mint = token_mint,
        associated_token::authority = vault_authority,
    )]
    pub token_vault: Account<'info, TokenAccount>,
    /// CHECK: this is a PDA
    #[account(seeds = [b"vault", state.key().as_ref()], bump)]
    pub vault_authority: AccountInfo<'info>,

    pub requester_access_controller: AccountLoader<'info, AccessController>,
    pub billing_access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct Close<'info> {
    #[account(mut, close = receiver)]
    pub state: AccountLoader<'info, State>,
    // Receives the SOL deposit
    #[account(mut)]
    pub receiver: SystemAccount<'info>,
    // Receives the remaining LINK amount
    #[account(mut, token::mint = state.load()?.config.token_mint)]
    pub token_receiver: Account<'info, TokenAccount>,
    #[account(address = state.load()?.config.owner @ ErrorCode::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(mut, address = state.load()?.config.token_vault)]
    pub token_vault: Account<'info, TokenAccount>,
    /// CHECK: This is a PDA
    #[account(seeds = [b"vault", state.key().as_ref()], bump = state.load()?.vault_nonce)]
    pub vault_authority: AccountInfo<'info>,

    pub token_program: Program<'info, Token>,
}

#[derive(Accounts)]
pub struct TransferOwnership<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    #[account(address = state.load()?.config.owner @ ErrorCode::Unauthorized)]
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    #[account(address = state.load()?.config.proposed_owner @ ErrorCode::Unauthorized)]
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
    #[account(address = proposal.load()?.owner @ ErrorCode::Unauthorized)]
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct ProposeConfig<'info> {
    #[account(mut)]
    pub proposal: AccountLoader<'info, Proposal>,
    #[account(address = proposal.load()?.owner @ ErrorCode::Unauthorized)]
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
    // Receives LINK amount from oracles with closed token accounts
    #[account(mut, token::mint = state.load()?.config.token_mint)]
    pub token_receiver: Account<'info, TokenAccount>,
    #[account(address = state.load()?.config.owner @ ErrorCode::Unauthorized)]
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
    // TODO: stop using remaining_accounts once nodes update
    // /// CHECK: Validated by sysvar::instructions::load_current_index_checked/load_instruction_at_checked
    // pub instructions: AccountInfo<'info>,
}

#[derive(Accounts)]
pub struct SetAccessController<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    #[account(address = state.load()?.config.owner @ ErrorCode::Unauthorized)]
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

/// Used by set_billing and pay_oracles
/// Expects all the payees listed in matching order to state.oracles as remaining_accounts
#[derive(Accounts)]
pub struct SetBilling<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,

    // Receives LINK amount from oracles with closed token accounts
    #[account(mut, token::mint = state.load()?.config.token_mint)]
    pub token_receiver: Account<'info, TokenAccount>,

    #[account(mut, address = state.load()?.config.token_vault)]
    pub token_vault: Account<'info, TokenAccount>,
    /// CHECK: This is a PDA
    #[account(seeds = [b"vault", state.key().as_ref()], bump = state.load()?.vault_nonce)]
    pub vault_authority: AccountInfo<'info>,

    pub token_program: Program<'info, Token>,
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

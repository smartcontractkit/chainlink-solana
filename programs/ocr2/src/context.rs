use anchor_lang::prelude::*;
use anchor_lang::solana_program::sysvar;
use anchor_spl::associated_token::AssociatedToken;
use anchor_spl::token::{Mint, Token, TokenAccount, Transfer};

use crate::state::{State, Transmissions};

use access_controller::AccessController;
use deviation_flagging_validator::{self as validator, Validator};

// NOTE: (has_one = name) is equivalent to a custom access_control

#[derive(Accounts)]
#[instruction(nonce: u8)]
pub struct Initialize<'info> {
    #[account(zero)]
    pub state: AccountLoader<'info, State>,
    #[account(zero)]
    pub transmissions: AccountLoader<'info, Transmissions>,
    pub payer: AccountInfo<'info>,
    pub owner: Signer<'info>,

    pub token_mint: Account<'info, Mint>,
    #[account(
        init_if_needed,
        payer = payer,
        associated_token::mint = token_mint,
        associated_token::authority = vault_authority,
    )]
    pub token_vault: Account<'info, TokenAccount>,
    #[account(seeds = [b"vault", state.key().as_ref()], bump = nonce)]
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
pub struct SetConfig<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct Transmit<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub transmitter: Signer<'info>,
    #[account(mut, address = state.load()?.transmissions)]
    pub transmissions: AccountLoader<'info, Transmissions>,

    #[account(address = validator::ID)]
    pub validator_program: AccountInfo<'info>,
    #[account(mut, owner = validator::ID, address = state.load()?.config.validator)]
    pub validator: AccountLoader<'info, Validator>,
    pub validator_authority: AccountInfo<'info>,
    #[account(owner = access_controller::ID)]
    pub validator_access_controller: AccountInfo<'info>,
}

#[derive(Accounts)]
pub struct SetAccessController<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    #[account(owner = access_controller::ID)]
    pub access_controller: AccountInfo<'info>,
}

#[derive(Accounts)]
pub struct RequestNewRound<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    pub access_controller: AccountLoader<'info, AccessController>,
}

#[derive(Accounts)]
pub struct SetValidatorConfig<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    #[account(owner = validator::ID)]
    pub validator: AccountLoader<'info, Validator>,
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
    #[account(seeds = [b"vault", state.key().as_ref()], bump = state.load()?.nonce)]
    pub vault_authority: AccountInfo<'info>,
    #[account(mut)]
    pub recipient: Account<'info, TokenAccount>,

    pub token_program: Program<'info, Token>,
}

impl<'info> WithdrawFunds<'info> {
    pub fn into_transfer(&self) -> CpiContext<'_, '_, '_, 'info, Transfer<'info>> {
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
    #[account(seeds = [b"vault", state.key().as_ref()], bump = state.load()?.nonce)]
    pub vault_authority: AccountInfo<'info>,
    #[account(mut)]
    pub payee: Account<'info, TokenAccount>,

    pub token_program: Program<'info, Token>,
}

impl<'info> WithdrawPayment<'info> {
    pub fn into_transfer(&self) -> CpiContext<'_, '_, '_, 'info, Transfer<'info>> {
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
    #[account(seeds = [b"vault", state.key().as_ref()], bump = state.load()?.nonce)]
    pub vault_authority: AccountInfo<'info>,

    pub token_program: Program<'info, Token>,
}

#[derive(Accounts)]
pub struct SetPayees<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct TransferPayeeship<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    pub transmitter: AccountInfo<'info>,
    pub payee: Account<'info, TokenAccount>,
    pub proposed_payee: Account<'info, TokenAccount>,
}

#[derive(Accounts)]
pub struct AcceptPayeeship<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, State>,
    pub authority: Signer<'info>,
    pub transmitter: AccountInfo<'info>,
    pub proposed_payee: Account<'info, TokenAccount>,
}

#[derive(Accounts)]
pub struct Query<'info> {
    pub state: AccountLoader<'info, State>,
    pub transmissions: AccountLoader<'info, Transmissions>,
    // TODO: we could allow reusing query buffers if we also required an authority and marked the buffer with it.
    // That way someone else couldn't hijack the buffer and use it instead.
    #[account(zero)]
    pub buffer: AccountInfo<'info>,
}

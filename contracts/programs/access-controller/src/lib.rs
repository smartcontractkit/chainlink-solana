use anchor_lang::prelude::*;
use anchor_lang::solana_program::sysvar;
use static_assertions::const_assert;
use std::mem;

use arrayvec::arrayvec;

#[cfg(feature = "mainnet")]
declare_id!("My11111111111111111111111111111111111111112");
#[cfg(feature = "testnet")]
declare_id!("My11111111111111111111111111111111111111112");
#[cfg(feature = "devnet")]
declare_id!("My11111111111111111111111111111111111111112");
#[cfg(not(any(feature = "mainnet", feature = "testnet", feature = "devnet")))]
declare_id!("2F5NEkMnCRkmahEAcQfTQcZv1xtGgrWFfjENtTwHLuKg");

#[constant]
pub const MAX_ADDRS: usize = 64;

#[zero_copy]
pub struct AccessList {
    xs: [Pubkey; MAX_ADDRS],
    len: u64,
}
arrayvec!(AccessList, Pubkey, u64);
const_assert!(
    mem::size_of::<AccessList>() == mem::size_of::<u64>() + mem::size_of::<Pubkey>() * MAX_ADDRS
);

#[account(zero_copy)]
pub struct AccessController {
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub access_list: AccessList,
}

#[program]
pub mod access_controller {
    use super::*;
    pub fn initialize(ctx: Context<Initialize>) -> ProgramResult {
        let mut state = ctx.accounts.state.load_init()?;
        state.owner = ctx.accounts.owner.key();
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn transfer_ownership(
        ctx: Context<TransferOwnership>,
        proposed_owner: Pubkey,
    ) -> ProgramResult {
        require!(proposed_owner != Pubkey::default(), InvalidInput);
        let state = &mut *ctx.accounts.state.load_mut()?;
        state.proposed_owner = proposed_owner;
        Ok(())
    }

    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> ProgramResult {
        let state = &mut *ctx.accounts.state.load_mut()?;
        require!(
            ctx.accounts.authority.key == &state.proposed_owner,
            Unauthorized
        );
        state.owner = std::mem::take(&mut state.proposed_owner);
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.owner))]
    pub fn add_access(ctx: Context<AddAccess>) -> ProgramResult {
        let mut state = ctx.accounts.state.load_mut()?;
        // if the len reaches array len, we're at capacity
        require!(state.access_list.remaining_capacity() > 0, Full);

        let address = ctx.accounts.address.key();

        match state.access_list.binary_search(&address) {
            // already present
            Ok(_i) => (),
            // not found, insert
            Err(i) => state.access_list.insert(i, address),
        }
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.owner))]
    pub fn remove_access(ctx: Context<RemoveAccess>) -> ProgramResult {
        let mut state = ctx.accounts.state.load_mut()?;
        let address = ctx.accounts.address.key();

        let index = state.access_list.binary_search(&address);
        if let Ok(index) = index {
            state.access_list.remove(index);
            // we don't need to sort again since the list is still sorted
        }
        Ok(())
    }
}

/// Check if `address` is on the access control list.
pub fn has_access(loader: &AccountLoader<AccessController>, address: &Pubkey) -> Result<bool> {
    let state = loader.load()?;
    Ok(state.access_list.binary_search(address).is_ok())
}

fn owner(state_loader: &AccountLoader<AccessController>, signer: &AccountInfo) -> Result<()> {
    let config = state_loader.load()?;
    require!(signer.key.eq(&config.owner), Unauthorized);
    Ok(())
}

#[error]
pub enum ErrorCode {
    #[msg("Unauthorized")]
    Unauthorized = 0,

    #[msg("Invalid input")]
    InvalidInput = 1,

    #[msg("Access list is full")]
    Full = 2,
}

#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(zero)]
    pub state: AccountLoader<'info, AccessController>,
    pub payer: AccountInfo<'info>,
    pub owner: Signer<'info>,

    #[account(address = sysvar::rent::ID)]
    pub rent: Sysvar<'info, Rent>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct TransferOwnership<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, AccessController>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(mut)]
    pub state: AccountLoader<'info, AccessController>,
    pub authority: Signer<'info>,
}

#[derive(Accounts)]
pub struct AddAccess<'info> {
    #[account(mut, has_one = owner)]
    pub state: AccountLoader<'info, AccessController>,
    pub owner: Signer<'info>,
    pub address: UncheckedAccount<'info>,
}

#[derive(Accounts)]
pub struct RemoveAccess<'info> {
    #[account(mut, has_one = owner)]
    pub state: AccountLoader<'info, AccessController>,
    pub owner: Signer<'info>,
    pub address: UncheckedAccount<'info>,
}

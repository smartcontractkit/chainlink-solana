use anchor_lang::prelude::*;

use crate::{
    common::{ACCOUNT_VERSION, STORE_DECIMALS},
    context::*,
    error::{AuthError, ProxyError},
    event::*,
};

pub fn initialize(ctx: Context<Initialize>, feed_admins: Vec<Pubkey>) -> Result<()> {
    let state_key = ctx.accounts.state.key();
    let state = &mut ctx.accounts.state.load_init()?;
    state.version = ACCOUNT_VERSION;
    state.owner = ctx.accounts.owner.key();
    state.store_program = ctx.accounts.store_program.key();
    state.store_state = ctx.accounts.store_state.key();

    require!(
        feed_admins.len() <= state.feed_admins.capacity(),
        ProxyError::FeedAdminsFull
    );
    for pair in feed_admins.windows(2) {
        require!(pair[0] < pair[1], ProxyError::FeedAdminsNotSorted);
    }
    for admin in feed_admins.iter() {
        require_keys_neq!(*admin, Pubkey::default(), ProxyError::InvalidAddress);
    }
    state.feed_admins.extend(&feed_admins);

    emit!(ProxyInitialized {
        state: state_key,
        store_program: state.store_program,
        store_state: state.store_state,
    });
    Ok(())
}

pub fn transfer_ownership(ctx: Context<OwnerOnly>, proposed_owner: Pubkey) -> Result<()> {
    let state = &mut ctx.accounts.state.load_mut()?;
    require_keys_neq!(
        proposed_owner,
        state.owner,
        ProxyError::InvalidProposedOwner
    );
    state.proposed_owner = proposed_owner;
    emit!(OwnershipTransferRequested {
        state: ctx.accounts.state.key(),
        from: state.owner,
        to: proposed_owner,
    });
    Ok(())
}

pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
    let state = &mut ctx.accounts.state.load_mut()?;
    let previous = state.owner;
    state.owner = std::mem::take(&mut state.proposed_owner);
    emit!(OwnershipTransferred {
        state: ctx.accounts.state.key(),
        from: previous,
        to: state.owner,
    });
    Ok(())
}

pub fn set_feed_admin(ctx: Context<OwnerOnly>, feed_admin: Pubkey, is_admin: bool) -> Result<()> {
    require_keys_neq!(feed_admin, Pubkey::default(), ProxyError::InvalidAddress);
    let state = &mut ctx.accounts.state.load_mut()?;
    match (state.feed_admins.binary_search(&feed_admin), is_admin) {
        (Err(index), true) => {
            require!(
                state.feed_admins.remaining_capacity() > 0,
                ProxyError::FeedAdminsFull
            );
            state.feed_admins.insert(index, feed_admin);
        }
        (Ok(index), false) => {
            state.feed_admins.remove(index);
        }
        // already in the requested state
        _ => {}
    }
    emit!(FeedAdminSet {
        state: ctx.accounts.state.key(),
        feed_admin,
        is_admin,
    });
    Ok(())
}

/// Point the proxy at another store. Readers keep the proxy address, they pass the new store's
/// accounts.
pub fn set_store(ctx: Context<SetStore>) -> Result<()> {
    let state = &mut ctx.accounts.state.load_mut()?;
    let (previous_program, previous_state) = (state.store_program, state.store_state);
    state.store_program = ctx.accounts.store_program.key();
    state.store_state = ctx.accounts.store_state.key();
    emit!(StoreSet {
        state: ctx.accounts.state.key(),
        previous_program,
        previous_state,
        new_program: state.store_program,
        new_state: state.store_state,
    });
    Ok(())
}

/// Set the fewest decimals `read_scaled` serves a data ID at. The default is the stored
/// decimals, so a feed only serves other decimals once an admin lowers its minimum.
pub fn set_min_decimals(
    ctx: Context<SetMinDecimals>,
    data_id: [u8; 16],
    min_decimals: u8,
) -> Result<()> {
    let state_key = ctx.accounts.state.key();
    {
        let state = ctx.accounts.state.load()?;
        let admin = ctx.accounts.admin.key();
        require!(
            admin == state.owner || state.feed_admins.binary_search(&admin).is_ok(),
            AuthError::Unauthorized
        );
    }
    require!(min_decimals <= STORE_DECIMALS, ProxyError::InvalidDecimals);

    let config = &mut ctx.accounts.feed_config;
    config.version = ACCOUNT_VERSION;
    config.state = state_key;
    config.data_id = data_id;
    config.min_decimals = min_decimals;

    emit!(MinDecimalsSet {
        state: state_key,
        data_id,
        min_decimals,
    });
    Ok(())
}

/// The fewest decimals `read_scaled` serves a data ID at.
pub fn get_min_decimals(ctx: Context<GetMinDecimals>, _data_id: [u8; 16]) -> Result<u8> {
    Ok(ctx
        .accounts
        .feed_config
        .as_ref()
        .map_or(STORE_DECIMALS, |config| config.min_decimals))
}

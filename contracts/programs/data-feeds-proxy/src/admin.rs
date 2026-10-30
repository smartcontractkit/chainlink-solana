use anchor_lang::prelude::*;

use crate::{
    common::{data_id_seed, ACCOUNT_VERSION, STORE_DECIMALS},
    context::*,
    error::ProxyError,
    event::*,
    state::FeedConfig,
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

    emit_cpi!(ProxyInitialized {
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
    emit_cpi!(OwnershipTransferRequested {
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
    emit_cpi!(OwnershipTransferred {
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
    emit_cpi!(FeedAdminSet {
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
    emit_cpi!(StoreSet {
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
            ProxyError::Unauthorized
        );
    }
    require!(min_decimals <= STORE_DECIMALS, ProxyError::InvalidDecimals);

    let config = &mut ctx.accounts.feed_config;
    // version 0 is a config created by this call; an existing one is rewritten only at the layout
    // this program knows
    require!(
        config.version == 0 || config.version == ACCOUNT_VERSION,
        ProxyError::InvalidAccountVersion
    );
    config.version = ACCOUNT_VERSION;
    config.state = state_key;
    config.data_id = data_id_seed(&data_id);
    config.min_decimals = min_decimals;

    emit_cpi!(MinDecimalsSet {
        state: state_key,
        data_id,
        min_decimals,
    });
    Ok(())
}

/// The fewest decimals `read_scaled` serves a data ID at.
pub fn get_min_decimals(ctx: Context<GetMinDecimals>, _data_id: [u8; 16]) -> Result<u8> {
    min_decimals(ctx.accounts.feed_config.as_deref())
}

/// The minimum decimals of an optional feed config, the stored decimals without one.
pub fn min_decimals(config: Option<&FeedConfig>) -> Result<u8> {
    match config {
        None => Ok(STORE_DECIMALS),
        Some(config) => {
            require_eq!(
                config.version,
                ACCOUNT_VERSION,
                ProxyError::InvalidAccountVersion
            );
            Ok(config.min_decimals)
        }
    }
}

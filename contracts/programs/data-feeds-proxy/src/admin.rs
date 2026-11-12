use anchor_lang::prelude::*;

use crate::{
    common::{data_id_seed, ACCOUNT_VERSION, STORED_DECIMALS},
    context::*,
    error::ProxyError,
    event::*,
    state::Feed,
    utils::{load_feed_into, store_feed},
};

pub fn initialize(ctx: Context<Initialize>, writer: Pubkey) -> Result<()> {
    require_keys_neq!(writer, Pubkey::default(), ProxyError::InvalidAddress);
    let state_key = ctx.accounts.state.key();
    let state = &mut ctx.accounts.state.load_init()?;
    state.version = ACCOUNT_VERSION;
    state.owner = ctx.accounts.owner.key();
    state.writer = writer;
    emit_cpi!(ProxyInitialized {
        state: state_key,
        writer,
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

/// Replace the trusted store, as Stellar's `set_cache`. Every feed stays where it is: round IDs and
/// history continue under the new store.
pub fn set_store(ctx: Context<OwnerOnly>, writer: Pubkey) -> Result<()> {
    require_keys_neq!(writer, Pubkey::default(), ProxyError::InvalidAddress);
    let state = &mut ctx.accounts.state.load_mut()?;
    let previous_writer = std::mem::replace(&mut state.writer, writer);
    emit_cpi!(StoreSet {
        state: ctx.accounts.state.key(),
        previous_writer,
        new_writer: writer,
    });
    Ok(())
}

/// Set the fewest decimals reads of a feed may ask for. New feeds start at the stored decimals,
/// so a feed only serves fewer once the owner lowers its minimum, as on Stellar.
pub fn set_min_decimals(
    ctx: Context<SetMinDecimals>,
    data_id: [u8; 16],
    min_decimals: u8,
) -> Result<()> {
    require!(min_decimals <= STORED_DECIMALS, ProxyError::InvalidDecimals);
    let state_key = ctx.accounts.state.key();
    let info = ctx.accounts.feed.to_account_info();
    let mut feed: Box<Feed> = bytemuck::zeroed_box();
    load_feed_into(&info, &state_key, &data_id_seed(&data_id), &mut feed)?;
    feed.min_decimals = min_decimals;
    store_feed(&info, &feed)?;
    emit_cpi!(MinDecimalsSet {
        state: state_key,
        data_id,
        min_decimals,
    });
    Ok(())
}

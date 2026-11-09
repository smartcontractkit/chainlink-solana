use anchor_lang::{prelude::*, Discriminator};

use crate::{
    context::FeedAdminAccounts,
    data_id,
    error::StoreError,
    event::*,
    state::FeedData,
    utils::{encode_description, load_feed_into, store, verify_feed_admin, EventEmitter},
};

/// Set the descriptions of feeds, e.g. "ETH / USD", in their feed accounts (an empty string clears
/// one). Each data ID's feed keeps its own description, also across instances of a feed.
pub fn set_feed_descriptions<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
    descriptions: Vec<String>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require!(
        data_ids.len() == descriptions.len() && data_ids.len() == ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );

    let state_key = ctx.accounts.state.key();
    let events = EventEmitter::new(
        ctx.accounts.event_authority.to_account_info(),
        ctx.bumps.event_authority,
    );
    // one buffer for the whole batch: the bump allocator never frees
    let mut feed: Box<FeedData> = bytemuck::zeroed_box();
    for ((id, description), info) in data_ids
        .iter()
        .zip(descriptions.iter())
        .zip(ctx.remaining_accounts.iter())
    {
        require!(data_id::is_valid_data_id(id), StoreError::InvalidDataId);
        load_feed_into(info, &state_key, id, &mut feed)?;
        feed.description = encode_description(description)?;
        store(info, &*feed)?;
        emit_description_set(&events, &state_key, id, description)?;
    }
    Ok(())
}

/// FeedDescriptionSet, written from the borrowed description instead of a copy.
pub fn emit_description_set(
    events: &EventEmitter<'_>,
    state: &Pubkey,
    data_id: &[u8; 16],
    description: &str,
) -> Result<()> {
    events.emit_with(FeedDescriptionSet::DISCRIMINATOR, |data| {
        state.serialize(data)?;
        data_id.serialize(data)?;
        description.serialize(data)?;
        Ok(())
    })
}

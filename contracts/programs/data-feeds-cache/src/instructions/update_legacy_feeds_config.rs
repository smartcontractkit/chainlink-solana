use crate::{
    context::UpdateLegacyFeedsConfig, event::LegacyFeedsConfigUpdated,
    utils::set_legacy_feeds_config,
};
use anchor_lang::prelude::*;

pub fn handler(
    ctx: Context<UpdateLegacyFeedsConfig>,
    data_ids: Vec<[u8; 16]>,
    write_disabled: Vec<bool>,
) -> Result<()> {
    emit!(LegacyFeedsConfigUpdated {
        state: ctx.accounts.state.key(),
        config: ctx.accounts.legacy_feeds_config.key()
    });

    let write_disabled: Vec<u8> = write_disabled
        .iter()
        .copied() // &bool → bool
        .map(|f| f as u8)
        .collect();

    set_legacy_feeds_config(
        ctx.accounts.legacy_feeds_config.load_mut()?,
        ctx.accounts.legacy_store.key(),
        ctx.remaining_accounts,
        &data_ids,
        &write_disabled,
    )
}

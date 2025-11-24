use anchor_lang::prelude::*;

use crate::{
    context::InitLegacyFeedsConfig, event::LegacyFeedsConfigInitialized,
    utils::set_legacy_feeds_config,
};

pub fn handler(ctx: Context<InitLegacyFeedsConfig>, data_ids: Vec<[u8; 16]>) -> Result<()> {
    emit!(LegacyFeedsConfigInitialized {
        state: ctx.accounts.state.key(),
        config: ctx.accounts.legacy_feeds_config.key()
    });

    set_legacy_feeds_config(
        ctx.accounts.legacy_feeds_config.load_init()?,
        ctx.accounts.legacy_store.key(),
        ctx.remaining_accounts,
        &data_ids,
        &vec![0_u8; data_ids.len()],
    )
}

use anchor_lang::prelude::*;

use crate::{
    context::CloseDecimalReport, error::DataCacheError, event::DecimalReportClosed,
    utils::verify_feed_admin,
};

pub fn handler(ctx: Context<CloseDecimalReport>, data_id: [u8; 16]) -> Result<()> {
    let state = &ctx.accounts.state.load()?;
    verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;

    let feed_config = ctx.accounts.feed_config.load()?;

    // if feed config workflow list is empty, then all permission accounts
    // have also been closed as well
    require!(
        feed_config.workflow_metadata.is_empty(),
        DataCacheError::FeedConfigListNotEmpty
    );

    emit!(DecimalReportClosed {
        state: ctx.accounts.state.key(),
        data_id
    });

    Ok(())
}

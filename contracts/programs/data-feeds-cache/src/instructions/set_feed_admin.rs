use anchor_lang::prelude::*;

use crate::{context::SetFeedAdmin, error::DataCacheError, event::FeedAdminUpdated};

pub fn handler(ctx: Context<SetFeedAdmin>, admin: Pubkey, is_admin: bool) -> Result<()> {
    require_keys_neq!(admin, Pubkey::default(), DataCacheError::InvalidAddress);

    let mut state = ctx.accounts.state.load_mut()?;
    let mut changed = false;

    match (is_admin, state.feed_admins.binary_search(&admin)) {
        (false, Ok(i)) => {
            state.feed_admins.remove(i);
            changed = true;
        }
        (true, Err(i)) => {
            state.feed_admins.insert(i, admin);
            changed = true;
        }
        _ => {}
    }

    if changed {
        emit!(FeedAdminUpdated {
            state: ctx.accounts.state.key(),
            admin,
            is_admin
        });
    }

    Ok(())
}

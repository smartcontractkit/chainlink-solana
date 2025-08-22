use anchor_lang::prelude::*;

use crate::{
    context::Initialize,
    error::DataCacheError,
    event::{CacheInitialized, FeedAdminUpdated},
};

/// Creates a new data cache instance with a dedicated state account.
/// Sets the initial feed admins and state configuration.
pub fn handler(ctx: Context<Initialize>, feed_admins: Vec<Pubkey>) -> Result<()> {
    let state = &mut ctx.accounts.state.load_init()?;
    state.owner = ctx.accounts.owner.key();
    state.forwarder_id = ctx.accounts.forwarder_program.key();

    let mut prev_admin = Pubkey::default();
    for admin in feed_admins.iter() {
        require!(
            &prev_admin < admin,
            DataCacheError::AddressesMustStrictlyIncrease
        );
        state.feed_admins.push(*admin);
        emit!(FeedAdminUpdated {
            state: ctx.accounts.state.key(),
            admin: *admin,
            is_admin: true,
        });
        prev_admin = *admin;
    }

    let (_, bump) = Pubkey::find_program_address(
        &[b"legacy_writer", ctx.accounts.state.key().as_ref()],
        &crate::ID,
    );
    state.legacy_writer_bump = bump;

    emit!({
        CacheInitialized {
            state: ctx.accounts.state.key(),
            forwarder_id: state.forwarder_id,
            legacy_writer_bump: state.legacy_writer_bump,
        }
    });

    Ok(())
}

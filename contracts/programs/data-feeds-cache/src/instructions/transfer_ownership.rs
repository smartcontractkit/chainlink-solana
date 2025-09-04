use anchor_lang::prelude::*;

use crate::{context::TransferOwnership, error::DataCacheError, event::OwnershipTransfer};

pub fn handler(ctx: Context<TransferOwnership>, proposed_owner: Pubkey) -> Result<()> {
    let state = &mut ctx.accounts.state.load_mut()?;
    require!(
        proposed_owner != Pubkey::default()
            && proposed_owner != state.owner
            && proposed_owner != state.proposed_owner,
        DataCacheError::InvalidProposedOwner
    );

    state.proposed_owner = proposed_owner;
    emit!(OwnershipTransfer {
        state: ctx.accounts.state.key(),
        current_owner: state.owner,
        proposed_owner
    });

    Ok(())
}

use anchor_lang::prelude::*;

use crate::{context::AcceptOwnership, event::OwnershipAcceptance};

pub fn handler(ctx: Context<AcceptOwnership>) -> Result<()> {
    let state = &mut ctx.accounts.state.load_mut()?;

    emit!(OwnershipAcceptance {
        state: ctx.accounts.state.key(),
        previous_owner: state.owner,
        new_owner: state.proposed_owner
    });

    state.owner = std::mem::take(&mut state.proposed_owner);

    Ok(())
}

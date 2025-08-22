use crate::{context::UpdateForwarder, event::ForwarderUpdated};
use anchor_lang::prelude::*;

pub fn handler(ctx: Context<UpdateForwarder>) -> Result<()> {
    let mut state = ctx.accounts.state.load_mut()?;

    emit!({
        ForwarderUpdated {
            previous_forwarder: state.forwarder_id,
            new_forwarder: ctx.accounts.forwarder_program.key(),
        }
    });

    state.forwarder_id = ctx.accounts.forwarder_program.key();

    Ok(())
}

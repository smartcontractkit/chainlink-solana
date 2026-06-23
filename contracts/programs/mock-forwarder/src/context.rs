use crate::common::ANCHOR_DISCRIMINATOR;
use crate::error::AuthError;
use crate::state::{ExecutionState, ForwarderState};
use crate::utils::{extract_raw_report, extract_transmission_id, report_size_ok};
use crate::ForwarderError;
use anchor_lang::prelude::*;

#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(
        init,
        payer = owner,
        space = ANCHOR_DISCRIMINATOR + ForwarderState::INIT_SPACE
    )]
    pub state: Account<'info, ForwarderState>,
    #[account(mut)]
    pub owner: Signer<'info>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct TransferOwnership<'info> {
    #[account(mut)]
    pub state: Account<'info, ForwarderState>,

    #[account(address = state.owner @ AuthError::Unauthorized)]
    pub current_owner: Signer<'info>,
}

#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(mut)]
    pub state: Account<'info, ForwarderState>,

    #[account(address = state.proposed_owner @ AuthError::Unauthorized)]
    pub proposed_owner: Signer<'info>,
}

#[derive(Accounts)]
#[instruction(data: Vec<u8>)]
pub struct Report<'info> {
    pub state: Account<'info, ForwarderState>,

    #[account(mut)]
    pub transmitter: Signer<'info>,

    /// CHECK: This is a PDA
    #[account(seeds = [b"forwarder", state.key().as_ref(), receiver_program.key().as_ref()], bump)]
    pub forwarder_authority: UncheckedAccount<'info>,

    #[account(
        init_if_needed,
        constraint = report_size_ok(&data) @ ForwarderError::InvalidReport,
        payer = transmitter,
        space = ANCHOR_DISCRIMINATOR + ExecutionState::INIT_SPACE,
        seeds = [
            b"execution_state",
            state.key().as_ref(),
            &extract_transmission_id(extract_raw_report(&data), receiver_program.key)
        ],
        bump
    )]
    pub execution_state: Account<'info, ExecutionState>,

    #[account(executable)]
    /// CHECK: Any executable program — Anchor `executable` constraint is enough.
    pub receiver_program: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,
    // remaining accounts passed through to receiver
}

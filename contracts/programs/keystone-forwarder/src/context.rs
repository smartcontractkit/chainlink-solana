use crate::common::ANCHOR_DISCRIMINATOR;
use crate::error::AuthError;
use crate::state::{ExecutionState, ForwarderState, OraclesConfig};
use crate::utils::{
    extract_config_id, extract_raw_report, extract_transmission_id, get_config_id, report_size_ok,
};
use crate::ForwarderError;
use anchor_lang::prelude::*;

#[derive(Accounts)]
pub struct Initialize<'info> {
    // the account is not a PDA but it is initialized by the program
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
#[instruction(don_id: u32, config_version: u32)]
pub struct InitOraclesConfig<'info> {
    pub state: Account<'info, ForwarderState>,

    #[account(
        init,
        payer = owner,
        seeds = [b"config", state.key().as_ref(), &get_config_id(don_id, config_version).to_be_bytes()],
        bump,
        space = ANCHOR_DISCRIMINATOR + OraclesConfig::INIT_SPACE,
    )]
    pub oracles_config: AccountLoader<'info, OraclesConfig>,

    #[account(mut, address = state.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>, // must be the same owner as the one in the state account

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(don_id: u32, config_version: u32)]
pub struct UpdateOraclesConfig<'info> {
    pub state: Account<'info, ForwarderState>,

    #[account(
        mut,
        seeds = [b"config", state.key().as_ref(), &get_config_id(don_id, config_version).to_be_bytes()],
        bump
    )]
    pub oracles_config: AccountLoader<'info, OraclesConfig>,

    #[account(mut, address = state.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,
}

#[derive(Accounts)]
#[instruction(don_id: u32, config_version: u32)]
pub struct CloseOraclesConfig<'info> {
    pub state: Account<'info, ForwarderState>,

    #[account(
        mut,
        seeds = [b"config", state.key().as_ref(), &get_config_id(don_id, config_version).to_be_bytes()],
        bump,
        close = owner
    )]
    pub oracles_config: AccountLoader<'info, OraclesConfig>,

    #[account(mut, address = state.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>, // must be the same owner as the one in the state account
}

#[derive(Accounts)]
#[instruction(data: Vec<u8>)]
pub struct Report<'info> {
    pub state: Account<'info, ForwarderState>,

    #[account(
        mut,
        constraint = report_size_ok(&data) @ ForwarderError::InvalidReport,
        seeds = [b"config", state.key().as_ref(), &extract_config_id(extract_raw_report(&data))],
        bump
    )]
    pub oracles_config: AccountLoader<'info, OraclesConfig>,

    #[account(mut)]
    pub transmitter: Signer<'info>,

    /// CHECK: This is a PDA
    #[account(seeds = [b"forwarder", state.key().as_ref(), receiver_program.key().as_ref()], bump)]
    pub forwarder_authority: UncheckedAccount<'info>,

    // it is dependent on the state.key(), a predetermined bump, workflow execution id, config_id, report_id
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
    /// CHECK: We don't use Program<> here since it can be any program, "executable" is enough
    pub receiver_program: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,
    // remaining accounts passed to receiver
}

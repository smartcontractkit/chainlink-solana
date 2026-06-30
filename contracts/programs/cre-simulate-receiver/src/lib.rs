//! Minimal stateless receiver for cre-cli Solana forwarder simulation.
//!
//! The forwarder CPIs into receivers with the account order:
//! `forwarder_state`, `forwarder_authority`, then any workflow-supplied
//! remaining accounts. This receiver needs no remaining accounts and only checks
//! that the signer PDA matches the calling forwarder state/program.

#![allow(deprecated)]

use anchor_lang::prelude::*;

declare_id!("EJwYCKxQ826qEUoFeGVyQLmBDGw6DY1hdg1QP5QcwhXa");

#[program]
pub mod cre_simulate_receiver {
    use super::*;

    pub fn on_report(ctx: Context<OnReport>, metadata: Vec<u8>, report: Vec<u8>) -> Result<()> {
        let forwarder_state = ctx.accounts.state.key();
        let forwarder_program = *ctx.accounts.state.to_account_info().owner;
        let (expected_authority, _bump) = Pubkey::find_program_address(
            &[b"forwarder", forwarder_state.as_ref(), crate::ID.as_ref()],
            &forwarder_program,
        );

        require_keys_eq!(
            expected_authority,
            ctx.accounts.forwarder_authority.key(),
            ReceiverError::InvalidForwarderAuthority
        );

        msg!(
            "cre_simulate_receiver on_report metadata_len={} report_len={}",
            metadata.len(),
            report.len()
        );

        emit!(ReceivedReport {
            forwarder: forwarder_program,
            forwarder_state,
            forwarder_authority: ctx.accounts.forwarder_authority.key(),
            metadata_len: metadata.len() as u32,
            report_len: report.len() as u32,
        });

        Ok(())
    }
}

#[derive(Accounts)]
pub struct OnReport<'info> {
    /// CHECK: Forwarder state account. Its owner is the deployed forwarder program.
    pub state: UncheckedAccount<'info>,

    /// PDA signer supplied by the forwarder CPI.
    pub forwarder_authority: Signer<'info>,
}

#[event]
pub struct ReceivedReport {
    pub forwarder: Pubkey,
    pub forwarder_state: Pubkey,
    pub forwarder_authority: Pubkey,
    pub metadata_len: u32,
    pub report_len: u32,
}

#[error_code]
pub enum ReceiverError {
    #[msg("forwarder_authority is not the PDA for this state, receiver, and forwarder program")]
    InvalidForwarderAuthority,
}

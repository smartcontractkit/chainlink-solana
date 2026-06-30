//! Supporting types for `mock_forwarder`: constants, errors, events, state
//! accounts, instruction contexts, and payload helpers.

use anchor_lang::prelude::{borsh::BorshDeserialize, *};
use anchor_lang::solana_program::hash;

// ---------- constants ----------

pub const ANCHOR_DISCRIMINATOR: usize = 8;
pub const REPORT_CONTEXT_LEN: usize = 96;
pub const SIGNATURE_LEN: usize = 65;
pub const FORWARDER_METADATA_LENGTH: usize = 45;
pub const METADATA_LENGTH: usize = 109;

// Anchor `on_report` discriminator — sha256("global:on_report")[..8]. Must match
// keystone-forwarder so receivers don't need to distinguish callers by discriminator.
pub const ON_REPORT_DISCRIMINATOR: [u8; 8] = [214, 173, 18, 221, 173, 148, 151, 208];

// ---------- errors ----------

#[error_code]
pub enum ForwarderError {
    #[msg("Report does not meet minimum length")]
    InvalidReport,

    #[msg("Execution already succeded")]
    ExecutionAlreadySucceded,

    #[msg("Forwarder Report Expected")]
    ForwarderReportExpected,

    #[msg("Invalid Account Hash")]
    InvalidAccountHash,
}

// ---------- events ----------

#[event]
pub struct ForwarderInitialize {
    pub state: Pubkey,
}

#[event]
pub struct ReportInProgress {
    pub state: Pubkey,
    pub transmission_id: [u8; 32],
}

#[event]
pub struct ReportProcessed {
    pub state: Pubkey,
    pub receiver: Pubkey,
    pub transmission_id: [u8; 32],
}

// ---------- state ----------

/// Empty marker account. Only its address matters — the report instruction uses
/// `state.key()` as a seed for the `forwarder_authority` PDA.
#[account]
#[derive(Default, InitSpace)]
pub struct ForwarderState {}

/// Per-transmission account. Persists so repeat submissions for the same
/// `transmission_id` abort — mirrors prod replay-protection semantics.
#[account]
#[derive(Default, InitSpace)]
pub struct ExecutionState {
    pub transmitter: Pubkey,
    pub transmission_id: [u8; 32],
    pub success: bool,
}

// ---------- instruction contexts ----------

#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(
        init,
        payer = payer,
        space = ANCHOR_DISCRIMINATOR + ForwarderState::INIT_SPACE
    )]
    pub state: Account<'info, ForwarderState>,
    #[account(mut)]
    pub payer: Signer<'info>,
    pub system_program: Program<'info, System>,
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

// ---------- payload helpers ----------

pub fn report_size_ok(data: &[u8]) -> bool {
    if !data.is_empty() {
        let num_signatures = data[0] as usize;
        data.len() > 1 + num_signatures * SIGNATURE_LEN + METADATA_LENGTH + REPORT_CONTEXT_LEN
    } else {
        false
    }
}

// data = len_signatures (1) | signatures (N*65) | raw_report (M) | report_context (96)
pub fn extract_raw_report(data: &[u8]) -> &[u8] {
    let num_signatures = data[0] as usize;
    let data = &data[1..];
    let data = &data[num_signatures * SIGNATURE_LEN..];
    &data[..data.len() - REPORT_CONTEXT_LEN]
}

// Layout matches keystone-forwarder; see `keystone-forwarder/src/utils.rs`.
pub fn extract_transmission_id(raw_report: &[u8], receiver: &Pubkey) -> [u8; 32] {
    let workflow_execution_id = &raw_report[1..33];
    let report_id = &raw_report[107..109];

    hash::hash(&[&receiver.to_bytes(), workflow_execution_id, report_id].concat()).to_bytes()
}

#[derive(BorshDeserialize)]
pub struct ForwarderReport {
    pub account_hash: [u8; 32],
    pub payload: Vec<u8>,
}

//! Dev/test mock of `keystone-forwarder` used by cre-cli `cre workflow simulate`.
//!
//! Matches keystone-forwarder's `report` CPI path and event shape but
//! **skips OCR signature verification and the `oracles_config` account**.
//! Solana analog of `chainlink-aptos/contracts/platform_mock/sources/mock_forwarder.move`,
//! except — because Solana receivers don't require pre-registration — the mock
//! does a real `invoke_signed` CPI into the workflow-supplied receiver instead
//! of routing through a mock-storage account.

// anchor-lang 0.31 #[program] macro emits AccountInfo::realloc; allow until we upgrade
#![allow(deprecated)]

use anchor_lang::prelude::*;

mod internal;
pub use internal::*;

declare_id!("7kuEAA3mSC1Tz8gQjnvH7bKFda9xSPRRin9SZbH49cNK");

/// Mock forwarder: relays chainlink reports to a receiver without verifying
/// any signatures or oracle config. Strictly for local simulation.
#[program]
pub mod mock_forwarder {
    use std::io::Cursor;

    use anchor_lang::solana_program::{
        hash, instruction::Instruction, program::invoke_signed,
    };

    use super::*;

    /// Creates a new (empty) `ForwarderState` account. The account only exists
    /// to anchor the `forwarder_authority` PDA — its contents are unused.
    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        emit!(ForwarderInitialize {
            state: ctx.accounts.state.key(),
        });
        Ok(())
    }

    /// Relays a report to the receiver program.
    ///
    /// Same payload layout as keystone-forwarder
    /// (`data = len_sigs (1) | signatures (N*65) | raw_report (M) | report_context (96)`),
    /// but signatures are **not** verified. Account-hash check + replay protection
    /// + receiver CPI all mirror prod so workflows assemble payloads the same way
    /// in simulation and in production.
    pub fn report<'info>(
        ctx: Context<'_, '_, '_, 'info, Report<'info>>,
        data: Vec<u8>,
    ) -> Result<()> {
        let raw_report = extract_raw_report(&data);

        let transmission_id =
            extract_transmission_id(raw_report, ctx.accounts.receiver_program.key);

        let execution_state = &mut ctx.accounts.execution_state;
        require!(
            !execution_state.success,
            ForwarderError::ExecutionAlreadySucceded
        );

        let forwarder_authority_pda = ctx.accounts.forwarder_authority.clone();

        // [forwarder_state, forwarder_authority, ...remaining_accounts] — identical
        // ordering to keystone-forwarder so receivers don't need to know which forwarder called them.
        let metas: Vec<AccountMeta> = std::iter::once(AccountMeta {
            pubkey: ctx.accounts.state.key(),
            is_signer: false,
            is_writable: false,
        })
        .chain(std::iter::once(AccountMeta {
            pubkey: *forwarder_authority_pda.key,
            is_signer: true,
            is_writable: false,
        }))
        .chain(ctx.remaining_accounts.iter().map(|acc| AccountMeta {
            pubkey: *acc.key,
            is_signer: false,
            is_writable: acc.is_writable,
        }))
        .collect();

        let account_infos: Vec<AccountInfo> = std::iter::once(ctx.accounts.state.to_account_info())
            .chain(std::iter::once(
                ctx.accounts.forwarder_authority.to_account_info(),
            ))
            .chain(ctx.remaining_accounts.iter().cloned())
            .collect();

        let forwarder_report = ForwarderReport::try_from_slice(&raw_report[METADATA_LENGTH..])
            .map_err(|_| ForwarderError::ForwarderReportExpected)?;

        let account_key_bytes = account_infos.iter().fold(
            Vec::with_capacity(account_infos.len() * 32),
            |mut buf, x| {
                buf.extend_from_slice(&x.key().to_bytes());
                buf
            },
        );
        let computed_account_hash = hash::hash(&account_key_bytes);

        require_eq!(
            computed_account_hash,
            anchor_lang::solana_program::hash::Hash::from(forwarder_report.account_hash),
            ForwarderError::InvalidAccountHash
        );

        let mut payload: Vec<u8> = Vec::with_capacity(
            ON_REPORT_DISCRIMINATOR.len()
                + 4
                + (METADATA_LENGTH - FORWARDER_METADATA_LENGTH)
                + 4
                + forwarder_report.payload.len(),
        );

        payload.extend_from_slice(&ON_REPORT_DISCRIMINATOR);
        let mut cursor = Cursor::new(&mut payload);
        cursor.set_position(ON_REPORT_DISCRIMINATOR.len() as u64);

        let metadata = raw_report[FORWARDER_METADATA_LENGTH..METADATA_LENGTH].to_vec();
        let report = forwarder_report.payload;
        metadata.serialize(&mut cursor)?;
        report.serialize(&mut cursor)?;

        let ix = Instruction::new_with_bytes(ctx.accounts.receiver_program.key(), &payload, metas);

        let forwarder_state = ctx.accounts.state.key();
        let receiver_program = ctx.accounts.receiver_program.key();

        let authority_bump = ctx.bumps.forwarder_authority;
        let signers_seeds = &[
            b"forwarder",
            forwarder_state.as_ref(),
            receiver_program.as_ref(),
            &[authority_bump],
        ];

        emit!(ReportInProgress {
            state: ctx.accounts.state.key(),
            transmission_id,
        });

        invoke_signed(&ix, &account_infos, &[signers_seeds])?;

        execution_state.transmitter = ctx.accounts.transmitter.key();
        execution_state.transmission_id = transmission_id;
        execution_state.success = true;

        emit!(ReportProcessed {
            state: ctx.accounts.state.key(),
            receiver: ctx.accounts.receiver_program.key(),
            transmission_id,
        });

        Ok(())
    }
}

use anchor_lang::prelude::*;
use anchor_lang::solana_program::{hash, hash::Hash, keccak, secp256k1_recover::*};

use common::{
    FORWARDER_METADATA_LENGTH, MAX_ORACLES, METADATA_LENGTH, ON_REPORT_DISCRIMINATOR,
    REPORT_CONTEXT_LEN, SIGNATURE_LEN, STATE_VERSION,
};

use events::{
    ConfigSet, ForwarderInitialize, OwnershipAcceptance, OwnershipTransfer, ReportInProgress,
    ReportProcessed,
};

use context::*;
pub use error::*;
pub use state::{ExecutionState, ForwarderState, OraclesConfig};
use utils::{extract_transmission_id, get_config_id};

mod common;
mod context;
mod error;
mod events;
mod state;
mod utils;

declare_id!("whV7Q5pi17hPPyaPksToDw1nMx6Lh8qmNWKFaLRQ4wz");

/// Forwarder authenticates chainlink reports and relays them to designated receiver programs.
#[program]
pub mod keystone_forwarder {
    use std::io::Cursor;

    use anchor_lang::solana_program::{instruction::Instruction, program::invoke_signed};

    use crate::utils::{report_size_ok, ForwarderReport};

    use super::*;

    // Receiver contract will implement this in Anchor (or equivalent in pure Rust)
    // pub fn on_report(ctx: Context<OnReport>, metadata: Vec<u8>, report: Vec<u8>) -> Result<()>
    // with the following declared accounts
    //
    // #[derive(Accounts)]
    // pub struct OnReport<'info> {
    //     // Note: a receiver program's on_report function does not necessarily need to directly authenticate the forwarder state.
    //     // Instead, it indirectly verifies the correct state by enforcing that the forwarder_authority is authorized.
    //     // WARNING: the FORWARDER_ID deployed in an environment may be different
    //     // than the one in source control (the chainlink keystone_forwarder crate). You need to view the official chainlink docs to determine
    //     // the correct FORWARDER_ID to use
    //     pub forwarder_state: Account<'info, ForwarderState>,

    //     #[account(seeds = [b"forwarder", forwarder_state.key().as_ref(), crate::ID.as_ref()], bump, seeds::program = cache_state.load()?.forwarder_id)]
    //     pub forwarder_authority: Signer<'info>,

    //     // remaining accounts
    // }

    /// Initializes a new Forwarder instance and stores data in its state account
    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        let state = &mut ctx.accounts.state;
        state.version = STATE_VERSION;
        state.owner = ctx.accounts.owner.key();

        emit!(ForwarderInitialize {
            state: state.key(),
            owner: ctx.accounts.owner.key(),
        });

        Ok(())
    }

    /// Step 1 of 2-step ownership process: propose a new owner
    pub fn transfer_ownership(
        ctx: Context<TransferOwnership>,
        proposed_owner: Pubkey,
    ) -> Result<()> {
        let state = &mut ctx.accounts.state;
        require!(
            proposed_owner != Pubkey::default()
                && proposed_owner != state.owner
                && proposed_owner != state.proposed_owner,
            ForwarderError::InvalidProposedOwner
        );

        state.proposed_owner = proposed_owner;

        emit!(OwnershipTransfer {
            state: state.key(),
            current_owner: state.owner,
            proposed_owner: state.proposed_owner
        });

        Ok(())
    }

    /// Step 2 of 2-step ownership process: accept ownership
    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        let state = &mut ctx.accounts.state;
        let state_previous_owner = state.owner;
        state.owner = state.proposed_owner;
        state.proposed_owner = Pubkey::default();

        emit!(OwnershipAcceptance {
            state: state.key(),
            previous_owner: state_previous_owner,
            new_owner: state.owner
        });

        Ok(())
    }

    /// Initialize oracles config which describes the set of oracles which
    /// are expected to sign a verified forwarder report. Many oracle config accounts
    /// may exist for a forwarder because more than one DON may be allowed to sign
    /// reports for a forwarder.
    pub fn init_oracles_config(
        ctx: Context<InitOraclesConfig>,
        don_id: u32,
        config_version: u32,
        f: u8,
        signer_addresses: Vec<[u8; 20]>,
    ) -> Result<()> {
        let config = &mut ctx.accounts.oracles_config.load_init()?;

        emit!(ConfigSet {
            state: ctx.accounts.state.key(),
            oracles_config: ctx.accounts.oracles_config.key(),
            don_id,
            config_version,
            f,
            signers: signer_addresses.clone(),
        });

        set_oracles_config(config, don_id, config_version, f, signer_addresses.clone())
    }

    /// Updates oracles config under circumstances where the designated
    /// signers or configuration parameters change
    pub fn update_oracles_config(
        ctx: Context<UpdateOraclesConfig>,
        don_id: u32,
        config_version: u32,
        f: u8,
        signer_addresses: Vec<[u8; 20]>,
    ) -> Result<()> {
        let config = &mut ctx.accounts.oracles_config.load_mut()?;

        emit!(ConfigSet {
            state: ctx.accounts.state.key(),
            oracles_config: ctx.accounts.oracles_config.key(),
            don_id,
            config_version,
            f,
            signers: signer_addresses.clone(),
        });

        set_oracles_config(config, don_id, config_version, f, signer_addresses)
    }

    /// Closes oracle config account
    pub fn close_oracles_config(
        _ctx: Context<CloseOraclesConfig>,
        _don_id: u32,
        _config_version: u32,
    ) -> Result<()> {
        Ok(())
    }

    /// The report instruction verifies the report by checking it's ECDSA signatures and ensuring that f + 1 nodes have signed the report.
    /// After verification it will create a PDA to store the execution state if it does not exist.
    /// The ctx.remaining_accounts accounts are passed on to the receiver, alongside the forwarder state account
    /// and forwarder authority signer.
    /// Available space for receiver payload is ~ 297 bytes. However, many factors will affect
    /// this number including adding more accounts in the ctx.remaining_accounts and/or using address
    /// lookup tables. Please refer to ../../docs/forwarder/README.md#L140
    // data = len_signatures (1) | signatures (N*65) | raw_report (M) | report_context (96)
    pub fn report<'info>(
        ctx: Context<'_, '_, '_, 'info, Report<'info>>,
        data: Vec<u8>,
    ) -> Result<()> {
        require!(report_size_ok(&data), ForwarderError::InvalidReport);

        let num_signatures = data[0] as usize;

        // get config
        let oracles_config = ctx.accounts.oracles_config.load()?;
        require_gte!(
            num_signatures,
            (oracles_config.f + 1) as usize,
            ForwarderError::InvalidSignatureCount
        );

        // extract signatures
        let data = &data[1..];
        let total_signature_len: usize = SIGNATURE_LEN * num_signatures;

        let signatures: &[u8] = &data[..total_signature_len];
        // raw_report | report context
        let data = &data[total_signature_len..];

        // Build the preimage the same way the OCR keyring does:
        // SHA256( [u8(len(raw_report))] || raw_report || ctx)
        let mut preimage = vec![0u8; 1 + data.len()];

        let raw_report_len = data.len() - REPORT_CONTEXT_LEN;
        // OCR keyring also does not error on overflow
        let raw_report_len_u8: u8 = raw_report_len as u8;

        preimage[0] = raw_report_len_u8;
        preimage[1..].copy_from_slice(data);

        let hashed_report = hash::hash(&preimage).to_bytes();

        verify_signatures(&hashed_report, signatures, &oracles_config, num_signatures)?;

        // slice raw_report from the report context
        let raw_report = &data[..raw_report_len];

        let transmission_id =
            extract_transmission_id(raw_report, ctx.accounts.receiver_program.key);

        let execution_state = &mut ctx.accounts.execution_state;

        require!(
            !execution_state.success,
            ForwarderError::ExecutionAlreadySucceded
        );

        // forward to the receiver program
        let forwarder_authority_pda = ctx.accounts.forwarder_authority.clone();

        // Create AccountMeta list, with forwarder state and forwarder authority PDA
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
            // assume that we (probably) won't support 3rd party accounts as signers
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

        // report should always be of type ForwarderReport because the account hash needs to be verified
        let forwarder_report = ForwarderReport::try_from_slice(&raw_report[METADATA_LENGTH..])
            .map_err(|_| ForwarderError::ForwarderReportExpected)?;
        // verify the hash of all accounts in the OnReport context (forwarder_state, forwarder_authority, ...remaining accounts)
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
            Hash::from(forwarder_report.account_hash),
            ForwarderError::InvalidAccountHash
        );

        let mut payload: Vec<u8> = Vec::with_capacity(
            ON_REPORT_DISCRIMINATOR.len()
                + 4
                + (METADATA_LENGTH - FORWARDER_METADATA_LENGTH)
                + 4
                + forwarder_report.payload.len(),
        );

        // payload begins with the Anchor discriminator
        payload.extend_from_slice(&ON_REPORT_DISCRIMINATOR);
        let mut cursor = Cursor::new(&mut payload);
        cursor.set_position(ON_REPORT_DISCRIMINATOR.len() as u64);

        // borsh serialization of metadata vector and report vector
        // metadata is just workflow_cid, workflow_name, workflow_owner, and report_id (see format above)
        let metadata = raw_report[FORWARDER_METADATA_LENGTH..METADATA_LENGTH].to_vec();
        let report = forwarder_report.payload;
        // Borsh serialize each part separately as Vec<u8>
        metadata.serialize(&mut cursor)?;
        report.serialize(&mut cursor)?;

        let ix = Instruction::new_with_bytes(ctx.accounts.receiver_program.key(), &payload, metas);

        // used to derive the forwarder authority PDA
        let forwarder_state = ctx.accounts.state.key();
        let receiver_program = ctx.accounts.receiver_program.key();

        // calculate the bump on-the-fly
        let (_, authority_bump) = Pubkey::find_program_address(
            &[
                b"forwarder",
                forwarder_state.as_ref(),
                receiver_program.as_ref(),
            ],
            &crate::ID,
        );

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

        // update execution state

        execution_state.transmitter = ctx.accounts.transmitter.key();
        execution_state.transmission_id = transmission_id;
        execution_state.success = true;

        emit!(ReportProcessed {
            state: ctx.accounts.state.key(),
            receiver: ctx.accounts.receiver_program.key(),
            transmission_id,
            result: true,
        });

        Ok(())
    }
}

#[inline(never)]
fn verify_signatures(
    hashed_report: &[u8; 32],
    signatures: &[u8],
    oracles_config: &OraclesConfig,
    num_signers: usize,
) -> Result<()> {
    let mut uniques: u32 = 0;
    for sig in signatures.chunks(SIGNATURE_LEN) {
        // sig is [R || S || V] format where V is 0 or 1
        let v = sig[64];

        let signer = secp256k1_recover(hashed_report, v, &sig[..64])
            .map_err(|_| ForwarderError::InvalidSignature)?;

        let signer_eth_address: [u8; 20] = keccak::hash(&signer.0).to_bytes()[12..32]
            .try_into()
            .map_err(|_| ForwarderError::UnauthorizedSigner)?;

        let index = oracles_config
            .signer_addresses
            .as_slice()
            .binary_search_by(|addr| addr.cmp(&signer_eth_address))
            .map_err(|_| ForwarderError::UnauthorizedSigner)?;

        uniques |= 1 << index;
    }

    require_eq!(
        uniques.count_ones() as usize,
        num_signers,
        ForwarderError::DuplicateSignatures
    );

    Ok(())
}

fn set_oracles_config(
    oracles_config: &mut OraclesConfig,
    don_id: u32,
    config_version: u32,
    f: u8,
    signer_addresses: Vec<[u8; 20]>,
) -> Result<()> {
    require_gt!(f, 0, ForwarderError::FaultToleranceMustBePositive);
    require_gte!(
        MAX_ORACLES,
        signer_addresses.len(),
        ForwarderError::ExcessSigners
    );
    require_gt!(
        signer_addresses.len(),
        (3 * f) as usize,
        ForwarderError::InsufficientSigners
    );

    let mut prev_signer = [0u8; 20];

    for &curr_signer in signer_addresses.iter() {
        // will also fail if there is a duplicate signer
        require!(
            curr_signer > prev_signer,
            ForwarderError::SignersNotSortedInIncreasingOrder
        );

        prev_signer = curr_signer;
    }

    oracles_config.config_id = get_config_id(don_id, config_version);
    oracles_config.f = f;
    oracles_config.signer_addresses.clear();

    oracles_config.signer_addresses.extend(&signer_addresses);

    Ok(())
}

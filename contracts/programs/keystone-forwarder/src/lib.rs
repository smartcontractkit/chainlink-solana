use anchor_lang::prelude::*;
use anchor_lang::solana_program::{hash, keccak, secp256k1_recover::*};

use common::{
    FORWARDER_METADATA_LENGTH, MAX_ORACLES, METADATA_LENGTH, ON_REPORT_DISCRIMINATOR,
    REPORT_CONTEXT_LEN, SIGNATURE_LEN, STATE_VERSION,
};

use events::{ConfigSet, InitializeEmit, OwnershipAcceptance, OwnershipTransfer, ReportProcessed};

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
#[program]
pub mod keystone_forwarder {
    use anchor_lang::solana_program::{instruction::Instruction, program::invoke_signed};

    use super::*;

    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        // store forwarder PDA bump for later
        let (_, authority_nonce) = Pubkey::find_program_address(
            &[b"forwarder", ctx.accounts.state.key().as_ref()],
            &crate::ID,
        );

        let state = &mut ctx.accounts.state;
        state.version = STATE_VERSION;
        state.authority_nonce = authority_nonce;
        state.owner = ctx.accounts.owner.key();

        emit!(InitializeEmit {
            owner: ctx.accounts.owner.key(),
            authority_nonce
        });

        Ok(())
    }

    pub fn transfer_ownership(
        ctx: Context<TransferOwnership>,
        proposed_owner: Pubkey,
    ) -> Result<()> {
        let state = &mut ctx.accounts.state;
        let state_current_owner = state.owner;
        state.proposed_owner = proposed_owner;

        emit!(OwnershipTransfer {
            current_owner: state_current_owner,
            proposed_owner: state.proposed_owner
        });

        Ok(())
    }

    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        let state = &mut ctx.accounts.state;
        let state_previous_owner = state.owner;
        state.owner = state.proposed_owner;
        state.proposed_owner = Pubkey::default();

        emit!(OwnershipAcceptance {
            previous_owner: state_previous_owner,
            new_owner: state.owner
        });

        Ok(())
    }

    pub fn init_oracles_config(
        ctx: Context<InitOraclesConfig>,
        don_id: u32,
        config_version: u32,
        f: u8,
        signer_addresses: Vec<[u8; 20]>,
    ) -> Result<()> {
        let config = &mut ctx.accounts.oracles_config;

        set_oracles_config(config, don_id, config_version, f, signer_addresses)
    }

    pub fn update_oracles_config(
        ctx: Context<UpdateOraclesConfig>,
        don_id: u32,
        config_version: u32,
        f: u8,
        signer_addresses: Vec<[u8; 20]>,
    ) -> Result<()> {
        let config = &mut ctx.accounts.oracles_config;

        set_oracles_config(config, don_id, config_version, f, signer_addresses)
    }

    pub fn close_oracles_config(
        _ctx: Context<CloseOraclesConfig>,
        _don_id: u32,
        _config_version: u32,
    ) -> Result<()> {
        Ok(())
    }

    // data =  len_signatures (1) | signatures (N*65) | raw_report (M) | report_context (96)
    pub fn report<'info>(
        ctx: Context<'_, '_, '_, 'info, Report<'info>>,
        data: Vec<u8>,
    ) -> Result<()> {
        let num_signatures = data[0] as usize;
        let min_data_size = 1 + num_signatures * SIGNATURE_LEN + REPORT_CONTEXT_LEN;

        require_gt!(data.len(), min_data_size, ForwarderError::InvalidReport);

        // get config
        let oracles_config = &ctx.accounts.oracles_config;
        let f = oracles_config.f;
        require_neq!(f, 0, ForwarderError::InvalidConfig);

        require_gte!(
            num_signatures,
            (f + 1) as usize,
            ForwarderError::InvalidSignatureCount
        );

        // extract signatures
        let data = &data[1..];
        let total_signature_len: usize = SIGNATURE_LEN * num_signatures;

        let signatures: &[u8] = &data[..total_signature_len];
        // raw_report | report context
        let data = &data[total_signature_len..];
        let hashed_report = hash::hash(data).to_bytes();

        verify_signatures(&hashed_report, signatures, oracles_config, num_signatures)?;

        // slice raw_report from the report context
        let raw_report_end = data.len() - REPORT_CONTEXT_LEN;
        let raw_report = &data[..raw_report_end];

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

        // payload begins with the Anchor discriminator
        let mut payload = ON_REPORT_DISCRIMINATOR.to_vec();
        // borsh serialization of metadata vector and report vector
        // metadata is just workflow_cid, workflow_name, workflow_owner, and report_id (see format above)
        let metadata = &raw_report[FORWARDER_METADATA_LENGTH..METADATA_LENGTH].to_vec();
        let report = &raw_report[METADATA_LENGTH..].to_vec();
        // Borsh serialize each part separately
        payload.extend(&metadata.try_to_vec()?);
        payload.extend(&report.try_to_vec()?);

        let ix = Instruction::new_with_bytes(ctx.accounts.receiver_program.key(), &payload, metas);

        // used to derive the forwarder authority PDA
        let forwarder_state = ctx.accounts.state.key();
        let signers_seeds = &[
            b"forwarder",
            forwarder_state.as_ref(),
            &[ctx.accounts.state.authority_nonce],
        ];

        invoke_signed(&ix, &account_infos, &[signers_seeds])?;

        // update execution state

        execution_state.transmitter = ctx.accounts.transmitter.key();
        execution_state.transmission_id = transmission_id;
        execution_state.success = true;

        emit!(ReportProcessed {
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
    oracles_config: &Account<OraclesConfig>,
    num_signers: usize,
) -> Result<()> {
    // ensure MAX_SIGNERS fit in the bits of uniques
    let mut uniques: u32 = 0;
    assert!(uniques.count_ones() + uniques.count_zeros() >= MAX_ORACLES as u32);

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
    oracles_config: &mut Account<OraclesConfig>,
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
    oracles_config.signer_addresses = signer_addresses.clone();

    emit!(ConfigSet {
        don_id,
        config_version,
        f,
        signers: signer_addresses,
    });

    Ok(())
}

//
// Receiver contract will implement this in Anchor (or equivalent in pure Rust)
// pub fn on_report(ctx: Context<OnReport>, metadata: Vec<u8>, report: Vec<u8>) -> Result<()>
// with the following declared accounts
//
// #[derive(Accounts)]
// pub struct OnReport<'info> {
//     #[account(owner = FORWARDER_ID)]
//     pub state: Account<'info, ForwarderState>,

//     /// CHECK: This is a PDA
//     /// Anchor is unable to compute PDA with other program id so must do inline check within on_report
//     /// #[account(seeds = [b"forwarder", state.key().as_ref()], bump = state.authority_nonce)]
//     pub forwarder_authority: Signer<'info>,

//     // remaining accounts passed in as well
// }

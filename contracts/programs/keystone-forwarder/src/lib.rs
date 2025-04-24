use anchor_lang::prelude::*;
use anchor_lang::solana_program::{hash, keccak, secp256k1_recover::*};

mod error;
pub use error::*;

declare_id!("whV7Q5pi17hPPyaPksToDw1nMx6Lh8qmNWKFaLRQ4wz");

pub const STATE_VERSION: u8 = 1;

pub const ANCHOR_DISCRIMINATOR: usize = 8;

pub const REPORT_CONTEXT_LEN: usize = 96;

pub const MAX_REPORT_SIGNERS: usize = 17;

pub const SIGNATURE_LEN: usize = 65;

pub const FORWARDER_METADATA_LENGTH: usize = 45;
pub const METADATA_LENGTH: usize = 109;
#[program]
pub mod keystone_forwarder {
    use anchor_lang::{
        solana_program::{instruction::Instruction, program::invoke_signed, system_instruction},
        Discriminator,
    };

    use super::*;

    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        // store forwarder PDA bump for later
        let (_authority_pubkey, authority_nonce) = Pubkey::find_program_address(
            &[b"forwarder", ctx.accounts.state.key().as_ref()],
            &crate::ID,
        );

        let state = &mut ctx.accounts.state;
        state.version = STATE_VERSION;
        state.authority_nonce = authority_nonce;
        state.owner = ctx.accounts.owner.key();

        Ok(())
    }

    pub fn transfer_ownership(
        ctx: Context<TransferOwnership>,
        proposed_owner: Pubkey,
    ) -> Result<()> {
        let state = &mut ctx.accounts.state;
        state.proposed_owner = proposed_owner;

        Ok(())
    }

    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        let state = &mut ctx.accounts.state;
        state.owner = state.proposed_owner;
        state.proposed_owner = Pubkey::default();

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

    // data =  bump (1) | len_signatures (1) | signatures (N*65) | raw_report (M) | report_context (96)
    pub fn report<'info>(
        ctx: Context<'_, '_, '_, 'info, Report<'info>>,
        data: Vec<u8>,
    ) -> Result<()> {
        let num_signatures = data[1] as usize;
        require!(
            num_signatures <= MAX_REPORT_SIGNERS,
            ForwarderError::MaxSignersLimit
        );

        // first u8 stores bump, second u8 stores the number of signatures
        let min_data_size = 1 + 1 + num_signatures * SIGNATURE_LEN + REPORT_CONTEXT_LEN;

        require!(
            data.len() > min_data_size.into(),
            ForwarderError::InvalidReport
        );

        // get config
        let oracles_config = &ctx.accounts.oracles_config;
        let f = oracles_config.f;
        require!(f != 0, ForwarderError::InvalidConfig);

        require!(
            num_signatures >= (f + 1).into(),
            ForwarderError::InvalidSignatureCount
        );

        // bump is calculated off-chain by the write target when computing the pda
        let execution_state_bump = data[0];

        // extract signatures
        let data = &data[2..];
        let total_signature_len: usize = SIGNATURE_LEN * num_signatures;

        let signatures: &[u8] = &data[..total_signature_len];
        // raw_report | report context
        let data = &data[total_signature_len..];
        let hashed_report = hash::hash(&data).to_bytes();

        verify_signatures(&hashed_report, signatures, oracles_config, num_signatures)?;

        // slice raw_report from the report context
        let raw_report_end = data.len() - REPORT_CONTEXT_LEN;
        let raw_report = &data[..raw_report_end];

        let transmission_id =
            extract_transmission_id(raw_report, ctx.accounts.receiver_program.key);

        let execution_state = &ctx.accounts.execution_state;

        if execution_state.data_is_empty() {
            let space = ANCHOR_DISCRIMINATOR + ExecutionState::INIT_SPACE;

            let rent = Rent::get()?.minimum_balance(space);

            let seeds: &[&[u8]] = &[
                b"execution_state",
                &transmission_id,
                &[execution_state_bump],
            ];

            invoke_signed(
                &system_instruction::create_account(
                    ctx.accounts.transmitter.key,
                    execution_state.key,
                    rent,
                    space as u64,
                    ctx.program_id,
                ),
                &[
                    ctx.accounts.transmitter.to_account_info(),
                    execution_state.to_account_info(),
                    ctx.accounts.system_program.to_account_info(),
                ],
                &[&seeds[..]],
            )?;
        } else {
            // revert if execution succeded already

            let execution_state_info = execution_state.to_account_info(); // or just AccountInfo
            let state =
                ExecutionState::try_deserialize(&mut &execution_state_info.data.borrow()[..])?;

            require!(!state.success, ForwarderError::ExecutionAlreadySucceded)
        }

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
        let mut payload = hash::hash("global:on_report".as_bytes()).to_bytes()[..8].to_vec();
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

        let _ = invoke_signed(&ix, &account_infos, &[signers_seeds]);

        // update execution state
        let mut dst = execution_state.try_borrow_mut_data()?;
        let execution_state = ExecutionState {
            transmitter: ctx.accounts.transmitter.key(),
            transmission_id: transmission_id,
            success: true,
        };
        dst[..ANCHOR_DISCRIMINATOR].copy_from_slice(&ExecutionState::discriminator());
        execution_state.serialize(&mut &mut dst[ANCHOR_DISCRIMINATOR..])?;

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
    assert!(uniques.count_ones() + uniques.count_zeros() >= MAX_REPORT_SIGNERS as u32);

    for sig in signatures.chunks(SIGNATURE_LEN.into()) {
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

    require!(
        uniques.count_ones() as usize == num_signers,
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
    require!(
        signer_addresses.len() <= MAX_REPORT_SIGNERS.into(),
        ForwarderError::MaxSignersLimit
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

    oracles_config.config_id = ((don_id as u64) << 32) | (config_version as u64);
    oracles_config.f = f;
    oracles_config.signer_addresses = signer_addresses;

    Ok(())
}

#[account]
#[derive(Default)]
pub struct ForwarderState {
    pub version: u8,
    pub authority_nonce: u8, // bump
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
}

impl ForwarderState {
    // version + authority_nonce + owner + proposed_owner + vec prefix for config vec
    pub const SPACE: usize = 1 + 1 + 32 + 32;
}

#[derive(Accounts)]
pub struct Initialize<'info> {
    // the account is not a PDA but it is initialized by the program
    #[account(
        init,
        payer = owner,
        space = 8 + ForwarderState::SPACE
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

// combination of solidity's OracleSet and the configId mapping
#[account]
pub struct OraclesConfig {
    config_id: u64,
    f: u8,
    signer_addresses: Vec<[u8; 20]>,
}

impl OraclesConfig {
    pub const INIT_SPACE: usize = 8 + 1 + 4;

    pub fn space_with_signers(num_signers: usize) -> usize {
        Self::INIT_SPACE + (num_signers * 20)
    }
}

fn get_config_id(don_id: u32, config_version: u32) -> u64 {
    ((don_id as u64) << 32) | (config_version as u64)
}

#[derive(Accounts)]
#[instruction(don_id: u32, config_version: u32, f: u8, signer_addresses: Vec<[u8; 20]>)]
pub struct InitOraclesConfig<'info> {
    pub state: Account<'info, ForwarderState>,

    #[account(
        init,
        payer = owner,
        seeds = [b"config", state.key().as_ref(), &get_config_id(don_id, config_version).to_be_bytes()],
        bump,
        space = ANCHOR_DISCRIMINATOR + OraclesConfig::space_with_signers(signer_addresses.len())
    )]
    oracles_config: Account<'info, OraclesConfig>,

    #[account(mut, address = state.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>, // must be the same owner as the one in the state account

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(don_id: u32, config_version: u32, f: u8, signer_addresses: Vec<[u8; 20]>)]
pub struct UpdateOraclesConfig<'info> {
    pub state: Account<'info, ForwarderState>,

    #[account(
        mut,
        seeds = [b"config", state.key().as_ref(), &get_config_id(don_id, config_version).to_be_bytes()],
        bump,
        realloc = ANCHOR_DISCRIMINATOR + OraclesConfig::space_with_signers(signer_addresses.len()),
        realloc::payer = owner,
        realloc::zero = true
    )]
    oracles_config: Account<'info, OraclesConfig>,

    #[account(mut, address = state.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    pub system_program: Program<'info, System>,
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
    oracles_config: Account<'info, OraclesConfig>,

    #[account(mut, address = state.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>, // must be the same owner as the one in the state account
}

// data =  bump (1) | len_signatures (1) | signatures (N*65) | raw_report (M) | report_context (96)
fn extract_raw_report(data: &[u8]) -> &[u8] {
    let _execution_state_bump = data[0];
    let num_signatures = data[1] as usize;
    let data = &data[2..];
    let _signatures = &data[..num_signatures * SIGNATURE_LEN];
    let data = &data[num_signatures * SIGNATURE_LEN..];
    let _report_context = &data[data.len() - REPORT_CONTEXT_LEN..];
    let raw_report = &data[..data.len() - REPORT_CONTEXT_LEN];

    return raw_report;
}

// version                offset   0, size  1
// workflow_execution_id  offset   1, size 32
// timestamp              offset  33, size  4
// don_id                 offset  37, size  4
// don_config_version     offset  41, size  4
// workflow_cid           offset  45, size 32
// workflow_name          offset  77, size 10
// workflow_owner         offset  87, size 20
// report_id              offset 107, size  2

fn extract_config_id(raw_report: &[u8]) -> [u8; 8] {
    // don_id | don_config_version
    raw_report[37..45].try_into().expect("Expected 8 bytes")
}

fn extract_transmission_id(raw_report: &[u8], receiver: &Pubkey) -> [u8; 32] {
    let workflow_execution_id = &raw_report[1..33];
    let report_id = &raw_report[107..109];

    // use sha-256 bc it's much cheaper than keccak-256
    hash::hash(&[&receiver.to_bytes(), workflow_execution_id, report_id].concat()).to_bytes()
}

#[derive(Accounts)]
#[instruction(data: Vec<u8>)]
pub struct Report<'info> {
    pub state: Account<'info, ForwarderState>,

    #[account(
        mut,
        seeds = [b"config", state.key().as_ref(), &extract_config_id(extract_raw_report(&data))],
        bump
    )]
    oracles_config: Account<'info, OraclesConfig>,

    #[account(mut)]
    pub transmitter: Signer<'info>,

    /// CHECK: This is a PDA
    #[account(seeds = [b"forwarder", state.key().as_ref()], bump = state.authority_nonce)]
    pub forwarder_authority: UncheckedAccount<'info>,

    // it is dependent on the state.key(), a predetermined bump, workflow execution id, config_id, report_id
    /// CHECK: existing account will be updated OR new account will be initialized
    #[account(
        mut,
        seeds = [
            b"execution_state", 
            &extract_transmission_id(extract_raw_report(&data), receiver_program.key)
        ],
        bump = data[0]
    )]
    pub execution_state: UncheckedAccount<'info>,

    #[account(executable)]
    /// CHECK: We don't use Program<> here since it can be any program, "executable" is enough
    pub receiver_program: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,
    // remaining accounts passed to receiver
}

#[account]
#[derive(Default, InitSpace)]
pub struct ExecutionState {
    pub transmitter: Pubkey,
    pub transmission_id: [u8; 32],
    // until failure states are reported by the write target, success will always be true
    pub success: bool,
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

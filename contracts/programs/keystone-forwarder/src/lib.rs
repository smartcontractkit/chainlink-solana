use anchor_lang::prelude::*;

declare_id!("6v9Lm94wiHXJf4HYoWoRj7JGb5YCDnsvybr9Y3seJ7po");

// TODO: ownable

pub const STATE_VERSION: u8 = 1;

#[account]
#[derive(Default)]
pub struct State {
    version: u8,
    authority_nonce: u8,
    owner: Pubkey,
}

#[account]
#[derive(Default)]
pub struct ExecutionState {}

#[error_code]
pub enum ErrorCode {
    #[msg("Unauthorized")]
    Unauthorized = 0,

    #[msg("Invalid input")]
    InvalidInput = 1,
}

#[program]
pub mod keystone_forwarder {
    use anchor_lang::solana_program::{instruction::Instruction, program::invoke_signed};

    use super::*;

    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        // Precompute the authority PDA bump
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

    // TODO: use raw &[u8] input to avoid serialization and encoding
    pub fn report(ctx: Context<Report>, data: Vec<u8>) -> Result<()> {
        const RAW_REPORT_LEN: usize = 1 + OFFSET;
        require!(data.len() > RAW_REPORT_LEN, ErrorCode::InvalidInput);
        let len = data[0] as usize;
        let data = &data[1..];
        let (raw_signatures, raw_report) = data.split_at(32 * len);

        // TODO: a way to store context inside data without limiting the receiver
        const OFFSET: usize = 32 + 32;
        // meta = (workflowID, workflowExecutionID)
        let (meta, data) = raw_report.split_at(OFFSET);

        // verify signature
        use anchor_lang::solana_program::{hash, keccak, secp256k1_recover::*};

        // 64 byte signature + 1 byte recovery id
        const SIGNATURE_LEN: usize = SECP256K1_SIGNATURE_LENGTH + 1;
        // raw_signatures is exactly sized
        require!(
            raw_signatures.len() % SIGNATURE_LEN == 0,
            ErrorCode::InvalidInput
        );
        // let signature_count = raw_signatures.len() / SIGNATURE_LEN;
        // require!(
        //     signature_count == usize::from(config.f) + 1,
        //     ErrorCode::InvalidInput
        // );

        let hash = hash::hash(&raw_report).to_bytes();

        let raw_signatures = raw_signatures.chunks(SIGNATURE_LEN);
        for signature in raw_signatures {
            // TODO:
        }

        // check if PDA exists, if so terminate the call

        // invoke_signed with forwarder authority
        let program_id = ctx.accounts.receiver_program.key();
        let accounts = vec![];
        let ix = Instruction::new_with_bytes(program_id, &raw_report, accounts);
        let account_infos = &[];
        let state_pubkey = ctx.accounts.state.key();
        let signers_seeds = &[
            b"forwarder",
            state_pubkey.as_ref(),
            &[ctx.accounts.state.authority_nonce],
        ];
        let _ = invoke_signed(&ix, account_infos, &[signers_seeds]);

        // mark tx as signed by initializing PDA via create_account instruction
        Ok(())
    }
}

#[derive(Accounts)]
pub struct Initialize<'info> {
    // space: 8 discriminator + u8 authority_nonce + 1 bump
    #[account(
        init,
        payer = owner,
        space = 8 + 1 + 1 + 32
    )]
    pub state: Account<'info, State>,
    #[account(mut)]
    pub owner: Signer<'info>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct Report<'info> {
    /// Forwarder state acccount
    #[account(mut)]
    pub state: Account<'info, State>,

    /// Transmitter, signing the current transaction call
    pub authority: Signer<'info>,

    /// Authority used for signing the receiver invocation
    /// CHECK: This is a PDA
    #[account(seeds = [b"forwarder", state.key().as_ref()], bump = state.authority_nonce)]
    pub forwarder_authority: AccountInfo<'info>,

    /// State PDA for the workflow execution represented by this report.
    /// TODO: we need to manually verify that it's the correct PDA since we need to unpack meta to get the execution ID
    #[account(mut)]
    // pub execution_state: Account<'info, ExecutionState>,
    /// CHECK: TODO:
    pub execution_state: UncheckedAccount<'info>,

    #[account(executable)]
    /// CHECK: We don't use Program<> here since it can be any program, "executable" is enough
    pub receiver_program: UncheckedAccount<'info>,
    // TODO: ensure receiver isn't forwarder itself?

    // remaining_accounts... get passed to receiver as is
}

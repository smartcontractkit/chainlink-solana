mod buffering;

use anchor_lang::prelude::*;
use buffering::{deserialize_from_buffer_account, Buffering};

declare_id!("85bivLENWAX36kyWC9zemZu9H3D88J79wXdHgR6ZmZHX");

pub const EXECUTION_REPORT_BUFFER: &[u8] = b"execution_report_buffer";
pub const CONFIG: &[u8] = b"config";

/// Static space allocated to any account: must always be added to space calculations.
pub const ANCHOR_DISCRIMINATOR: usize = 8;

#[program]
pub mod buffer_payload {
    use super::*;

    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        let config = &mut ctx.accounts.config;
        config.owner = ctx.accounts.authority.key();

        Ok(())
    }

    pub fn execute<'info>(
        ctx: Context<'_, '_, 'info, 'info, ExecuteContext<'info>>,
        report: Vec<u8>,
        fail: bool,
    ) -> Result<()> {
        if report.is_empty() {
            require!(!fail, Error::ForcedFailure);
            let (_, buffered_bytes) = deserialize_from_buffer_account(
                ctx.remaining_accounts
                    .last()
                    .ok_or(Error::ReportUnavailable)?,
            )?;

            let buffer = Account::<Buffer>::try_from(ctx.remaining_accounts.last().unwrap())?;
            require!(buffer.is_complete(), Error::Incomplete);
            let report_length = buffer.report_length.try_into().unwrap();
            require!(buffered_bytes == report_length, Error::Incomplete);

            buffer.close(ctx.accounts.authority.to_account_info())?;
        }
        // no-op if report provided directly

        Ok(())
    }

    pub fn buffer_execution_report<'info>(
        ctx: Context<'_, '_, 'info, 'info, BufferContext<'info>>,
        _buffer_id: Vec<u8>,
        report_length: u32,
        chunk: Vec<u8>,
        chunk_index: u8,
        num_chunks: u8,
    ) -> Result<()> {
        ctx.accounts
            .buffer
            .add_chunk(report_length, &chunk, chunk_index, num_chunks)
    }

    pub fn close_execution_report_buffer(
        _ctx: Context<CloseBufferContext>,
        _buffer_id: Vec<u8>,
    ) -> Result<()> {
        Ok(())
    }
}

#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(
        init,
        payer = authority,
        seeds = [CONFIG],
        bump,
        space = ANCHOR_DISCRIMINATOR + Config::INIT_SPACE
    )]
    pub config: Account<'info, Config>,

    #[account(mut)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct ExecuteContext<'info> {
    #[account(mut)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(buffer_id: Vec<u8>, report_length: u32, chunk: Vec<u8>, chunk_index: u8)]
pub struct BufferContext<'info> {
    #[account(
        init_if_needed,
        payer = authority,
        seeds = [EXECUTION_REPORT_BUFFER, &buffer_id, authority.key().as_ref()],
        bump,
        space = ANCHOR_DISCRIMINATOR + Buffer::INIT_SPACE + report_length as usize
    )]
    pub buffer: Account<'info, Buffer>,

    #[account(
        seeds = [CONFIG],
        bump,
    )]
    pub config: Account<'info, Config>,

    #[account(mut)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(buffer_id: Vec<u8>)]
pub struct CloseBufferContext<'info> {
    #[account(
        mut,
        seeds = [EXECUTION_REPORT_BUFFER, &buffer_id, authority.key().as_ref()],
        bump,
        close = authority
    )]
    pub buffer: Account<'info, Buffer>,

    #[account(
        seeds = [CONFIG],
        bump,
    )]
    pub config: Account<'info, Config>,

    #[account(mut)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

#[account]
#[derive(InitSpace, Debug)]
pub struct Buffer {
    pub version: u8,
    pub chunk_bitmap: u64,
    pub total_chunks: u8,
    pub chunk_length: u32,
    pub report_length: u32,
    #[max_len(0)]
    pub data: Vec<u8>,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct Report {
    pub report: Vec<u8>,
}

#[account]
#[derive(InitSpace, Debug)]
pub struct Config {
    pub owner: Pubkey,
}

#[error_code]
pub enum Error {
    #[msg("The report buffer already contains that chunk")]
    AlreadyContainsChunk = 1000,
    #[msg("The report buffer is already initialized")]
    AlreadyInitialized,
    #[msg("Invalid length for report buffer")]
    InvalidLength,
    #[msg("Chunk lies outside the report buffer")]
    InvalidChunkIndex,
    #[msg("Chunk size is too small.")]
    ChunkSizeTooSmall,
    #[msg("Invalid chunk size. Remember that the last chunk should be right-padded with zeros.")]
    InvalidChunkSize,
    #[msg("Report buffer is not complete: chunks are missing")]
    Incomplete,
    #[msg("Report wasn't provided via buffer")]
    ReportUnavailable,
    #[msg("Failed to deserialize report")]
    FailedToDeserializeReport,
    #[msg("Forced failure")]
    ForcedFailure,
}

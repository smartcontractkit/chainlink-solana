use anchor_lang::prelude::*;
use std::mem::size_of;

declare_id!("6AfuXF6HapDUhQfE4nQG9C1SGtA1YjP3icaJyRfU4RyE");

#[program]
pub mod contract_reader_interface {
    use super::*;

    pub fn initialize(ctx: Context<Initialize>, test_idx: u64, value: u64) -> Result<()> {
        let account = &mut ctx.accounts.data;

        account.u64_value = value;
        account.u64_slice = [3, 4].to_vec();
        account.idx = test_idx;
        account.bump = ctx.bumps.data;

        Ok(())
    }
}

#[derive(Accounts)]
#[instruction(test_idx: u64)]
pub struct Initialize<'info> {
    // derived test PDA
    #[account(
        init,
        payer = signer,
        space = size_of::<DataAccount>() + 8,
        seeds=[b"data".as_ref(), test_idx.to_le_bytes().as_ref()],
        bump)]
    pub data: Account<'info, DataAccount>,

    #[account(mut)]
    pub signer: Signer<'info>,

    pub system_program: Program<'info, System>,
}

#[account]
pub struct DataAccount {
    pub idx: u64,
    pub bump: u8,
    pub u64_value: u64,
    pub u64_slice: Vec<u64>,
}

use anchor_lang::prelude::*;
use std::mem::size_of;

declare_id!("9SFyk8NmGYh5D612mJwUYhguCRY9cFgaS2vksrigepjf");

#[program]
pub mod contract_reader_interface_secondary {
    use super::*;

    pub fn initialize(ctx: Context<Initialize>, test_idx: u64, value: u64) -> Result<()> {
        let account = &mut ctx.accounts.data;
        account.u64_value = value;
        account.idx = test_idx;
        account.bump = ctx.bumps.data;
        Ok(())
    }
}

#[derive(Accounts)]
#[instruction(test_idx: u64)]
pub struct Initialize<'info> {
    #[account(mut)]
    pub signer: Signer<'info>,

    // derived test PDA
    #[account(
        init,
        payer = signer,
        space = size_of::<Data>() + 8,
        seeds=[b"data".as_ref(), test_idx.to_le_bytes().as_ref()],
        bump)]
    pub data: Account<'info, Data>,

    pub system_program: Program<'info, System>,
}

#[account]
pub struct Data {
    pub u64_value: u64,
    pub idx: u64,
    pub bump: u8
}

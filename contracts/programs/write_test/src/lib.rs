use anchor_lang::prelude::*;

declare_id!("39vbQVpEMtZtg3e6ZSE7nBSzmNZptmW45WnLkbqEe4TU");

#[program]
pub mod write_test {
    use super::*;

    pub fn initialize(ctx: Context<Initialize>, lookup_table: Pubkey) -> Result<()> {
        let data = &mut ctx.accounts.data_account;
        data.version = 1;
        data.administrator = ctx.accounts.admin.key();
        data.pending_administrator = Pubkey::default();
        data.lookup_table = lookup_table;
    
        Ok(())
    }
    
}

#[derive(Accounts)]
pub struct Initialize<'info> {
    /// PDA account, derived from seeds and created by the System Program in this instruction
    #[account(
        init,                 // Initialize the account
        payer = admin,        // Specify the payer
        space = DataAccount::SIZE, // Specify the account size
        seeds = [b"data"],    // Define the PDA seeds
        bump                  // Use the bump seed
    )]
    pub data_account: Account<'info, DataAccount>,

    /// Admin account that pays for PDA creation and signs the transaction
    #[account(mut)]
    pub admin: Signer<'info>,

    /// System Program is required for PDA creation
    pub system_program: Program<'info, System>,
}

#[account]
pub struct DataAccount {
    pub version: u8,
    pub administrator: Pubkey,
    pub pending_administrator: Pubkey,
    pub lookup_table: Pubkey,
}

impl DataAccount {
    /// The total size of the `DataAccount` struct, including the discriminator
    pub const SIZE: usize = 8 + 1 + 32 * 3; // 8 bytes for discriminator + 1 byte for version + 32 bytes * 3 pubkeys
}

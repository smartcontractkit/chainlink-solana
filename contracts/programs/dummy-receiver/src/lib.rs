use anchor_lang::prelude::*;
use keystone_forwarder::ForwarderState;
use keystone_forwarder::ID as FORWARDER_ID;

declare_id!("5z38tFCAmcPJb1DXUHSoKQhR8qQ8o9aNZ8rZFWe6gH4L");

// THIS IS UN-AUDITED CODE USED FOR TESTING PURPOSES ONLY
// DO NOT USE THIS CODE IN PRODUCTION.

#[program]
pub mod dummy_receiver {
    use super::*;

    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        ctx.accounts.report_state.forwarder_authority = ctx.accounts.forwarder_authority.key();
        Ok(())
    }

    pub fn on_report<'info>(
        ctx: Context<'_, '_, 'info, 'info, OnReport<'info>>,
        metadata: Vec<u8>,
        report: Vec<u8>,
    ) -> Result<()> {

        // verify 
        // 1. forwarder authority signer is authorized by this program
        // 2. forwarder authority signer belongs to (is a PDA derived from) forwarder state

        // 1
        require!(
            ctx.accounts.forwarder_authority.key() == ctx.accounts.report_state.forwarder_authority,
            AuthError::Unauthorized
        );

        // 2
        let (expected_pda, expected_bump) = Pubkey::find_program_address(
            &[b"forwarder", ctx.accounts.state.key().as_ref()],
            &FORWARDER_ID,
        );
        require!(
            expected_pda == ctx.accounts.forwarder_authority.key()
                && expected_bump == ctx.accounts.state.authority_nonce,
            AuthError::Unauthorized
        );

        // in a production setting you'd also want to verify the metadata too...

        ctx.accounts.report_state.report = report;
        ctx.accounts.report_state.metadata = metadata;

        // note: alternative account implementation could pass as ctx.remaining_accounts
        // however that requires more work

        // let account_info = &ctx.remaining_accounts[0];
        // let mut latest_report: Account<'info, LatestReport> = Account::try_from(account_info)?;
        // latest_report.metadata = metadata;
        // latest_report.report = report;

        // // includes anchor discriminator by default
        // latest_report.try_serialize(&mut &mut account_info.data.borrow_mut()[..])?;

        Ok(())
    }
}

#[error_code]
pub enum AuthError {
    #[msg("The signer is unauthorized")]
    Unauthorized,
}

#[account]
#[derive(Default)]
pub struct LatestReport {
    pub metadata: Vec<u8>,
    pub report: Vec<u8>,
    pub forwarder_authority: Pubkey,
}

#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(
        init,
        payer = signer,
        space = 8 + 4 + 4 + 65 + 32 // [64 (metadata) + 1 (report)] = 65
    )]
    pub report_state: Account<'info, LatestReport>,

    #[account(mut)]
    pub signer: Signer<'info>,

    /// CHECK: this is the expected signer of "on_report"
    #[account()]
    pub forwarder_authority: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct OnReport<'info> {
    #[account(owner = FORWARDER_ID)]
    pub state: Account<'info, ForwarderState>,

    /// CHECK: This is a PDA
    /// Anchor is unable to compute PDA with other program id so you
    /// must do inline check within on_report
    /// #[account(seeds = [b"forwarder", state.key().as_ref()], bump = state.authority_nonce)]
    pub forwarder_authority: Signer<'info>,
    // remaining accounts passed in as well

    #[account(mut)]
    pub report_state: Account<'info, LatestReport>,
}

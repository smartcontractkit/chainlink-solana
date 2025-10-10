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

    // as a mock receiver
    pub fn on_report<'info>(
        ctx: Context<'_, '_, 'info, 'info, OnReport<'info>>,
        metadata: Vec<u8>,
        report: Vec<u8>,
    ) -> Result<()> {
        // verify
        // 1. forwarder authority signer belongs to (is a PDA derived from) forwarder state (done in the anchor constraint!)
        // 2. forwarder authority signer is authorized by this program
        // 3. report metadata (not done in this dummy example)

        // 2
        require!(
            ctx.accounts.forwarder_authority.key() == ctx.accounts.report_state.forwarder_authority,
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

    // pub fn initialize_mock_legacy_store(_ctx: Context<InitializeMockLegacyStore>) -> Result<()> {
    //     Ok(())
    // }

    // as a mock legacy store
    pub fn cache_submit(ctx: Context<CacheSubmit>, rounds: Vec<CacheTransmission>) -> Result<()> {
        // // just assume that ctx.remaining_accounts[0] is the LatestCacheSubmission account

        // let mut dst = ctx.remaining_accounts[0].try_borrow_mut_data()?;
        // let new_submission = LatestCacheSubmission {
        //     signer: ctx.accounts.authority.key(),
        //     data: rounds.clone(),
        //     accounts: ctx.remaining_accounts.iter().map(|x| x.key()).collect()
        // };
        // new_submission.serialize(&mut &mut dst[8..])?;

        emit!(Submit {
            rounds,
            feeds: ctx.remaining_accounts.iter().map(|x| x.key()).collect()
        });

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
    // #[account(address = report_state.key() @ AuthError::Unauthorized)]
    pub forwarder_authority: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct OnReport<'info> {
    #[account(owner = FORWARDER_ID)]
    pub state: Account<'info, ForwarderState>,

    // note the forwarder authority is a PDA signer
    #[account(seeds = [b"forwarder", state.key().as_ref(), crate::ID.as_ref()], bump, seeds::program = FORWARDER_ID)]
    pub forwarder_authority: Signer<'info>,

    #[account(mut)]
    pub report_state: Account<'info, LatestReport>,
    // remaining accounts may be passed in
}

// legacy store

// #[account]
// #[derive(Default, InitSpace)]
// pub struct LatestCacheSubmission {
//     pub signer: Pubkey,
//     #[max_len(10)]
//     pub data: Vec<CacheTransmission>, // just overallocate space
//     #[max_len(10)]
//     pub accounts: Vec<Pubkey>,        // just overallocate space
// }

// #[derive(Accounts)]
// pub struct InitializeMockLegacyStore<'info> {
//     #[account(mut)]
//     pub signer: Signer<'info>,

//      #[account(
//         init,
//         payer = signer,
//         space = 8 + LatestCacheSubmission::INIT_SPACE,
//     )]
//     pub cache_submission: Account<'info, LatestCacheSubmission>,

//     pub system_program: Program<'info, System>,
// }

#[derive(Accounts)]
pub struct CacheSubmit<'info> {
    pub authority: Signer<'info>, // N OCR2 feeds in ctx.remaining_accounts
                                  // #[account(mut)]
                                  // pub feed: Account<'info, Transmissions>,
}

#[event]
pub struct Submit {
    rounds: Vec<CacheTransmission>,
    feeds: Vec<Pubkey>,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, InitSpace)]
pub struct CacheTransmission {
    pub timestamp: u32,
    pub answer: u128,
}

use anchor_lang::{prelude::*, Discriminator};

use crate::{
    common::ANCHOR_DISCRIMINATOR,
    context::InitDecimalReports,
    error::DataCacheError,
    event::DecimalReportInitialized,
    state::DecimalReport,
    utils::{create_account, verify_feed_admin},
};

pub fn handler<'info>(
    ctx: Context<'_, '_, 'info, 'info, InitDecimalReports<'info>>,
    data_ids: Vec<[u8; 16]>,
) -> Result<()> {
    // check feed admin here
    let state = &ctx.accounts.state.load()?;
    verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;

    let state_key = ctx.accounts.state.key();

    let data_ids_account_infos = ctx.remaining_accounts;

    require_eq!(
        data_ids.len(),
        data_ids_account_infos.len(),
        DataCacheError::ArrayLengthMismatch
    );

    for (i, data_id) in data_ids.iter().enumerate() {
        let curr_report_account_info = &data_ids_account_infos[i];

        let (decimal_report, bump) = Pubkey::find_program_address(
            &[b"decimal_report", state_key.as_ref(), data_id],
            &crate::ID,
        );

        require_keys_eq!(
            decimal_report,
            *curr_report_account_info.key,
            DataCacheError::AccountMismatch
        );

        // only initialize if required
        if curr_report_account_info.data_is_empty() {
            let space = ANCHOR_DISCRIMINATOR + DecimalReport::INIT_SPACE;
            let seeds: &[&[u8]] = &[b"decimal_report", state_key.as_ref(), data_id, &[bump]];
            let payer = ctx.accounts.feed_admin.clone();
            create_account(
                space,
                seeds,
                payer,
                curr_report_account_info.clone(),
                crate::ID,
                ctx.accounts.system_program.to_account_info(),
            )?;

            let mut dst = curr_report_account_info.try_borrow_mut_data()?;
            dst[..ANCHOR_DISCRIMINATOR].copy_from_slice(&DecimalReport::discriminator());

            emit!(DecimalReportInitialized {
                state: ctx.accounts.state.key(),
                data_id: *data_id
            });
        } else {
            // verify expected format
            DecimalReport::try_deserialize(&mut &curr_report_account_info.data.borrow()[..])?;
        }
    }

    Ok(())
}

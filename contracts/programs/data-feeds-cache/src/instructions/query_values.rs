use crate::{context::QueryValues, error::DataCacheError, state::DecimalReport};
use anchor_lang::prelude::*;

pub fn handler<'info>(
    ctx: Context<'_, '_, 'info, 'info, QueryValues<'info>>,
    data_ids: Vec<[u8; 16]>,
) -> Result<Vec<DecimalReport>> {
    require_eq!(
        data_ids.len(),
        ctx.remaining_accounts.len(),
        DataCacheError::ArrayLengthMismatch
    );

    let mut reports = Vec::new();

    for (i, data_id) in data_ids.iter().enumerate() {
        let (decimal_report, _) = Pubkey::find_program_address(
            &[
                b"decimal_report",
                ctx.accounts.cache_state.key().as_ref(),
                data_id,
            ],
            &crate::ID,
        );

        let report_account_info = &ctx.remaining_accounts[i];

        require_keys_eq!(
            decimal_report,
            *report_account_info.key,
            DataCacheError::AccountMismatch
        );

        let r = Account::<DecimalReport>::try_from(report_account_info)?;

        reports.push(DecimalReport {
            timestamp: r.timestamp,
            answer: r.answer,
        });
    }

    Ok(reports)
}

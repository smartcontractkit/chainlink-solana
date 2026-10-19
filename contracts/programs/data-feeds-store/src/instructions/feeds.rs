use anchor_lang::prelude::*;

use crate::{
    common::{ACCOUNT_VERSION, FEED_SEED, MAX_ACCOUNT_DATA_INCREASE},
    context::FeedAdminAccounts,
    data_id,
    error::StoreError,
    event::*,
    logic::{check_max_stored_reports, MaxStoredReportsError},
    state::{feed_account_len, FeedData},
    utils::{close_account, create_pda_account, load_feed, store, verify_feed_admin},
};

fn validate_non_svr(id: &[u8; 16]) -> Result<()> {
    require!(
        !data_id::is_zero(id) && data_id::is_valid_non_svr(id),
        StoreError::InvalidDataId
    );
    Ok(())
}

/// Create the history accounts of non-SVR data IDs with the store's default ring size. Accounts
/// that already exist are left untouched.
pub fn init_feeds<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    non_svr_data_ids: Vec<[u8; 16]>,
) -> Result<()> {
    let state = ctx.accounts.state.load()?;
    verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;
    require_eq!(
        non_svr_data_ids.len(),
        ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );

    let state_key = ctx.accounts.state.key();
    for (id, info) in non_svr_data_ids.iter().zip(ctx.remaining_accounts.iter()) {
        validate_non_svr(id)?;

        let (address, bump) =
            Pubkey::find_program_address(&[FEED_SEED, state_key.as_ref(), id], &crate::ID);
        require_keys_eq!(address, *info.key, StoreError::AccountMismatch);

        if !info.data_is_empty() {
            load_feed(info, &state_key, id)?;
            continue;
        }

        let max_stored_reports = state.default_max_stored_reports;
        create_pda_account(
            feed_account_len(max_stored_reports),
            &[FEED_SEED, state_key.as_ref(), id, &[bump]],
            &ctx.accounts.feed_admin.to_account_info(),
            info,
            &ctx.accounts.system_program.to_account_info(),
        )?;

        let mut feed: Box<FeedData> = bytemuck::zeroed_box();
        feed.version = ACCOUNT_VERSION;
        feed.data_id = *id;
        feed.state = state_key;
        feed.max_stored_reports = max_stored_reports;
        store(info, &*feed)?;

        emit!(FeedInitialized {
            state: state_key,
            non_svr_data_id: *id,
            max_stored_reports,
        });
    }
    Ok(())
}

/// Close feed history accounts, returning their rent to the feed admin.
pub fn close_feeds<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    non_svr_data_ids: Vec<[u8; 16]>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require_eq!(
        non_svr_data_ids.len(),
        ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );

    let state_key = ctx.accounts.state.key();
    for (id, info) in non_svr_data_ids.iter().zip(ctx.remaining_accounts.iter()) {
        load_feed(info, &state_key, id)?;
        close_account(info, &ctx.accounts.feed_admin.to_account_info())?;
        emit!(FeedClosed {
            state: state_key,
            non_svr_data_id: *id,
        });
    }
    Ok(())
}

/// Set the number of non-SVR reports retained per feed. The first configuration is free,
/// afterwards the ring can only grow, and only while it has never wrapped. Growing is limited to
/// 10KiB (320 reports) per instruction.
pub fn set_max_stored_reports<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    non_svr_data_ids: Vec<[u8; 16]>,
    max_stored_reports: Vec<u64>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require!(
        non_svr_data_ids.len() == max_stored_reports.len()
            && non_svr_data_ids.len() == ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );

    let state_key = ctx.accounts.state.key();
    let feed_admin = ctx.accounts.feed_admin.to_account_info();
    let rent = Rent::get()?;

    for ((id, new_max), info) in non_svr_data_ids
        .iter()
        .zip(max_stored_reports.iter().copied())
        .zip(ctx.remaining_accounts.iter())
    {
        validate_non_svr(id)?;
        let mut feed = load_feed(info, &state_key, id)?;

        check_max_stored_reports(&feed, new_max).map_err(|e| match e {
            MaxStoredReportsError::Invalid => StoreError::InvalidMaxStoredReports,
            MaxStoredReportsError::NotIncreased => StoreError::MaxStoredReportsNotIncreased,
            MaxStoredReportsError::Locked => StoreError::MaxStoredReportsLocked,
            MaxStoredReportsError::TooLow => StoreError::MaxStoredReportsTooLow,
        })?;

        let old_len = info.data_len();
        let new_len = feed_account_len(new_max);
        match new_len.cmp(&old_len) {
            std::cmp::Ordering::Greater => {
                require!(
                    new_len - old_len <= MAX_ACCOUNT_DATA_INCREASE,
                    StoreError::InvalidMaxStoredReports
                );
                let top_up = rent
                    .minimum_balance(new_len)
                    .saturating_sub(info.lamports());
                if top_up > 0 {
                    anchor_lang::system_program::transfer(
                        CpiContext::new(
                            ctx.accounts.system_program.to_account_info(),
                            anchor_lang::system_program::Transfer {
                                from: feed_admin.clone(),
                                to: info.clone(),
                            },
                        ),
                        top_up,
                    )?;
                }
                info.resize(new_len)?;
                info.try_borrow_mut_data()?[old_len..].fill(0);
            }
            std::cmp::Ordering::Less => {
                info.resize(new_len)?;
                let refund = info
                    .lamports()
                    .saturating_sub(rent.minimum_balance(new_len));
                **info.try_borrow_mut_lamports()? -= refund;
                **feed_admin.try_borrow_mut_lamports()? += refund;
            }
            std::cmp::Ordering::Equal => {}
        }

        feed.max_stored_reports = new_max;
        feed.max_stored_reports_configured = 1;
        store(info, &*feed)?;

        emit!(MaxStoredReportsSet {
            state: state_key,
            non_svr_data_id: *id,
            max_stored_reports: new_max,
        });
    }
    Ok(())
}

/// Set the accepted answer range of feeds, shared by their SVR paths. The (i128::MIN, i128::MAX)
/// pair clears the range.
pub fn set_min_max_answers<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    non_svr_data_ids: Vec<[u8; 16]>,
    min_answers: Vec<i128>,
    max_answers: Vec<i128>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require!(
        non_svr_data_ids.len() == min_answers.len()
            && non_svr_data_ids.len() == max_answers.len()
            && non_svr_data_ids.len() == ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );

    let state_key = ctx.accounts.state.key();
    for (i, (id, info)) in non_svr_data_ids
        .iter()
        .zip(ctx.remaining_accounts.iter())
        .enumerate()
    {
        validate_non_svr(id)?;
        let mut feed = load_feed(info, &state_key, id)?;
        let (min_answer, max_answer) = (min_answers[i], max_answers[i]);

        if min_answer == i128::MIN && max_answer == i128::MAX {
            // unbounded pair, same as never configured
            feed.min_max_configured = 0;
            feed.min_answer = 0;
            feed.max_answer = 0;
        } else {
            require!(min_answer <= max_answer, StoreError::InvalidMinMaxRange);
            feed.min_max_configured = 1;
            feed.min_answer = min_answer;
            feed.max_answer = max_answer;
        }
        store(info, &*feed)?;

        emit!(MinMaxAnswerSet {
            state: state_key,
            non_svr_data_id: *id,
            min_answer,
            max_answer,
        });
    }
    Ok(())
}

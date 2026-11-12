//! Instructions only the trusted store calls. They emit no events: the store emits them, so a
//! write needs no event accounts.

use anchor_lang::prelude::*;

use crate::{
    common::{
        data_id_seed, ACCOUNT_VERSION, FEED_SEED, MAX_ACCOUNT_DATA_INCREASE, STORED_DECIMALS,
    },
    context::{WriterOnly, WriterPayer},
    error::ProxyError,
    logic::{self, check_max_stored_reports, MaxStoredReportsError, Now},
    state::{feed_account_len, Feed, NewRound},
    utils::{
        create_pda_account, encode_description, is_uninitialized, load_any_feed_into,
        load_feed_into, store_feed, RingMut,
    },
};

/// Create feeds (remaining accounts: their PDAs, `bumps` their bumps) with an empty description,
/// the stored decimals as their minimum and a ring of `max_stored_reports`. A feed that already
/// exists is checked and left as it is, so a create can be repeated.
pub fn create_feeds<'info>(
    ctx: Context<'_, '_, 'info, 'info, WriterPayer<'info>>,
    data_ids: Vec<[u8; 16]>,
    bumps: Vec<u8>,
    max_stored_reports: u64,
) -> Result<()> {
    require!(
        data_ids.len() == bumps.len() && data_ids.len() == ctx.remaining_accounts.len(),
        ProxyError::ArrayLengthMismatch
    );
    require!(
        max_stored_reports >= crate::common::MIN_MAX_STORED_REPORTS
            && feed_account_len(max_stored_reports) <= MAX_ACCOUNT_DATA_INCREASE,
        ProxyError::InvalidMaxStoredReports
    );
    let state_key = ctx.accounts.state.key();
    let payer = ctx.accounts.payer.to_account_info();
    let system_program = ctx.accounts.system_program.to_account_info();
    let mut feed: Box<Feed> = bytemuck::zeroed_box();
    for ((id, bump), info) in data_ids
        .iter()
        .zip(bumps.iter())
        .zip(ctx.remaining_accounts.iter())
    {
        let seed = data_id_seed(id);
        let seeds: &[&[u8]] = &[FEED_SEED, state_key.as_ref(), &seed, &[*bump]];
        let address = Pubkey::create_program_address(seeds, &crate::ID)
            .map_err(|_| ProxyError::AccountMismatch)?;
        require_keys_eq!(address, *info.key, ProxyError::AccountMismatch);

        if !is_uninitialized(info) && *info.owner == crate::ID {
            load_feed_into(info, &state_key, &seed, &mut feed)?;
            continue;
        }
        create_pda_account(
            feed_account_len(max_stored_reports),
            seeds,
            &payer,
            info,
            &system_program,
        )?;
        bytemuck::bytes_of_mut(&mut *feed).fill(0);
        feed.version = ACCOUNT_VERSION;
        feed.min_decimals = STORED_DECIMALS;
        feed.data_id = seed;
        feed.state = state_key;
        feed.max_stored_reports = max_stored_reports;
        store_feed(info, &feed)?;
    }
    Ok(())
}

/// Set descriptions (remaining accounts: the feeds), an empty string clears one.
pub fn set_feed_descriptions<'info>(
    ctx: Context<'_, '_, 'info, 'info, WriterOnly<'info>>,
    data_ids: Vec<[u8; 16]>,
    descriptions: Vec<String>,
) -> Result<()> {
    require!(
        data_ids.len() == descriptions.len() && data_ids.len() == ctx.remaining_accounts.len(),
        ProxyError::ArrayLengthMismatch
    );
    let state_key = ctx.accounts.state.key();
    let mut feed: Box<Feed> = bytemuck::zeroed_box();
    for ((id, description), info) in data_ids
        .iter()
        .zip(descriptions.iter())
        .zip(ctx.remaining_accounts.iter())
    {
        load_feed_into(info, &state_key, &data_id_seed(id), &mut feed)?;
        feed.description = encode_description(description)?;
        store_feed(info, &feed)?;
    }
    Ok(())
}

/// Freeze or unfreeze reads of feeds (remaining accounts: the feeds). Writes continue either way.
pub fn set_feed_frozen<'info>(
    ctx: Context<'_, '_, 'info, 'info, WriterOnly<'info>>,
    data_ids: Vec<[u8; 16]>,
    frozen: bool,
) -> Result<()> {
    require_eq!(
        data_ids.len(),
        ctx.remaining_accounts.len(),
        ProxyError::ArrayLengthMismatch
    );
    let state_key = ctx.accounts.state.key();
    let mut feed: Box<Feed> = bytemuck::zeroed_box();
    for (id, info) in data_ids.iter().zip(ctx.remaining_accounts.iter()) {
        load_feed_into(info, &state_key, &data_id_seed(id), &mut feed)?;
        feed.frozen = frozen as u8;
        store_feed(info, &feed)?;
    }
    Ok(())
}

/// Grow the rings of feeds (remaining accounts: the feeds), `payer` topping up their rent. A ring
/// only grows, only before it wraps, and by at most 10KiB (320 rounds) per instruction.
pub fn set_max_stored_reports<'info>(
    ctx: Context<'_, '_, 'info, 'info, WriterPayer<'info>>,
    data_ids: Vec<[u8; 16]>,
    max_stored_reports: Vec<u64>,
) -> Result<()> {
    require!(
        data_ids.len() == max_stored_reports.len()
            && data_ids.len() == ctx.remaining_accounts.len(),
        ProxyError::ArrayLengthMismatch
    );
    let state_key = ctx.accounts.state.key();
    let payer = ctx.accounts.payer.to_account_info();
    let rent = Rent::get()?;
    let mut feed: Box<Feed> = bytemuck::zeroed_box();
    for ((id, new_max), info) in data_ids
        .iter()
        .zip(max_stored_reports.iter().copied())
        .zip(ctx.remaining_accounts.iter())
    {
        load_feed_into(info, &state_key, &data_id_seed(id), &mut feed)?;
        check_max_stored_reports(&feed, new_max).map_err(|e| match e {
            MaxStoredReportsError::Invalid => ProxyError::InvalidMaxStoredReports,
            MaxStoredReportsError::NotIncreased => ProxyError::MaxStoredReportsNotIncreased,
            MaxStoredReportsError::Locked => ProxyError::MaxStoredReportsLocked,
            MaxStoredReportsError::TooLow => ProxyError::MaxStoredReportsTooLow,
        })?;

        let old_len = info.data_len();
        let new_len = feed_account_len(new_max);
        require!(
            new_len > old_len && new_len - old_len <= MAX_ACCOUNT_DATA_INCREASE,
            ProxyError::InvalidMaxStoredReports
        );
        let top_up = rent
            .minimum_balance(new_len)
            .saturating_sub(info.lamports());
        if top_up > 0 {
            anchor_lang::system_program::transfer(
                CpiContext::new(
                    ctx.accounts.system_program.to_account_info(),
                    anchor_lang::system_program::Transfer {
                        from: payer.clone(),
                        to: info.clone(),
                    },
                ),
                top_up,
            )?;
        }
        info.resize(new_len)?;
        info.try_borrow_mut_data()?[old_len..].fill(0);

        feed.max_stored_reports = new_max;
        store_feed(info, &feed)?;
    }
    Ok(())
}

/// Append one round per feed (remaining accounts: the feeds, writable, in order; a feed may appear
/// more than once). Each round must be newer than its feed's latest: the store checks that first,
/// this is the backstop, so a stale round fails the whole write.
pub fn write_rounds<'info>(
    ctx: Context<'_, '_, 'info, 'info, WriterOnly<'info>>,
    rounds: Vec<NewRound>,
) -> Result<()> {
    require_eq!(
        rounds.len(),
        ctx.remaining_accounts.len(),
        ProxyError::ArrayLengthMismatch
    );
    let state_key = ctx.accounts.state.key();
    let clock = Clock::get()?;
    let now = Now {
        slot: clock.slot,
        timestamp: clock.unix_timestamp as u32,
    };
    let mut feed: Box<Feed> = bytemuck::zeroed_box();
    for (round, info) in rounds.iter().zip(ctx.remaining_accounts.iter()) {
        load_any_feed_into(info, &state_key, &mut feed)?;
        require!(
            round.observations_timestamp > feed.latest_observations_timestamp,
            ProxyError::StaleRound
        );
        require!(info.is_writable, ProxyError::AccountNotWritable);
        {
            let mut data = info.try_borrow_mut_data()?;
            let mut ring = RingMut::new(&mut data[..]);
            logic::store_round(
                &mut feed,
                &mut ring,
                round.observations_timestamp,
                round.answer,
                now,
            );
        }
        store_feed(info, &feed)?;
    }
    Ok(())
}

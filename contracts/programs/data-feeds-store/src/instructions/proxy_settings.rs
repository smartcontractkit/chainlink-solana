//! Feed admin settings kept in the proxy's feed accounts (remaining accounts: the data IDs' proxy
//! feed PDAs, in order), made by the store's `feed_writer` through the proxy. A feed that doesn't
//! exist yet is created first, the feed admin paying, so a data ID can be configured before its
//! first report.

use anchor_lang::prelude::*;
use data_feeds_proxy::cpi::accounts::{WriterOnly, WriterPayer};

use crate::{
    common::FEED_WRITER_SEED,
    context::FeedAdminProxy,
    data_id,
    error::StoreError,
    event::*,
    utils::{is_uninitialized, validate_data_id, verify_feed_admin, EventEmitter},
};

/// The proxy feed address of a data ID under the store's proxy, and its bump.
pub fn proxy_feed_address(
    proxy_program: &Pubkey,
    proxy_state: &Pubkey,
    id: &[u8; 16],
) -> (Pubkey, u8) {
    Pubkey::find_program_address(
        &[
            data_feeds_proxy::common::FEED_SEED,
            proxy_state.as_ref(),
            &data_id::to_seed(id),
        ],
        proxy_program,
    )
}

/// Check the admin and the feed addresses, and create the feeds that don't exist yet.
fn ensure_feeds<'info>(
    ctx: &Context<'_, '_, 'info, 'info, FeedAdminProxy<'info>>,
    data_ids: &[[u8; 16]],
    events: &EventEmitter<'info>,
) -> Result<()> {
    let state = ctx.accounts.state.load()?;
    verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;
    require_eq!(
        data_ids.len(),
        ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );
    let (proxy_program, proxy_state) = (state.proxy_program, state.proxy_state);
    let mut bumps = Vec::with_capacity(data_ids.len());
    let mut created = Vec::with_capacity(data_ids.len());
    for (id, info) in data_ids.iter().zip(ctx.remaining_accounts.iter()) {
        validate_data_id(id)?;
        let (address, bump) = proxy_feed_address(&proxy_program, &proxy_state, id);
        require_keys_eq!(address, *info.key, StoreError::AccountMismatch);
        bumps.push(bump);
        if is_uninitialized(info) {
            created.push(*id);
        }
    }
    let state_key = ctx.accounts.state.key();
    let writer_seeds: &[&[u8]] = &[
        FEED_WRITER_SEED,
        state_key.as_ref(),
        &[state.feed_writer_bump],
    ];
    let max_stored_reports = state.default_max_stored_reports;
    drop(state);
    data_feeds_proxy::cpi::create_feeds(
        CpiContext::new_with_signer(
            ctx.accounts.proxy_program.to_account_info(),
            WriterPayer {
                writer: ctx.accounts.feed_writer.to_account_info(),
                state: ctx.accounts.proxy_state.to_account_info(),
                payer: ctx.accounts.feed_admin.to_account_info(),
                system_program: ctx.accounts.system_program.to_account_info(),
            },
            &[writer_seeds],
        )
        .with_remaining_accounts(ctx.remaining_accounts.to_vec()),
        data_ids.to_vec(),
        bumps,
        max_stored_reports,
    )?;
    if !created.is_empty() {
        events.emit(&FeedsCreated {
            state: state_key,
            data_ids: created,
            max_stored_reports,
        })?;
    }
    Ok(())
}

fn writer_only<'info>(
    ctx: &Context<'_, '_, 'info, 'info, FeedAdminProxy<'info>>,
) -> WriterOnly<'info> {
    WriterOnly {
        writer: ctx.accounts.feed_writer.to_account_info(),
        state: ctx.accounts.proxy_state.to_account_info(),
    }
}

fn events<'info>(
    ctx: &Context<'_, '_, 'info, 'info, FeedAdminProxy<'info>>,
) -> EventEmitter<'info> {
    EventEmitter::new(
        ctx.accounts.event_authority.to_account_info(),
        ctx.bumps.event_authority,
    )
}

/// Set feed descriptions, e.g. "ETH / USD", an empty string clears one.
pub fn set_feed_descriptions<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminProxy<'info>>,
    data_ids: Vec<[u8; 16]>,
    descriptions: Vec<String>,
) -> Result<()> {
    require_eq!(
        data_ids.len(),
        descriptions.len(),
        StoreError::ArrayLengthMismatch
    );
    let events = events(&ctx);
    ensure_feeds(&ctx, &data_ids, &events)?;
    let state_key = ctx.accounts.state.key();
    let bump = ctx.accounts.state.load()?.feed_writer_bump;
    data_feeds_proxy::cpi::set_feed_descriptions(
        CpiContext::new_with_signer(
            ctx.accounts.proxy_program.to_account_info(),
            writer_only(&ctx),
            &[&[FEED_WRITER_SEED, state_key.as_ref(), &[bump]]],
        )
        .with_remaining_accounts(ctx.remaining_accounts.to_vec()),
        data_ids.clone(),
        descriptions.clone(),
    )?;
    for (id, description) in data_ids.into_iter().zip(descriptions) {
        events.emit(&FeedDescriptionSet {
            state: state_key,
            data_id: id,
            description,
        })?;
    }
    Ok(())
}

/// Freeze or unfreeze reads of feeds: frozen feeds refuse proxy reads, writes continue.
pub fn set_feed_frozen<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminProxy<'info>>,
    data_ids: Vec<[u8; 16]>,
    frozen: bool,
) -> Result<()> {
    let events = events(&ctx);
    ensure_feeds(&ctx, &data_ids, &events)?;
    let state_key = ctx.accounts.state.key();
    let bump = ctx.accounts.state.load()?.feed_writer_bump;
    data_feeds_proxy::cpi::set_feed_frozen(
        CpiContext::new_with_signer(
            ctx.accounts.proxy_program.to_account_info(),
            writer_only(&ctx),
            &[&[FEED_WRITER_SEED, state_key.as_ref(), &[bump]]],
        )
        .with_remaining_accounts(ctx.remaining_accounts.to_vec()),
        data_ids.clone(),
        frozen,
    )?;
    for id in data_ids {
        events.emit(&FeedFrozenSet {
            state: state_key,
            data_id: id,
            frozen,
        })?;
    }
    Ok(())
}

/// Grow the number of rounds feeds keep: only up, only before a feed's ring wraps, at most 10KiB
/// (320 rounds) per feed per instruction.
pub fn set_max_stored_reports<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminProxy<'info>>,
    data_ids: Vec<[u8; 16]>,
    max_stored_reports: Vec<u64>,
) -> Result<()> {
    require_eq!(
        data_ids.len(),
        max_stored_reports.len(),
        StoreError::ArrayLengthMismatch
    );
    let events = events(&ctx);
    ensure_feeds(&ctx, &data_ids, &events)?;
    let state_key = ctx.accounts.state.key();
    let bump = ctx.accounts.state.load()?.feed_writer_bump;
    data_feeds_proxy::cpi::set_max_stored_reports(
        CpiContext::new_with_signer(
            ctx.accounts.proxy_program.to_account_info(),
            WriterPayer {
                writer: ctx.accounts.feed_writer.to_account_info(),
                state: ctx.accounts.proxy_state.to_account_info(),
                payer: ctx.accounts.feed_admin.to_account_info(),
                system_program: ctx.accounts.system_program.to_account_info(),
            },
            &[&[FEED_WRITER_SEED, state_key.as_ref(), &[bump]]],
        )
        .with_remaining_accounts(ctx.remaining_accounts.to_vec()),
        data_ids.clone(),
        max_stored_reports.clone(),
    )?;
    for (id, max) in data_ids.into_iter().zip(max_stored_reports) {
        events.emit(&MaxStoredReportsSet {
            state: state_key,
            data_id: id,
            max_stored_reports: max,
        })?;
    }
    Ok(())
}

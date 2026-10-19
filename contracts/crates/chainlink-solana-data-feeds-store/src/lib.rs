//! CPI accessors for the Chainlink Data Feeds store. Reads go through the store's read
//! instructions, so SVR path selection and per-proxy decimals apply.
//!
//! Proxy reads take the proxy config account, the non-SVR feed account of its data ID and, for SVR
//! data IDs, the data ID config account.

use anchor_lang::prelude::*;
use data_feeds_store::cpi::accounts::{DirectRead, ProxyRead};
use data_feeds_store::state::RoundData;

pub use data_feeds_store::ID as DATA_FEEDS_STORE_ID;

fn proxy_read<'info>(
    proxy: AccountInfo<'info>,
    feed: AccountInfo<'info>,
    data_id_config: Option<AccountInfo<'info>>,
) -> ProxyRead<'info> {
    ProxyRead {
        proxy,
        feed,
        data_id_config,
        feed_description: None,
        svr_path_description: None,
    }
}

/// Latest round data for a proxy, answer downscaled to the proxy's decimals.
pub fn latest_round_data<'info>(
    store_program: AccountInfo<'info>,
    proxy: AccountInfo<'info>,
    feed: AccountInfo<'info>,
    data_id_config: Option<AccountInfo<'info>>,
) -> Result<RoundData> {
    let ctx = CpiContext::new(store_program, proxy_read(proxy, feed, data_id_config));
    Ok(data_feeds_store::cpi::latest_round_data(ctx)?.get())
}

/// Round data for a shared-history round, zero when the proxy cannot be served it.
pub fn get_round_data<'info>(
    store_program: AccountInfo<'info>,
    proxy: AccountInfo<'info>,
    feed: AccountInfo<'info>,
    data_id_config: Option<AccountInfo<'info>>,
    round_id: u64,
) -> Result<RoundData> {
    let ctx = CpiContext::new(store_program, proxy_read(proxy, feed, data_id_config));
    Ok(data_feeds_store::cpi::get_round_data(ctx, round_id)?.get())
}

/// Decimals the proxy serves.
pub fn decimals<'info>(
    store_program: AccountInfo<'info>,
    proxy: AccountInfo<'info>,
    feed: AccountInfo<'info>,
) -> Result<u8> {
    let ctx = CpiContext::new(store_program, proxy_read(proxy, feed, None));
    Ok(data_feeds_store::cpi::decimals(ctx)?.get())
}

/// Latest round data for any data ID, bypassing proxy configuration.
#[allow(clippy::too_many_arguments)]
pub fn get_latest_round_data<'info>(
    store_program: AccountInfo<'info>,
    state: AccountInfo<'info>,
    feed: AccountInfo<'info>,
    data_id_config: Option<AccountInfo<'info>>,
    data_id: [u8; 16],
    decimals: u8,
    fallback_time: u32,
    max_sync_iterations: u32,
) -> Result<RoundData> {
    let ctx = CpiContext::new(
        store_program,
        DirectRead {
            state,
            feed,
            data_id_config,
        },
    );
    Ok(data_feeds_store::cpi::get_latest_round_data(
        ctx,
        data_id,
        decimals,
        fallback_time,
        max_sync_iterations,
    )?
    .get())
}

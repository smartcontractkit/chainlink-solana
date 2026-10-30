//! CPI accessors for the Chainlink Data Feeds proxy. One proxy serves every feed: pass the data ID
//! and, for `read_scaled`, the decimals you want.
//!
//! The proxy address (its program id and state account) is what stays stable. A read also passes
//! the accounts of the store behind the proxy: its program, its state and the data ID's feed, which
//! clients resolve from the proxy state (`store_program`, `store_state`).

use anchor_lang::prelude::*;
use data_feeds_proxy::cpi::accounts::{Read, ReadScaled};

pub use data_feeds_proxy::state::Round;
pub use data_feeds_proxy::ID as DATA_FEEDS_PROXY_ID;

/// The latest round of a data ID, or the round with `round_id`, with the store's 18 decimals.
pub fn read<'info>(
    proxy_program: AccountInfo<'info>,
    proxy_state: AccountInfo<'info>,
    store_program: AccountInfo<'info>,
    store_state: AccountInfo<'info>,
    feed: AccountInfo<'info>,
    data_id: [u8; 16],
    round_id: Option<u64>,
) -> Result<Round> {
    let ctx = CpiContext::new(
        proxy_program,
        Read {
            state: proxy_state,
            store_program,
            store_state,
            feed,
        },
    );
    Ok(data_feeds_proxy::cpi::read(ctx, data_id, round_id)?.get())
}

/// Like [`read`], with the answer at `decimals`: at least the data ID's minimum (18 unless an admin
/// lowered it, pass its `feed_config` account to use a lower one) and at most 18. Fails if a
/// non-zero answer would round to zero.
#[allow(clippy::too_many_arguments)]
pub fn read_scaled<'info>(
    proxy_program: AccountInfo<'info>,
    proxy_state: AccountInfo<'info>,
    store_program: AccountInfo<'info>,
    store_state: AccountInfo<'info>,
    feed: AccountInfo<'info>,
    feed_config: Option<AccountInfo<'info>>,
    data_id: [u8; 16],
    decimals: u8,
    round_id: Option<u64>,
) -> Result<Round> {
    let ctx = CpiContext::new(
        proxy_program,
        ReadScaled {
            state: proxy_state,
            store_program,
            store_state,
            feed,
            feed_config,
        },
    );
    Ok(data_feeds_proxy::cpi::read_scaled(ctx, data_id, decimals, round_id)?.get())
}

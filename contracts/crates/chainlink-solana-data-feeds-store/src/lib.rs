//! CPI client for the reader interface of the Chainlink Data Feeds store: the latest or a stored
//! round of a data ID (answer with 18 decimals), the latest rounds of several data IDs, the stored
//! decimals and the feed description.
//!
//! Proxies read through this interface, so a store version that keeps it can replace the store
//! behind a proxy. Reads that need other decimals go through the data feeds proxy, not here.

use anchor_lang::prelude::*;
use data_feeds_store::cpi::accounts::{DescriptionRead, FeedRead, FeedsRead};

pub use data_feeds_store::state::ReaderRound;
pub use data_feeds_store::ID as DATA_FEEDS_STORE_ID;

/// Latest round of a data ID. Fails with the store's `NoDataPresent` while the feed has no report.
pub fn get_latest_round<'info>(
    store_program: AccountInfo<'info>,
    state: AccountInfo<'info>,
    feed: AccountInfo<'info>,
    data_id: [u8; 16],
) -> Result<ReaderRound> {
    let ctx = CpiContext::new(store_program, FeedRead { state, feed });
    Ok(data_feeds_store::cpi::get_latest_round(ctx, data_id)?.get())
}

/// Latest rounds of up to 31 data IDs, `feeds` in the same order. A feed without a report yields
/// a zero round (round ID 0).
pub fn get_latest_rounds<'info>(
    store_program: AccountInfo<'info>,
    state: AccountInfo<'info>,
    feeds: Vec<AccountInfo<'info>>,
    data_ids: Vec<[u8; 16]>,
) -> Result<Vec<ReaderRound>> {
    let ctx = CpiContext::new(store_program, FeedsRead { state }).with_remaining_accounts(feeds);
    Ok(data_feeds_store::cpi::get_latest_rounds(ctx, data_ids)?.get())
}

/// A stored round of a data ID. Fails with the store's `NoDataPresent` for a round that was never
/// written or has been overwritten.
pub fn get_round<'info>(
    store_program: AccountInfo<'info>,
    state: AccountInfo<'info>,
    feed: AccountInfo<'info>,
    data_id: [u8; 16],
    round_id: u64,
) -> Result<ReaderRound> {
    let ctx = CpiContext::new(store_program, FeedRead { state, feed });
    Ok(data_feeds_store::cpi::get_round(ctx, data_id, round_id)?.get())
}

/// Decimals the store keeps answers with.
pub fn get_decimals<'info>(
    store_program: AccountInfo<'info>,
    state: AccountInfo<'info>,
    data_id: [u8; 16],
) -> Result<u8> {
    let ctx = CpiContext::new(
        store_program,
        DescriptionRead {
            state,
            feed_description: None,
        },
    );
    Ok(data_feeds_store::cpi::get_decimals(ctx, data_id)?.get())
}

/// Description of the feed of a data ID, empty when none is set. Pass the feed description
/// account to get it.
pub fn get_description<'info>(
    store_program: AccountInfo<'info>,
    state: AccountInfo<'info>,
    feed_description: Option<AccountInfo<'info>>,
    data_id: [u8; 16],
) -> Result<String> {
    let ctx = CpiContext::new(
        store_program,
        DescriptionRead {
            state,
            feed_description,
        },
    );
    Ok(data_feeds_store::cpi::get_description(ctx, data_id)?.get())
}

use anchor_lang::prelude::*;
use data_feeds_cache::state::WorkflowMetadata;
use solana_program::account_info::AccountInfo;

/// The data ids passed in must match the ordering of the decimal reports input.
/// Developer is responsible for deriving the correct PDAs for the report before calling the function
pub fn query_values<'info>(
    data_cache_program: AccountInfo<'info>,
    cache_state: AccountInfo<'info>,
    data_ids: Vec<[u8; 16]>,
    decimal_reports: Vec<AccountInfo<'info>>,
) -> Result<Vec<data_feeds_cache::state::DecimalReport>> {
    let cpi_accounts = data_feeds_cache::cpi::accounts::QueryValues { cache_state };

    let cpi_ctx =
        CpiContext::new(data_cache_program, cpi_accounts).with_remaining_accounts(decimal_reports);

    let values = data_feeds_cache::cpi::query_values(cpi_ctx, data_ids)?.get();

    Ok(values)
}

/// Returns a feed's workflow metadata. Chunks return values by `max_count`.
/// Can be used on-chain to verify a feed's configuration.
/// If `start_index` is out of bounds the function will return an empty array.
/// If `max_count = 0` then function will return the entire workflow metadata list.
pub fn query_feed_metadata<'info>(
    data_cache_program: AccountInfo<'info>,
    cache_state: AccountInfo<'info>,
    feed_config: AccountInfo<'info>,
    data_id: [u8; 16],
    start_index: u8,
    max_count: u8,
) -> Result<Vec<WorkflowMetadata>> {
    let cpi_accounts = data_feeds_cache::cpi::accounts::QueryFeedMetadata {
        cache_state,
        feed_config,
    };

    let cpi_ctx = CpiContext::new(data_cache_program, cpi_accounts);

    let values =
        data_feeds_cache::cpi::query_feed_metadata(cpi_ctx, data_id, start_index, max_count)?.get();

    Ok(values)
}

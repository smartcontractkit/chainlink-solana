//! Reads through the proxy. Every read calls the configured store's reader interface and returns
//! its round, `read_scaled` after rescaling the answer.

use anchor_lang::prelude::*;
use chainlink_solana_data_feeds_store::{get_latest_round, get_round};

use crate::{
    common::STORE_DECIMALS,
    context::{Read, ReadScaled},
    error::ProxyError,
    state::Round,
};

fn fetch<'info>(
    store_program: &UncheckedAccount<'info>,
    store_state: &UncheckedAccount<'info>,
    feed: &UncheckedAccount<'info>,
    data_id: [u8; 16],
    round_id: Option<u64>,
) -> Result<Round> {
    let (program, state, feed) = (
        store_program.to_account_info(),
        store_state.to_account_info(),
        feed.to_account_info(),
    );
    let round = match round_id {
        None => get_latest_round(program, state, feed, data_id)?,
        Some(round_id) => get_round(program, state, feed, data_id, round_id)?,
    };
    Ok(round.into())
}

/// Rescale a stored answer to `decimals`, truncating toward zero. A non-zero answer that would
/// become zero is an error, so a reader never mistakes a tiny price for none.
pub fn scale(answer: i128, decimals: u8) -> Result<i128> {
    let scaled = answer / 10i128.pow(u32::from(STORE_DECIMALS - decimals));
    require!(answer == 0 || scaled != 0, ProxyError::RoundsToZero);
    Ok(scaled)
}

/// The latest round of a data ID, or the given round, at the store's decimals.
pub fn read(ctx: Context<Read>, data_id: [u8; 16], round_id: Option<u64>) -> Result<Round> {
    let accounts = &ctx.accounts;
    fetch(
        &accounts.store_program,
        &accounts.store_state,
        &accounts.feed,
        data_id,
        round_id,
    )
}

/// The latest round of a data ID, or the given round, with the answer at `decimals`.
pub fn read_scaled(
    ctx: Context<ReadScaled>,
    data_id: [u8; 16],
    decimals: u8,
    round_id: Option<u64>,
) -> Result<Round> {
    let accounts = &ctx.accounts;
    let min_decimals = crate::admin::min_decimals(accounts.feed_config.as_deref())?;
    require!(
        decimals >= min_decimals && decimals <= STORE_DECIMALS,
        ProxyError::InvalidDecimals
    );

    let mut round = fetch(
        &accounts.store_program,
        &accounts.store_state,
        &accounts.feed,
        data_id,
        round_id,
    )?;
    round.answer = scale(round.answer, decimals)?;
    Ok(round)
}

#[cfg(test)]
mod tests {
    use super::*;

    const E18: i128 = 1_000_000_000_000_000_000;

    #[test]
    fn scales_and_truncates_toward_zero() {
        assert_eq!(
            scale(1_234_567_891 * E18 / 1_000_000_000, 8).unwrap(),
            123_456_789
        );
        assert_eq!(scale(-15 * E18 / 10, 0).unwrap(), -1);
        assert_eq!(scale(42, 18).unwrap(), 42);
        assert_eq!(scale(0, 8).unwrap(), 0, "zero stays zero");
    }

    #[test]
    fn a_non_zero_answer_never_rounds_to_zero() {
        assert!(scale(1, 8).is_err());
        assert!(scale(-1, 8).is_err());
        assert!(scale(E18 / 10, 0).is_err(), "0.1 at 0 decimals");
        assert_eq!(
            scale(10_000_000_000, 8).unwrap(),
            1,
            "exactly one unit survives"
        );
    }
}

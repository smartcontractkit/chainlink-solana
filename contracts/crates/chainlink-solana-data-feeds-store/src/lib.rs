//! CPI client for the reader interface of the Chainlink Data Feeds store: the latest or a stored
//! round of a data ID (answer with 18 decimals), the latest rounds of several data IDs, the stored
//! decimals and the feed description.
//!
//! Proxies read through this interface, so a store version that keeps it can replace the store
//! behind a proxy. Reads that need other decimals go through the data feeds proxy, not here.
//!
//! The store program, its state and the feed accounts come from the caller. Pin the store program
//! and state you trust (and check `feed` is the data ID's feed of that state, or let the store do
//! it): any program can answer a read through this interface. Each read takes the answer only from
//! the program it called and fails, rather than panics, on a missing or malformed answer.

use anchor_lang::prelude::*;
use anchor_lang::solana_program::program::get_return_data;
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
    let program = store_program.key();
    let ctx = CpiContext::new(store_program, FeedRead { state, feed });
    data_feeds_store::cpi::get_latest_round(ctx, data_id)?;
    returned(&program)
}

/// Latest rounds of up to 31 data IDs, `feeds` in the same order. A feed without a report yields
/// a zero round (round ID 0).
pub fn get_latest_rounds<'info>(
    store_program: AccountInfo<'info>,
    state: AccountInfo<'info>,
    feeds: Vec<AccountInfo<'info>>,
    data_ids: Vec<[u8; 16]>,
) -> Result<Vec<ReaderRound>> {
    let program = store_program.key();
    let ctx = CpiContext::new(store_program, FeedsRead { state }).with_remaining_accounts(feeds);
    data_feeds_store::cpi::get_latest_rounds(ctx, data_ids)?;
    returned(&program)
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
    let program = store_program.key();
    let ctx = CpiContext::new(store_program, FeedRead { state, feed });
    data_feeds_store::cpi::get_round(ctx, data_id, round_id)?;
    returned(&program)
}

/// Decimals the store keeps answers with.
pub fn get_decimals<'info>(
    store_program: AccountInfo<'info>,
    state: AccountInfo<'info>,
    data_id: [u8; 16],
) -> Result<u8> {
    let program = store_program.key();
    let ctx = CpiContext::new(
        store_program,
        DescriptionRead {
            state,
            feed_description: None,
        },
    );
    data_feeds_store::cpi::get_decimals(ctx, data_id)?;
    returned(&program)
}

/// Description of the feed of a data ID, empty when none is set. Pass the feed description
/// account to get it.
pub fn get_description<'info>(
    store_program: AccountInfo<'info>,
    state: AccountInfo<'info>,
    feed_description: Option<AccountInfo<'info>>,
    data_id: [u8; 16],
) -> Result<String> {
    let program = store_program.key();
    let ctx = CpiContext::new(
        store_program,
        DescriptionRead {
            state,
            feed_description,
        },
    );
    data_feeds_store::cpi::get_description(ctx, data_id)?;
    returned(&program)
}

/// The return data of the last call, which must come from `program`.
fn returned<T: AnchorDeserialize>(program: &Pubkey) -> Result<T> {
    match get_return_data() {
        Some((from, data)) if from == *program => T::try_from_slice(&data).map_err(|_| {
            msg!("Malformed answer from {}", program);
            ProgramError::InvalidAccountData.into()
        }),
        _ => {
            msg!("No answer from {}", program);
            Err(ProgramError::IncorrectProgramId.into())
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use anchor_lang::solana_program::program_stubs::{set_syscall_stubs, SyscallStubs};
    use std::sync::Mutex;

    static RETURN_DATA: Mutex<Option<(Pubkey, Vec<u8>)>> = Mutex::new(None);

    struct Stubs;
    impl SyscallStubs for Stubs {
        fn sol_get_return_data(&self) -> Option<(Pubkey, Vec<u8>)> {
            RETURN_DATA.lock().unwrap().clone()
        }
    }

    #[test]
    fn takes_the_answer_only_from_the_called_program() {
        set_syscall_stubs(Box::new(Stubs));
        let store = Pubkey::new_unique();
        let answer = |data: Option<(Pubkey, Vec<u8>)>| {
            *RETURN_DATA.lock().unwrap() = data;
            returned::<u8>(&store)
        };
        assert_eq!(answer(Some((store, vec![18]))).unwrap(), 18);
        assert_eq!(
            answer(None).unwrap_err(),
            ProgramError::IncorrectProgramId.into()
        );
        assert_eq!(
            answer(Some((Pubkey::new_unique(), vec![18]))).unwrap_err(),
            ProgramError::IncorrectProgramId.into()
        );
        assert_eq!(
            answer(Some((store, vec![]))).unwrap_err(),
            ProgramError::InvalidAccountData.into()
        );
    }
}

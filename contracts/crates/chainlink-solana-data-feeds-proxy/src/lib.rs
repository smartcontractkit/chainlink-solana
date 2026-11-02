//! CPI accessors for the Chainlink Data Feeds proxy. One proxy serves every feed: pass the data ID
//! and, for `read_scaled`, the decimals you want.
//!
//! The proxy address (its program id and state account) is what stays stable. A read also passes
//! the accounts of the store behind the proxy: its program, its state and the data ID's feed, which
//! clients resolve from the proxy state (`store_program`, `store_state`).
//!
//! Reads go only to the proxy program this crate is built for ([`DATA_FEEDS_PROXY_ID`]); anyone can
//! create a proxy state under it, so pin the proxy state you trust. The proxy checks the store
//! accounts against that state. Each read takes the answer only from the proxy and fails, rather
//! than panics, on a missing or malformed answer.
//!
//! Round IDs are those of the store behind the proxy: they restart if the proxy owner points it to
//! another store (the proxy emits `StoreSet`), so don't carry round IDs across that change.

use anchor_lang::prelude::*;
use anchor_lang::solana_program::program::get_return_data;
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
    require_keys_eq!(
        proxy_program.key(),
        DATA_FEEDS_PROXY_ID,
        ErrorCode::InvalidProgramId
    );
    let ctx = CpiContext::new(
        proxy_program,
        Read {
            state: proxy_state,
            store_program,
            store_state,
            feed,
        },
    );
    data_feeds_proxy::cpi::read(ctx, data_id, round_id)?;
    returned()
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
    require_keys_eq!(
        proxy_program.key(),
        DATA_FEEDS_PROXY_ID,
        ErrorCode::InvalidProgramId
    );
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
    data_feeds_proxy::cpi::read_scaled(ctx, data_id, decimals, round_id)?;
    returned()
}

/// The return data of the last call, which must come from the proxy.
fn returned() -> Result<Round> {
    match get_return_data() {
        Some((from, data)) if from == DATA_FEEDS_PROXY_ID => {
            Round::try_from_slice(&data).map_err(|_| {
                msg!("Malformed answer from the proxy");
                ProgramError::InvalidAccountData.into()
            })
        }
        _ => {
            msg!("No answer from the proxy");
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
    fn takes_the_answer_only_from_the_proxy() {
        set_syscall_stubs(Box::new(Stubs));
        let answer = |data: Option<(Pubkey, Vec<u8>)>| {
            *RETURN_DATA.lock().unwrap() = data;
            returned()
        };
        let round = Round {
            round_id: 7,
            answer: -42,
            observations_timestamp: 1_700_000_000,
            recorded_timestamp: 1_700_000_001,
        };
        let encoded = round.try_to_vec().unwrap();
        assert_eq!(
            answer(Some((DATA_FEEDS_PROXY_ID, encoded.clone()))).unwrap(),
            round
        );
        assert_eq!(
            answer(None).unwrap_err(),
            ProgramError::IncorrectProgramId.into()
        );
        assert_eq!(
            answer(Some((Pubkey::new_unique(), encoded.clone()))).unwrap_err(),
            ProgramError::IncorrectProgramId.into()
        );
        assert_eq!(
            answer(Some((DATA_FEEDS_PROXY_ID, encoded[..10].to_vec()))).unwrap_err(),
            ProgramError::InvalidAccountData.into()
        );
    }

    #[test]
    fn only_calls_the_proxy_it_is_built_for() {
        let key = Pubkey::new_unique();
        let mut lamports = 0;
        let mut data = vec![];
        let owner = Pubkey::default();
        let info = AccountInfo::new(
            &key,
            false,
            false,
            &mut lamports,
            &mut data,
            &owner,
            true,
            0,
        );
        // the program check comes before any account is used, so one account stands in for all
        let err = read(
            info.clone(),
            info.clone(),
            info.clone(),
            info.clone(),
            info,
            [0; 16],
            None,
        )
        .unwrap_err();
        assert!(format!("{err:?}").contains("InvalidProgramId"), "{err:?}");
    }
}

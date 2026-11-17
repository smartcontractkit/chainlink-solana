//! Readers for the Chainlink Data Feeds store on Solana. Each data ID has one feed account: pass it
//! with the decimals you want, at least the feed's minimum and at most 18.
//!
//! Two ways to read the same data:
//! - **Decode the feed account** ([`read_latest_round`], [`read_round`]): no call to another
//!   program. The account must be owned by the store program this crate is built for
//!   ([`DATA_FEEDS_STORE_ID`]) and belong to the store state you pass; only the program's upgrade
//!   authority creates store states, but pin the one you trust.
//! - **Call the store** ([`latest_round`], [`get_round`]): one call, the store checks the account.
//!
//! Both refuse a frozen feed, decimals below the feed's minimum, and a non-zero answer that would
//! round to zero. Round IDs keep increasing for the life of a feed.

use anchor_lang::prelude::*;
use anchor_lang::solana_program::program::get_return_data;
use data_feeds_store::{
    cpi::accounts::FeedRead,
    instructions::reads::to_round,
    logic,
    state::Feed,
    utils::{decode_feed_into, RingRef},
};

pub use data_feeds_store::common::STORED_DECIMALS;
/// The errors reads fail with, e.g. `FeedFrozen` or `NoDataPresent`.
pub use data_feeds_store::error::StoreError;
pub use data_feeds_store::logic::{scale, ScaleError};
pub use data_feeds_store::state::Round;
pub use data_feeds_store::utils::feed_address;
pub use data_feeds_store::ID as DATA_FEEDS_STORE_ID;

fn decode<'a>(
    feed: &'a AccountInfo,
    store_state: &Pubkey,
    data_id: &[u8; 16],
) -> Result<(Box<Feed>, std::cell::Ref<'a, &'a mut [u8]>)> {
    require_keys_eq!(*feed.owner, DATA_FEEDS_STORE_ID, StoreError::InvalidAccount);
    let data = feed.try_borrow_data()?;
    let mut header: Box<Feed> = bytemuck::zeroed_box();
    decode_feed_into(&data, store_state, data_id, &mut header)?;
    Ok((header, data))
}

/// The latest round of a data ID at `decimals`, decoded from its feed account.
pub fn read_latest_round(
    feed: &AccountInfo,
    store_state: &Pubkey,
    data_id: [u8; 16],
    decimals: u8,
) -> Result<Round> {
    let (header, _) = decode(feed, store_state, &data_id)?;
    to_round(&header, logic::latest_round(&header), decimals)
}

/// A stored round of a data ID at `decimals`, decoded from its feed account.
pub fn read_round(
    feed: &AccountInfo,
    store_state: &Pubkey,
    data_id: [u8; 16],
    round_id: u64,
    decimals: u8,
) -> Result<Round> {
    let (header, data) = decode(feed, store_state, &data_id)?;
    let round = logic::stored_round(round_id, &header, &RingRef::new(&data[..]));
    to_round(&header, round, decimals)
}

/// The latest round of a data ID at `decimals`, through a call to the store.
pub fn latest_round<'info>(
    store_program: AccountInfo<'info>,
    store_state: AccountInfo<'info>,
    feed: AccountInfo<'info>,
    data_id: [u8; 16],
    decimals: u8,
) -> Result<Round> {
    require_keys_eq!(
        store_program.key(),
        DATA_FEEDS_STORE_ID,
        ErrorCode::InvalidProgramId
    );
    let ctx = CpiContext::new(
        store_program,
        FeedRead {
            state: store_state,
            feed,
        },
    );
    data_feeds_store::cpi::latest_round(ctx, data_id, decimals)?;
    returned()
}

/// A stored round of a data ID at `decimals`, through a call to the store.
pub fn get_round<'info>(
    store_program: AccountInfo<'info>,
    store_state: AccountInfo<'info>,
    feed: AccountInfo<'info>,
    data_id: [u8; 16],
    round_id: u64,
    decimals: u8,
) -> Result<Round> {
    require_keys_eq!(
        store_program.key(),
        DATA_FEEDS_STORE_ID,
        ErrorCode::InvalidProgramId
    );
    let ctx = CpiContext::new(
        store_program,
        FeedRead {
            state: store_state,
            feed,
        },
    );
    data_feeds_store::cpi::get_round(ctx, data_id, round_id, decimals)?;
    returned()
}

/// The return data of the last call, which must come from the store.
fn returned() -> Result<Round> {
    match get_return_data() {
        Some((from, data)) if from == DATA_FEEDS_STORE_ID => {
            Round::try_from_slice(&data).map_err(|_| {
                msg!("Malformed answer from the store");
                ProgramError::InvalidAccountData.into()
            })
        }
        _ => {
            msg!("No answer from the store");
            Err(ProgramError::IncorrectProgramId.into())
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use anchor_lang::solana_program::program_stubs::{set_syscall_stubs, SyscallStubs};
    use anchor_lang::Discriminator;
    use data_feeds_store::common::{ACCOUNT_VERSION, ANCHOR_DISCRIMINATOR};
    use data_feeds_store::data_id::to_seed;
    use data_feeds_store::state::{feed_account_len, StoredReport, FEED_RING_OFFSET};
    use std::mem::size_of;
    use std::sync::Mutex;

    static RETURN_DATA: Mutex<Option<(Pubkey, Vec<u8>)>> = Mutex::new(None);

    struct Stubs;
    impl SyscallStubs for Stubs {
        fn sol_get_return_data(&self) -> Option<(Pubkey, Vec<u8>)> {
            RETURN_DATA.lock().unwrap().clone()
        }
    }

    #[test]
    fn takes_the_answer_only_from_the_store() {
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
            answer(Some((DATA_FEEDS_STORE_ID, encoded.clone()))).unwrap(),
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
            answer(Some((DATA_FEEDS_STORE_ID, encoded[..10].to_vec()))).unwrap_err(),
            ProgramError::InvalidAccountData.into()
        );
    }

    const E18: i128 = 1_000_000_000_000_000_000;
    const MAX: u64 = 4;

    /// A feed account with rounds `1..=rounds` (answers 100, 200, ... with 18 decimals) in a ring
    /// of 4, minimum read decimals 8.
    fn feed_account(state: &Pubkey, data_id: &[u8; 16], rounds: u64, frozen: bool) -> Vec<u8> {
        let mut data = vec![0u8; feed_account_len(MAX)];
        let mut header: Feed = bytemuck::Zeroable::zeroed();
        header.version = ACCOUNT_VERSION;
        header.frozen = frozen as u8;
        header.min_decimals = 8;
        header.data_id = to_seed(data_id);
        header.state = *state;
        header.max_stored_reports = MAX;
        for r in 1..=rounds {
            let report = StoredReport {
                answer: r as i128 * 100 * E18,
                observations_timestamp: r as u32 * 10,
                recorded_timestamp: r as u32 * 10 + 1,
                recorded_slot: r,
            };
            set_report(&mut data, r, &report);
            header.latest_answer = report.answer;
            header.latest_observations_timestamp = report.observations_timestamp;
            header.latest_recorded_timestamp = report.recorded_timestamp;
        }
        header.round_id = rounds;
        data[..ANCHOR_DISCRIMINATOR].copy_from_slice(Feed::DISCRIMINATOR);
        data[ANCHOR_DISCRIMINATOR..ANCHOR_DISCRIMINATOR + size_of::<Feed>()]
            .copy_from_slice(bytemuck::bytes_of(&header));
        data
    }

    fn set_report(data: &mut [u8], round: u64, report: &StoredReport) {
        let offset = FEED_RING_OFFSET + (round % MAX) as usize * size_of::<StoredReport>();
        data[offset..offset + size_of::<StoredReport>()]
            .copy_from_slice(bytemuck::bytes_of(report));
    }

    /// Run `f` on a feed account with the given data, owned by `owner`.
    fn with_account<T>(data: &mut [u8], owner: &Pubkey, f: impl FnOnce(&AccountInfo) -> T) -> T {
        let key = Pubkey::new_unique();
        let mut lamports = 0;
        let info = AccountInfo::new(&key, false, false, &mut lamports, data, owner, false, 0);
        f(&info)
    }

    fn err(e: StoreError) -> Error {
        e.into()
    }

    #[test]
    fn decodes_feed_accounts() {
        let state = Pubkey::new_unique();
        let id = [1u8; 16];
        let mut data = feed_account(&state, &id, 3, false);
        with_account(&mut data, &DATA_FEEDS_STORE_ID, |info| {
            let latest = read_latest_round(info, &state, id, 8).unwrap();
            assert_eq!((latest.round_id, latest.answer), (3, 300_00000000));
            let round = read_round(info, &state, id, 2, 18).unwrap();
            assert_eq!(
                (round.round_id, round.answer, round.observations_timestamp),
                (2, 200 * E18, 20)
            );
            let below = read_latest_round(info, &state, id, 7);
            assert_eq!(below.unwrap_err(), err(StoreError::InvalidDecimals));
            let above = read_latest_round(info, &state, id, 19);
            assert_eq!(above.unwrap_err(), err(StoreError::InvalidDecimals));
            let other_store = read_latest_round(info, &Pubkey::new_unique(), id, 8);
            assert_eq!(other_store.unwrap_err(), err(StoreError::AccountMismatch));
            let other_id = read_latest_round(info, &state, [2u8; 16], 8);
            assert_eq!(other_id.unwrap_err(), err(StoreError::AccountMismatch));
            let future = read_round(info, &state, id, 4, 8);
            assert_eq!(future.unwrap_err(), err(StoreError::NoDataPresent));
        });
    }

    #[test]
    fn serves_only_rounds_in_the_ring() {
        let state = Pubkey::new_unique();
        let id = [1u8; 16];
        let mut empty = feed_account(&state, &id, 0, false);
        with_account(&mut empty, &DATA_FEEDS_STORE_ID, |info| {
            let none = read_latest_round(info, &state, id, 8);
            assert_eq!(none.unwrap_err(), err(StoreError::NoDataPresent));
        });
        // rounds 1..=5 in a ring of 4: round 1 was overwritten by round 5
        let mut wrapped = feed_account(&state, &id, 5, false);
        with_account(&mut wrapped, &DATA_FEEDS_STORE_ID, |info| {
            let overwritten = read_round(info, &state, id, 1, 8);
            assert_eq!(overwritten.unwrap_err(), err(StoreError::NoDataPresent));
            assert_eq!(
                read_round(info, &state, id, 2, 18).unwrap().answer,
                200 * E18
            );
            assert_eq!(read_latest_round(info, &state, id, 18).unwrap().round_id, 5);
        });
    }

    #[test]
    fn refuses_an_answer_that_would_read_as_zero() {
        let state = Pubkey::new_unique();
        let id = [1u8; 16];
        let mut data = feed_account(&state, &id, 3, false);
        let tiny = StoredReport {
            answer: 1,
            ..Default::default()
        };
        set_report(&mut data, 3, &tiny);
        with_account(&mut data, &DATA_FEEDS_STORE_ID, |info| {
            let round = read_round(info, &state, id, 3, 8);
            assert_eq!(round.unwrap_err(), err(StoreError::RoundsToZero));
            assert_eq!(read_round(info, &state, id, 3, 18).unwrap().answer, 1);
        });
    }

    #[test]
    fn refuses_frozen_feeds() {
        let state = Pubkey::new_unique();
        let id = [1u8; 16];
        let mut data = feed_account(&state, &id, 3, true);
        with_account(&mut data, &DATA_FEEDS_STORE_ID, |info| {
            let latest = read_latest_round(info, &state, id, 8);
            assert_eq!(latest.unwrap_err(), err(StoreError::FeedFrozen));
            let round = read_round(info, &state, id, 2, 8);
            assert_eq!(round.unwrap_err(), err(StoreError::FeedFrozen));
        });
    }

    #[test]
    fn refuses_accounts_that_are_not_feeds_of_this_store_program() {
        let state = Pubkey::new_unique();
        let id = [1u8; 16];
        let read = |data: &mut Vec<u8>, owner: &Pubkey| {
            with_account(data, owner, |info| read_latest_round(info, &state, id, 8))
        };

        let mut data = feed_account(&state, &id, 3, false);
        let foreign = read(&mut data, &Pubkey::new_unique());
        assert_eq!(foreign.unwrap_err(), err(StoreError::InvalidAccount));

        let mut data = feed_account(&state, &id, 3, false);
        data[0] ^= 1;
        let discriminator = read(&mut data, &DATA_FEEDS_STORE_ID);
        assert_eq!(discriminator.unwrap_err(), err(StoreError::InvalidAccount));

        let mut data = feed_account(&state, &id, 3, false);
        data[ANCHOR_DISCRIMINATOR] = ACCOUNT_VERSION + 1;
        let version = read(&mut data, &DATA_FEEDS_STORE_ID);
        assert_eq!(version.unwrap_err(), err(StoreError::InvalidAccountVersion));

        // a header claiming a ring the account doesn't hold
        let mut data = feed_account(&state, &id, 3, false);
        data.truncate(feed_account_len(MAX) - 1);
        let short = read(&mut data, &DATA_FEEDS_STORE_ID);
        assert_eq!(short.unwrap_err(), err(StoreError::AccountMismatch));
    }

    #[test]
    fn calls_only_the_store_program() {
        let (state, feed) = (Pubkey::new_unique(), Pubkey::new_unique());
        let other = Pubkey::new_unique();
        let (mut l1, mut l2, mut l3) = (0, 0, 0);
        let (mut d1, mut d2, mut d3) = (vec![], vec![], vec![]);
        let program = AccountInfo::new(&other, false, false, &mut l1, &mut d1, &other, true, 0);
        let state_info = AccountInfo::new(&state, false, false, &mut l2, &mut d2, &other, false, 0);
        let feed_info = AccountInfo::new(&feed, false, false, &mut l3, &mut d3, &other, false, 0);
        let invalid: Error = ErrorCode::InvalidProgramId.into();
        let latest = latest_round(
            program.clone(),
            state_info.clone(),
            feed_info.clone(),
            [1; 16],
            8,
        );
        assert_eq!(latest.unwrap_err(), invalid);
        let round = get_round(program, state_info, feed_info, [1; 16], 1, 8);
        assert_eq!(round.unwrap_err(), invalid);
    }
}

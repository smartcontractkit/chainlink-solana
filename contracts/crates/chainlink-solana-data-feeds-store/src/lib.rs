//! Readers for the Chainlink Data Feeds store on Solana. Each data ID has one feed account. You
//! pass it with the decimals you want, which must be at least the feed's minimum and at most 18.
//!
//! There are two ways to read the same data, and both take the store state you trust. Each
//! deployment of the store program is meant to hold one store, but the program accepts more than
//! one, so pin the state's address in your program rather than taking it from your caller.
//!
//! Every read the store program offers to readers is available both ways:
//!
//! | What it returns              | Decode the feed account | Call the store       |
//! |------------------------------|-------------------------|----------------------|
//! | The latest round             | [`read_latest_round`]   | [`latest_round`]     |
//! | A stored round               | [`read_round`]          | [`get_round`]        |
//! | The stored decimals (18)     | [`read_decimals`]       | [`decimals`]         |
//! | The feed's description       | [`read_description`]    | [`description`]      |
//! | The fewest decimals to read  | [`read_min_decimals`]   | [`get_min_decimals`] |
//!
//! - Decoding reads the feed account in your program, with no call to another program. Before it
//!   trusts the bytes, the crate checks that the account is owned by the store program this crate
//!   is built for ([`DATA_FEEDS_STORE_ID`]), carries the feed discriminator and version, and
//!   belongs to the store state and data ID you pass.
//! - Calling runs the store's own read instruction, which makes the same account checks. Before
//!   the call, the crate checks that the program is the store program and that the store state is
//!   the one you pinned (`expected_state`). After the call, it accepts the answer only if the
//!   store program returned it.
//!
//! Every read makes these account checks, whatever it returns, because a reader given another
//! store's or another data ID's feed would otherwise get that feed's price or description without
//! noticing.
//!
//! The round reads also refuse a frozen feed, decimals outside the feed's minimum to 18, a round
//! that isn't stored, and a non-zero answer that would round to zero. The other reads don't check
//! the read freeze, as in the store program, because they return no price. Round IDs keep
//! increasing for the life of a feed.
//!
//! The store's `check_feed_permission` instruction, which says whether a workflow may write a data
//! ID, is for tooling rather than readers, so the crate doesn't wrap it.

use anchor_lang::prelude::*;
use anchor_lang::solana_program::program::get_return_data;
use data_feeds_store::{
    cpi::accounts::FeedRead,
    instructions::reads::to_round,
    logic,
    state::Feed,
    utils::{decode_description, decode_feed_into, RingRef},
};

pub use data_feeds_store::common::STORED_DECIMALS;
/// The errors reads fail with, for example `FeedFrozen` or `NoDataPresent`.
pub use data_feeds_store::error::StoreError;
pub use data_feeds_store::logic::{scale, ScaleError};
pub use data_feeds_store::state::Round;
pub use data_feeds_store::utils::feed_address;
pub use data_feeds_store::ID as DATA_FEEDS_STORE_ID;

/// Checks that `feed` is a feed account of the store program for `store_state` and `data_id`, and
/// returns its header and its borrowed data.
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

/// Returns the latest round of a data ID at `decimals`, decoded from its feed account.
pub fn read_latest_round(
    feed: &AccountInfo,
    store_state: &Pubkey,
    data_id: [u8; 16],
    decimals: u8,
) -> Result<Round> {
    let (header, _) = decode(feed, store_state, &data_id)?;
    to_round(&header, logic::latest_round(&header), decimals)
}

/// Returns a stored round of a data ID at `decimals`, decoded from its feed account.
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

/// Returns the decimals the store keeps answers in, which is 18 and also the most a read can ask
/// for, after checking that `feed` is the data ID's feed account of `store_state`. It fails for a
/// data ID whose feed doesn't exist, as the store's `decimals` instruction does.
pub fn read_decimals(feed: &AccountInfo, store_state: &Pubkey, data_id: [u8; 16]) -> Result<u8> {
    decode(feed, store_state, &data_id)?;
    Ok(STORED_DECIMALS)
}

/// Returns the description of a data ID's feed, decoded from its feed account, or an empty string
/// when none is set.
pub fn read_description(
    feed: &AccountInfo,
    store_state: &Pubkey,
    data_id: [u8; 16],
) -> Result<String> {
    let (header, _) = decode(feed, store_state, &data_id)?;
    Ok(decode_description(&header.description))
}

/// Returns the fewest decimals a read of a data ID may ask for, decoded from its feed account.
pub fn read_min_decimals(
    feed: &AccountInfo,
    store_state: &Pubkey,
    data_id: [u8; 16],
) -> Result<u8> {
    let (header, _) = decode(feed, store_state, &data_id)?;
    Ok(header.min_decimals)
}

/// Checks, before a call to the store, that `store_program` is the store program and that
/// `store_state` is the state the caller pinned.
fn check_call(
    store_program: &AccountInfo,
    store_state: &AccountInfo,
    expected_state: &Pubkey,
) -> Result<()> {
    require_keys_eq!(
        store_program.key(),
        DATA_FEEDS_STORE_ID,
        ErrorCode::InvalidProgramId
    );
    require_keys_eq!(
        store_state.key(),
        *expected_state,
        StoreError::AccountMismatch
    );
    Ok(())
}

/// Returns the latest round of a data ID at `decimals` by calling the store program.
/// `expected_state` is the store state you trust; the call fails with `AccountMismatch` if
/// `store_state` is another one.
pub fn latest_round<'info>(
    store_program: AccountInfo<'info>,
    store_state: AccountInfo<'info>,
    expected_state: &Pubkey,
    feed: AccountInfo<'info>,
    data_id: [u8; 16],
    decimals: u8,
) -> Result<Round> {
    check_call(&store_program, &store_state, expected_state)?;
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

/// Returns a stored round of a data ID at `decimals` by calling the store program.
/// `expected_state` is the store state you trust; the call fails with `AccountMismatch` if
/// `store_state` is another one.
pub fn get_round<'info>(
    store_program: AccountInfo<'info>,
    store_state: AccountInfo<'info>,
    expected_state: &Pubkey,
    feed: AccountInfo<'info>,
    data_id: [u8; 16],
    round_id: u64,
    decimals: u8,
) -> Result<Round> {
    check_call(&store_program, &store_state, expected_state)?;
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

/// Returns the decimals the store keeps answers in (18) by calling the store program, which fails
/// for a data ID whose feed doesn't exist. `expected_state` is the store state you trust.
pub fn decimals<'info>(
    store_program: AccountInfo<'info>,
    store_state: AccountInfo<'info>,
    expected_state: &Pubkey,
    feed: AccountInfo<'info>,
    data_id: [u8; 16],
) -> Result<u8> {
    check_call(&store_program, &store_state, expected_state)?;
    let ctx = CpiContext::new(
        store_program,
        FeedRead {
            state: store_state,
            feed,
        },
    );
    data_feeds_store::cpi::decimals(ctx, data_id)?;
    returned()
}

/// Returns the description of a data ID's feed by calling the store program. `expected_state` is
/// the store state you trust.
pub fn description<'info>(
    store_program: AccountInfo<'info>,
    store_state: AccountInfo<'info>,
    expected_state: &Pubkey,
    feed: AccountInfo<'info>,
    data_id: [u8; 16],
) -> Result<String> {
    check_call(&store_program, &store_state, expected_state)?;
    let ctx = CpiContext::new(
        store_program,
        FeedRead {
            state: store_state,
            feed,
        },
    );
    data_feeds_store::cpi::description(ctx, data_id)?;
    returned()
}

/// Returns the fewest decimals a read of a data ID may ask for by calling the store program.
/// `expected_state` is the store state you trust.
pub fn get_min_decimals<'info>(
    store_program: AccountInfo<'info>,
    store_state: AccountInfo<'info>,
    expected_state: &Pubkey,
    feed: AccountInfo<'info>,
    data_id: [u8; 16],
) -> Result<u8> {
    check_call(&store_program, &store_state, expected_state)?;
    let ctx = CpiContext::new(
        store_program,
        FeedRead {
            state: store_state,
            feed,
        },
    );
    data_feeds_store::cpi::get_min_decimals(ctx, data_id)?;
    returned()
}

/// Decodes the value the store just returned. The return data must come from the store program,
/// so no other program can answer in its place.
fn returned<T: AnchorDeserialize>() -> Result<T> {
    match get_return_data() {
        Some((from, data)) if from == DATA_FEEDS_STORE_ID => {
            T::try_from_slice(&data).map_err(|_| {
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
    use data_feeds_store::common::{ANCHOR_DISCRIMINATOR, FEED_ACCOUNT_VERSION};
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
            returned::<Round>()
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

    /// Builds a feed account with rounds 1 to `rounds` (answers 100, 200, ... with 18 decimals) in
    /// a ring of 4, with a minimum of 8 read decimals.
    fn feed_account(state: &Pubkey, data_id: &[u8; 16], rounds: u64, frozen: bool) -> Vec<u8> {
        let mut data = vec![0u8; feed_account_len(MAX)];
        let mut header: Feed = bytemuck::Zeroable::zeroed();
        header.version = FEED_ACCOUNT_VERSION;
        header.frozen = frozen as u8;
        header.min_decimals = 8;
        header.description[..7].copy_from_slice(b"BTC/USD");
        header.data_id = to_seed(data_id);
        header.state = *state;
        header.max_stored_reports = MAX;
        for r in 1..=rounds {
            let report = StoredReport {
                answer: r as i128 * 100 * E18,
                observations_timestamp: r * 10,
                recorded_timestamp: r * 10 + 1,
                recorded_slot: r,
                _padding0: [0; 8],
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

    /// Writes a round into a feed account's ring at the round's index.
    fn set_report(data: &mut [u8], round: u64, report: &StoredReport) {
        let offset = FEED_RING_OFFSET + (round % MAX) as usize * size_of::<StoredReport>();
        data[offset..offset + size_of::<StoredReport>()]
            .copy_from_slice(bytemuck::bytes_of(report));
    }

    /// Runs `f` on a feed account with the given data, owned by `owner`.
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
        // Rounds 1 to 5 in a ring of 4: round 5 overwrote round 1.
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
    fn decodes_the_other_reads_with_the_same_account_checks() {
        let state = Pubkey::new_unique();
        let id = [1u8; 16];
        let mut data = feed_account(&state, &id, 3, false);
        with_account(&mut data, &DATA_FEEDS_STORE_ID, |info| {
            assert_eq!(read_decimals(info, &state, id).unwrap(), 18);
            assert_eq!(read_description(info, &state, id).unwrap(), "BTC/USD");
            assert_eq!(read_min_decimals(info, &state, id).unwrap(), 8);
            let other_store = Pubkey::new_unique();
            for result in [
                read_decimals(info, &other_store, id).map(|_| ()),
                read_description(info, &other_store, id).map(|_| ()),
                read_min_decimals(info, &other_store, id).map(|_| ()),
                read_description(info, &state, [2u8; 16]).map(|_| ()),
            ] {
                assert_eq!(result.unwrap_err(), err(StoreError::AccountMismatch));
            }
        });
        let mut data = feed_account(&state, &id, 3, false);
        with_account(&mut data, &Pubkey::new_unique(), |info| {
            let foreign = read_description(info, &state, id);
            assert_eq!(foreign.unwrap_err(), err(StoreError::InvalidAccount));
        });
        // As in the store program, only round reads check the read freeze.
        let mut frozen = feed_account(&state, &id, 3, true);
        with_account(&mut frozen, &DATA_FEEDS_STORE_ID, |info| {
            assert_eq!(read_description(info, &state, id).unwrap(), "BTC/USD");
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
        data[ANCHOR_DISCRIMINATOR] = FEED_ACCOUNT_VERSION + 1;
        let version = read(&mut data, &DATA_FEEDS_STORE_ID);
        assert_eq!(version.unwrap_err(), err(StoreError::InvalidAccountVersion));

        // The header claims a ring that the account doesn't hold.
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
            &state,
            feed_info.clone(),
            [1; 16],
            8,
        );
        assert_eq!(latest.unwrap_err(), invalid);
        let round = get_round(
            program.clone(),
            state_info.clone(),
            &state,
            feed_info.clone(),
            [1; 16],
            1,
            8,
        );
        assert_eq!(round.unwrap_err(), invalid);
        let (p, s, f) = (program.clone(), state_info.clone(), feed_info.clone());
        assert_eq!(decimals(p, s, &state, f, [1; 16]).unwrap_err(), invalid);
        let (p, s, f) = (program.clone(), state_info.clone(), feed_info.clone());
        assert_eq!(description(p, s, &state, f, [1; 16]).unwrap_err(), invalid);
        let min = get_min_decimals(program, state_info, &state, feed_info, [1; 16]);
        assert_eq!(min.unwrap_err(), invalid);
    }

    #[test]
    fn calls_only_with_the_pinned_store_state() {
        let (state, feed) = (Pubkey::new_unique(), Pubkey::new_unique());
        let (store, pinned) = (DATA_FEEDS_STORE_ID, Pubkey::new_unique());
        let (mut l1, mut l2, mut l3) = (0, 0, 0);
        let (mut d1, mut d2, mut d3) = (vec![], vec![], vec![]);
        let program = AccountInfo::new(&store, false, false, &mut l1, &mut d1, &store, true, 0);
        let state_info = AccountInfo::new(&state, false, false, &mut l2, &mut d2, &store, false, 0);
        let feed_info = AccountInfo::new(&feed, false, false, &mut l3, &mut d3, &store, false, 0);
        let latest = latest_round(
            program.clone(),
            state_info.clone(),
            &pinned,
            feed_info.clone(),
            [1; 16],
            8,
        );
        assert_eq!(latest.unwrap_err(), err(StoreError::AccountMismatch));
        let round = get_round(
            program.clone(),
            state_info.clone(),
            &pinned,
            feed_info.clone(),
            [1; 16],
            1,
            8,
        );
        let mismatch = err(StoreError::AccountMismatch);
        assert_eq!(round.unwrap_err(), mismatch);
        let (p, s, f) = (program.clone(), state_info.clone(), feed_info.clone());
        assert_eq!(decimals(p, s, &pinned, f, [1; 16]).unwrap_err(), mismatch);
        let (p, s, f) = (program.clone(), state_info.clone(), feed_info.clone());
        assert_eq!(
            description(p, s, &pinned, f, [1; 16]).unwrap_err(),
            mismatch
        );
        let min = get_min_decimals(program, state_info, &pinned, feed_info, [1; 16]);
        assert_eq!(min.unwrap_err(), mismatch);
    }
}

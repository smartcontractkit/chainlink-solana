//! Readers for the Chainlink Data Feeds proxy on Solana. One proxy owns every feed account: pass
//! the data ID's feed account and the decimals you want, at least the feed's minimum and at most 18.
//!
//! Two ways to read the same data:
//! - **Decode the feed account** ([`read_latest_round`], [`read_round`]): no call to another
//!   program. The account must be owned by the proxy program this crate is built for
//!   ([`DATA_FEEDS_PROXY_ID`]) and belong to the proxy state you pass; anyone can create a proxy
//!   state under the program, so pin the one you trust.
//! - **Call the proxy** ([`latest_round`], [`get_round`]): one call, the proxy checks the account.
//!
//! Both refuse a frozen feed, decimals below the feed's minimum, and a non-zero answer that would
//! round to zero. Round IDs keep increasing for the life of a feed, also when the store writing it
//! is replaced.

use anchor_lang::prelude::*;
use anchor_lang::solana_program::program::get_return_data;
use data_feeds_proxy::{
    common::data_id_seed,
    cpi::accounts::FeedRead,
    error::ProxyError,
    logic,
    reads::to_round,
    state::Feed,
    utils::{decode_feed_into, RingRef},
};

pub use data_feeds_proxy::common::{feed_address, STORED_DECIMALS};
pub use data_feeds_proxy::logic::scale;
pub use data_feeds_proxy::state::Round;
pub use data_feeds_proxy::ID as DATA_FEEDS_PROXY_ID;

fn decode<'a>(
    feed: &'a AccountInfo,
    proxy_state: &Pubkey,
    data_id: &[u8; 16],
) -> Result<(Box<Feed>, std::cell::Ref<'a, &'a mut [u8]>)> {
    require_keys_eq!(*feed.owner, DATA_FEEDS_PROXY_ID, ProxyError::InvalidAccount);
    let data = feed.try_borrow_data()?;
    let mut header: Box<Feed> = bytemuck::zeroed_box();
    decode_feed_into(&data, proxy_state, &data_id_seed(data_id), &mut header)?;
    Ok((header, data))
}

/// The latest round of a data ID at `decimals`, decoded from its feed account.
pub fn read_latest_round(
    feed: &AccountInfo,
    proxy_state: &Pubkey,
    data_id: [u8; 16],
    decimals: u8,
) -> Result<Round> {
    let (header, _) = decode(feed, proxy_state, &data_id)?;
    let round = logic::latest_round(&header).ok_or(ProxyError::NoDataPresent)?;
    to_round(&header, round, decimals)
}

/// A stored round of a data ID at `decimals`, decoded from its feed account.
pub fn read_round(
    feed: &AccountInfo,
    proxy_state: &Pubkey,
    data_id: [u8; 16],
    round_id: u64,
    decimals: u8,
) -> Result<Round> {
    let (header, data) = decode(feed, proxy_state, &data_id)?;
    let round = logic::stored_round(round_id, &header, &RingRef::new(&data[..]))
        .ok_or(ProxyError::NoDataPresent)?;
    to_round(&header, round, decimals)
}

/// The latest round of a data ID at `decimals`, through a call to the proxy.
pub fn latest_round<'info>(
    proxy_program: AccountInfo<'info>,
    proxy_state: AccountInfo<'info>,
    feed: AccountInfo<'info>,
    data_id: [u8; 16],
    decimals: u8,
) -> Result<Round> {
    require_keys_eq!(
        proxy_program.key(),
        DATA_FEEDS_PROXY_ID,
        ErrorCode::InvalidProgramId
    );
    let ctx = CpiContext::new(
        proxy_program,
        FeedRead {
            state: proxy_state,
            feed,
        },
    );
    data_feeds_proxy::cpi::latest_round(ctx, data_id, decimals)?;
    returned()
}

/// A stored round of a data ID at `decimals`, through a call to the proxy.
pub fn get_round<'info>(
    proxy_program: AccountInfo<'info>,
    proxy_state: AccountInfo<'info>,
    feed: AccountInfo<'info>,
    data_id: [u8; 16],
    round_id: u64,
    decimals: u8,
) -> Result<Round> {
    require_keys_eq!(
        proxy_program.key(),
        DATA_FEEDS_PROXY_ID,
        ErrorCode::InvalidProgramId
    );
    let ctx = CpiContext::new(
        proxy_program,
        FeedRead {
            state: proxy_state,
            feed,
        },
    );
    data_feeds_proxy::cpi::get_round(ctx, data_id, round_id, decimals)?;
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
    use anchor_lang::Discriminator;
    use data_feeds_proxy::state::{feed_account_len, StoredReport, FEED_RING_OFFSET};
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

    const E18: i128 = 1_000_000_000_000_000_000;

    /// A feed account with rounds 1..=3 (answers 100, 200, 300 with 18 decimals).
    fn feed_account(state: &Pubkey, data_id: &[u8; 16], frozen: bool) -> Vec<u8> {
        let max = 4;
        let mut data = vec![0u8; feed_account_len(max)];
        let mut header: Feed = bytemuck::Zeroable::zeroed();
        header.version = 1;
        header.frozen = frozen as u8;
        header.min_decimals = 8;
        header.data_id = data_id_seed(data_id);
        header.state = *state;
        header.max_stored_reports = max;
        for r in 1..=3u64 {
            let report = StoredReport {
                answer: r as i128 * 100 * E18,
                observations_timestamp: r as u32 * 10,
                recorded_timestamp: r as u32 * 10 + 1,
                recorded_slot: r,
            };
            let offset = FEED_RING_OFFSET + (r % max) as usize * 32;
            data[offset..offset + 32].copy_from_slice(bytemuck::bytes_of(&report));
        }
        header.round_id = 3;
        header.latest_answer = 300 * E18;
        header.latest_observations_timestamp = 30;
        header.latest_recorded_timestamp = 31;
        data[..8].copy_from_slice(Feed::DISCRIMINATOR);
        data[8..8 + 256].copy_from_slice(bytemuck::bytes_of(&header));
        data
    }

    #[test]
    fn decodes_feed_accounts() {
        let (key, state) = (Pubkey::new_unique(), Pubkey::new_unique());
        let id = [1u8; 16];
        let mut lamports = 0;
        let mut data = feed_account(&state, &id, false);
        let info = AccountInfo::new(
            &key,
            false,
            false,
            &mut lamports,
            &mut data,
            &DATA_FEEDS_PROXY_ID,
            false,
            0,
        );
        let latest = read_latest_round(&info, &state, id, 8).unwrap();
        assert_eq!((latest.round_id, latest.answer), (3, 300_00000000));
        let round = read_round(&info, &state, id, 2, 18).unwrap();
        assert_eq!(
            (round.round_id, round.answer, round.observations_timestamp),
            (2, 200 * E18, 20)
        );
        assert!(
            read_latest_round(&info, &state, id, 7).is_err(),
            "below the minimum"
        );
        assert!(
            read_latest_round(&info, &Pubkey::new_unique(), id, 8).is_err(),
            "other proxy"
        );
        assert!(
            read_latest_round(&info, &state, [2u8; 16], 8).is_err(),
            "other data ID"
        );
        assert!(
            read_round(&info, &state, id, 4, 8).is_err(),
            "no such round"
        );
    }

    #[test]
    fn refuses_frozen_feeds_and_other_owners() {
        let (key, state) = (Pubkey::new_unique(), Pubkey::new_unique());
        let id = [1u8; 16];
        let mut lamports = 0;
        let mut data = feed_account(&state, &id, true);
        let frozen = AccountInfo::new(
            &key,
            false,
            false,
            &mut lamports,
            &mut data,
            &DATA_FEEDS_PROXY_ID,
            false,
            0,
        );
        assert!(read_latest_round(&frozen, &state, id, 8).is_err());

        let mut lamports = 0;
        let mut data = feed_account(&state, &id, false);
        let other = Pubkey::new_unique();
        let foreign = AccountInfo::new(
            &key,
            false,
            false,
            &mut lamports,
            &mut data,
            &other,
            false,
            0,
        );
        assert!(read_latest_round(&foreign, &state, id, 8).is_err());
    }
}

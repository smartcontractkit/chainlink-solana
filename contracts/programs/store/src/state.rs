use crate::{ErrorCode, FEED_VERSION};
use anchor_lang::prelude::*;

#[constant]
pub const HEADER_SIZE: usize = 192;

#[account(zero_copy)]
pub struct Store {
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub lowering_access_controller: Pubkey,
}

#[repr(C)]
#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct NewTransmission {
    pub timestamp: u64,
    pub answer: i128,
}

#[repr(C)]
#[derive(
    Debug, Default, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, bytemuck::Pod, bytemuck::Zeroable,
)]
pub struct Transmission {
    pub slot: u64,
    pub timestamp: u32,
    pub _padding0: u32,
    pub answer: i128,
    pub _padding1: u64,
    pub _padding2: u64,
}

use std::cell::RefMut;
use std::mem::size_of;

/// Two ringbuffers
/// - Live one that has a day's worth of data that's updated every second
/// - Historical one that stores historical data
pub struct Feed<'a> {
    pub header: &'a mut Transmissions,
    live: RefMut<'a, [Transmission]>,
    historical: RefMut<'a, [Transmission]>,
}

#[account]
pub struct Transmissions {
    pub version: u8,
    pub state: u8,
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub writer: Pubkey,
    /// Raw UTF-8 byte string
    pub description: [u8; 32],
    pub decimals: u8,
    pub flagging_threshold: u32,
    pub latest_round_id: u32,
    pub granularity: u8,
    pub live_length: u32,
    live_cursor: u32,
    historical_cursor: u32,
}

impl Transmissions {
    pub const NORMAL: u8 = 0;
    pub const FLAGGED: u8 = 1;
}

pub fn with_store<'a, 'info: 'a, F, T>(
    account: &'a mut Account<'info, Transmissions>,
    f: F,
) -> Result<T>
where
    F: FnOnce(&mut Feed) -> T,
{
    // Only try reading feeds matching the current version.
    if account.version != FEED_VERSION {
        return Err(ErrorCode::InvalidVersion.into());
    }

    let n = account.live_length as usize;

    let info = account.to_account_info();
    let data = info.try_borrow_mut_data()?;

    // two ringbuffers, live data and historical with smaller granularity
    let (live, historical) = RefMut::map_split(data, |data| {
        // skip the header
        let (_header, data) = data.split_at_mut(8 + HEADER_SIZE); // discriminator + header size
        let (live, historical) = data.split_at_mut(n * size_of::<Transmission>());
        // NOTE: no try_map_split available..
        let live = bytemuck::try_cast_slice_mut::<_, Transmission>(live).unwrap();
        let historical = bytemuck::try_cast_slice_mut::<_, Transmission>(historical).unwrap();
        (live, historical)
    });

    let mut store = Feed {
        header: account,
        live,
        historical,
    };

    Ok(f(&mut store))
}

impl<'a> Feed<'a> {
    pub fn insert(&mut self, round: Transmission) {
        self.header.latest_round_id += 1;

        // insert into live data
        self.live[self.header.live_cursor as usize] = round;
        self.header.live_cursor = (self.header.live_cursor + 1) % self.live.len() as u32;

        if self.header.latest_round_id % self.header.granularity as u32 == 0 {
            // insert into historical data
            self.historical[self.header.historical_cursor as usize] = round;
            self.header.historical_cursor =
                (self.header.historical_cursor + 1) % self.historical.len() as u32;
        }
    }

    pub fn latest(&self) -> Option<Transmission> {
        if self.header.latest_round_id == 0 {
            return None;
        }

        let len = self.header.live_length;
        // Handle wraparound
        let i = (self.header.live_cursor + len.saturating_sub(1)) % len;

        Some(self.live[i as usize])
    }

    pub fn fetch(&self, round_id: u32) -> Option<Transmission> {
        if self.header.latest_round_id < round_id {
            return None;
        }

        let latest_round_id = self.header.latest_round_id;
        let granularity = self.header.granularity as u32;

        // if in live range, fetch from live set
        let live_start = latest_round_id.saturating_sub((self.live.len() as u32).saturating_sub(1));
        // if in historical range, fetch from closest
        let historical_end = latest_round_id - (latest_round_id % granularity);
        let historical_start = historical_end
            .saturating_sub(granularity * (self.historical.len() as u32).saturating_sub(1));

        if (live_start..=latest_round_id).contains(&round_id) {
            // live data
            let offset = latest_round_id - round_id;
            let offset = offset + 1; // + 1 because we're looking for the element before the cursor

            // with overflow-checks = true the else branch could abort
            #[allow(clippy::unnecessary_lazy_evaluations)]
            let index = self
                .header
                .live_cursor
                .checked_sub(offset)
                .unwrap_or_else(|| self.live.len() as u32 - (offset - self.header.live_cursor));

            Some(self.live[index as usize])
        } else if (historical_start..=historical_end).contains(&round_id) {
            // historical data
            let round_id = round_id - (round_id % granularity);
            let offset = (historical_end - round_id) / granularity;
            let offset = offset + 1; // + 1 because we're looking for the element before the cursor

            // with overflow-checks = true the else branch could abort
            #[allow(clippy::unnecessary_lazy_evaluations)]
            let index = self
                .header
                .historical_cursor
                .checked_sub(offset)
                .unwrap_or_else(|| {
                    self.historical.len() as u32 - (offset - self.header.historical_cursor)
                });

            Some(self.historical[index as usize])
        } else {
            None
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn transmissions() {
        let live_length = 2;
        let historical_length = 3;
        let mut data = vec![
            0;
            8 + HEADER_SIZE
                + (live_length + historical_length) * size_of::<Transmission>()
        ];
        let header = &mut data[..8 + HEADER_SIZE]; // use a subslice to ensure the header fits into HEADER_SIZE bytes
        let mut cursor = std::io::Cursor::new(header);

        // insert the initial header with some granularity
        Transmissions {
            version: 2,
            state: Transmissions::NORMAL,
            owner: Pubkey::default(),
            proposed_owner: Pubkey::default(),
            writer: Pubkey::default(),
            description: [0; 32],
            decimals: 18,
            flagging_threshold: 1000,
            latest_round_id: 0,
            granularity: 5,
            live_length: live_length as u32,
            live_cursor: 0,
            historical_cursor: 0,
        }
        .try_serialize(&mut cursor)
        .unwrap();

        let mut lamports = 0u64;

        let pubkey = Pubkey::default();
        let owner = Transmissions::owner();
        let info = AccountInfo::new(
            &pubkey,
            false,
            false,
            &mut lamports,
            &mut data,
            &owner,
            false,
            0,
        );
        let mut account = Account::try_from(&info).unwrap();

        with_store(&mut account, |store| {
            for i in 1..=20 {
                store.insert(Transmission {
                    slot: u64::from(i),
                    answer: i128::from(i),
                    timestamp: i,
                    ..Default::default()
                });
            }

            assert_eq!(store.fetch(21), None);
            // Live range returns precise round
            assert_eq!(
                store.fetch(20),
                Some(Transmission {
                    slot: 20,
                    answer: 20,
                    timestamp: 20,
                    ..Default::default()
                })
            );
            assert_eq!(
                store.fetch(19),
                Some(Transmission {
                    slot: 19,
                    answer: 19,
                    timestamp: 19,
                    ..Default::default()
                })
            );
            // Historical range rounds down
            assert_eq!(
                store.fetch(18),
                Some(Transmission {
                    slot: 15,
                    answer: 15,
                    timestamp: 15,
                    ..Default::default()
                })
            );
            assert_eq!(
                store.fetch(15),
                Some(Transmission {
                    slot: 15,
                    answer: 15,
                    timestamp: 15,
                    ..Default::default()
                })
            );
            assert_eq!(
                store.fetch(14),
                Some(Transmission {
                    slot: 10,
                    answer: 10,
                    timestamp: 10,
                    ..Default::default()
                })
            );
            assert_eq!(
                store.fetch(10),
                Some(Transmission {
                    slot: 10,
                    answer: 10,
                    timestamp: 10,
                    ..Default::default()
                })
            );
            // Out of range
            assert_eq!(store.fetch(9), None);
        })
        .unwrap();
    }
}

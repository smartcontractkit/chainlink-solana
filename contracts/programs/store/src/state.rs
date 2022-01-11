use anchor_lang::prelude::*;
use arrayvec::arrayvec;

const MAX_FLAGS: usize = 128;

#[zero_copy]
pub struct Flags {
    xs: [Pubkey; MAX_FLAGS],
    len: u64,
}

arrayvec!(Flags, Pubkey, u64);

#[account(zero_copy)]
pub struct Store {
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub raising_access_controller: Pubkey,
    pub lowering_access_controller: Pubkey,

    pub flags: Flags,
}

#[repr(C)]
#[derive(
    Debug,
    Default,
    Clone,
    Copy,
    PartialEq,
    Eq,
    PartialOrd,
    Ord,
    bytemuck::Pod,
    bytemuck::Zeroable,
    AnchorSerialize,
    AnchorDeserialize,
)]
pub struct Transmission {
    pub timestamp: u64, // TODO: size back down to u32 and add padding?
    pub answer: i128,
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
    pub store: Pubkey,
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

pub fn with_store<'a, 'info: 'a, F, T>(
    account: &'a mut Account<'info, Transmissions>,
    f: F,
) -> Result<T, ProgramError>
where
    F: FnOnce(&mut Feed) -> T,
{
    let n = account.live_length as usize;

    let info = account.to_account_info();
    let data = info.try_borrow_mut_data()?;

    // two ringbuffers, live data and historical with smaller granularity
    let (live, historical) = RefMut::map_split(data, |data| {
        // skip the header
        let (_header, data) = data.split_at_mut(8 + 128); // discriminator + header size
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
        let mut data = vec![0; 8 + 128 + (2 + 3) * size_of::<Transmission>()];
        let header = &mut data[..8 + 128]; // use a subslice to ensure the header fits into 128 bytes
        let mut cursor = std::io::Cursor::new(header);

        // insert the initial header with some granularity
        Transmissions {
            version: 1,
            store: Pubkey::default(),
            writer: Pubkey::default(),
            description: [0; 32],
            decimals: 18,
            flagging_threshold: 1000,
            latest_round_id: 0,
            granularity: 5,
            live_length: 2,
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
                    answer: i128::from(i),
                    timestamp: i,
                });
            }

            assert_eq!(store.fetch(21), None);
            // Live range returns precise round
            assert_eq!(
                store.fetch(20),
                Some(Transmission {
                    answer: 20,
                    timestamp: 20
                })
            );
            assert_eq!(
                store.fetch(19),
                Some(Transmission {
                    answer: 19,
                    timestamp: 19
                })
            );
            // Historical range rounds down
            assert_eq!(
                store.fetch(18),
                Some(Transmission {
                    answer: 15,
                    timestamp: 15
                })
            );
            assert_eq!(
                store.fetch(15),
                Some(Transmission {
                    answer: 15,
                    timestamp: 15
                })
            );
            assert_eq!(
                store.fetch(14),
                Some(Transmission {
                    answer: 10,
                    timestamp: 10
                })
            );
            assert_eq!(
                store.fetch(10),
                Some(Transmission {
                    answer: 10,
                    timestamp: 10
                })
            );
            // Out of range
            assert_eq!(store.fetch(9), None);
        })
        .unwrap();
    }
}

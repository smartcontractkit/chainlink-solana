use anchor_lang::prelude::*;
use static_assertions::const_assert;

pub use anchor_lang::solana_program::secp256k1_recover::Secp256k1Pubkey;

use arrayvec::arrayvec;

// NOTE: ALL types in this file have to be verified to contain no padding via `cargo rustc -- -Zprint-type-sizes`!

// 19 is what we can achieve with Solana's resource constraints
#[constant]
pub const MAX_ORACLES: usize = 19;
// OCR2 is designed for a maximum of 31 oracles, and there are various assumptions made around this value.
const_assert!(MAX_ORACLES <= 31);

#[zero_copy]
pub struct Billing {
    pub observation_payment_gjuels: u32,
    pub transmission_payment_gjuels: u32,
}

#[zero_copy]
#[derive(Default)]
pub struct LeftoverPayment {
    pub payee: Pubkey,
    pub amount: u64,
}

#[zero_copy]
pub struct Oracles {
    xs: [Oracle; 19], // sadly we can't use const https://github.com/project-serum/anchor/issues/632
    len: u64,
}
arrayvec!(Oracles, Oracle, u64);

#[zero_copy]
pub struct LeftoverPayments {
    xs: [LeftoverPayment; 19], // sadly we can't use const https://github.com/project-serum/anchor/issues/632
    len: u64,
}
arrayvec!(LeftoverPayments, LeftoverPayment, u64);

#[account(zero_copy)]
pub struct State {
    pub version: u8,
    pub nonce: u8,
    _padding0: u16,
    _padding1: u32,
    pub config: Config,
    pub oracles: Oracles,
    pub leftover_payments: LeftoverPayments,
    pub transmissions: Pubkey,
}

#[zero_copy]
pub struct OffchainConfig {
    pub version: u64,
    xs: [u8; 4096],
    len: u64, // u64 since we need to be aligned
}
arrayvec!(OffchainConfig, u8, u64);

#[zero_copy]
pub struct Config {
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,

    /// LINK SPL token account.
    pub token_mint: Pubkey,
    /// LINK SPL token vault.
    pub token_vault: Pubkey,
    /// Access controller program managing access to `RequestNewRound`.
    pub requester_access_controller: Pubkey,
    /// Access controller program managing access to billing.
    pub billing_access_controller: Pubkey,

    pub min_answer: i128,
    pub max_answer: i128,

    /// Raw UTF-8 byte string
    pub description: [u8; 32],

    pub decimals: u8,
    pub f: u8,
    pub round: u8,
    _padding0: u8,
    pub epoch: u32,
    pub latest_aggregator_round_id: u32,
    pub latest_transmitter: Pubkey,

    pub config_count: u32,
    pub latest_config_digest: [u8; 32],
    pub latest_config_block_number: u64,

    pub billing: Billing,
    pub validator: Pubkey,
    pub flagging_threshold: u32,
    _padding1: u32,

    pub offchain_config: OffchainConfig,
    // a staging area which will swap onto data on commit
    pub pending_offchain_config: OffchainConfig,
}

impl Config {
    pub fn config_digest_from_data(
        &self,
        contract_address: &Pubkey,
        oracles: &[Oracle],
    ) -> [u8; 32] {
        let onchain_config = Vec::new(); // TODO

        // NOTE: keccak256 is also available, but SHA256 is faster
        use anchor_lang::solana_program::hash;
        // NOTE: calling hash::hashv is orders of magnitude cheaper than using Hasher::hashv
        let mut data: Vec<&[u8]> = Vec::with_capacity(9 + 2 * oracles.len());
        let addr = contract_address.to_bytes();
        data.push(&addr);
        let count = self.config_count.to_be_bytes();
        data.push(&count);
        let n = [oracles.len() as u8]; // safe because it will always fit in MAX_ORACLES
        data.push(&n);
        for oracle in oracles {
            data.push(&oracle.signer.key);
        }
        for oracle in oracles {
            data.push(oracle.transmitter.as_ref());
        }
        let f = &[self.f];
        data.push(f);
        let onchain_config_len = (onchain_config.len() as u32).to_be_bytes();
        data.push(&onchain_config_len);
        data.push(&onchain_config);
        let offchain_version = self.offchain_config.version.to_be_bytes();
        data.push(&offchain_version);
        let offchain_config_len = (self.offchain_config.len() as u32).to_be_bytes();
        data.push(&offchain_config_len);
        data.push(&self.offchain_config);
        let result = hash::hashv(&data);

        let mut result: [u8; 32] = result.to_bytes();
        // prefix masking
        result[0] = 0x00;
        result[1] = 0x03;
        result
    }
}

#[zero_copy]
pub struct SigningKey {
    pub key: [u8; 20],
}

#[zero_copy]
pub struct Oracle {
    pub transmitter: Pubkey,
    /// secp256k1 signing key for submissions
    pub signer: SigningKey,
    /// Payee address to pay out rewards to
    pub payee: Pubkey,
    /// will be zeroed out if empty
    pub proposed_payee: Pubkey,

    /// Rewards from round_id up until now
    pub from_round_id: u32,

    /// `transmit()` reimbursements
    pub payment: u64,
}

impl Default for Oracle {
    fn default() -> Self {
        Self {
            transmitter: Pubkey::default(),
            signer: SigningKey { key: [0u8; 20] },
            payee: Pubkey::default(),
            proposed_payee: Pubkey::default(),
            from_round_id: 0,
            payment: 0,
        }
    }
}

#[repr(C)]
#[derive(
    Debug, Default, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, bytemuck::Pod, bytemuck::Zeroable,
)]
pub struct Transmission {
    pub timestamp: u64,
    pub answer: i128,
}

use std::cell::RefMut;
use std::mem::size_of;

/// Two ringbuffers
/// - Live one that has a day's worth of data that's updated every second
/// - Historical one that stores historical data
pub struct Store<'a> {
    header: &'a mut Transmissions,
    live: RefMut<'a, [Transmission]>,
    historical: RefMut<'a, [Transmission]>,
}

// TODO: the modulus and initial ringbuffer size to be configurable
#[account]
pub struct Transmissions {
    latest_round_id: u32,
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
    F: FnOnce(&mut Store) -> T,
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

    let mut store = Store {
        header: account,
        live,
        historical,
    };

    Ok(f(&mut store))
}

impl<'a> Store<'a> {
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
        // TODO: ensure this is how it works with the actual feed
        let mut s = &mut data[0..8 + 128];

        // insert the initial header with some granularity
        Transmissions {
            latest_round_id: 0,
            granularity: 5,
            live_length: 2,
            live_cursor: 0,
            historical_cursor: 0,
        }
        .try_serialize(&mut s)
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

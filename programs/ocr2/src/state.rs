use anchor_lang::prelude::*;
use static_assertions::const_assert;
use std::mem;

pub use anchor_lang::solana_program::secp256k1_recover::Secp256k1Pubkey;

use arrayvec::arrayvec;

// 19 is what we can achieve with Solana's resource constraints
pub const MAX_ORACLES: usize = 19;
// OCR2 is designed for a maximum of 31 oracles, and there are various assumptions made around this value.
const_assert!(MAX_ORACLES <= 31);

#[zero_copy]
pub struct Billing {
    pub observation_payment: u32,
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
    len: u8,
}
arrayvec!(Oracles, Oracle, u8);

#[account(zero_copy)] // TODO: force repr(C) here
pub struct State {
    pub nonce: u8,
    pub config: Config,
    pub oracles: Oracles,
    pub leftover_payments: [LeftoverPayment; 19],
    pub leftover_payments_len: u8,
    pub transmissions: Pubkey,
}
const_assert!(
    mem::size_of::<State>()
        == mem::size_of::<Config>()
            + 1
            + 1
            + mem::size_of::<Oracle>() * MAX_ORACLES
            + mem::size_of::<(Pubkey, u64)>() * MAX_ORACLES
            + 1
            + mem::size_of::<Pubkey>()
);

#[zero_copy]
pub struct OffchainConfig {
    pub version: u64,
    xs: [u8; 4096],
    len: u64, // u64 since we need to be aligned
}
arrayvec!(OffchainConfig, u8, u64);

#[zero_copy]
pub struct Config {
    pub version: u8,

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

    pub decimals: u8,
    /// Raw UTF-8 byte string
    pub description: [u8; 32],

    pub f: u8,

    pub config_count: u32,
    pub latest_config_digest: [u8; 32],
    pub latest_config_block_number: u64,

    pub latest_aggregator_round_id: u32,
    pub epoch: u32,
    pub round: u8,

    pub billing: Billing,
    pub validator: Pubkey,
    pub flagging_threshold: u32,

    pub offchain_config: OffchainConfig,
    // a staging area which will swap onto data on commit
    pub pending_offchain_config: OffchainConfig,
}
const_assert!(mem::size_of::<Config>() == 352 + 4096 + 8 + 4096 + 8 + 8 + 8); // bytes

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
        let n = [oracles.len() as u8];
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

    /// `transmit()` reimbursements
    pub payment: u64,
    /// Rewards from round_id up until now
    pub from_round_id: u32,
}
const_assert!(mem::size_of::<Oracle>() == 128); // bytes

impl Default for Oracle {
    fn default() -> Self {
        Self {
            transmitter: Pubkey::default(),
            signer: SigningKey { key: [0u8; 20] },
            payee: Pubkey::default(),
            proposed_payee: Pubkey::default(),
            payment: 0,
            from_round_id: 0,
        }
    }
}

#[zero_copy]
#[derive(Debug, Default, PartialEq, Eq, PartialOrd, Ord)]
pub struct Transmission {
    pub answer: i128,
    pub timestamp: u32,
}
const_assert!(mem::size_of::<Transmission>() == 20); // bytes

#[account(zero_copy)] // TODO: force repr(C) here
pub struct Transmissions {
    pub latest_round_id: u32,
    // Current offset
    pub cursor: u32,
    // 524_280 = approx. 10MB ~= 10485760 / 20
    pub transmissions: [Transmission; 8192], // temporarily lowered for devnet
}

impl Transmissions {
    pub fn store_round(&mut self, round: Transmission) {
        self.latest_round_id += 1;
        self.transmissions[self.cursor as usize] = round;
        self.cursor = (self.cursor + 1) % self.transmissions.len() as u32;
    }

    pub fn fetch_round(&self, round_id: u32) -> Option<Transmission> {
        if self.latest_round_id < round_id {
            return None;
        }

        let diff = self.latest_round_id - round_id;

        if diff as usize > self.transmissions.len() {
            return None;
        }

        let diff = diff + 1; // + 1 because we're looking for the element before the cursor
        let index = self
            .cursor
            .checked_sub(diff)
            .unwrap_or_else(|| self.transmissions.len() as u32 - (diff - self.cursor));

        let transmission = &self.transmissions[index as usize];
        (transmission.timestamp != 0).then(|| *transmission)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn transmissions() {
        let layout = std::alloc::Layout::new::<Transmissions>();
        let mut data: Box<Transmissions> = unsafe {
            let ptr = std::alloc::alloc_zeroed(layout).cast();
            Box::from_raw(ptr)
        };

        // manipulate the data so that the first round is placed on the other end of the circular buffer
        data.transmissions[8191] = Transmission {
            answer: 1,
            timestamp: 1,
        };
        data.latest_round_id += 1;

        data.store_round(Transmission {
            answer: 2,
            timestamp: 2,
        });
        data.store_round(Transmission {
            answer: 3,
            timestamp: 3,
        });

        assert_eq!(
            data.fetch_round(1),
            Some(Transmission {
                answer: 1,
                timestamp: 1
            })
        );
        assert_eq!(
            data.fetch_round(2),
            Some(Transmission {
                answer: 2,
                timestamp: 2
            })
        );
        assert_eq!(
            data.fetch_round(3),
            Some(Transmission {
                answer: 3,
                timestamp: 3
            })
        );
    }
}

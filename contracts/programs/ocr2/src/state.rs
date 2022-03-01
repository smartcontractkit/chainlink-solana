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

#[constant]
pub const DIGEST_SIZE: usize = 32;

#[zero_copy]
pub struct Billing {
    pub observation_payment_gjuels: u32,
    pub transmission_payment_gjuels: u32,
}

#[zero_copy]
pub struct Oracles {
    xs: [Oracle; MAX_ORACLES],
    len: u64,
}
arrayvec!(Oracles, Oracle, u64);

#[account(zero_copy)]
pub struct Proposal {
    pub version: u8,
    pub owner: Pubkey,
    pub state: u8, // NOTE: can't use bool or enum because all bit patterns need to be valid for bytemuck/transmute
    pub f: u8,
    _padding0: u8,
    _padding1: u32,
    /// Set by set_payees, used to verify payee's token type matches the aggregator token type.
    pub token_mint: Pubkey,
    pub oracles: ProposedOracles,
    pub offchain_config: OffchainConfig,
}
impl Proposal {
    pub const NEW: u8 = 0;
    pub const FINALIZED: u8 = 1;

    pub fn digest(&self) -> [u8; DIGEST_SIZE] {
        use anchor_lang::solana_program::hash;
        let mut data: Vec<&[u8]> = Vec::with_capacity(1 + 3 * self.oracles.len() + 5);
        let n = [self.oracles.len() as u8]; // safe because it will always fit in MAX_ORACLES
        data.push(&n);
        for oracle in self.oracles.as_ref() {
            data.push(&oracle.signer.key);
            data.push(oracle.transmitter.as_ref());
            data.push(oracle.payee.as_ref());
        }
        let f = &[self.f];
        data.push(f);
        data.push(self.token_mint.as_ref());
        let offchain_version = self.offchain_config.version.to_be_bytes();
        data.push(&offchain_version);
        let offchain_config_len = (self.offchain_config.len() as u32).to_be_bytes();
        data.push(&offchain_config_len);
        data.push(&self.offchain_config);
        let result = hash::hashv(&data);
        result.to_bytes()
    }
}

#[zero_copy]
/// A subset of the [Oracles] type to save space.
pub struct ProposedOracle {
    pub transmitter: Pubkey,
    /// secp256k1 signing key for submissions
    pub signer: SigningKey,
    pub _padding: u32, // 4 bytes padding to align 20 byte signer
    /// Payee address to pay out rewards to
    pub payee: Pubkey,
}
#[zero_copy]
pub struct ProposedOracles {
    xs: [ProposedOracle; MAX_ORACLES],
    len: u64,
}
arrayvec!(ProposedOracles, ProposedOracle, u64);

#[account(zero_copy)]
pub struct State {
    pub version: u8,
    pub vault_nonce: u8,
    _padding0: u16,
    _padding1: u32,
    pub feed: Pubkey,
    pub config: Config,
    pub offchain_config: OffchainConfig,
    pub oracles: Oracles,
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

    pub f: u8,
    pub round: u8,
    _padding0: u16,
    pub epoch: u32,
    pub latest_aggregator_round_id: u32,
    pub latest_transmitter: Pubkey,

    pub config_count: u32,
    pub latest_config_digest: [u8; DIGEST_SIZE],
    pub latest_config_block_number: u64,

    pub billing: Billing,
}

impl Config {
    pub fn config_digest_from_data(
        &self,
        program_id: &Pubkey,
        aggregator_address: &Pubkey,
        offchain_config: &OffchainConfig,
        oracles: &[Oracle],
    ) -> [u8; DIGEST_SIZE] {
        // calculate onchain_config from stored config
        let mut onchain_config = vec![1]; // version

        // the ocr plugin expects i192 encoded values, so we need to sign extend to make the digest match
        if self.min_answer.is_negative() {
            onchain_config.extend_from_slice(&[0xFF; 8]);
        } else {
            // 0 or positive
            onchain_config.extend_from_slice(&[0x00; 8]);
        }
        onchain_config.extend_from_slice(&self.min_answer.to_be_bytes());

        // the ocr plugin expects i192 encoded values, so we need to sign extend to make the digest match
        if self.max_answer.is_negative() {
            onchain_config.extend_from_slice(&[0xFF; 8]);
        } else {
            // 0 or positive
            onchain_config.extend_from_slice(&[0x00; 8]);
        }
        onchain_config.extend_from_slice(&self.max_answer.to_be_bytes());

        // NOTE: keccak256 is also available, but SHA256 is faster
        use anchor_lang::solana_program::hash;
        // NOTE: calling hash::hashv is orders of magnitude cheaper than using Hasher::hashv
        let mut data: Vec<&[u8]> = Vec::with_capacity(10 + 2 * oracles.len());
        let program_addr = program_id.to_bytes();
        data.push(&program_addr);
        let aggregator_addr = aggregator_address.to_bytes();
        data.push(&aggregator_addr);
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
        let offchain_version = offchain_config.version.to_be_bytes();
        data.push(&offchain_version);
        let offchain_config_len = (offchain_config.len() as u32).to_be_bytes();
        data.push(&offchain_config_len);
        data.push(offchain_config);
        let result = hash::hashv(&data);

        let mut result: [u8; DIGEST_SIZE] = result.to_bytes();
        // prefix masking
        result[0] = 0x00;
        result[1] = 0x03;
        result
    }
}

// Use a newtype if it becomes possible: https://github.com/project-serum/anchor/issues/607
#[zero_copy]
#[derive(Default)]
pub struct SigningKey {
    pub key: [u8; 20],
}

#[zero_copy]
#[derive(Default)]
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
    pub payment_gjuels: u64,
}

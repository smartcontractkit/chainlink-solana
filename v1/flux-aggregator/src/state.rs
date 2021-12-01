//! Storage:
//! ----
//! 4kb aggregator state
//! ----
//! u64 current_pos
//! ... round data

use borsh::{BorshDeserialize, BorshSerialize};
use solana_program::{clock::UnixTimestamp, program_pack::IsInitialized, pubkey::Pubkey};

pub const MAX_ORACLES: usize = 8;

pub type Timestamp = UnixTimestamp;
pub type Value = u128;

#[derive(Clone, Copy, Eq, PartialEq, BorshSerialize, BorshDeserialize, Default, Debug)]
#[repr(C)]
pub struct Submission(pub Timestamp, pub Value);

unsafe impl bytemuck::Zeroable for Submission {
    fn zeroed() -> Self {
        Self::default()
    }
}
unsafe impl bytemuck::Pod for Submission {}

/// Define the type of state stored in accounts
#[derive(Debug, BorshSerialize, BorshDeserialize)]
pub struct Aggregator {
    /// Set to true after initialization.
    pub is_initialized: bool,

    pub version: u32,

    pub config: Config,
    /// When the config was last updated.
    pub updated_at: Timestamp,

    /// The aggregator owner is allowed to modify it's config.
    pub owner: Pubkey,

    /// A set of current submissions, one per oracle. Array index corresponds to oracle index.
    pub submissions: [Submission; MAX_ORACLES], // TODO: submissions needs to be config.oracles sized
    /// The current median answer.
    pub answer: Option<Value>,
}

impl IsInitialized for Aggregator {
    fn is_initialized(&self) -> bool {
        self.is_initialized
    }
}

#[derive(Debug, Clone, PartialEq, Eq, BorshSerialize, BorshDeserialize)]
pub struct Config {
    /// A list of oracles allowed to submit answers.
    pub oracles: Vec<Pubkey>, // TODO: maintain a MAX_ORACLES
    /// Number of submissions required to produce an answer. Must be larger than 0.
    pub min_answer_threshold: u8,
    /// Offset in number of seconds before a submission is considered stale.
    pub staleness_threshold: u8,
    pub decimals: u8,
}

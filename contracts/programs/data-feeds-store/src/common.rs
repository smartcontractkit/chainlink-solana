pub const ANCHOR_DISCRIMINATOR: usize = 8;

/// Version written to every account the program creates, bumped on layout changes.
pub const ACCOUNT_VERSION: u8 = 1;

/// Decimals every report answer is stored with.
pub const STORED_DECIMALS: u8 = 18;

pub const MAX_FEED_ADMINS: usize = 64;

pub const MAX_WORKFLOW_METADATAS: usize = 16;

pub const MAX_CALLBACK_ACCOUNTS: usize = 4;

/// Max UTF-8 bytes of a feed description.
pub const DESCRIPTION_LEN: usize = 64;

/// Keep at least 2 stored reports so a previous round stays readable.
pub const MIN_MAX_STORED_REPORTS: u64 = 2;

/// Most data IDs one `get_latest_rounds` call reads: its Borsh `Vec<ReaderRound>` (4 + 32 bytes
/// each) must fit the 1,024 bytes of return data.
pub const MAX_BATCH_READ: usize = 31;

/// Accounts created or grown inside a program can change by at most 10KiB per instruction.
pub const MAX_ACCOUNT_DATA_INCREASE: usize = 10_240;

pub const ZERO_ADDRESS: [u8; 20] = [0; 20];

pub const ZERO_NAME: [u8; 10] = [0; 10];

// derived from hash::hash("global:on_report_callback".as_bytes()).to_bytes()[..8]
pub const ON_REPORT_CALLBACK_DISCRIMINATOR: [u8; 8] = [3, 132, 93, 104, 23, 223, 163, 62];

/// PDA owning tokens and SOL sent to the store, recoverable by the owner. It signs nothing else.
pub const STORE_AUTHORITY_SEED: &[u8] = b"store_authority";
/// PDA signing the report callbacks of one callback program: `[seed, state, program]`. Each
/// program gets its own signer, so a callback program can only act as its own authority (e.g. the
/// DF1 store's writer), never as another program's.
pub const CALLBACK_AUTHORITY_SEED: &[u8] = b"callback_authority";
/// Owner-managed allowlist entry of a callback program: `[seed, state, program]`.
pub const CALLBACK_PROGRAM_SEED: &[u8] = b"callback_program";
pub const FEED_SEED: &[u8] = b"feed";
pub const FEED_DESCRIPTION_SEED: &[u8] = b"feed_description";

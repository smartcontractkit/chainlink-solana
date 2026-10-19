pub const ANCHOR_DISCRIMINATOR: usize = 8;

/// Version written to every account the program creates, bumped on layout changes.
pub const ACCOUNT_VERSION: u8 = 1;

/// Decimals every report answer is stored with, and the upper bound for a reader's decimals.
pub const STORED_DECIMALS: u8 = 18;

/// Aggregator-compatible `version()`.
pub const STORE_VERSION: u64 = 1;

pub const MAX_FEED_ADMINS: usize = 64;

pub const MAX_WORKFLOW_METADATAS: usize = 16;

pub const MAX_CALLBACK_ACCOUNTS: usize = 4;

/// Max UTF-8 bytes of a feed or SVR path description.
pub const DESCRIPTION_LEN: usize = 64;

/// Keep at least 2 stored reports so the non-SVR sync can reach a previous round.
pub const MIN_MAX_STORED_REPORTS: u64 = 2;

/// Accounts created or grown inside a program can change by at most 10KiB per instruction.
pub const MAX_ACCOUNT_DATA_INCREASE: usize = 10_240;

pub const ZERO_ADDRESS: [u8; 20] = [0; 20];

pub const ZERO_NAME: [u8; 10] = [0; 10];

// derived from hash::hash("global:on_report_callback".as_bytes()).to_bytes()[..8]
pub const ON_REPORT_CALLBACK_DISCRIMINATOR: [u8; 8] = [3, 132, 93, 104, 23, 223, 163, 62];

pub const STORE_AUTHORITY_SEED: &[u8] = b"store_authority";
pub const FEED_SEED: &[u8] = b"feed";
pub const DATA_ID_SEED: &[u8] = b"data_id";
pub const PROXY_SEED: &[u8] = b"proxy";
pub const FEED_DESCRIPTION_SEED: &[u8] = b"feed_description";
pub const SVR_PATH_DESCRIPTION_SEED: &[u8] = b"svr_path_description";

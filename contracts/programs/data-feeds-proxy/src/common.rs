pub const ANCHOR_DISCRIMINATOR: usize = 8;

/// Version written to every account the program creates, bumped on layout changes.
pub const ACCOUNT_VERSION: u8 = 1;

/// Decimals of the answers the store's reader interface returns, and the upper bound for the
/// decimals a caller can ask `read_scaled` for.
pub const STORE_DECIMALS: u8 = 18;

pub const MAX_FEED_ADMINS: usize = 16;

pub const FEED_CONFIG_SEED: &[u8] = b"feed_config";

pub const ANCHOR_DISCRIMINATOR: usize = 8;

/// Version written to every account the program creates, bumped on layout changes.
pub const ACCOUNT_VERSION: u8 = 1;

/// Decimals every answer is stored with, and the most decimals a read can ask for.
pub const STORED_DECIMALS: u8 = 18;

/// Max UTF-8 bytes of a feed description.
pub const DESCRIPTION_LEN: usize = 64;

/// Keep at least 2 stored rounds so a previous round stays readable.
pub const MIN_MAX_STORED_REPORTS: u64 = 2;

/// Accounts created or grown inside a program can change by at most 10KiB per instruction.
pub const MAX_ACCOUNT_DATA_INCREASE: usize = 10_240;

pub const FEED_SEED: &[u8] = b"feed";

/// A data ID as accounts derive their addresses from, zero padded to 32 bytes, so a wider data ID
/// format later keeps every account address.
pub fn data_id_seed(data_id: &[u8; 16]) -> [u8; 32] {
    let mut seed = [0u8; 32];
    seed[..16].copy_from_slice(data_id);
    seed
}

/// The feed account address of a data ID under a proxy state.
pub fn feed_address(
    state: &anchor_lang::prelude::Pubkey,
    data_id: &[u8; 16],
) -> (anchor_lang::prelude::Pubkey, u8) {
    anchor_lang::prelude::Pubkey::find_program_address(
        &[FEED_SEED, state.as_ref(), &data_id_seed(data_id)],
        &crate::ID,
    )
}

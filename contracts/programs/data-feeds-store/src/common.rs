pub const ANCHOR_DISCRIMINATOR: usize = 8;

/// Version written to every account the program creates, bumped on layout changes.
pub const ACCOUNT_VERSION: u8 = 1;

/// Decimals every answer is stored with, and the most decimals a read can ask for.
pub const STORED_DECIMALS: u8 = 18;

pub const MAX_FEED_ADMINS: usize = 64;

pub const MAX_WORKFLOW_METADATAS: usize = 16;

/// Max UTF-8 bytes of a feed description.
pub const DESCRIPTION_LEN: usize = 64;

/// Keep at least 2 stored rounds so a previous round stays readable.
pub const MIN_MAX_STORED_REPORTS: u64 = 2;

/// Accounts created or grown inside a program can change by at most 10KiB per instruction.
pub const MAX_ACCOUNT_DATA_INCREASE: usize = 10_240;

pub const ZERO_ADDRESS: [u8; 20] = [0; 20];

pub const ZERO_NAME: [u8; 10] = [0; 10];

/// A data ID's feed: `[seed, state, data ID zero padded to 32 bytes]`.
pub const FEED_SEED: &[u8] = b"feed";
/// PDA owning tokens and SOL sent to the store, recoverable by the owner. It signs nothing else.
pub const STORE_AUTHORITY_SEED: &[u8] = b"store_authority";
/// The store's signer towards DF1, set as the writer of DF1 feeds it keeps written: `[seed, state]`.
pub const DF1_WRITER_SEED: &[u8] = b"df1_writer";

/// The DF1 store's `Transmissions` feed accounts and `submit` instruction, as deployed. The store
/// checks everything `submit` could fail on before calling it.
pub mod df1 {
    /// sha256("account:Transmissions")[..8]
    pub const TRANSMISSIONS_DISCRIMINATOR: [u8; 8] = [96, 179, 69, 66, 128, 129, 73, 117];
    /// sha256("global:submit")[..8]
    pub const SUBMIT_DISCRIMINATOR: [u8; 8] = [88, 166, 102, 181, 162, 127, 170, 48];
    /// The only feed layout `submit` writes
    pub const FEED_VERSION: u8 = 2;
    // Borsh offsets of the header fields, discriminator included
    pub const VERSION: usize = 8;
    pub const WRITER: usize = 74;
    pub const DECIMALS: usize = 138;
    pub const LATEST_ROUND_ID: usize = 143;
    pub const GRANULARITY: usize = 147;
    pub const LIVE_LENGTH: usize = 148;
    pub const LIVE_CURSOR: usize = 152;
    pub const HISTORICAL_CURSOR: usize = 156;
    /// Discriminator plus the fixed 192-byte header, after which the live ring starts
    pub const RINGS: usize = 8 + 192;
    /// One `Transmission`: slot u64, timestamp u32, padding, answer i128, padding
    pub const TRANSMISSION_LEN: usize = 48;
    /// Offset of the timestamp within a `Transmission`
    pub const TRANSMISSION_TIMESTAMP: usize = 8;
}

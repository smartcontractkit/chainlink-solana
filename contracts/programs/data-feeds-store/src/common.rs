/// The number of bytes Anchor puts at the start of every account to tag its type.
pub const ANCHOR_DISCRIMINATOR: usize = 8;

/// The layout version of a store state account. It is increased when that account's layout changes,
/// independently of the other account types.
pub const STORE_STATE_VERSION: u8 = 1;
/// The layout version of a feed account. It is increased when that account's layout changes,
/// independently of the other account types.
pub const FEED_ACCOUNT_VERSION: u8 = 1;
/// The layout version of a DF1 link marker account. It is increased when that account's layout
/// changes, independently of the other account types.
pub const DF1_LINK_VERSION: u8 = 1;

/// The number of decimals every answer is stored with. It is also the most decimals a read can
/// ask for.
pub const STORED_DECIMALS: u8 = 18;

/// The most feed admins a store can have.
pub const MAX_FEED_ADMINS: usize = 64;

/// The most workflows a workflow list can hold, whether it is the store's default list or a
/// feed's own list.
pub const MAX_WORKFLOW_METADATAS: usize = 16;

/// The most UTF-8 bytes a feed description can have.
pub const DESCRIPTION_LEN: usize = 64;

/// The fewest rounds a feed can keep. With two, the round before the latest stays readable.
pub const MIN_MAX_STORED_REPORTS: u64 = 2;

/// The most a program can grow an account in one instruction, 10 KiB. Solana counts the growth
/// from the account's length when the instruction started, and an account the program creates
/// counts as grown from zero, so a feed created by the program holds at most 185 rounds.
pub const MAX_ACCOUNT_DATA_INCREASE: usize = 10_240;

/// The all-zero workflow owner. Workflow lists refuse it, because it means the owner is unset.
pub const ZERO_ADDRESS: [u8; 20] = [0; 20];

/// The all-zero workflow name. Workflow lists refuse it, because it means the name is unset.
pub const ZERO_NAME: [u8; 10] = [0; 10];

/// The seed of a store state's address, the PDA `["state", store ID as 8 little-endian bytes]`.
/// `initialize` creates the state at this address, so one program can hold one store per store ID.
pub const STATE_SEED: &[u8] = b"state";
/// The seed of a feed's address, the PDA `["feed", state, data ID zero padded to 32 bytes]`. Each
/// data ID has exactly one feed in each store.
pub const FEED_SEED: &[u8] = b"feed";
/// The seed of the store authority's address, the PDA `["store_authority", state]`. The store
/// authority only holds tokens and SOL sent to the store, which the owner can recover. It signs
/// nothing else.
pub const STORE_AUTHORITY_SEED: &[u8] = b"store_authority";
/// The seed of the `df1_writer` address, the PDA `["df1_writer", state]`. The store signs its
/// `submit` calls to DF1 as this address, so each linked DF1 feed must name it as its writer.
pub const DF1_WRITER_SEED: &[u8] = b"df1_writer";
/// The seed of a DF1 link marker's address, the PDA `["df1_link", state, DF1 feed]`. The marker
/// records which data ID links that DF1 feed, so that no two data IDs can write it.
pub const DF1_LINK_SEED: &[u8] = b"df1_link";

/// Constants for reading DF1 feed accounts and calling DF1's `submit`. The store reads a DF1 feed
/// as raw bytes at these offsets, which follow DF1's `Transmissions` header and `Transmission`
/// round in `programs/store/src/state.rs`. It uses them to check, before calling `submit`,
/// everything that could make `submit` fail.
pub mod df1 {
    /// The first 8 bytes of every DF1 feed account. They are Anchor's tag for the `Transmissions`
    /// type, sha256("account:Transmissions")[..8], and tell a DF1 feed from DF1's other accounts.
    pub const TRANSMISSIONS_DISCRIMINATOR: [u8; 8] = [96, 179, 69, 66, 128, 129, 73, 117];
    /// The first 8 bytes of a `submit` call's data. They are Anchor's tag for the instruction,
    /// sha256("global:submit")[..8], and tell DF1 which instruction to run.
    pub const SUBMIT_DISCRIMINATOR: [u8; 8] = [88, 166, 102, 181, 162, 127, 170, 48];
    /// The only DF1 feed layout version that `submit` accepts.
    pub const FEED_VERSION: u8 = 2;

    // The offsets below count from the start of the account, including the 8-byte tag.

    /// The offset of the feed's layout version (u8).
    pub const VERSION: usize = 8;
    /// The offset of the feed's writer (32 bytes), the only key `submit` accepts as its signer.
    /// While OCR2 writes the feed, this is OCR2's key. The DF1 feed's owner switches it to this
    /// store's `df1_writer` PDA with DF1's `set_writer` when the feed moves to CRE.
    pub const WRITER: usize = 74;
    /// The offset of the feed's decimals (u8). The store rescales its 18-decimal answers to them.
    pub const DECIMALS: usize = 138;
    /// The offset of the feed's round counter (u32). `submit` fails if the counter would go past
    /// `u32::MAX`.
    pub const LATEST_ROUND_ID: usize = 143;
    /// The offset of the feed's granularity (u8). A round whose ID is a multiple of it is also
    /// written to the historical ring.
    pub const GRANULARITY: usize = 147;
    /// The offset of the number of rounds the live ring holds (u32).
    pub const LIVE_LENGTH: usize = 148;
    /// The offset of the live ring's cursor (u32), the index `submit` writes next.
    pub const LIVE_CURSOR: usize = 152;
    /// The offset of the historical ring's cursor (u32), the index `submit` writes next.
    pub const HISTORICAL_CURSOR: usize = 156;

    /// The offset at which the rounds start, after the 8-byte tag and DF1's fixed 192-byte header
    /// (`HEADER_SIZE`, which leaves room after the fields above). The live ring comes first, and
    /// the historical ring takes the rest of the account.
    pub const RINGS: usize = 8 + 192;
    /// The size of one DF1 round (`Transmission`): a u64 slot, a u32 timestamp, 4 padding bytes,
    /// an i128 answer and 16 padding bytes.
    pub const TRANSMISSION_LEN: usize = 48;
    /// The offset of the timestamp within a round. The store reads it to find the timestamp of
    /// the DF1 feed's latest round.
    pub const TRANSMISSION_TIMESTAMP: usize = 8;
}

pub const STATE_VERSION: u8 = 1;

pub const ANCHOR_DISCRIMINATOR: usize = 8;

pub const REPORT_CONTEXT_LEN: usize = 96;

// our don size is directly limited by the transaction size during on_report's signature verification
pub const MAX_ORACLES: usize = 16;

pub const SIGNATURE_LEN: usize = 65;

pub const FORWARDER_METADATA_LENGTH: usize = 45;

pub const METADATA_LENGTH: usize = 109;

pub const MAX_ACCTS: usize = 64;

pub const ON_REPORT_DISCRIMINATOR: [u8; 8] = [214, 173, 18, 221, 173, 148, 151, 208];

pub const STATE_VERSION: u8 = 1;

pub const ANCHOR_DISCRIMINATOR: usize = 8;

pub const REPORT_CONTEXT_LEN: usize = 96;

pub const SIGNATURE_LEN: usize = 65;

pub const FORWARDER_METADATA_LENGTH: usize = 45;

pub const METADATA_LENGTH: usize = 109;

// Anchor `on_report` discriminator — sha256("global:on_report")[..8]. Must match
// keystone-forwarder so receivers don't need to distinguish callers by discriminator.
pub const ON_REPORT_DISCRIMINATOR: [u8; 8] = [214, 173, 18, 221, 173, 148, 151, 208];

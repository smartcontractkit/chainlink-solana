pub const ANCHOR_DISCRIMINATOR: usize = 8;

pub const ZERO_DATA_ID: [u8; 16] = [0; 16];

pub const ZERO_ADDRESS: [u8; 20] = [0; 20];

pub const MAX_WORKFLOW_METADATAS: usize = 16;

// derived from hash::hash("global:cache_submit".as_bytes()).to_bytes()[..8].to_vec()
pub const SUBMIT_DISCRIMINATOR: [u8; 8] = [173, 69, 171, 96, 179, 143, 243, 226];

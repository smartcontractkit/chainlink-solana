//! A port of the EVM `DataId.sol`. A data ID is one 16-byte word, laid out as follows:
//!
//! ```text
//! byte   0               8        9           11         12         16
//!        +---------------+--------+-----------+----------+----------+
//!        | CompactFeedId | Index  | SvrPathId | Instance | Reserved |
//!        |     bytes8    | uint8  |   bytes2  |  bytes1  | 4 bytes  |
//!        +---------------+--------+-----------+----------+----------+
//! ```
//!
//! The store serves only data IDs whose SVR path ID is zero. Non-zero SVR path IDs are reserved
//! for SVR, which isn't supported yet. The instance and the reserved bytes carry no rule: two data
//! IDs that differ only there are separate feeds, with separate histories and descriptions.

/// A data ID as it appears in reports, instructions and events: 16 bytes.
pub type DataId = [u8; 16];

/// A data ID as accounts store it and derive their addresses from: zero padded to 32 bytes. Data
/// IDs are 16 bytes in reports and events, as on EVM, but accounts are keyed on 32 bytes so that a
/// wider data ID format later keeps every existing account address.
pub type DataIdSeed = [u8; 32];

/// Bytes 9 and 10 of a data ID hold its SVR path ID.
const SVR_PATH: std::ops::Range<usize> = 9..11;

/// Returns whether every byte of the data ID is zero.
pub fn is_zero(data_id: &DataId) -> bool {
    *data_id == [0; 16]
}

/// Returns the 32-byte seed a data ID's accounts are keyed on: the data ID followed by 16 zero
/// bytes.
pub fn to_seed(data_id: &DataId) -> DataIdSeed {
    let mut seed = [0u8; 32];
    seed[..16].copy_from_slice(data_id);
    seed
}

/// Returns whether a data ID has a non-zero SVR path ID. The store doesn't support SVR yet, and
/// these bytes stay reserved so that data IDs keep their layout if SVR is added later.
pub fn has_svr_path(data_id: &DataId) -> bool {
    data_id[SVR_PATH] != [0, 0]
}

/// Returns whether the store serves a data ID, which needs the data ID to be non-zero and its SVR
/// path ID to be zero.
pub fn is_valid_data_id(data_id: &DataId) -> bool {
    !is_zero(data_id) && !has_svr_path(data_id)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn id(feed: [u8; 8], index: u8, path: [u8; 2], instance: u8, reserved: [u8; 4]) -> DataId {
        let mut id = [0u8; 16];
        id[..8].copy_from_slice(&feed);
        id[8] = index;
        id[9..11].copy_from_slice(&path);
        id[11] = instance;
        id[12..].copy_from_slice(&reserved);
        id
    }

    const FEED: [u8; 8] = [0x00, 0x03, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66];

    #[test]
    fn svr_paths_are_reserved() {
        let plain = id(FEED, 1, [0, 0], 2, [0; 4]);
        let svr = id(FEED, 1, [0, 7], 2, [0; 4]);
        assert!(!has_svr_path(&plain));
        assert!(has_svr_path(&svr));
        assert!(is_valid_data_id(&plain));
        assert!(!is_valid_data_id(&svr), "SVR path IDs are not served");
        assert!(!is_valid_data_id(&[0; 16]));
    }

    #[test]
    fn seed_zero_pads_the_data_id() {
        let full = id(FEED, 3, [0, 0], 4, [9, 9, 9, 9]);
        let seed = to_seed(&full);
        assert_eq!(&seed[..16], &full);
        assert_eq!(&seed[16..], &[0u8; 16]);
    }

    #[test]
    fn reserved_bytes_are_part_of_the_id() {
        let a = id(FEED, 0, [0, 0], 0, [0; 4]);
        let b = id(FEED, 0, [0, 0], 0, [0, 0, 0, 1]);
        assert!(is_valid_data_id(&a) && is_valid_data_id(&b));
        // They are separate feeds: different seeds give different feed accounts.
        assert_ne!(to_seed(&a), to_seed(&b));
        let state = anchor_lang::prelude::Pubkey::new_unique();
        assert_ne!(
            crate::utils::feed_address(&state, &a),
            crate::utils::feed_address(&state, &b)
        );
    }
}

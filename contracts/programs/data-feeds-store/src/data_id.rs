//! Port of `DataId.sol`. A data ID is one 16-byte word laid out as:
//!
//! ```text
//! byte   0               8        9           11         12         16
//!        +---------------+--------+-----------+----------+----------+
//!        | CompactFeedId | Index  | SvrPathId | Instance | Reserved |
//!        |     bytes8    | uint8  |   bytes2  |  bytes1  | 4 bytes  |
//!        +---------------+--------+-----------+----------+----------+
//! ```
//!
//!   data ID            SVR path ID zero (non-zero SVR path IDs are reserved, not supported yet)
//!   indexed feed ID    SVR path ID, instance and reserved bytes zero
//!
//! Reserved bytes carry no invariant on a data ID, two data IDs differing only there keep separate
//! histories. An indexed feed ID drops them along with the path and the instance.

pub type DataId = [u8; 16];

/// A data ID as accounts store it and derive their addresses from: zero padded to 32 bytes. Data
/// IDs are 16 bytes on the wire, in events and in callback arguments (as on EVM), accounts are
/// keyed on 32 bytes so a wider data ID format later keeps every existing account address.
pub type DataIdSeed = [u8; 32];

const SVR_PATH: std::ops::Range<usize> = 9..11;
const INSTANCE: usize = 11;
const RESERVED: std::ops::Range<usize> = 12..16;

pub fn is_zero(data_id: &DataId) -> bool {
    *data_id == [0; 16]
}

/// The 32-byte account key of a data ID: the data ID followed by 16 zero bytes.
pub fn to_seed(data_id: &DataId) -> DataIdSeed {
    let mut seed = [0u8; 32];
    seed[..16].copy_from_slice(data_id);
    seed
}

/// Whether a data ID carries an SVR path ID. The store does not support SVR yet, the bytes stay
/// reserved so data IDs keep their layout if SVR is added later.
pub fn has_svr_path(data_id: &DataId) -> bool {
    data_id[SVR_PATH] != [0, 0]
}

/// The indexed feed ID of the feed a data ID belongs to, dropping the SVR path ID, the instance
/// and the reserved bytes.
pub fn to_indexed_feed_id(data_id: &DataId) -> DataId {
    let mut id = *data_id;
    id[SVR_PATH].fill(0);
    id[INSTANCE] = 0;
    id[RESERVED].fill(0);
    id
}

/// Whether a data ID is one the store serves: not zero, with a zero SVR path ID.
pub fn is_valid_data_id(data_id: &DataId) -> bool {
    !is_zero(data_id) && !has_svr_path(data_id)
}

/// Whether a data ID is a valid indexed feed ID: SVR path ID, instance and reserved bytes zero.
pub fn is_valid_indexed_feed_id(data_id: &DataId) -> bool {
    !is_zero(data_id)
        && !has_svr_path(data_id)
        && data_id[INSTANCE] == 0
        && data_id[RESERVED] == [0; 4]
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
    fn indexed_feed_id() {
        let full = id(FEED, 3, [1, 0], 4, [9, 9, 9, 9]);
        assert_eq!(to_indexed_feed_id(&full), id(FEED, 3, [0, 0], 0, [0; 4]));
        assert!(is_valid_indexed_feed_id(&id(FEED, 3, [0, 0], 0, [0; 4])));
        assert!(!is_valid_indexed_feed_id(&id(FEED, 3, [0, 0], 1, [0; 4])));
        assert!(!is_valid_indexed_feed_id(&id(
            FEED,
            3,
            [0, 0],
            0,
            [0, 0, 0, 1]
        )));
        assert!(!is_valid_indexed_feed_id(&id(FEED, 3, [0, 1], 0, [0; 4])));
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
        assert_ne!(
            a, b,
            "two IDs differing only in the reserved bytes are distinct"
        );
        assert_eq!(to_indexed_feed_id(&a), to_indexed_feed_id(&b));
    }
}

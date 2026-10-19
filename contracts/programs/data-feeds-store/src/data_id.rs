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
//!   data ID            any SVR path ID, any instance
//!   non-SVR data ID    SVR path ID zero
//!   SVR data ID        SVR path ID non-zero
//!   indexed feed ID    SVR path ID, instance and reserved bytes zero
//!
//! Reserved bytes carry no invariant on a data ID, two data IDs differing only there keep separate
//! histories. An indexed feed ID drops them along with the path and the instance.

pub type DataId = [u8; 16];
pub type SvrPathId = [u8; 2];

const SVR_PATH: std::ops::Range<usize> = 9..11;
const INSTANCE: usize = 11;
const RESERVED: std::ops::Range<usize> = 12..16;

pub fn is_zero(data_id: &DataId) -> bool {
    *data_id == [0; 16]
}

/// Whether a data ID targets an SVR path.
pub fn is_svr(data_id: &DataId) -> bool {
    data_id[SVR_PATH] != [0, 0]
}

pub fn svr_path_id(data_id: &DataId) -> SvrPathId {
    [data_id[9], data_id[10]]
}

/// The non-SVR data ID of the feed instance a data ID belongs to, clearing the SVR path ID.
pub fn to_non_svr(data_id: &DataId) -> DataId {
    let mut id = *data_id;
    id[SVR_PATH].fill(0);
    id
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

/// Whether a data ID is a valid non-SVR data ID.
pub fn is_valid_non_svr(data_id: &DataId) -> bool {
    !is_svr(data_id)
}

/// Whether a data ID is a valid SVR data ID, which also rejects the zero data ID.
pub fn is_valid_svr(data_id: &DataId) -> bool {
    is_svr(data_id)
}

/// Whether a data ID is a valid indexed feed ID: SVR path ID, instance and reserved bytes zero.
pub fn is_valid_indexed_feed_id(data_id: &DataId) -> bool {
    !is_svr(data_id) && data_id[INSTANCE] == 0 && data_id[RESERVED] == [0; 4]
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
    fn svr_path() {
        let non_svr = id(FEED, 1, [0, 0], 2, [0; 4]);
        let svr = id(FEED, 1, [0, 7], 2, [0; 4]);
        assert!(!is_svr(&non_svr));
        assert!(is_svr(&svr));
        assert_eq!(svr_path_id(&svr), [0, 7]);
        assert_eq!(to_non_svr(&svr), non_svr);
        assert!(is_valid_svr(&svr) && !is_valid_svr(&non_svr) && !is_valid_svr(&[0; 16]));
        assert!(is_valid_non_svr(&non_svr) && !is_valid_non_svr(&svr));
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
    fn reserved_bytes_are_part_of_the_id() {
        let a = id(FEED, 0, [0, 0], 0, [0; 4]);
        let b = id(FEED, 0, [0, 0], 0, [0, 0, 0, 1]);
        assert_ne!(to_non_svr(&a), to_non_svr(&b));
        assert_eq!(to_indexed_feed_id(&a), to_indexed_feed_id(&b));
    }
}

//! Chainlink feed client for Solana.
#![deny(rustdoc::all)]
#![allow(rustdoc::missing_doc_code_examples)]
#![deny(missing_docs)]

pub(crate) mod data_feeds_store_v1 {
    use borsh::{BorshDeserialize, BorshSerialize};
    use solana_program::pubkey::Pubkey;

    solana_program::declare_id!("HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny");

    pub(crate) const TRANSMISSIONS_DISCRIMINATOR: [u8; 8] = [96, 179, 69, 66, 128, 129, 73, 117];

    /// Transmissions includes the header information
    #[derive(BorshSerialize, BorshDeserialize, Clone)]
    pub(crate) struct Transmissions {
        pub(crate) version: u8,
        pub(crate) state: u8,
        pub(crate) owner: Pubkey,
        pub(crate) proposed_owner: Pubkey,
        pub(crate) writer: Pubkey,
        pub(crate) description: [u8; 32],
        pub(crate) decimals: u8,
        pub(crate) flagging_threshold: u32,
        pub(crate) latest_round_id: u32,
        pub(crate) granularity: u8,
        pub(crate) live_length: u32,
        pub(crate) live_cursor: u32,
        pub(crate) historical_cursor: u32,
    }

    pub(crate) const HEADER_SIZE: usize = 192;

    /// Internal representation of a single transmission
    #[repr(C)]
    #[derive(
        Debug,
        Default,
        Clone,
        Copy,
        PartialEq,
        Eq,
        PartialOrd,
        Ord,
        bytemuck::Pod,
        bytemuck::Zeroable,
    )]
    pub(crate) struct Transmission {
        pub(crate) slot: u64,
        pub(crate) timestamp: u32,
        pub(crate) _padding0: u32,
        pub(crate) answer: i128,
        pub(crate) _padding1: u64,
        pub(crate) _padding2: u64,
    }
}

/// Version 2 of SDK directly reads feed account data.
/// SDK deserializes underlying account layout and returns a
/// user-friendly `Feed` struct which can be used to return additional data.
/// Deserializing or reading account layout or Borsh deserializing
/// on your own by the client is highly discouraged and not supported by Chainlink
/// (due to underlying data layout changes)
/// You should rely on this SDK and deal with the `Feed` struct only.
pub mod v2 {
    use borsh::{BorshDeserialize, BorshSerialize};
    use bytemuck::pod_read_unaligned;
    use std::fmt;
    use std::{cell::Ref, convert::TryInto, mem::size_of};

    use super::data_feeds_store_v1::{
        Transmission, Transmissions, HEADER_SIZE, ID, TRANSMISSIONS_DISCRIMINATOR,
    };
    /// Represents a single oracle round.
    #[derive(BorshSerialize, BorshDeserialize)]
    pub struct Round {
        /// The round id.
        pub round_id: u32,
        /// Slot at the time the report was received on chain.
        pub slot: u64,
        /// Round timestamp, as reported by the oracle.
        pub timestamp: u32,
        /// Current answer, formatted to `decimals` decimal places.
        pub answer: i128,
    }

    /// Read Errors
    #[derive(Debug)]
    pub enum ReadError {
        /// Invalid Account Owner
        InvalidOwner,
        /// Invalid Discriminator
        InvalidDiscriminator,
        /// Account invalid
        InvalidAccount,
        /// Transmissions deserialization failed
        DeserializeFailed,
        /// Feed Length is not 1
        FeedLengthInvalid,
        /// Feed data missing
        MalformedData,
        /// No transmission found
        TransmissionNotFound,
    }

    // Implement Display so it can be formatted nicely
    impl fmt::Display for ReadError {
        fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
            match self {
                ReadError::InvalidOwner => write!(f, "Invalid account owner"),
                ReadError::InvalidDiscriminator => write!(f, "Invalid discriminator"),
                ReadError::InvalidAccount => write!(f, "Invalid account"),
                ReadError::DeserializeFailed => write!(f, "Failed to deserialize transmissions"),
                ReadError::FeedLengthInvalid => write!(f, "Feed length invalid"),
                ReadError::MalformedData => write!(f, "Malformed feed data"),
                ReadError::TransmissionNotFound => write!(f, "No transmission found"),
            }
        }
    }

    // Implement std::error::Error so it works with `?` and libraries
    impl std::error::Error for ReadError {}

    /// Feed consists of metadata header and transmission
    pub struct Feed {
        /// Header contains important metadata
        _header: Transmissions,
        /// Contains a single transmission
        _live: Transmission,
    }

    impl Feed {
        /// Returns round data for the latest round.
        pub fn latest_round_data(&self) -> Option<Round> {
            if self._header.latest_round_id == 0 {
                return None;
            }

            Some(Round {
                round_id: self._header.latest_round_id,
                slot: self._live.slot,
                timestamp: self._live.timestamp,
                answer: self._live.answer,
            })
        }

        /// Returns the feed description.
        pub fn description(&self) -> [u8; 32] {
            self._header.description
        }

        /// Returns the amount of decimal places.
        pub fn decimals(&self) -> u8 {
            self._header.decimals
        }

        /// Query the feed version.
        pub fn version(&self) -> u8 {
            self._header.version
        }
    }

    /// Reads the feed account’s data slice.
    ///
    /// Example:
    /// ```ignore
    /// read_feed_v2(account_info.try_borrow_data()?, account_info.owner.to_bytes());
    /// ```
    ///
    /// The caller is responsible for providing both:
    /// - the account’s **data** (via `AccountInfo::try_borrow_data()`), and  
    /// - the account’s **owner** (via `AccountInfo::owner.to_bytes()`).
    ///
    /// Ensure these values come from the same feed account.
    ///
    // DEV: Method does not expose `AccountInfo` to avoid
    // dependency from `anchor-lang` or `solana-program` version
    pub fn read_feed_v2(
        data: Ref<&mut [u8]>,
        owner: [u8; 32],
    ) -> std::result::Result<Feed, ReadError> {
        if !data.starts_with(&TRANSMISSIONS_DISCRIMINATOR) {
            return Err(ReadError::InvalidDiscriminator);
        }

        if owner != ID.to_bytes() {
            return Err(ReadError::InvalidOwner);
        }

        let header = Transmissions::deserialize(&mut &data[8..])
            .map_err(|_| ReadError::DeserializeFailed)?;

        // Validate that only one transmission is present. The SDK previously supported
        // multiple transmissions but is now restricted to single transmission feeds.// multiple transmissions but is now restricted to single transmission feeds.
        if header.live_length != 1 {
            return Err(ReadError::FeedLengthInvalid);
        }

        if header.latest_round_id == 0 {
            return Err(ReadError::TransmissionNotFound);
        }

        let (_header, rest) = data.split_at(8 + HEADER_SIZE);

        let array: &[u8; 48] = rest
            .get(..size_of::<Transmission>())
            .and_then(|s| s.try_into().ok())
            .ok_or(ReadError::MalformedData)?;

        let live_transmission: Transmission = pod_read_unaligned(array);

        let feed = Feed {
            _header: header,
            _live: live_transmission,
        };

        Ok(feed)
    }
}

#[cfg(test)]
mod tests {
    use borsh::BorshSerialize;
    use std::convert::TryInto;
    use std::mem::size_of;

    use super::data_feeds_store_v1::{Transmission, Transmissions, HEADER_SIZE, ID};
    use super::v2::read_feed_v2;
    use anchor_lang::prelude::{AccountInfo, Pubkey as AnchorPubkey};
    use solana_program::{hash, pubkey::Pubkey as SolanaPubkey};

    fn mock_account_info<'a>(
        key: &'a AnchorPubkey,
        is_signer: bool,
        is_writable: bool,
        lamports: &'a mut u64,
        data: &'a mut [u8],
        owner: &'a AnchorPubkey,
    ) -> AccountInfo<'a> {
        AccountInfo::new(
            key,
            is_signer,
            is_writable,
            lamports,
            data,
            owner,
            false, // executable
            0,     // rent_epoch
        )
    }

    fn discriminator(name: &str) -> [u8; 8] {
        let preimage = format!("account:{}", name);
        let result = hash::hash(preimage.as_bytes()).to_bytes();
        result[0..8].try_into().unwrap()
    }

    #[test]
    fn test_feed_read() -> Result<(), Box<dyn std::error::Error>> {
        pub const T_START: usize = 8 + HEADER_SIZE;

        pub const T_END: usize = 8 + HEADER_SIZE + size_of::<Transmission>();

        let mut buffer = [0u8; 8 + HEADER_SIZE + size_of::<Transmission>()];

        let header = Transmissions {
            version: 1,
            state: 0,
            owner: SolanaPubkey::default(),
            proposed_owner: SolanaPubkey::default(),
            writer: SolanaPubkey::default(),
            description: [0; 32],
            decimals: 8,
            flagging_threshold: 42,
            latest_round_id: 10,
            granularity: 1,
            live_length: 1,
            live_cursor: 0,
            historical_cursor: 0,
        };

        let discriminator = discriminator("Transmissions"); // [u8; 8]
        print!("discriminator {:?}", discriminator);
        buffer[..8].copy_from_slice(&discriminator);

        header.serialize(&mut &mut buffer[8..]).unwrap();

        let dummy_tx = Transmission {
            slot: 123,
            timestamp: 1,
            _padding0: 0,
            answer: 12,
            _padding1: 0,
            _padding2: 2,
        };

        let tx_bytes = bytemuck::bytes_of(&dummy_tx);
        buffer[T_START..T_END].copy_from_slice(tx_bytes);

        let mut new_buffer = [0u8; 8 + HEADER_SIZE + size_of::<Transmission>()];

        // copy to new buffer to make the data unaligned intentionally
        new_buffer.copy_from_slice(&buffer);

        let key = AnchorPubkey::new_unique();
        let owner = AnchorPubkey::new_unique();
        let mut lamports = 0;

        // test unaligned data
        let account =
            mock_account_info(&key, true, true, &mut lamports, &mut new_buffer[..], &owner);

        // We pass in the owner program ID this way for testing purposes only.
        // For ordinary usage in production applications you must pass in owner (as bytes)
        // from the AccountInfo struct. See `read_feed_v2` comments for more detail.
        let feed = read_feed_v2(account.try_borrow_data()?, ID.to_bytes())?;

        let round = feed.latest_round_data().unwrap();
        assert_eq!(round.answer, 12);

        // test aligned data
        let other_account = mock_account_info(&key, true, true, &mut lamports, &mut buffer, &owner);
        let other_feed = read_feed_v2(other_account.try_borrow_data()?, ID.to_bytes())?;

        let other_round = other_feed.latest_round_data().unwrap();
        assert_eq!(other_round.answer, 12);

        Ok(())
    }
}

//! Chainlink feed client for Solana.
#![deny(rustdoc::all)]
#![allow(rustdoc::missing_doc_code_examples)]
#![deny(missing_docs)]

use anchor_lang::prelude::{borsh::{BorshDeserialize, BorshSerialize}, *};
use std::{convert::TryInto, mem::size_of};
use bytemuck;

use crate::data_feeds_v1_store::{Transmission, Transmissions, HEADER_SIZE, TRANSMISSIONS_DISCRIMINATOR};

pub(crate) mod data_feeds_v1_store {
    use anchor_lang::prelude::{borsh::{BorshDeserialize, BorshSerialize}, *};

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

    #[constant]
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

/// Potential Errors
#[derive(Debug)]
pub enum ReadError {
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
}

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

/// doc
pub fn read_feed_v2(account: AccountInfo) ->  std::result::Result<Feed, ReadError> {
    let data = account.try_borrow_data().map_err(|_| ReadError::InvalidAccount)?;

     if !data.starts_with(&TRANSMISSIONS_DISCRIMINATOR) {
        return Err(ReadError::InvalidDiscriminator);
    }

    let header = Transmissions::deserialize(&mut &data[8..])
        .map_err(|_| ReadError::DeserializeFailed)?;

    if header.live_length != 1 {
        return Err(ReadError::FeedLengthInvalid);
    }

    let (_header, rest) = data.split_at(8 + HEADER_SIZE);

    let array: &[u8; 48] = rest
        .get(..size_of::<Transmission>())
        .and_then(|s| s.try_into().ok())
        .ok_or(ReadError::MalformedData)?;

    let live_transmission = *bytemuck::from_bytes::<Transmission>(array);

    let feed = Feed {
        _header: header,
        _live: live_transmission,
    };

    Ok(feed)
}

#[cfg(test)]
mod tests {
    use std::convert::TryInto;

    use super::{data_feeds_v1_store::HEADER_SIZE, Transmission, Transmissions, read_feed_v2};
    use anchor_lang::{solana_program::{hash}, prelude::{AccountInfo, AnchorSerialize, Pubkey, *}};

    fn mock_account_info<'a>(
        key: &'a Pubkey,
        is_signer: bool,
        is_writable: bool,
        lamports: &'a mut u64,
        data: &'a mut [u8],
        owner: &'a Pubkey,
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
    fn test_feed_read() {
        #[constant]
        pub const T_START: usize = 8 + HEADER_SIZE;

        #[constant]
        pub const T_END: usize = 8 + HEADER_SIZE + size_of::<Transmission>();

        let alignment = std::mem::align_of::<Transmission>();

        print!("alignment {:?}", alignment);

        let mut buffer = [0u8; 8 + HEADER_SIZE + size_of::<Transmission>()];

        let header = Transmissions {
            version: 1,
            state: 0,
            owner: Pubkey::default(),
            proposed_owner: Pubkey::default(),
            writer: Pubkey::default(),
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

        // let data = RefCell::new(&mut buffer[..]);

        let key = Pubkey::new_unique();
        let owner = Pubkey::new_unique();
        let mut lamports = 0;

        let account = mock_account_info(
            &key,
            true,
            true,
            &mut lamports,
            &mut buffer[..],
            &owner,
        );

        let feed = read_feed_v2(account).unwrap();

        let round = feed.latest_round_data().unwrap();
        assert_eq!(round.answer, 12);
    }


}



//! Chainlink feed client for Solana.
#![deny(rustdoc::all)]
#![allow(rustdoc::missing_doc_code_examples)]
#![deny(missing_docs)]
extern crate borsh as external_borsh;
use external_borsh::{BorshDeserialize, BorshSerialize};

use solana_program::{
    account_info::AccountInfo,
    instruction::{AccountMeta, Instruction},
    program::invoke,
    program_error::ProgramError,
    pubkey::Pubkey,
};

// The library uses this to verify the keys
solana_program::declare_id!("HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny");

#[derive(BorshSerialize, BorshDeserialize)]
enum Query {
    Version,
    Decimals,
    Description,
    RoundData { round_id: u32 },
    LatestRoundData,
    Aggregator,
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

fn query<'info, T: BorshDeserialize>(
    program_id: AccountInfo<'info>,
    feed: AccountInfo<'info>,
    scope: Query,
) -> Result<T, ProgramError> {
    use std::io::{Cursor, Write};

    const QUERY_INSTRUCTION_DISCRIMINATOR: &[u8] =
        &[0x27, 0xfb, 0x82, 0x9f, 0x2e, 0x88, 0xa4, 0xa9];

    // Avoid array resizes by using the maximum response size as the initial capacity.
    const MAX_SIZE: usize = QUERY_INSTRUCTION_DISCRIMINATOR.len() + std::mem::size_of::<Pubkey>();

    let mut data = Cursor::new(Vec::with_capacity(MAX_SIZE));
    data.write_all(QUERY_INSTRUCTION_DISCRIMINATOR)?;
    scope.serialize(&mut data)?;

    let ix = Instruction {
        program_id: *program_id.key,
        accounts: vec![AccountMeta::new_readonly(*feed.key, false)],
        data: data.into_inner(),
    };

    invoke(&ix, &[feed.clone()])?;

    let (_key, data) =
        solana_program::program::get_return_data().expect("chainlink store had no return_data!");
    let data = T::try_from_slice(&data)?;
    Ok(data)
}

/// Query the feed version.
pub fn version<'info>(
    program_id: AccountInfo<'info>,
    feed: AccountInfo<'info>,
) -> Result<u8, ProgramError> {
    query(program_id, feed, Query::Version)
}

/// Returns the amount of decimal places.
pub fn decimals<'info>(
    program_id: AccountInfo<'info>,
    feed: AccountInfo<'info>,
) -> Result<u8, ProgramError> {
    query(program_id, feed, Query::Decimals)
}

/// Returns the feed description.
pub fn description<'info>(
    program_id: AccountInfo<'info>,
    feed: AccountInfo<'info>,
) -> Result<String, ProgramError> {
    query(program_id, feed, Query::Description)
}

/// Returns round data for the latest round.
pub fn latest_round_data<'info>(
    program_id: AccountInfo<'info>,
    feed: AccountInfo<'info>,
) -> Result<Round, ProgramError> {
    query(program_id, feed, Query::LatestRoundData)
}

/// Returns the address of the underlying OCR2 aggregator.
pub fn aggregator<'info>(
    program_id: AccountInfo<'info>,
    feed: AccountInfo<'info>,
) -> Result<Pubkey, ProgramError> {
    query(program_id, feed, Query::Aggregator)
}

/// Support direct reads from feed accounts
pub mod direct {
    use super::Round;
    use std::{cell::Ref, convert::TryInto, mem::size_of};

    use anchor_lang::prelude::*;

    ///
    #[constant]
    pub const HEADER_SIZE: usize = 192;

    /// Potential Errors
    #[derive(Debug)]
    pub enum ReadError {
        /// Transmissions deserialization failed
        DeserializeFailed,
        /// Feed Length is not 1
        FeedLengthInvalid,
        /// Feed data missing
        MalformedData,
    }

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
    pub struct Transmission {
        slot: u64,
        timestamp: u32,
        _padding0: u32,
        answer: i128,
        _padding1: u64,
        _padding2: u64,
    }

    /// Feed consists of metadata header and transmission
    pub struct Feed {
        /// Header contains important metadata
        header: Transmissions,
        /// Contains a single transmission
        live: Transmission,
    }

    /// Transmissions includes the header information
    #[account]
    pub struct Transmissions {
        version: u8,
        state: u8,
        owner: Pubkey,
        proposed_owner: Pubkey,
        writer: Pubkey,
        description: [u8; 32],
        decimals: u8,
        flagging_threshold: u32,
        latest_round_id: u32,
        granularity: u8,
        live_length: u32,
        live_cursor: u32,
        historical_cursor: u32,
    }

    /// Reads account's data. Pass in account_info.try_borrow_data()?;
    pub fn read<'a>(data: Ref<&'a mut [u8]>) -> std::result::Result<Feed, ReadError> {
        let header = Transmissions::try_deserialize(&mut &data[..])
            .map_err(|_| ReadError::DeserializeFailed)?;

        let (_header, rest) = data.split_at(8 + HEADER_SIZE);

        let array: &[u8; 48] = rest
            .get(..size_of::<Transmission>())
            .and_then(|s| s.try_into().ok())
            .ok_or(ReadError::MalformedData)?;

        let live_trans = *bytemuck::from_bytes::<Transmission>(array);

        let feed = Feed {
            header,
            live: live_trans,
        };

        if feed.header.live_length != 1 {
            return Err(ReadError::FeedLengthInvalid);
        }

        Ok(feed)
    }

    impl Feed {
        /// Returns round data for the latest round.
        pub fn latest_round_data(&self) -> Option<Round> {
            if self.header.latest_round_id == 0 {
                return None;
            }

            Some(Round {
                round_id: self.header.latest_round_id,
                slot: self.live.slot,
                timestamp: self.live.timestamp,
                answer: self.live.answer,
            })
        }

        /// Returns the feed description.
        pub fn description(&self) -> [u8; 32] {
            self.header.description
        }

        /// Returns the amount of decimal places.
        pub fn decimals(&self) -> u8 {
            self.header.decimals
        }

        /// Returns the address of the underlying OCR2 aggregator.
        pub fn aggregator(&self) -> Pubkey {
            self.header.writer
        }

        /// Query the feed version.
        pub fn version(&self) -> u8 {
            self.header.version
        }
    }

    #[test]
    fn test_feed_read() {
        use anchor_lang::AnchorSerialize;
        use anchor_lang::Discriminator;
        use std::cell::RefCell;

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

        let discriminator = Transmissions::discriminator(); // [u8; 8]
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

        let data = RefCell::new(&mut buffer[..]);

        print!("hello asdfadfa");
        let feed = read(data.borrow()).unwrap();
        print!("pinecone");

        assert_eq!(feed.header.version, 1);
        assert_eq!(feed.live.slot, 123);

        let x = feed.latest_round_data().unwrap();

        assert_eq!(x.answer, 12);
    }
}

//! Chainlink feed client for Solana.
#![deny(rustdoc::all)]
#![allow(rustdoc::missing_doc_code_examples)]
#![deny(missing_docs)]

use borsh::{BorshDeserialize, BorshSerialize};

use solana_program::{
    account_info::AccountInfo,
    instruction::{AccountMeta, Instruction},
    program::invoke,
    entrypoint::ProgramResult,
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

/// Checks that the supplied program ID is the correct one for chainlink
pub fn check_program_account(chainlink_program_id: &Pubkey) -> ProgramResult {
    if chainlink_program_id != &id() {
        return Err(ProgramError::IncorrectProgramId);
    }
    Ok(())
}


fn query<T: BorshDeserialize>(
    chainlink_program_id: &Pubkey,
    feed: AccountInfo,
    scope: Query,
) -> Result<T, ProgramError> {
    use std::io::{Cursor, Write};
    check_program_account(chainlink_program_id)?;

    const QUERY_INSTRUCTION_DISCRIMINATOR: &[u8] =
        &[0x27, 0xfb, 0x82, 0x9f, 0x2e, 0x88, 0xa4, 0xa9];

    // Avoid array resizes by using the maximum response size as the initial capacity.
    const MAX_SIZE: usize = QUERY_INSTRUCTION_DISCRIMINATOR.len() + std::mem::size_of::<Pubkey>();

    let mut data = Cursor::new(Vec::with_capacity(MAX_SIZE));
    data.write_all(QUERY_INSTRUCTION_DISCRIMINATOR)?;
    scope.serialize(&mut data)?;

    let ix = Instruction {
        program_id: *chainlink_program_id,
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
pub fn version(
    program_id: &Pubkey,
    feed: AccountInfo,
) -> Result<u8, ProgramError> {
    query(program_id, feed, Query::Version)
}

/// Returns the amount of decimal places.
pub fn decimals(
    program_id: &Pubkey,
    feed: AccountInfo,
) -> Result<u8, ProgramError> {
    query(program_id, feed, Query::Decimals)
}

/// Returns the feed description.
pub fn description(
    program_id: &Pubkey,
    feed: AccountInfo,
) -> Result<String, ProgramError> {
    query(program_id, feed, Query::Description)
}

/// Returns round data for the latest round.
pub fn latest_round_data(
    program_id: &Pubkey,
    feed: AccountInfo,
) -> Result<Round, ProgramError> {
    query(program_id, feed, Query::LatestRoundData)
}

/// Returns the address of the underlying OCR2 aggregator.
pub fn aggregator(
    program_id: &Pubkey,
    feed: AccountInfo,
) -> Result<Pubkey, ProgramError> {
    query(program_id, feed, Query::Aggregator)
}

pub mod entrypoint;
pub mod error;
pub mod instruction;
pub mod processor;
pub mod state;

pub fn decimal(i: usize, decimals: u8) -> u128 {
    let decimals = 10u128.pow(decimals as u32);

    i as u128 * decimals
}

use solana_program::{account_info::AccountInfo, program_error::ProgramError, pubkey::Pubkey};

solana_program::declare_id!("2yqG9bzKHD59MxD9q7ExLvnDhNycB3wkvKXFQSpBoiaE");

/// Returns the current price, `None` if unavailable.
pub fn get_price(
    program_id: &Pubkey,
    account_info: &AccountInfo,
) -> Result<Option<state::Value>, ProgramError> {
    // TODO: use bytemuck with offset to avoid deserializing the whole account
    processor::get_account_data::<state::Aggregator>(account_info, program_id)
        .map(|state| state.answer)
}

/// Returns a list of (oracle, submission) pairs.
pub fn get_submissions(
    program_id: &Pubkey,
    account_info: &AccountInfo,
) -> Result<Vec<(Pubkey, Option<state::Submission>)>, ProgramError> {
    let state = processor::get_account_data::<state::Aggregator>(account_info, program_id)?;

    let data = state
        .config
        .oracles
        .iter()
        .copied()
        .zip(
            state
                .submissions
                .iter()
                .copied()
                .map(|state::Submission(timestamp, value)| {
                    // map invalid data as None
                    if timestamp != 0 {
                        Some(state::Submission(timestamp, value))
                    } else {
                        None
                    }
                }),
        )
        .collect();

    Ok(data)
}

pub fn get_round(
    program_id: &Pubkey,
    account_info: &AccountInfo,
    timestamp: state::Timestamp,
) -> Result<Option<state::Submission>, ProgramError> {
    let _state = processor::get_account_data::<state::Aggregator>(account_info, program_id)?;
    let (pos, rounds) = processor::get_rounds(account_info);
    let i = *pos as usize;

    // iterate over past round values in reverse
    let mut iter = {
        // first search backwards from [..pos]
        rounds[..i]
            .iter()
            .rev()
            // then backwards from [pos..]
            .chain(rounds[i..].iter().rev().take_while(|submission| {
                // immediately stop on zero values (take_while)
                **submission != state::Submission::default()
            }))
    };

    // find the first value right before `timestamp`
    let item = iter.find(|state::Submission(t, _)| *t < timestamp).copied();

    Ok(item)
}

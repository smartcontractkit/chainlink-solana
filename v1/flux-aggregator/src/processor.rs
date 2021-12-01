use solana_program::{
    account_info::{next_account_info, AccountInfo},
    borsh::try_from_slice_unchecked,
    clock::Clock,
    entrypoint::ProgramResult,
    msg,
    program_error::ProgramError,
    program_pack::IsInitialized,
    pubkey::Pubkey,
    rent::Rent,
    sysvar::Sysvar,
};

use borsh::{BorshDeserialize, BorshSerialize};

use crate::{error::Error, instruction::AggregatorInstruction, state::*};

use std::cell::RefMut;

/// Deserializes account and checks it's initialized and owned by the specified program
#[must_use]
pub fn get_account_data<T: BorshDeserialize + IsInitialized>(
    account_info: &AccountInfo,
    owner_program_id: &Pubkey,
) -> Result<T, ProgramError> {
    if account_info.data_is_empty() {
        return Err(ProgramError::UninitializedAccount);
    }
    if account_info.owner != owner_program_id {
        return Err(Error::OwnerMismatch.into());
    }

    let account: T = try_from_slice_unchecked(&account_info.data.borrow()[..4096])?;
    if !account.is_initialized() {
        Err(ProgramError::UninitializedAccount)
    } else {
        Ok(account)
    }
}

/// A set of historical answers.
#[must_use]
pub fn get_rounds<'a>(account: &'a AccountInfo) -> (RefMut<'a, u64>, RefMut<'a, [Submission]>) {
    RefMut::map_split(account.data.borrow_mut(), |data| get_rounds_from_data(data))
}

pub fn get_state_from_data<'a>(data: &'a [u8]) -> Option<Aggregator> {
    let data = &data[..4096];
    try_from_slice_unchecked(&data[..4096]).ok()
}

pub fn get_rounds_from_data<'a>(data: &mut [u8]) -> (&mut u64, &mut [Submission]) {
    let data = &mut data[4096..];
    let (pos, rounds) = data.split_at_mut(8);
    let pos: &mut u64 = bytemuck::from_bytes_mut(pos);
    // align remaining data, trimming any excess bytes. Otherwise bytemuck can panic.
    let len = rounds.len();
    let size = std::mem::size_of::<Submission>();
    let rounds = &mut rounds[..(len / size) * size];
    let rounds = bytemuck::try_cast_slice_mut::<_, Submission>(rounds).unwrap();
    // TODO: this stalls if it fails
    (pos, rounds)
}

pub fn process_instruction(
    program_id: &Pubkey,
    accounts: &[AccountInfo],
    instruction_data: &[u8],
) -> ProgramResult {
    msg!("Beginning processing");
    let instruction = AggregatorInstruction::try_from_slice(instruction_data)
        .map_err(|_| ProgramError::InvalidInstructionData)?;

    match instruction {
        AggregatorInstruction::Initialize(config) => {
            msg!("initialize");
            process_initialize(program_id, accounts, config)
        }
        AggregatorInstruction::Submit { timestamp, value } => {
            msg!("submit");
            process_submit(program_id, accounts, timestamp, value)
        }
    }
}

pub fn process_initialize(
    program_id: &Pubkey,
    accounts: &[AccountInfo],
    config: Config,
) -> ProgramResult {
    let accounts_iter = &mut accounts.iter();
    let aggregator = next_account_info(accounts_iter)?;
    let aggregator_data_len = aggregator.data_len();
    let owner = next_account_info(accounts_iter)?;
    let rent = &Rent::from_account_info(next_account_info(accounts_iter)?)?;
    let clock = &Clock::from_account_info(next_account_info(accounts_iter)?)?;

    if owner.key == &Pubkey::default() {
        msg!("The owner cannot be `Pubkey::default()`.");
        return Err(ProgramError::InvalidArgument);
    }

    if !owner.is_signer {
        Err(Error::MissingSignature)?;
    }

    if !rent.is_exempt(aggregator.lamports(), aggregator_data_len) {
        msg!("Account is not rent exempt.");
        return Err(Error::NotRentExempt.into());
    }

    if config.min_answer_threshold <= 1 {
        return Err(ProgramError::InvalidArgument);
    }

    let state = match get_account_data::<Aggregator>(aggregator, program_id) {
        Ok(mut state) => {
            // this is a reconfigure
            state.config = config;
            state.submissions = Default::default();
            state.answer = Default::default();
            state.updated_at = clock.unix_timestamp;
            state
        }
        Err(ProgramError::UninitializedAccount) => {
            // we're initializing a new account
            Aggregator {
                is_initialized: true,
                version: 1,
                config,
                owner: *owner.key,
                submissions: Default::default(),
                answer: Default::default(),
                updated_at: clock.unix_timestamp,
            }
        }
        Err(err) => return Err(err),
    };

    // TODO: ensure we have enough space to serialize the whole struct (& Box heap)

    state.serialize(&mut aggregator.data.borrow_mut().as_mut())?;

    Ok(())
}

pub fn process_submit(
    program_id: &Pubkey,
    accounts: &[AccountInfo],
    timestamp: Timestamp,
    value: Value,
) -> ProgramResult {
    let accounts_iter = &mut accounts.iter();
    let aggregator = next_account_info(accounts_iter)?;
    let oracle = next_account_info(accounts_iter)?;
    let clock = &Clock::from_account_info(next_account_info(accounts_iter)?)?;

    if oracle.key == &Pubkey::default() {
        msg!("The oracle cannot be `Pubkey::default()`.");
        return Err(ProgramError::InvalidArgument);
    }

    if !oracle.is_signer {
        Err(Error::MissingSignature)?;
    }

    let mut state = get_account_data::<Aggregator>(aggregator, program_id)?;

    // check if it's a valid oracle, find it's index
    let oracle_index = state
        .config
        .oracles
        .iter()
        .position(|o| o == oracle.key)
        .ok_or(Error::InvalidOracle)?;

    state.submissions[oracle_index] = Submission(timestamp, value);

    // filter out valid submissions
    let mut submissions: Vec<_> = state
        .submissions
        .iter()
        .filter(|Submission(timestamp, _value)| {
            // skip empty submissions
            *timestamp != 0 &&
            // skip stale submissions
            clock.unix_timestamp.saturating_sub(*timestamp) <= state.config.staleness_threshold as i64
            // TODO: what if the clock here drifts too far from submission timestamps?
            // a) use the current submission timestamp and/or the highest timestamp we see
            // b) use the unix_timestamp available when the value is submitted
        })
        .map(|Submission(_, value)| value) // TODO: maybe filter_map
        .collect();

    if submissions.len() >= state.config.min_answer_threshold as usize {
        // calculate the new round
        submissions.sort();

        let median = match submissions.len() {
            0 => unreachable!(), // should not occur
            1 => *submissions[0],
            len => {
                let mid = len / 2;
                let v1 = submissions[mid - 1];
                let v2 = submissions[mid];

                (v1 + v2) / 2
            }
        };
        state.answer = Some(median);

        // commit old result
        let (mut pos, mut rounds) = get_rounds(aggregator);
        rounds[*pos as usize] = Submission(clock.unix_timestamp, median);
        *pos = (*pos + 1) % rounds.len() as u64;
    }

    state.serialize(&mut aggregator.data.borrow_mut().as_mut())?;

    Ok(())
}

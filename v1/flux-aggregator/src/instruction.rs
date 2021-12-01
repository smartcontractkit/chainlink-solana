use borsh::{BorshDeserialize, BorshSerialize};
use solana_program::{
    instruction::{AccountMeta, Instruction},
    program_error::ProgramError,
    pubkey::Pubkey,
    sysvar,
};

use crate::state::*;

/// Instructions supported by the generic Name Registry program
#[derive(Clone, BorshSerialize, BorshDeserialize, PartialEq)]
pub enum AggregatorInstruction {
    /// Accounts:
    ///   0. [writable] The aggregator to initialize
    ///   1. [sign] Owner
    ///   2. [] Rent sysvar
    ///   3. [] Clock sysvar
    Initialize(Config),
    /// Accounts:
    ///   0. [writable] The aggregator
    ///   1. [sign] Oracle
    ///   2. [] Clock sysvar
    Submit { timestamp: Timestamp, value: Value },
}

pub fn initialize(
    program_id: Pubkey,
    aggregator: &Pubkey,
    owner: &Pubkey,
    config: Config,
) -> Result<Instruction, ProgramError> {
    // TODO: check_program_account(program_id)?;
    let instruction_data = AggregatorInstruction::Initialize(config);

    let data = instruction_data.try_to_vec().unwrap();

    let accounts = vec![
        AccountMeta::new(*aggregator, false),
        AccountMeta::new(*owner, true),
        AccountMeta::new_readonly(sysvar::rent::id(), false),
        AccountMeta::new_readonly(sysvar::clock::id(), false),
    ];

    Ok(Instruction {
        program_id,
        accounts,
        data,
    })
}

pub fn submit(
    program_id: Pubkey,
    aggregator: &Pubkey,
    oracle: &Pubkey,
    timestamp: Timestamp,
    value: Value,
) -> Result<Instruction, ProgramError> {
    // TODO: check_program_account(program_id)?;
    let instruction_data = AggregatorInstruction::Submit { timestamp, value };

    let data = instruction_data.try_to_vec().unwrap();

    let accounts = vec![
        AccountMeta::new(*aggregator, false),
        AccountMeta::new(*oracle, true),
        AccountMeta::new_readonly(sysvar::clock::id(), false),
    ];

    Ok(Instruction {
        program_id,
        accounts,
        data,
    })
}

pub fn reconfigure(
    program_id: Pubkey,
    aggregator: &Pubkey,
    owner: &Pubkey,
    config: Config,
) -> Result<Instruction, ProgramError> {
    initialize(program_id, aggregator, owner, config)
}

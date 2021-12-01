//! Error types

use num_derive::FromPrimitive;
use solana_program::{decode_error::DecodeError, program_error::ProgramError};
use thiserror::Error;

/// Errors that may be returned by the program.
#[derive(Clone, Debug, Eq, Error, FromPrimitive, PartialEq)]
pub enum Error {
    /// Lamport balance below rent-exempt threshold.
    #[error("Lamport balance below rent-exempt threshold")]
    NotRentExempt,
    /// Insufficient funds for the operation requested.
    #[error("Insufficient funds")]
    InsufficientFunds,
    /// Owner does not match.
    #[error("Owner does not match")]
    OwnerMismatch,
    /// Missing signature.
    #[error("Missing signature")]
    MissingSignature,
    /// Invalid oracle
    #[error("Invalid oracle")]
    InvalidOracle,
    /// The account cannot be initialized because it is already being used.
    #[error("Already in use")]
    AlreadyInUse,
    /// State is uninitialized.
    #[error("State is unititialized")]
    UninitializedState,
    /// Invalid instruction
    #[error("Invalid instruction")]
    InvalidInstruction,
    /// State is invalid for requested operation.
    #[error("State is invalid for requested operation")]
    InvalidState,
    /// Operation overflowed
    #[error("Operation overflowed")]
    Overflow,
}
impl From<Error> for ProgramError {
    fn from(e: Error) -> Self {
        ProgramError::Custom(e as u32)
    }
}
impl<T> DecodeError<T> for Error {
    fn type_of() -> &'static str {
        "Error"
    }
}

// impl PrintProgramError for Error {
//     fn print<E>(&self)
//     where
//         E: 'static
//             + std::error::Error
//             + DecodeError<E>
//             + PrintProgramError
//             + num_traits::FromPrimitive,
//     {
//         msg!("{}", self)
//     }
// }

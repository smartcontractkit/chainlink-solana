use anchor_lang::prelude::*;

use crate::{Buffer, Error, Report};

/// Borrowed from chainlink-ccip's offramp program
/// https://github.com/smartcontractkit/chainlink-ccip/blob/main/chains/solana/contracts/programs/ccip-offramp/src/instructions/v1/buffering.rs
pub trait Buffering {
    fn is_initialized(&self) -> bool;
    fn filled_chunks(&self) -> u8;
    fn is_complete(&self) -> bool;
    fn bytes(&self) -> Result<&[u8]>;
    fn add_chunk(
        &mut self,
        report_length: u32,
        chunk: &[u8],
        chunk_index: u8,
        num_chunks: u8,
    ) -> Result<()>;
}

impl Buffering for Buffer {
    fn is_initialized(&self) -> bool {
        !self.data.is_empty()
    }

    fn filled_chunks(&self) -> u8 {
        self.chunk_bitmap.count_ones() as u8
    }

    fn is_complete(&self) -> bool {
        self.is_initialized() && self.filled_chunks() == self.total_chunks
    }

    fn bytes(&self) -> Result<&[u8]> {
        require!(self.is_complete(), Error::Incomplete);

        Ok(&self.data)
    }

    fn add_chunk(
        &mut self,
        report_length: u32,
        chunk: &[u8],
        chunk_index: u8,
        num_chunks: u8,
    ) -> Result<()> {
        require!(
            num_chunks > 0 && chunk_index < num_chunks,
            Error::InvalidChunkIndex
        );
        require!(
            !chunk.is_empty() && chunk.len() <= report_length as usize,
            Error::InvalidChunkSize
        );

        if !self.is_initialized() {
            self.initialize(report_length, chunk.len() as u32, num_chunks, chunk_index)?;
        }

        require_eq!(
            self.data.len(),
            report_length as usize,
            Error::InvalidLength
        );

        let chunk_mask = 1u64 << chunk_index;
        require!(
            chunk_mask & self.chunk_bitmap == 0,
            Error::AlreadyContainsChunk
        );

        require_gte!(
            self.chunk_length,
            chunk.len() as u32,
            Error::InvalidChunkSize
        );

        if chunk.len() < self.chunk_length as usize {
            // Only the terminator (last chunk) can be smaller than the others.
            require_eq!(chunk_index, self.total_chunks - 1, Error::InvalidChunkSize);
        }

        require_gt!(self.total_chunks, chunk_index, Error::InvalidChunkIndex);

        let start = self.chunk_length as usize * chunk_index as usize;
        let end = self.data.len().min(start + chunk.len());
        self.data[start..end].copy_from_slice(chunk);
        self.chunk_bitmap |= chunk_mask;

        Ok(())
    }
}

impl Buffer {
    fn initialize(
        &mut self,
        report_length: u32,
        chunk_length: u32,
        total_chunks: u8,
        chunk_index: u8,
    ) -> Result<()> {
        require!(!self.is_initialized(), Error::AlreadyInitialized);
        require!(
            report_length > 0 && chunk_length <= report_length && chunk_length > 0,
            Error::InvalidLength
        );
        require!(
            total_chunks < 64 && total_chunks > 0,
            Error::InvalidChunkSize
        );

        // If we're initializing with the last chunk, it could be smaller
        // than the rest, so we calculate the chunk size based on the expected
        // size of all the others.
        let is_last = chunk_index == total_chunks - 1;
        let global_chunk_length = if is_last && total_chunks > 1 {
            (report_length - chunk_length) / (total_chunks as u32 - 1)
        } else {
            chunk_length
        };

        require_eq!(
            total_chunks as u32,
            div_ceil(report_length, global_chunk_length),
            Error::InvalidLength,
        );
        self.chunk_length = global_chunk_length;
        self.total_chunks = total_chunks;
        self.data.resize(report_length as usize, 0);
        self.report_length = report_length;

        Ok(())
    }
}

pub fn deserialize_from_buffer_account(
    execution_report_buffer: &AccountInfo,
) -> Result<(Report, usize)> {
    // Ensures the buffer is initialized, and owned by the program.
    require_keys_eq!(
        *execution_report_buffer.owner,
        crate::ID,
        Error::ReportUnavailable
    );
    let buffer = Buffer::try_deserialize(&mut execution_report_buffer.data.borrow().as_ref())?;

    Ok((
        Report::deserialize(&mut buffer.bytes()?).map_err(|_| Error::FailedToDeserializeReport)?,
        buffer.data.len(),
    ))
}

fn div_ceil<T: Into<u32>>(a: T, b: T) -> u32 {
    let (a, b) = (a.into(), b.into());
    assert!(a + b - 1 > 0);
    (a + b - 1) / b
}

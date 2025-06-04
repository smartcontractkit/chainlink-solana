use anchor_lang::prelude::*;

use crate::{Report, Error, Buffer};

/// Borrowed from chainlink-ccip's offramp program
/// TODO: Add link to buffering.rs in chainlink-ccip repo once merged to develop
pub trait Buffering {
    fn is_initialized(&self) -> bool;
    fn filled_chunks(&self) -> u32;
    fn is_complete(&self) -> bool;
    fn bytes(&self) -> Result<&[u8]>;
    fn initialize(&mut self, report_length: u32, chunk_length: u32) -> Result<()>;
    fn add_chunk(&mut self, report_length: u32, chunk: &[u8], chunk_index: u8) -> Result<()>;
    fn recover_wrong_size(
        &mut self,
        report_length: u32,
        new_chunk: &[u8],
        chunk_index: u8,
    ) -> Result<()>;
}

impl Buffering for Buffer {
    fn is_initialized(&self) -> bool {
        self.total_chunks > 0
    }

    fn filled_chunks(&self) -> u32 {
        self.chunk_bitmap.count_ones()
    }

    fn is_complete(&self) -> bool {
        self.is_initialized() && self.filled_chunks() == self.total_chunks
    }

    fn bytes(&self) -> Result<&[u8]> {
        require!(
            self.is_complete(),
            Error::Incomplete
        );

        Ok(&self.data)
    }

    fn initialize(&mut self, report_length: u32, chunk_length: u32) -> Result<()> {
        require!(
            !self.is_initialized(),
            Error::AlreadyInitialized
        );
        require!(
            chunk_length > 0 && report_length >= chunk_length,
            Error::InvalidLength
        );
        self.data.resize(report_length as usize, 0);
        self.total_chunks = (report_length + chunk_length - 1) / chunk_length;
        require_gt!(
            64,
            self.total_chunks,
            Error::ChunkSizeTooSmall
        );
        self.chunk_length = chunk_length;
        self.report_length = report_length;
        Ok(())
    }

    fn add_chunk(&mut self, report_length: u32, chunk: &[u8], chunk_index: u8) -> Result<()> {
        if !self.is_initialized() {
            self.initialize(report_length, chunk.len() as u32)?;
        }

        require_eq!(
            self.report_length,
            report_length,
            Error::InvalidLength
        );

        let chunk_mask = 1u64 << chunk_index;
        require!(
            chunk_mask & self.chunk_bitmap == 0,
            Error::AlreadyContainsChunk
        );

        if chunk.len() as u32 > self.chunk_length {
            // We hit the special case where the first received chunk was the last one
            // in the buffer (terminator), which may be smaller than all others. It's okay,
            // we can recover in place.
            return self.recover_wrong_size(report_length, chunk, chunk_index);
        }

        require_gte!(
            self.chunk_length,
            chunk.len() as u32,
            Error::InvalidChunkSize
        );

        if chunk.len() < self.chunk_length as usize {
            // Only the terminator (last chunk) can be smaller than the others.
            require_eq!(
                chunk_index as u32,
                self.total_chunks - 1,
                Error::InvalidChunkSize
            );
        }

        require_gt!(
            self.total_chunks,
            chunk_index as u32,
            Error::InvalidChunkIndex
        );

        let start = self.chunk_length as usize * chunk_index as usize;
        let end = self.data.len().min(start + chunk.len());
        self.data[start..end].copy_from_slice(chunk);
        self.chunk_bitmap |= chunk_mask;

        Ok(())
    }

    fn recover_wrong_size(
        &mut self,
        report_length: u32,
        new_chunk: &[u8],
        chunk_index: u8,
    ) -> Result<()> {
        // Only makes sense to recover if we got the first chunk wrong (because it was the buffer
        // terminator). Any size mismatch beyond that means the user is sending the chunks incorrectly.
        require_eq!(
            self.filled_chunks(),
            1,
            Error::InvalidChunkSize
        );

        // We extract what we now know is the terminator
        let terminator_index = self.chunk_bitmap.trailing_zeros() as u8;
        let mut terminator = vec![0u8; self.chunk_length as usize];
        let start = terminator_index as usize * self.chunk_length as usize;
        let end = start + terminator.len();
        terminator.copy_from_slice(&self.data[start..end]);

        // We reset the buffer metadata. It's okay to leave the old data in, as it will be clobbered.
        self.chunk_bitmap = 0;
        self.total_chunks = 0;
        self.chunk_length = 0;

        // We reinsert the new chunk and terminator, which will be accepted as it's smaller. From now
        // on, we won't accept bigger chunks than this again.
        self.add_chunk(report_length, new_chunk, chunk_index)?;
        self.add_chunk(report_length, &terminator, terminator_index)?;
        Ok(())
    }
}

pub fn deserialize_from_buffer_account(
    report_buffer: &AccountInfo,
) -> Result<(Report, usize)> {
    // Ensures the buffer is initialized, and owned by the program.
    require_keys_eq!(
        *report_buffer.owner,
        crate::ID,
        Error::ReportUnavailable
    );
    let buffer = Buffer::try_deserialize(
        &mut report_buffer.data.borrow().as_ref(),
    )?;

    Ok((
        Report::deserialize(&mut buffer.bytes()?)
            .map_err(|_| Error::FailedToDeserializeReport)?,
        buffer.data.len(),
    ))
}

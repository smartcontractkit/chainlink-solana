//! The DF1 write-through: keeping existing DF1 feed accounts written with this store's rounds, via
//! DF1's unchanged `submit`, signed by the store's `df1_writer` PDA (set as each DF1 feed's writer).
//!
//! Solana can't contain a failed call to another program: a `submit` that fails aborts every
//! report in the transaction. So everything `submit` could fail on is checked here first, and a
//! round DF1 can't take is skipped for DF1 only (the store still writes it). `submit` itself does no
//! ordering (OCR2 ordered rounds for it), so the store only writes rounds newer than DF1's latest.

use anchor_lang::{
    prelude::*,
    solana_program::{
        instruction::{AccountMeta, Instruction},
        program::invoke_signed,
    },
};

use crate::{
    common::{df1, DF1_WRITER_SEED, STORED_DECIMALS},
    state::Df1SkipReason,
};

fn u32_at(data: &[u8], offset: usize) -> u32 {
    u32::from_le_bytes(data[offset..offset + 4].try_into().unwrap())
}

/// Whether `info` is a DF1 feed account of `program` that `submit` can write: the account layout,
/// its version and the ring geometry `submit` indexes without checking. Not the writer or the round.
#[allow(clippy::manual_is_multiple_of)] // is_multiple_of is newer than the SBF toolchain
pub fn check_layout(
    info: &AccountInfo,
    program: &Pubkey,
) -> std::result::Result<(), Df1SkipReason> {
    if info.owner != program {
        return Err(Df1SkipReason::InvalidAccount);
    }
    let data = info
        .try_borrow_data()
        .map_err(|_| Df1SkipReason::InvalidAccount)?;
    if data.len() < df1::RINGS || data[..8] != df1::TRANSMISSIONS_DISCRIMINATOR {
        return Err(Df1SkipReason::InvalidAccount);
    }
    if data[df1::VERSION] != df1::FEED_VERSION {
        return Err(Df1SkipReason::InvalidVersion);
    }
    let live_length = u64::from(u32_at(&data, df1::LIVE_LENGTH));
    let live_cursor = u64::from(u32_at(&data, df1::LIVE_CURSOR));
    let rings = (data.len() - df1::RINGS) as u64;
    let transmission = df1::TRANSMISSION_LEN as u64;
    // `submit` splits the data into a live and a historical ring and indexes both by cursor, so a
    // geometry it would panic on is not a feed it can write
    if live_length == 0
        || live_cursor >= live_length
        || rings < live_length * transmission
        || (rings - live_length * transmission) % transmission != 0
    {
        return Err(Df1SkipReason::InvalidAccount);
    }
    let historical_length = (rings - live_length * transmission) / transmission;
    if historical_length > 0
        && (data[df1::GRANULARITY] == 0
            || u64::from(u32_at(&data, df1::HISTORICAL_CURSOR)) >= historical_length)
    {
        return Err(Df1SkipReason::InvalidAccount);
    }
    Ok(())
}

/// Rescale an answer with `STORED_DECIMALS` to a DF1 feed's decimals, truncating toward zero.
pub fn rescale(answer: i128, decimals: u8) -> Option<i128> {
    if decimals <= STORED_DECIMALS {
        Some(answer / 10i128.checked_pow(u32::from(STORED_DECIMALS - decimals))?)
    } else {
        answer.checked_mul(10i128.checked_pow(u32::from(decimals - STORED_DECIMALS))?)
    }
}

/// Check a DF1 feed can take a round, returning the answer at its decimals.
pub fn check_round(
    info: &AccountInfo,
    program: &Pubkey,
    writer: &Pubkey,
    observations_timestamp: u32,
    answer: i128,
) -> std::result::Result<i128, Df1SkipReason> {
    check_layout(info, program)?;
    if !info.is_writable {
        return Err(Df1SkipReason::NotWritable);
    }
    let data = info
        .try_borrow_data()
        .map_err(|_| Df1SkipReason::InvalidAccount)?;
    if data[df1::WRITER..df1::WRITER + 32] != writer.to_bytes() {
        return Err(Df1SkipReason::NotWriter);
    }
    let latest_round_id = u32_at(&data, df1::LATEST_ROUND_ID);
    if latest_round_id == u32::MAX {
        return Err(Df1SkipReason::RoundIdOverflow);
    }
    if latest_round_id > 0 {
        let live_length = u64::from(u32_at(&data, df1::LIVE_LENGTH));
        let live_cursor = u64::from(u32_at(&data, df1::LIVE_CURSOR));
        let latest = ((live_cursor + live_length - 1) % live_length) as usize;
        let latest_timestamp = u32_at(
            &data,
            df1::RINGS + latest * df1::TRANSMISSION_LEN + df1::TRANSMISSION_TIMESTAMP,
        );
        if observations_timestamp <= latest_timestamp {
            return Err(Df1SkipReason::Stale);
        }
    }
    rescale(answer, data[df1::DECIMALS]).ok_or(Df1SkipReason::AnswerOverflow)
}

/// The `submit` instruction, reused for every DF1 write of a batch (the bump allocator never frees).
pub struct Submit<'info> {
    program: AccountInfo<'info>,
    writer: AccountInfo<'info>,
    writer_bump: u8,
    ix: Instruction,
    infos: Vec<AccountInfo<'info>>,
}

impl<'info> Submit<'info> {
    pub fn new(program: AccountInfo<'info>, writer: AccountInfo<'info>, writer_bump: u8) -> Self {
        Self {
            ix: Instruction {
                program_id: *program.key,
                accounts: Vec::with_capacity(2),
                data: Vec::with_capacity(8 + 8 + 16),
            },
            infos: Vec::with_capacity(3),
            program,
            writer,
            writer_bump,
        }
    }

    pub fn program(&self) -> &Pubkey {
        self.program.key
    }

    pub fn writer(&self) -> &Pubkey {
        self.writer.key
    }

    /// Write a round to a DF1 feed that passed [`check_round`].
    pub fn call(
        &mut self,
        state: &Pubkey,
        feed: &AccountInfo<'info>,
        observations_timestamp: u32,
        answer: i128,
    ) -> Result<()> {
        self.ix.accounts.clear();
        self.ix.accounts.push(AccountMeta::new(*feed.key, false));
        self.ix
            .accounts
            .push(AccountMeta::new_readonly(*self.writer.key, true));
        // NewTransmission { timestamp: u64, answer: i128 }
        self.ix.data.clear();
        self.ix.data.extend_from_slice(&df1::SUBMIT_DISCRIMINATOR);
        self.ix
            .data
            .extend_from_slice(&u64::from(observations_timestamp).to_le_bytes());
        self.ix.data.extend_from_slice(&answer.to_le_bytes());
        self.infos.clear();
        self.infos.push(feed.clone());
        self.infos.push(self.writer.clone());
        self.infos.push(self.program.clone());
        invoke_signed(
            &self.ix,
            &self.infos,
            &[&[DF1_WRITER_SEED, state.as_ref(), &[self.writer_bump]]],
        )
        .map_err(Into::into)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const E18: i128 = 1_000_000_000_000_000_000;

    /// A DF1 feed account with `live` live and `historical` historical transmissions.
    fn feed(
        writer: &Pubkey,
        decimals: u8,
        live: u32,
        historical: u32,
        latest_round_id: u32,
        latest_timestamp: u32,
    ) -> Vec<u8> {
        let mut data = vec![0u8; df1::RINGS + (live + historical) as usize * df1::TRANSMISSION_LEN];
        data[..8].copy_from_slice(&df1::TRANSMISSIONS_DISCRIMINATOR);
        data[df1::VERSION] = df1::FEED_VERSION;
        data[df1::WRITER..df1::WRITER + 32].copy_from_slice(writer.as_ref());
        data[df1::DECIMALS] = decimals;
        data[df1::LATEST_ROUND_ID..df1::LATEST_ROUND_ID + 4]
            .copy_from_slice(&latest_round_id.to_le_bytes());
        data[df1::GRANULARITY] = 30;
        data[df1::LIVE_LENGTH..df1::LIVE_LENGTH + 4].copy_from_slice(&live.to_le_bytes());
        // latest round in live slot 0, cursor after it
        let cursor = if latest_round_id > 0 { 1 % live } else { 0 };
        data[df1::LIVE_CURSOR..df1::LIVE_CURSOR + 4].copy_from_slice(&cursor.to_le_bytes());
        let ts = df1::RINGS + df1::TRANSMISSION_TIMESTAMP;
        data[ts..ts + 4].copy_from_slice(&latest_timestamp.to_le_bytes());
        data
    }

    fn check(
        data: &mut [u8],
        owner: &Pubkey,
        program: &Pubkey,
        writer: &Pubkey,
        writable: bool,
        obs: u32,
    ) -> std::result::Result<i128, Df1SkipReason> {
        let key = Pubkey::new_unique();
        let mut lamports = 1;
        let info = AccountInfo::new(&key, false, writable, &mut lamports, data, owner, false, 0);
        check_round(&info, program, writer, obs, 123_456_789_012 * E18 / 1000)
    }

    #[test]
    fn accepts_a_writable_feed_it_writes_and_rescales() {
        let (program, writer) = (Pubkey::new_unique(), Pubkey::new_unique());
        let mut data = feed(&writer, 8, 4, 2, 7, 100);
        assert_eq!(
            check(&mut data, &program, &program, &writer, true, 101),
            Ok(12_345_678_901_200_000)
        );
        let mut new = feed(&writer, 8, 4, 0, 0, 0);
        assert!(check(&mut new, &program, &program, &writer, true, 1).is_ok());
    }

    #[test]
    fn skips_what_submit_would_fail_on() {
        let (program, writer) = (Pubkey::new_unique(), Pubkey::new_unique());
        let other = Pubkey::new_unique();
        let base = feed(&writer, 8, 4, 2, 7, 100);

        let mut d = base.clone();
        assert_eq!(
            check(&mut d, &other, &program, &writer, true, 101),
            Err(Df1SkipReason::InvalidAccount)
        );
        assert_eq!(
            check(&mut d, &program, &program, &other, true, 101),
            Err(Df1SkipReason::NotWriter)
        );
        assert_eq!(
            check(&mut d, &program, &program, &writer, false, 101),
            Err(Df1SkipReason::NotWritable)
        );
        assert_eq!(
            check(&mut d, &program, &program, &writer, true, 100),
            Err(Df1SkipReason::Stale)
        );

        let mut d = base.clone();
        d[df1::VERSION] = 1;
        assert_eq!(
            check(&mut d, &program, &program, &writer, true, 101),
            Err(Df1SkipReason::InvalidVersion)
        );

        let mut d = base.clone();
        d[0] ^= 1;
        assert_eq!(
            check(&mut d, &program, &program, &writer, true, 101),
            Err(Df1SkipReason::InvalidAccount)
        );

        let mut d = base.clone();
        d.truncate(d.len() - 1);
        assert_eq!(
            check(&mut d, &program, &program, &writer, true, 101),
            Err(Df1SkipReason::InvalidAccount),
            "ring bytes not a whole number of transmissions"
        );

        let mut d = base.clone();
        d[df1::GRANULARITY] = 0;
        assert_eq!(
            check(&mut d, &program, &program, &writer, true, 101),
            Err(Df1SkipReason::InvalidAccount),
            "historical ring with zero granularity"
        );

        let mut d = base.clone();
        d[df1::LIVE_CURSOR..df1::LIVE_CURSOR + 4].copy_from_slice(&4u32.to_le_bytes());
        assert_eq!(
            check(&mut d, &program, &program, &writer, true, 101),
            Err(Df1SkipReason::InvalidAccount),
            "cursor outside the live ring"
        );

        let mut d = base.clone();
        d[df1::LATEST_ROUND_ID..df1::LATEST_ROUND_ID + 4].copy_from_slice(&u32::MAX.to_le_bytes());
        assert_eq!(
            check(&mut d, &program, &program, &writer, true, 101),
            Err(Df1SkipReason::RoundIdOverflow)
        );

        let mut d = feed(&writer, 60, 4, 0, 0, 0);
        assert_eq!(
            check(&mut d, &program, &program, &writer, true, 1),
            Err(Df1SkipReason::AnswerOverflow)
        );
    }

    #[test]
    fn rescales_to_df1_decimals() {
        assert_eq!(rescale(15 * E18 / 10, 8), Some(150_000_000));
        assert_eq!(rescale(-15 * E18 / 10, 0), Some(-1));
        assert_eq!(rescale(7, 18), Some(7));
        assert_eq!(rescale(7, 20), Some(700));
        assert_eq!(rescale(i128::MAX, 19), None);
    }
}

//! Writes this store's rounds to existing DF1 feed accounts. The store calls DF1's unchanged
//! `submit`, signed by its `df1_writer` PDA, which each linked DF1 feed must name as its writer.
//!
//! On Solana a program can't catch a failure in a program it calls, so a failed `submit` would
//! abort every report in the transaction. The store therefore first checks everything `submit`
//! could fail on. When DF1 can't take a round, the store skips the DF1 write only and still stores
//! the round. `submit` doesn't check that rounds arrive in order, because OCR2 ordered them before
//! calling it, so the store only writes rounds newer than the DF1 feed's latest.
//!
//! ```text
//! transmitter's transaction
//!   forwarder `report`       checks the DON's signatures and the hash of the account keys
//!     store `on_report`      called by the forwarder, signed by the forwarder's authority PDA
//!       store (itself)       one call per event, signed by the event authority PDA
//!       DF1 `submit`         one call per DF1 write, signed by the `df1_writer` PDA
//! ```
//!
//! A failure at any level aborts the whole transaction. Every call above counts toward Solana's
//! limit of 64 instructions per transaction.

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

/// Reads a little-endian u32 at `offset`.
fn u32_at(data: &[u8], offset: usize) -> u32 {
    u32::from_le_bytes(data[offset..offset + 4].try_into().unwrap())
}

/// Checks that an account owned by `owner` with `data` is a DF1 feed whose layout `submit` can
/// handle. A failed call into another program can't be caught on Solana, so it would abort the
/// whole batch of reports, and every retry of it. [`check_round`] adds the checks that can change
/// after a feed is linked: the writer, the round counter and the round itself. `set_df1_feeds`
/// runs only this check, so a DF1 feed can be linked before its owner makes this store its writer.
///
/// A DF1 feed keeps its own history, separate from the ring in this store's feed accounts. After
/// its header it holds two rings of 48-byte rounds (offsets in [`crate::common::df1`]):
///
/// ```text
/// 0       8                               200
/// +-------+-------------------------------+----------------------+---------------------------+
/// |  tag  |  header (192 bytes)           |  live ring           |  historical ring          |
/// +-------+-------------------------------+----------------------+---------------------------+
///           version, writer, decimals,      live_length rounds,    the rest of the account,
///           latest_round_id, granularity,   next written at        every granularity-th
///           live_length and both cursors    live_cursor            round, next written at
///                                                                  historical_cursor
/// ```
///
/// It checks, in order:
/// 1. the DF1 program owns the account, which is at least 200 bytes and starts with the
///    `Transmissions` tag;
/// 2. the version is 2, the only one `submit` accepts;
/// 3. `live_length` is above 0, `live_cursor` is below it, and the live ring fits in the account;
/// 4. the historical ring is a whole number of rounds;
/// 5. if there is a historical ring, `granularity` is above 0 and `historical_cursor` is inside it.
///
/// `submit` uses the lengths and cursors as array indexes and `granularity` as a divisor without
/// checking them, so a bad value would make it panic. DF1 sets valid values when it creates a
/// feed. They are checked anyway because the check is cheap and a feed admin could link the wrong
/// account.
pub fn check_feed(
    owner: &Pubkey,
    data: &[u8],
    program: &Pubkey,
) -> std::result::Result<(), Df1SkipReason> {
    if owner != program {
        return Err(Df1SkipReason::InvalidAccount);
    }
    if data.len() < df1::RINGS || data[..8] != df1::TRANSMISSIONS_DISCRIMINATOR {
        return Err(Df1SkipReason::InvalidAccount);
    }
    if data[df1::VERSION] != df1::FEED_VERSION {
        return Err(Df1SkipReason::InvalidVersion);
    }
    let live_length = u64::from(u32_at(data, df1::LIVE_LENGTH));
    let live_cursor = u64::from(u32_at(data, df1::LIVE_CURSOR));
    let rings = (data.len() - df1::RINGS) as u64;
    let transmission = df1::TRANSMISSION_LEN as u64;
    let live_bytes = live_length * transmission;
    if live_length == 0 || live_cursor >= live_length || rings < live_bytes {
        return Err(Df1SkipReason::InvalidAccount);
    }
    let historical_bytes = rings - live_bytes;
    let historical_length = historical_bytes / transmission;
    // A ring area that ends in a partial round would also make `submit` fail.
    if historical_length * transmission != historical_bytes {
        return Err(Df1SkipReason::InvalidAccount);
    }
    if historical_length > 0
        && (data[df1::GRANULARITY] == 0
            || u64::from(u32_at(data, df1::HISTORICAL_CURSOR)) >= historical_length)
    {
        return Err(Df1SkipReason::InvalidAccount);
    }
    Ok(())
}

/// Rescales an answer from 18 decimals to a DF1 feed's decimals, truncating toward zero. Returns
/// `None` if the scale factor or result overflows. [`check_round`] rejects a non-zero answer that
/// would become zero.
pub fn rescale(answer: i128, decimals: u8) -> Option<i128> {
    if decimals <= STORED_DECIMALS {
        Some(answer / 10i128.checked_pow(u32::from(STORED_DECIMALS - decimals))?)
    } else {
        answer.checked_mul(10i128.checked_pow(u32::from(decimals - STORED_DECIMALS))?)
    }
}

/// Checks that a DF1 feed, owned by `owner` with `data`, can take a round, and returns the answer
/// at the feed's decimals. It checks the layout with [`check_feed`], that `writer` (this store's
/// `df1_writer` PDA) is the feed's writer, that the feed's round counter isn't at its maximum,
/// that the round is newer than the feed's latest (`submit` doesn't check order), that the
/// observation timestamp fits DF1's u32 storage, that the answer fits the feed's decimals, and
/// that a non-zero answer doesn't become zero.
pub fn check_round(
    owner: &Pubkey,
    data: &[u8],
    program: &Pubkey,
    writer: &Pubkey,
    observations_timestamp: u64,
    answer: i128,
) -> std::result::Result<i128, Df1SkipReason> {
    check_feed(owner, data, program)?;
    if data[df1::WRITER..df1::WRITER + 32] != writer.to_bytes() {
        return Err(Df1SkipReason::NotWriter);
    }
    let latest_round_id = u32_at(data, df1::LATEST_ROUND_ID);
    if latest_round_id == u32::MAX {
        return Err(Df1SkipReason::RoundIdOverflow);
    }
    // DF1's `submit` takes u64 but stores it as u32 with a silent cast. Do not let it wrap.
    if observations_timestamp > u64::from(u32::MAX) {
        return Err(Df1SkipReason::TimestampOverflow);
    }
    if latest_round_id > 0 {
        let live_length = u64::from(u32_at(data, df1::LIVE_LENGTH));
        let live_cursor = u64::from(u32_at(data, df1::LIVE_CURSOR));
        // The live index written last is the one before the cursor, wrapping, as in DF1's
        // `latest`.
        let latest = ((live_cursor + live_length - 1) % live_length) as usize;
        let latest_timestamp = u32_at(
            data,
            df1::RINGS + latest * df1::TRANSMISSION_LEN + df1::TRANSMISSION_TIMESTAMP,
        );
        if observations_timestamp <= u64::from(latest_timestamp) {
            return Err(Df1SkipReason::Stale);
        }
    }
    let scaled = rescale(answer, data[df1::DECIMALS]).ok_or(Df1SkipReason::AnswerOverflow)?;
    if answer != 0 && scaled == 0 {
        return Err(Df1SkipReason::RoundsToZero);
    }
    Ok(scaled)
}

/// Calls DF1's `submit`, signed by the store's `df1_writer` PDA. One instruction buffer is reused
/// for every DF1 write in a batch, because a program's heap is never freed and a new buffer per
/// write would add up.
pub struct Submit<'info> {
    program: AccountInfo<'info>,
    writer: AccountInfo<'info>,
    writer_bump: u8,
    ix: Instruction,
    infos: Vec<AccountInfo<'info>>,
}

impl<'info> Submit<'info> {
    /// Prepares `submit` calls to `program`, signed by `writer`, the `df1_writer` PDA with bump
    /// `writer_bump`.
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

    /// Returns the DF1 program's ID.
    pub fn program(&self) -> &Pubkey {
        self.program.key
    }

    /// Returns the `df1_writer` PDA's address.
    pub fn writer(&self) -> &Pubkey {
        self.writer.key
    }

    /// Writes a round to a DF1 feed. The caller must first have checked the feed and the round
    /// with [`check_round`], and must not hold a borrow of the feed's data.
    pub fn call(
        &mut self,
        state: &Pubkey,
        feed: &AccountInfo<'info>,
        observations_timestamp: u64,
        answer: i128,
    ) -> Result<()> {
        self.ix.accounts.clear();
        self.ix.accounts.push(AccountMeta::new(*feed.key, false));
        self.ix
            .accounts
            .push(AccountMeta::new_readonly(*self.writer.key, true));
        // `submit`'s argument is the Borsh encoding of
        // `NewTransmission { timestamp: u64, answer: i128 }`.
        self.ix.data.clear();
        self.ix.data.extend_from_slice(&df1::SUBMIT_DISCRIMINATOR);
        self.ix
            .data
            .extend_from_slice(&observations_timestamp.to_le_bytes());
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

    /// Builds a DF1 feed account's data with `live` live rounds and `historical` historical
    /// rounds. The latest round, if any, is at live index 0 with the cursor after it.
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
        let cursor = if latest_round_id > 0 { 1 % live } else { 0 };
        data[df1::LIVE_CURSOR..df1::LIVE_CURSOR + 4].copy_from_slice(&cursor.to_le_bytes());
        let ts = df1::RINGS + df1::TRANSMISSION_TIMESTAMP;
        data[ts..ts + 4].copy_from_slice(&latest_timestamp.to_le_bytes());
        data
    }

    /// Runs `check_round` on `data` owned by `owner`, for a round observed at `obs` with an
    /// answer of 123,456,789.012.
    fn check(
        data: &mut [u8],
        owner: &Pubkey,
        program: &Pubkey,
        writer: &Pubkey,
        obs: u64,
    ) -> std::result::Result<i128, Df1SkipReason> {
        check_round(
            owner,
            data,
            program,
            writer,
            obs,
            123_456_789_012 * E18 / 1000,
        )
    }

    #[test]
    fn accepts_a_feed_it_writes_and_rescales() {
        let (program, writer) = (Pubkey::new_unique(), Pubkey::new_unique());
        let mut data = feed(&writer, 8, 4, 2, 7, 100);
        assert_eq!(
            check(&mut data, &program, &program, &writer, 101),
            Ok(12_345_678_901_200_000)
        );
        let mut new = feed(&writer, 8, 4, 0, 0, 0);
        assert!(check(&mut new, &program, &program, &writer, 1).is_ok());
    }

    #[test]
    fn skips_what_submit_would_fail_on() {
        let (program, writer) = (Pubkey::new_unique(), Pubkey::new_unique());
        let other = Pubkey::new_unique();
        let base = feed(&writer, 8, 4, 2, 7, 100);

        let mut d = base.clone();
        assert_eq!(
            check(&mut d, &other, &program, &writer, 101),
            Err(Df1SkipReason::InvalidAccount)
        );
        assert_eq!(
            check(&mut d, &program, &program, &other, 101),
            Err(Df1SkipReason::NotWriter)
        );
        assert_eq!(
            check(&mut d, &program, &program, &writer, 100),
            Err(Df1SkipReason::Stale)
        );

        let mut d = base.clone();
        d[df1::VERSION] = 1;
        assert_eq!(
            check(&mut d, &program, &program, &writer, 101),
            Err(Df1SkipReason::InvalidVersion)
        );

        let mut d = base.clone();
        d[0] ^= 1;
        assert_eq!(
            check(&mut d, &program, &program, &writer, 101),
            Err(Df1SkipReason::InvalidAccount)
        );

        let mut d = base.clone();
        d.truncate(d.len() - 1);
        assert_eq!(
            check(&mut d, &program, &program, &writer, 101),
            Err(Df1SkipReason::InvalidAccount),
            "ring bytes not a whole number of transmissions"
        );

        let mut d = base.clone();
        d[df1::GRANULARITY] = 0;
        assert_eq!(
            check(&mut d, &program, &program, &writer, 101),
            Err(Df1SkipReason::InvalidAccount),
            "historical ring with zero granularity"
        );

        let mut d = base.clone();
        d[df1::LIVE_CURSOR..df1::LIVE_CURSOR + 4].copy_from_slice(&4u32.to_le_bytes());
        assert_eq!(
            check(&mut d, &program, &program, &writer, 101),
            Err(Df1SkipReason::InvalidAccount),
            "cursor outside the live ring"
        );

        let mut d = base.clone();
        d[df1::LATEST_ROUND_ID..df1::LATEST_ROUND_ID + 4].copy_from_slice(&u32::MAX.to_le_bytes());
        assert_eq!(
            check(&mut d, &program, &program, &writer, 101),
            Err(Df1SkipReason::RoundIdOverflow)
        );

        let mut d = feed(&writer, 60, 4, 0, 0, 0);
        assert_eq!(
            check(&mut d, &program, &program, &writer, 1),
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

    #[test]
    fn refuses_nonzero_answers_that_would_become_zero() {
        let (program, writer) = (Pubkey::new_unique(), Pubkey::new_unique());
        let smallest = E18 / 100_000_000;
        for answer in [1, -1, smallest - 1, -(smallest - 1)] {
            let data = feed(&writer, 8, 4, 0, 0, 0);
            assert_eq!(
                check_round(&program, &data, &program, &writer, 1, answer),
                Err(Df1SkipReason::RoundsToZero)
            );
        }
        for (answer, expected) in [(0, 0), (smallest, 1), (-smallest, -1)] {
            let data = feed(&writer, 8, 4, 0, 0, 0);
            assert_eq!(
                check_round(&program, &data, &program, &writer, 1, answer),
                Ok(expected)
            );
        }
    }

    #[test]
    fn refuses_timestamps_df1_would_wrap() {
        let (program, writer) = (Pubkey::new_unique(), Pubkey::new_unique());
        let mut data = feed(&writer, 8, 4, 0, 0, 0);
        assert!(check(&mut data, &program, &program, &writer, u64::from(u32::MAX)).is_ok());
        assert_eq!(
            check(
                &mut data,
                &program,
                &program,
                &writer,
                u64::from(u32::MAX) + 1
            ),
            Err(Df1SkipReason::TimestampOverflow)
        );
    }

    mod properties {
        use super::*;
        use proptest::prelude::*;

        /// Returns whether DF1's `submit` can write this account without panicking, whatever the
        /// round. `submit` splits the data after the header into whole live and historical rounds,
        /// writes the live ring at its cursor and, for every `granularity`-th round, writes the
        /// historical ring at its cursor. This models `with_store`, `insert` and `latest` in
        /// `programs/store`.
        fn submit_can_write(data: &[u8]) -> bool {
            if data.len() < df1::RINGS {
                return false;
            }
            let rings = data.len() - df1::RINGS;
            let live = u32_at(data, df1::LIVE_LENGTH) as usize;
            let live_bytes = live * df1::TRANSMISSION_LEN;
            if live == 0 || live_bytes > rings {
                return false;
            }
            // The ring area must be whole rounds, or DF1's cast to rounds fails.
            let historical = (rings - live_bytes) / df1::TRANSMISSION_LEN;
            if historical * df1::TRANSMISSION_LEN != rings - live_bytes {
                return false;
            }
            (u32_at(data, df1::LIVE_CURSOR) as usize) < live
                && (historical == 0
                    || (data[df1::GRANULARITY] != 0
                        && (u32_at(data, df1::HISTORICAL_CURSOR) as usize) < historical))
        }

        proptest! {
            #[test]
            fn check_feed_accepts_exactly_what_submit_can_write(
                live in 0u32..6,
                live_cursor in 0u32..8,
                historical_bytes in 0usize..(6 * df1::TRANSMISSION_LEN + 47),
                granularity in 0u8..3,
                historical_cursor in 0u32..8,
                cut in 0usize..df1::RINGS + 1,
            ) {
                let program = Pubkey::new_unique();
                let mut data = feed(&Pubkey::new_unique(), 8, live.max(1), 0, 0, 0);
                data.resize(data.len() + historical_bytes, 0);
                data[df1::LIVE_LENGTH..df1::LIVE_LENGTH + 4].copy_from_slice(&live.to_le_bytes());
                data[df1::LIVE_CURSOR..df1::LIVE_CURSOR + 4].copy_from_slice(&live_cursor.to_le_bytes());
                data[df1::GRANULARITY] = granularity;
                data[df1::HISTORICAL_CURSOR..df1::HISTORICAL_CURSOR + 4]
                    .copy_from_slice(&historical_cursor.to_le_bytes());
                // Sometimes the account is too short to hold the header.
                if cut > 0 && cut < data.len() && cut <= df1::RINGS {
                    data.truncate(cut);
                }
                let safe = submit_can_write(&data);
                prop_assert_eq!(check_feed(&program, &data, &program).is_ok(), safe);
            }

            #[test]
            fn rescaling_is_exact_or_refused(answer in any::<i128>(), decimals in 0u8..=38) {
                let result = rescale(answer, decimals);
                if decimals <= STORED_DECIMALS {
                    // The answer is truncated toward zero, so a small non-zero answer can
                    // become 0.
                    let unit = 10u128.pow(u32::from(STORED_DECIMALS - decimals));
                    let (a, s) = (answer.unsigned_abs(), result.unwrap().unsigned_abs());
                    prop_assert!(s * unit <= a && a < (s + 1) * unit);
                } else {
                    let unit = 10i128.pow(u32::from(decimals - STORED_DECIMALS));
                    match result {
                        Some(scaled) => {
                            prop_assert_eq!(scaled % unit, 0);
                            prop_assert_eq!(scaled / unit, answer);
                        }
                        None => prop_assert!(answer.unsigned_abs() > i128::MAX as u128 / unit as u128),
                    }
                }
            }

            #[test]
            fn df1_writes_never_turn_nonzero_answers_into_zero(
                answer in any::<i128>(),
                decimals in 0u8..=STORED_DECIMALS,
            ) {
                let (program, writer) = (Pubkey::new_unique(), Pubkey::new_unique());
                let data = feed(&writer, decimals, 4, 0, 0, 0);
                let result = check_round(&program, &data, &program, &writer, 1, answer);
                let scaled = rescale(answer, decimals).unwrap();
                if answer != 0 && scaled == 0 {
                    prop_assert_eq!(result, Err(Df1SkipReason::RoundsToZero));
                } else {
                    prop_assert_eq!(result, Ok(scaled));
                }
            }
        }
    }
}

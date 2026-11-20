//! Stores rounds, finds stored rounds and rescales answers, ported from the non-SVR part of the
//! EVM `DataFeedsStore`. The functions work only on a feed header and its ring, never on accounts,
//! so they can be unit tested and reused by readers that decode feed accounts themselves.

use crate::{
    common::{MIN_MAX_STORED_REPORTS, STORED_DECIMALS},
    state::{Feed, StoredReport},
};

/// The chain's current slot and Unix time, read once per `on_report`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Now {
    /// The current slot (the chain's clock tick).
    pub slot: u64,
    /// The current Unix time, in seconds.
    pub timestamp: u32,
}

/// A feed's ring of stored rounds. Round r is kept at index `r % max_stored_reports`.
pub trait Ring {
    /// Returns the round stored at `index`.
    fn get(&self, index: u64) -> StoredReport;
    /// Stores a round at `index`, replacing what was there.
    fn set(&mut self, index: u64, report: StoredReport);
}

#[cfg(test)]
impl Ring for Vec<StoredReport> {
    fn get(&self, index: u64) -> StoredReport {
        self[index as usize]
    }

    fn set(&mut self, index: u64, report: StoredReport) {
        self[index as usize] = report;
    }
}

/// A round as reads see it before rescaling, with the answer at 18 decimals.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct StoredRound {
    /// The round's ID.
    pub round_id: u64,
    /// The answer, with 18 decimals.
    pub answer: i128,
    /// The time, in Unix seconds, at which the round was observed offchain.
    pub observations_timestamp: u32,
    /// The time, in Unix seconds, at which the round was written onchain.
    pub recorded_timestamp: u32,
}

/// Returns the ring index of a round.
fn ring_index(round_id: u64, max_stored_reports: u64) -> u64 {
    round_id % max_stored_reports
}

/// Returns whether a round can still be read from the ring. That is true for round IDs from 1 to
/// the latest that are among the last `max_stored_reports` rounds.
pub fn round_in_window(round_id: u64, latest_round_id: u64, max_stored_reports: u64) -> bool {
    if round_id == 0 || round_id > latest_round_id {
        return false;
    }
    latest_round_id - round_id < max_stored_reports
}

/// Appends a round to the ring, updates the header's latest round and returns the new round ID.
/// The caller must already have checked that the round is newer than the latest one.
pub fn store_round(
    feed: &mut Feed,
    ring: &mut impl Ring,
    observations_timestamp: u32,
    answer: i128,
    now: Now,
) -> u64 {
    feed.round_id += 1;
    let round_id = feed.round_id;

    ring.set(
        ring_index(round_id, feed.max_stored_reports),
        StoredReport {
            answer,
            observations_timestamp,
            recorded_timestamp: now.timestamp,
            recorded_slot: now.slot,
        },
    );

    feed.latest_answer = answer;
    feed.latest_observations_timestamp = observations_timestamp;
    feed.latest_recorded_timestamp = now.timestamp;
    feed.latest_recorded_slot = now.slot;

    round_id
}

/// Returns the latest round, or `None` if the feed has never stored one.
pub fn latest_round(feed: &Feed) -> Option<StoredRound> {
    if feed.round_id == 0 {
        return None;
    }
    Some(StoredRound {
        round_id: feed.round_id,
        answer: feed.latest_answer,
        observations_timestamp: feed.latest_observations_timestamp,
        recorded_timestamp: feed.latest_recorded_timestamp,
    })
}

/// Returns a stored round, or `None` if it was never stored or has since been overwritten in the
/// ring.
pub fn stored_round(round_id: u64, feed: &Feed, ring: &impl Ring) -> Option<StoredRound> {
    if !round_in_window(round_id, feed.round_id, feed.max_stored_reports) {
        return None;
    }
    let stored = ring.get(ring_index(round_id, feed.max_stored_reports));
    Some(StoredRound {
        round_id,
        answer: stored.answer,
        observations_timestamp: stored.observations_timestamp,
        recorded_timestamp: stored.recorded_timestamp,
    })
}

/// The reasons `check_max_stored_reports` refuses a new ring size.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum MaxStoredReportsError {
    /// The new ring size is below `MIN_MAX_STORED_REPORTS` (2), which would leave no previous
    /// round readable.
    Invalid,
    /// The new ring size is not larger than the feed's current one. A ring can only grow.
    NotIncreased,
    /// The feed has stored at least as many rounds as its ring holds, so the ring has wrapped.
    /// Growing it now would leave some rounds where reads no longer look for them.
    Locked,
}

/// Checks that a feed's ring can grow to `new_max_stored_reports`. A ring can only grow, and only
/// while it has never wrapped. Until then every stored round r sits at index r, which is also
/// where the larger ring looks for it, so no round has to move. EVM has the same rule. The new
/// size then always exceeds the latest round ID.
///
/// ```text
///   index    0     1     2     3     4     5
///   N = 4    -     r1    r2    r3
///   N = 6    -     r1    r2    r3    -     -     r % 6 = r, so every round is still found
/// ```
///
/// After round 4, a ring of 4 has wrapped: round 4 is at index 0. Grown to 6, a read of round 4
/// would look at index 4 and find nothing, so the change is refused with `Locked`.
pub fn check_max_stored_reports(
    feed: &Feed,
    new_max_stored_reports: u64,
) -> Result<(), MaxStoredReportsError> {
    if new_max_stored_reports < MIN_MAX_STORED_REPORTS {
        return Err(MaxStoredReportsError::Invalid);
    }
    let current = feed.max_stored_reports;
    if new_max_stored_reports <= current {
        return Err(MaxStoredReportsError::NotIncreased);
    }
    if feed.round_id >= current {
        return Err(MaxStoredReportsError::Locked);
    }
    Ok(())
}

/// The reasons a read at some number of decimals is refused.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ScaleError {
    /// The reader asked for fewer decimals than the feed's minimum, or for more than 18.
    InvalidDecimals,
    /// The answer is not zero, but at the requested decimals it would truncate to zero. Reading it
    /// as zero could be mistaken for no price.
    RoundsToZero,
}

/// Checks that a reader may ask for `decimals` from a feed whose minimum is `min_decimals`.
/// `decimals` must be between that minimum and 18.
pub fn check_decimals(decimals: u8, min_decimals: u8) -> Result<(), ScaleError> {
    if decimals < min_decimals || decimals > STORED_DECIMALS {
        return Err(ScaleError::InvalidDecimals);
    }
    Ok(())
}

/// Rescales an answer from 18 decimals to `decimals` (at most 18), truncating toward zero. It
/// refuses a non-zero answer that would become zero, so a reader never mistakes a tiny price for no
/// price.
pub fn scale(answer: i128, decimals: u8) -> Result<i128, ScaleError> {
    if decimals > STORED_DECIMALS {
        return Err(ScaleError::InvalidDecimals);
    }
    let scaled = answer / 10i128.pow(u32::from(STORED_DECIMALS - decimals));
    if answer != 0 && scaled == 0 {
        return Err(ScaleError::RoundsToZero);
    }
    Ok(scaled)
}

#[cfg(test)]
mod tests {
    use super::*;
    use bytemuck::Zeroable;

    const E18: i128 = 1_000_000_000_000_000_000;

    fn feed(max_stored_reports: u64) -> (Feed, Vec<StoredReport>) {
        let mut feed = Feed::zeroed();
        feed.max_stored_reports = max_stored_reports;
        (
            feed,
            vec![StoredReport::default(); max_stored_reports as usize],
        )
    }

    fn at(slot: u64, timestamp: u32) -> Now {
        Now { slot, timestamp }
    }

    #[test]
    fn rounds_and_ring_wrap() {
        let (mut f, mut ring) = feed(3);
        assert_eq!(latest_round(&f), None, "no round yet");
        for i in 1..=5u32 {
            assert_eq!(
                store_round(&mut f, &mut ring, i, i as i128, at(i as u64, 100 + i)),
                i as u64
            );
        }
        assert_eq!(f.round_id, 5);
        // Rounds 3 to 5 are still in the ring.
        assert!(!round_in_window(2, 5, 3));
        assert!(round_in_window(3, 5, 3));
        assert_eq!(stored_round(3, &f, &ring).unwrap().answer, 3);
        assert_eq!(stored_round(2, &f, &ring), None);
        assert_eq!(stored_round(6, &f, &ring), None);
        assert_eq!(stored_round(0, &f, &ring), None);
        assert_eq!(
            latest_round(&f),
            Some(StoredRound {
                round_id: 5,
                answer: 5,
                observations_timestamp: 5,
                recorded_timestamp: 105,
            })
        );
    }

    #[test]
    fn max_stored_reports_rules() {
        let (mut f, mut ring) = feed(3);
        assert_eq!(
            check_max_stored_reports(&f, 1),
            Err(MaxStoredReportsError::Invalid)
        );
        assert_eq!(
            check_max_stored_reports(&f, 3),
            Err(MaxStoredReportsError::NotIncreased)
        );
        assert_eq!(check_max_stored_reports(&f, 4), Ok(()));
        store_round(&mut f, &mut ring, 1, 1, at(1, 1));
        store_round(&mut f, &mut ring, 2, 2, at(2, 2));
        assert_eq!(check_max_stored_reports(&f, 4), Ok(()));
        store_round(&mut f, &mut ring, 3, 3, at(3, 3));
        assert_eq!(
            check_max_stored_reports(&f, 9),
            Err(MaxStoredReportsError::Locked)
        );
    }

    #[test]
    fn scales_and_truncates_toward_zero() {
        assert_eq!(
            scale(1_234_567_891 * E18 / 1_000_000_000, 8).unwrap(),
            123_456_789
        );
        assert_eq!(scale(-15 * E18 / 10, 0).unwrap(), -1);
        assert_eq!(scale(42, 18).unwrap(), 42);
        assert_eq!(scale(0, 8).unwrap(), 0, "zero stays zero");
        assert_eq!(scale(1, 19), Err(ScaleError::InvalidDecimals));
    }

    #[test]
    fn a_non_zero_answer_never_rounds_to_zero() {
        assert_eq!(scale(1, 8), Err(ScaleError::RoundsToZero));
        assert_eq!(scale(-1, 8), Err(ScaleError::RoundsToZero));
        assert_eq!(scale(E18 / 10, 0), Err(ScaleError::RoundsToZero));
        assert_eq!(scale(10_000_000_000, 8).unwrap(), 1);
    }

    #[test]
    fn decimals_bounds() {
        assert_eq!(check_decimals(8, 8), Ok(()));
        assert_eq!(check_decimals(7, 8), Err(ScaleError::InvalidDecimals));
        assert_eq!(check_decimals(18, 18), Ok(()));
        assert_eq!(check_decimals(19, 0), Err(ScaleError::InvalidDecimals));
    }

    mod properties {
        use super::*;
        use proptest::prelude::*;

        /// Stores `answers` as rounds 1 to n in a new ring of `max` rounds.
        fn stored(max: u64, answers: &[i128]) -> (Feed, Vec<StoredReport>) {
            let (mut f, mut ring) = feed(max);
            for (i, answer) in answers.iter().enumerate() {
                let t = i as u32 + 1;
                store_round(&mut f, &mut ring, t, *answer, at(t as u64, t));
            }
            (f, ring)
        }

        /// Returns what a read of `round_id` must return: the answer stored as that round while
        /// it is among the last `max` rounds, and nothing otherwise.
        fn expected(round_id: u64, max: u64, answers: &[i128]) -> Option<i128> {
            let n = answers.len() as u64;
            (round_id >= 1 && round_id <= n && n - round_id < max)
                .then(|| answers[round_id as usize - 1])
        }

        proptest! {
            #[test]
            fn the_ring_serves_exactly_the_last_rounds(
                max in 2u64..12,
                answers in prop::collection::vec(any::<i128>(), 0..40),
            ) {
                let (f, ring) = stored(max, &answers);
                let n = answers.len() as u64;
                prop_assert_eq!(f.round_id, n);
                prop_assert_eq!(latest_round(&f).map(|r| r.answer), answers.last().copied());
                for round_id in 0..=n + 1 {
                    let read = stored_round(round_id, &f, &ring).map(|r| r.answer);
                    prop_assert_eq!(read, expected(round_id, max, &answers), "round {}", round_id);
                }
            }

            #[test]
            fn growing_before_a_wrap_keeps_every_round(
                max in 2u64..12,
                grow in 1u64..12,
                answers in prop::collection::vec(any::<i128>(), 0..24),
                later in prop::collection::vec(any::<i128>(), 0..24),
            ) {
                let (mut f, mut ring) = stored(max, &answers);
                let new_max = max + grow;
                let result = check_max_stored_reports(&f, new_max);
                if f.round_id >= max {
                    prop_assert_eq!(result, Err(MaxStoredReportsError::Locked));
                    return Ok(());
                }
                prop_assert_eq!(result, Ok(()));
                prop_assert!(new_max > f.round_id, "a grown ring holds every round so far");
                // The program grows the account with zeroed rounds and keeps the header's latest
                // round, so the model does the same.
                ring.resize(new_max as usize, StoredReport::default());
                f.max_stored_reports = new_max;
                let mut all = answers.clone();
                for answer in later {
                    let t = all.len() as u32 + 1;
                    store_round(&mut f, &mut ring, t, answer, at(t as u64, t));
                    all.push(answer);
                }
                for round_id in 0..=all.len() as u64 + 1 {
                    let read = stored_round(round_id, &f, &ring).map(|r| r.answer);
                    prop_assert_eq!(read, expected(round_id, new_max, &all), "round {}", round_id);
                }
            }

            #[test]
            fn the_ring_never_shrinks(max in 2u64..12, smaller in 0u64..12) {
                let (f, _) = stored(max, &[]);
                let new_max = smaller.min(max);
                let expected = if new_max < MIN_MAX_STORED_REPORTS {
                    MaxStoredReportsError::Invalid
                } else {
                    MaxStoredReportsError::NotIncreased
                };
                prop_assert_eq!(check_max_stored_reports(&f, new_max), Err(expected));
            }

            #[test]
            fn scaling_truncates_and_never_reads_a_price_as_zero(
                answer in any::<i128>(),
                decimals in 0u8..=20,
            ) {
                let result = scale(answer, decimals);
                if decimals > STORED_DECIMALS {
                    prop_assert_eq!(result, Err(ScaleError::InvalidDecimals));
                    return Ok(());
                }
                let unit = 10u128.pow(u32::from(STORED_DECIMALS - decimals));
                match result {
                    Ok(scaled) => {
                        // The answer is truncated toward zero: |scaled| units fit in |answer|, and
                        // one more unit doesn't.
                        let (a, s) = (answer.unsigned_abs(), scaled.unsigned_abs());
                        prop_assert!(s * unit <= a && a < (s + 1) * unit);
                        prop_assert!(scaled == 0 || (scaled < 0) == (answer < 0));
                        prop_assert!(answer == 0 || scaled != 0);
                    }
                    Err(e) => {
                        prop_assert_eq!(e, ScaleError::RoundsToZero);
                        prop_assert!(answer != 0 && answer.unsigned_abs() < unit);
                    }
                }
            }
        }
    }
}

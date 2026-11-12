//! Round storage, round selection and rescaling, ported from the non-SVR part of the EVM
//! `DataFeedsStore`. Pure functions over a feed header and its circular buffer, so they can be unit
//! tested and reused by readers that decode feed accounts themselves.

use crate::{
    common::{MIN_MAX_STORED_REPORTS, STORED_DECIMALS},
    state::{Feed, StoredReport},
};

/// Current slot and unix timestamp.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Now {
    pub slot: u64,
    pub timestamp: u32,
}

/// A feed's circular buffer, indexed by storage slot (`round_id % max_stored_reports`).
pub trait Ring {
    fn get(&self, slot: u64) -> StoredReport;
    fn set(&mut self, slot: u64, report: StoredReport);
}

impl Ring for Vec<StoredReport> {
    fn get(&self, slot: u64) -> StoredReport {
        self[slot as usize]
    }

    fn set(&mut self, slot: u64, report: StoredReport) {
        self[slot as usize] = report;
    }
}

/// A round as served to a reader, answer with `STORED_DECIMALS`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct StoredRound {
    pub round_id: u64,
    pub answer: i128,
    pub observations_timestamp: u32,
    pub recorded_timestamp: u32,
}

fn storage_slot(round_id: u64, max_stored_reports: u64) -> u64 {
    round_id % max_stored_reports
}

/// Whether a round ID is still within the circular buffer's retained window.
pub fn round_in_window(round_id: u64, latest_round_id: u64, max_stored_reports: u64) -> bool {
    if round_id == 0 || round_id > latest_round_id {
        return false;
    }
    latest_round_id - round_id < max_stored_reports
}

/// Append a round to the feed's circular buffer and latest fields, returning its round ID. The
/// caller has checked it is newer than the latest round.
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
        storage_slot(round_id, feed.max_stored_reports),
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

/// The latest round, `None` while the feed has never received one.
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

/// A stored round, `None` when it was never written or has been overwritten in the ring.
pub fn stored_round(round_id: u64, feed: &Feed, ring: &impl Ring) -> Option<StoredRound> {
    if !round_in_window(round_id, feed.round_id, feed.max_stored_reports) {
        return None;
    }
    let stored = ring.get(storage_slot(round_id, feed.max_stored_reports));
    Some(StoredRound {
        round_id,
        answer: stored.answer,
        observations_timestamp: stored.observations_timestamp,
        recorded_timestamp: stored.recorded_timestamp,
    })
}

/// Why a ring size change is rejected.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum MaxStoredReportsError {
    Invalid,
    NotIncreased,
    Locked,
    TooLow,
}

/// The ring can only grow, and only while it has never wrapped: every stored round r then still
/// sits at slot r, so the larger ring keeps every round where reads look for it.
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
    if new_max_stored_reports <= feed.round_id {
        return Err(MaxStoredReportsError::TooLow);
    }
    Ok(())
}

/// Why a read at some decimals is refused.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ScaleError {
    /// Below the feed's minimum or above the stored decimals
    InvalidDecimals,
    /// A non-zero answer would read as zero
    RoundsToZero,
}

/// Whether a reader may ask for `decimals` from a feed with `min_decimals`.
pub fn check_decimals(decimals: u8, min_decimals: u8) -> Result<(), ScaleError> {
    if decimals < min_decimals || decimals > STORED_DECIMALS {
        return Err(ScaleError::InvalidDecimals);
    }
    Ok(())
}

/// Rescale an answer with `STORED_DECIMALS` to `decimals` (at most `STORED_DECIMALS`), truncating
/// toward zero. A non-zero answer that would become zero is an error, so a reader never mistakes a
/// tiny price for none.
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
        // rounds 3..=5 retained
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
}

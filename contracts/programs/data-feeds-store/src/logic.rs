//! Report storage and round selection, ported from the non-SVR part of the EVM `DataFeedsStore`.
//! Pure functions over a feed header and its circular buffer so they can be unit tested and reused
//! offchain.

use crate::state::{FeedData, StoredReport};

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

/// A round as served to a reader.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct LatestReport {
    pub round_id: u64,
    pub answer: i128,
    pub observations_timestamp: u32,
    pub recorded_timestamp: u32,
}

/// Why a report was not applied.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Rejection {
    /// Observation timestamp leads the clock by more than the allowed forward duration
    FutureReport,
    /// Not newer than the latest report, carries the latest observations timestamp
    Stale(u32),
    /// Outside the feed's accepted answer range, carries (min, max)
    OutOfRange(i128, i128),
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

pub fn is_future_report(observations_timestamp: u32, now: Now, max_forward_secs: u32) -> bool {
    u64::from(observations_timestamp) > u64::from(now.timestamp) + u64::from(max_forward_secs)
}

/// Whether an answer falls inside the feed's accepted range, with the (min, max) applied.
pub fn answer_in_range(feed: &FeedData, answer: i128) -> (bool, i128, i128) {
    if feed.min_max_configured == 0 {
        return (true, i128::MIN, i128::MAX);
    }
    (
        feed.min_answer <= answer && answer <= feed.max_answer,
        feed.min_answer,
        feed.max_answer,
    )
}

/// Checks a report, returning the round ID it would be appended under.
pub fn check_report(
    feed: &FeedData,
    observations_timestamp: u32,
    answer: i128,
    now: Now,
    max_forward_secs: u32,
) -> Result<u64, Rejection> {
    if is_future_report(observations_timestamp, now, max_forward_secs) {
        return Err(Rejection::FutureReport);
    }
    if observations_timestamp <= feed.latest_observations_timestamp {
        return Err(Rejection::Stale(feed.latest_observations_timestamp));
    }
    let (in_range, min, max) = answer_in_range(feed, answer);
    if !in_range {
        return Err(Rejection::OutOfRange(min, max));
    }
    Ok(feed.round_id + 1)
}

/// Store a report as a new round in the feed's circular buffer and latest fields.
pub fn store_report(
    feed: &mut FeedData,
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

/// The latest round, `None` while the feed has never received a report.
pub fn latest_report(feed: &FeedData) -> Option<LatestReport> {
    if feed.round_id == 0 {
        return None;
    }
    Some(LatestReport {
        round_id: feed.round_id,
        answer: feed.latest_answer,
        observations_timestamp: feed.latest_observations_timestamp,
        recorded_timestamp: feed.latest_recorded_timestamp,
    })
}

/// A stored round, `None` when it was never written or has been overwritten in the ring.
pub fn stored_report(round_id: u64, feed: &FeedData, ring: &impl Ring) -> Option<LatestReport> {
    if !round_in_window(round_id, feed.round_id, feed.max_stored_reports) {
        return None;
    }
    let stored = ring.get(storage_slot(round_id, feed.max_stored_reports));
    Some(LatestReport {
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

/// The first configuration is free, afterwards the ring size can only grow. Resizing is only safe
/// while the ring has never wrapped: every stored round r then still sits at slot r.
pub fn check_max_stored_reports(
    feed: &FeedData,
    new_max_stored_reports: u64,
) -> Result<(), MaxStoredReportsError> {
    if new_max_stored_reports < crate::common::MIN_MAX_STORED_REPORTS {
        return Err(MaxStoredReportsError::Invalid);
    }
    let current = feed.max_stored_reports;
    if feed.max_stored_reports_configured != 0 && new_max_stored_reports <= current {
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

#[cfg(test)]
mod tests {
    use super::*;
    use bytemuck::Zeroable;

    fn feed(max_stored_reports: u64) -> (FeedData, Vec<StoredReport>) {
        let mut feed = FeedData::zeroed();
        feed.max_stored_reports = max_stored_reports;
        (
            feed,
            vec![StoredReport::default(); max_stored_reports as usize],
        )
    }

    fn at(slot: u64, timestamp: u32) -> Now {
        Now { slot, timestamp }
    }

    fn report(
        feed: &mut FeedData,
        ring: &mut Vec<StoredReport>,
        obs: u32,
        answer: i128,
        now: Now,
    ) -> u64 {
        let round = check_report(feed, obs, answer, now, 60).unwrap();
        assert_eq!(store_report(feed, ring, obs, answer, now), round);
        round
    }

    #[test]
    fn rounds_and_ring_wrap() {
        let (mut f, mut ring) = feed(3);
        assert_eq!(latest_report(&f), None, "no round yet");
        for i in 1..=5u32 {
            assert_eq!(
                report(&mut f, &mut ring, i, i as i128, at(i as u64, 100 + i)),
                i as u64
            );
        }
        assert_eq!(f.round_id, 5);
        // rounds 3..=5 retained
        assert!(!round_in_window(2, 5, 3));
        assert!(round_in_window(3, 5, 3));
        assert_eq!(stored_report(3, &f, &ring).unwrap().answer, 3);
        assert_eq!(stored_report(2, &f, &ring), None);
        assert_eq!(stored_report(6, &f, &ring), None);
        assert_eq!(stored_report(0, &f, &ring), None);
        assert_eq!(
            latest_report(&f),
            Some(LatestReport {
                round_id: 5,
                answer: 5,
                observations_timestamp: 5,
                recorded_timestamp: 105,
            })
        );
    }

    #[test]
    fn rejects_stale_future_and_out_of_range() {
        let (mut f, mut ring) = feed(4);
        report(&mut f, &mut ring, 10, 1, at(1, 100));
        assert_eq!(
            check_report(&f, 10, 1, at(2, 100), 60),
            Err(Rejection::Stale(10))
        );
        assert_eq!(
            check_report(&f, 161, 1, at(2, 100), 60),
            Err(Rejection::FutureReport)
        );
        assert!(check_report(&f, 160, 1, at(2, 100), 60).is_ok());

        f.min_max_configured = 1;
        f.min_answer = -5;
        f.max_answer = 5;
        assert_eq!(
            check_report(&f, 11, 6, at(2, 100), 60),
            Err(Rejection::OutOfRange(-5, 5))
        );
        assert!(check_report(&f, 11, -5, at(2, 100), 60).is_ok());
    }

    #[test]
    fn max_stored_reports_rules() {
        let (mut f, mut ring) = feed(3);
        assert_eq!(
            check_max_stored_reports(&f, 1),
            Err(MaxStoredReportsError::Invalid)
        );
        // first configuration may shrink
        assert_eq!(check_max_stored_reports(&f, 2), Ok(()));
        f.max_stored_reports_configured = 1;
        assert_eq!(
            check_max_stored_reports(&f, 3),
            Err(MaxStoredReportsError::NotIncreased)
        );
        assert_eq!(check_max_stored_reports(&f, 4), Ok(()));
        report(&mut f, &mut ring, 1, 1, at(1, 1));
        report(&mut f, &mut ring, 2, 2, at(2, 2));
        assert_eq!(check_max_stored_reports(&f, 4), Ok(()));
        report(&mut f, &mut ring, 3, 3, at(3, 3));
        assert_eq!(
            check_max_stored_reports(&f, 9),
            Err(MaxStoredReportsError::Locked)
        );

        let (mut f, mut ring) = feed(4);
        report(&mut f, &mut ring, 1, 1, at(1, 1));
        report(&mut f, &mut ring, 2, 2, at(2, 2));
        assert_eq!(
            check_max_stored_reports(&f, 2),
            Err(MaxStoredReportsError::TooLow)
        );
    }
}

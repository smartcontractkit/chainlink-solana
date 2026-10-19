//! Report storage and read selection, ported from the EVM `DataFeedsStore`. Pure functions over a
//! feed header, its circular buffer and an SVR path slot so they can be unit tested and reused
//! offchain. "Same block" on EVM is "same slot" here: a report's recorded slot decides visibility,
//! its recorded unix timestamp feeds the fallback window.

use crate::common::STORED_DECIMALS;
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

/// The report an SVR path selected, plus whether the path is disabled.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct SvrSlot {
    pub disabled: bool,
    pub round_id: u64,
    pub answer: i128,
    pub observations_timestamp: u32,
    pub recorded_timestamp: u32,
    pub recorded_slot: u64,
}

/// A resolved read.
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

/// What an accepted SVR report does.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum SvrPlan {
    /// Fresher than the whole shared history: append a new round, served to this SVR path
    /// immediately and to the non-SVR feed from the next slot on.
    Append { round_id: u64 },
    /// At or behind the shared history head: the arrival is dropped and the path adopts the head.
    AdoptHead(LatestReport),
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

/// Checks a non-SVR report, returning the round ID it would be appended under.
pub fn check_non_svr_report(
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

/// Checks an SVR report against its path slot and decides whether it appends or adopts the head.
/// SVR paths share the non-SVR feed's accepted answer range.
pub fn check_svr_report(
    feed: &FeedData,
    slot: &SvrSlot,
    observations_timestamp: u32,
    answer: i128,
    now: Now,
    max_forward_secs: u32,
) -> Result<SvrPlan, Rejection> {
    if is_future_report(observations_timestamp, now, max_forward_secs) {
        return Err(Rejection::FutureReport);
    }
    if observations_timestamp <= slot.observations_timestamp {
        return Err(Rejection::Stale(slot.observations_timestamp));
    }
    let (in_range, min, max) = answer_in_range(feed, answer);
    if !in_range {
        return Err(Rejection::OutOfRange(min, max));
    }

    if observations_timestamp > feed.latest_observations_timestamp {
        Ok(SvrPlan::Append {
            round_id: feed.round_id + 1,
        })
    } else {
        Ok(SvrPlan::AdoptHead(LatestReport {
            round_id: feed.round_id,
            answer: feed.latest_answer,
            observations_timestamp: feed.latest_observations_timestamp,
            recorded_timestamp: feed.latest_recorded_timestamp,
        }))
    }
}

/// Store a report as a new round in the feed's circular buffer and latest fields.
pub fn store_report(
    feed: &mut FeedData,
    ring: &mut impl Ring,
    observations_timestamp: u32,
    answer: i128,
    now: Now,
    is_svr_authored: bool,
) -> u64 {
    feed.round_id += 1;
    let round_id = feed.round_id;
    let max_stored_reports = feed.max_stored_reports;

    ring.set(
        storage_slot(round_id, max_stored_reports),
        StoredReport {
            answer,
            observations_timestamp,
            recorded_timestamp: now.timestamp,
            recorded_slot: now.slot,
        },
    );

    // An SVR-authored round is exclusive to its path for the slot it landed in, so only an ongoing
    // same-slot SVR run holds the visible round back. Any other write releases everything before it.
    let mut visible_round_id = round_id;
    if is_svr_authored {
        let previous_visible_round_id = feed.visible_round_id;

        // A previous head that was not itself visible was SVR-authored, so a matching recorded slot
        // means this round extends that same-slot run and the visible round holds.
        visible_round_id =
            if now.slot == feed.latest_recorded_slot && previous_visible_round_id != round_id - 1 {
                previous_visible_round_id
            } else {
                round_id - 1
            };

        // A run longer than the ring would leave the visible round evicted.
        if visible_round_id != 0 && round_id - visible_round_id >= max_stored_reports {
            visible_round_id = round_id - max_stored_reports + 1;
        }
    }

    feed.visible_round_id = visible_round_id;
    feed.latest_answer = answer;
    feed.latest_observations_timestamp = observations_timestamp;
    feed.latest_recorded_timestamp = now.timestamp;
    feed.latest_recorded_slot = now.slot;

    round_id
}

/// Apply an accepted SVR plan, updating the shared history (append only) and the path slot.
pub fn apply_svr_plan(
    feed: &mut FeedData,
    ring: &mut impl Ring,
    slot: &mut SvrSlot,
    plan: SvrPlan,
    observations_timestamp: u32,
    answer: i128,
    now: Now,
) {
    match plan {
        SvrPlan::Append { .. } => {
            let round_id = store_report(feed, ring, observations_timestamp, answer, now, true);
            slot.round_id = round_id;
            slot.answer = answer;
            slot.observations_timestamp = observations_timestamp;
            slot.recorded_timestamp = now.timestamp;
            slot.recorded_slot = now.slot;
        }
        SvrPlan::AdoptHead(head) => {
            slot.round_id = head.round_id;
            slot.answer = head.answer;
            slot.observations_timestamp = head.observations_timestamp;
            slot.recorded_timestamp = head.recorded_timestamp;
            slot.recorded_slot = feed.latest_recorded_slot;
        }
    }
}

/// Latest non-SVR report. A visible round behind the head means the head was SVR-authored,
/// exclusive to its SVR path for the slot it landed in, so the non-SVR feed keeps serving the
/// visible round until the next slot.
pub fn non_svr_report(feed: &FeedData, ring: &impl Ring, now: Now) -> LatestReport {
    if feed.visible_round_id != feed.round_id && feed.latest_recorded_slot == now.slot {
        // Zero means no round has ever been released, so the feed serves an empty report.
        if feed.visible_round_id == 0 {
            return LatestReport::default();
        }
        let visible = ring.get(storage_slot(feed.visible_round_id, feed.max_stored_reports));
        return LatestReport {
            round_id: feed.visible_round_id,
            answer: visible.answer,
            observations_timestamp: visible.observations_timestamp,
            recorded_timestamp: visible.recorded_timestamp,
        };
    }

    LatestReport {
        round_id: feed.round_id,
        answer: feed.latest_answer,
        observations_timestamp: feed.latest_observations_timestamp,
        recorded_timestamp: feed.latest_recorded_timestamp,
    }
}

/// Scans backwards through recent non-SVR rounds for one old enough to serve a stale SVR path,
/// the default (zero) report when none is found.
pub fn sync_report(
    feed: &FeedData,
    ring: &impl Ring,
    fallback_time: u32,
    max_sync_iterations: u32,
    min_report_timestamp: u32,
    now: Now,
) -> LatestReport {
    let mut latest_round_id = feed.round_id;
    let max_stored_reports = feed.max_stored_reports;
    let max_sync_iterations = u64::from(max_sync_iterations);
    let mut earliest_round_id = 0;

    // Measured from the head, before the adjustment below hides it.
    if latest_round_id > max_stored_reports {
        earliest_round_id = latest_round_id - max_stored_reports;
    }

    // Same-slot SVR-authored rounds have not been released to the sync scan yet.
    if feed.latest_recorded_slot == now.slot {
        latest_round_id = feed.visible_round_id;
    }

    // The iteration budget is counted from where the scan starts, not from the head.
    if latest_round_id > max_sync_iterations
        && latest_round_id - max_sync_iterations > earliest_round_id
    {
        earliest_round_id = latest_round_id - max_sync_iterations;
    }

    let mut round = latest_round_id;
    while round > earliest_round_id {
        let stored = ring.get(storage_slot(round, max_stored_reports));
        if stored.recorded_timestamp < min_report_timestamp {
            break;
        }
        if u64::from(stored.recorded_timestamp) + u64::from(fallback_time)
            <= u64::from(now.timestamp)
        {
            return LatestReport {
                round_id: round,
                answer: stored.answer,
                observations_timestamp: stored.observations_timestamp,
                recorded_timestamp: stored.recorded_timestamp,
            };
        }
        round -= 1;
    }

    LatestReport::default()
}

/// Latest value for a data ID: the non-SVR feed, or for an SVR data ID its path slot, falling back
/// to a non-SVR round once the slot is older than the fallback window, or to the non-SVR feed
/// entirely while the path is disabled.
pub fn latest_report(
    feed: &FeedData,
    ring: &impl Ring,
    svr: Option<&SvrSlot>,
    fallback_time: u32,
    max_sync_iterations: u32,
    now: Now,
) -> LatestReport {
    let slot = match svr {
        None => return non_svr_report(feed, ring, now),
        Some(slot) if slot.disabled => return non_svr_report(feed, ring, now),
        Some(slot) => slot,
    };

    if u64::from(slot.recorded_timestamp) + u64::from(fallback_time) < u64::from(now.timestamp) {
        let synced = sync_report(
            feed,
            ring,
            fallback_time,
            max_sync_iterations,
            slot.recorded_timestamp,
            now,
        );
        if synced.observations_timestamp != 0 {
            return synced;
        }
    }

    LatestReport {
        round_id: slot.round_id,
        answer: slot.answer,
        observations_timestamp: slot.observations_timestamp,
        recorded_timestamp: slot.recorded_timestamp,
    }
}

/// A stored round as visible to a reader, the default (zero) report when it cannot be served.
/// An enabled SVR path only exposes shared history up to the round its slot selected and reads a
/// same-slot SVR-authored round right away. Non-SVR and SVR-disabled readers follow the non-SVR
/// rule and wait out the slot.
pub fn stored_report(
    round_id: u64,
    feed: &FeedData,
    ring: &impl Ring,
    svr: Option<&SvrSlot>,
    fallback_time: u32,
    now: Now,
) -> StoredReport {
    let (reads_as_svr_path, svr_path_round) = match svr {
        Some(slot) if !slot.disabled => (true, slot.round_id),
        _ => (false, 0),
    };

    if !round_in_window(round_id, feed.round_id, feed.max_stored_reports) {
        return StoredReport::default();
    }

    // A same-slot SVR-authored round belongs to the path that authored it. Every other reader, the
    // non-SVR feed or another SVR path, waits out the slot.
    if round_id > feed.visible_round_id
        && round_id > svr_path_round
        && feed.latest_recorded_slot == now.slot
    {
        return StoredReport::default();
    }

    let stored = ring.get(storage_slot(round_id, feed.max_stored_reports));

    // Past its own selected round an SVR path reads shared history it did not author. Such a round
    // is released once it has aged out of the fallback window, the same condition `sync_report`
    // serves on, so the latest and historical reads agree.
    if reads_as_svr_path
        && round_id > svr_path_round
        && u64::from(stored.recorded_timestamp) + u64::from(fallback_time)
            > u64::from(now.timestamp)
    {
        return StoredReport::default();
    }

    stored
}

/// Downscale a stored answer to `decimals`, at most the stored decimals, truncating toward zero.
pub fn downscale(answer: i128, decimals: u8) -> i128 {
    if decimals >= STORED_DECIMALS {
        return answer;
    }
    answer / 10i128.pow(u32::from(STORED_DECIMALS - decimals))
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

    const E18: i128 = 1_000_000_000_000_000_000;

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

    fn non_svr(
        feed: &mut FeedData,
        ring: &mut Vec<StoredReport>,
        obs: u32,
        answer: i128,
        now: Now,
    ) -> u64 {
        let round = check_non_svr_report(feed, obs, answer, now, 60).unwrap();
        assert_eq!(store_report(feed, ring, obs, answer, now, false), round);
        round
    }

    fn svr(
        feed: &mut FeedData,
        ring: &mut Vec<StoredReport>,
        slot: &mut SvrSlot,
        obs: u32,
        answer: i128,
        now: Now,
    ) -> SvrPlan {
        let plan = check_svr_report(feed, slot, obs, answer, now, 60).unwrap();
        apply_svr_plan(feed, ring, slot, plan, obs, answer, now);
        plan
    }

    #[test]
    fn non_svr_rounds_and_ring_wrap() {
        let (mut f, mut ring) = feed(3);
        for i in 1..=5u32 {
            assert_eq!(
                non_svr(&mut f, &mut ring, i, i as i128, at(i as u64, 100 + i)),
                i as u64
            );
        }
        assert_eq!(f.round_id, 5);
        assert_eq!(f.visible_round_id, 5);
        // rounds 3..=5 retained
        assert!(!round_in_window(2, 5, 3));
        assert!(round_in_window(3, 5, 3));
        let r = stored_report(3, &f, &ring, None, 0, at(10, 200));
        assert_eq!(r.answer, 3);
        assert_eq!(
            stored_report(2, &f, &ring, None, 0, at(10, 200)),
            StoredReport::default()
        );
        assert_eq!(
            stored_report(6, &f, &ring, None, 0, at(10, 200)),
            StoredReport::default()
        );
        assert_eq!(non_svr_report(&f, &ring, at(10, 200)).answer, 5);
    }

    #[test]
    fn rejects_stale_future_and_out_of_range() {
        let (mut f, mut ring) = feed(4);
        non_svr(&mut f, &mut ring, 10, 1, at(1, 100));
        assert_eq!(
            check_non_svr_report(&f, 10, 1, at(2, 100), 60),
            Err(Rejection::Stale(10))
        );
        assert_eq!(
            check_non_svr_report(&f, 161, 1, at(2, 100), 60),
            Err(Rejection::FutureReport)
        );
        assert!(check_non_svr_report(&f, 160, 1, at(2, 100), 60).is_ok());

        f.min_max_configured = 1;
        f.min_answer = -5;
        f.max_answer = 5;
        assert_eq!(
            check_non_svr_report(&f, 11, 6, at(2, 100), 60),
            Err(Rejection::OutOfRange(-5, 5))
        );
        assert!(check_non_svr_report(&f, 11, -5, at(2, 100), 60).is_ok());
    }

    #[test]
    fn svr_append_is_exclusive_for_its_slot() {
        let (mut f, mut ring) = feed(8);
        let mut path = SvrSlot::default();
        non_svr(&mut f, &mut ring, 10, 100, at(1, 1000));

        // SVR report fresher than the head appends round 2, exclusive to the path in slot 5
        assert_eq!(
            svr(&mut f, &mut ring, &mut path, 11, 111, at(5, 1004)),
            SvrPlan::Append { round_id: 2 }
        );
        assert_eq!(f.round_id, 2);
        assert_eq!(f.visible_round_id, 1);
        assert_eq!(path.round_id, 2);

        // same slot: non-SVR readers still see round 1, the path sees round 2
        assert_eq!(non_svr_report(&f, &ring, at(5, 1004)).answer, 100);
        assert_eq!(
            latest_report(&f, &ring, Some(&path), 60, 5, at(5, 1004)).answer,
            111
        );
        assert_eq!(
            stored_report(2, &f, &ring, None, 0, at(5, 1004)),
            StoredReport::default()
        );
        assert_eq!(
            stored_report(2, &f, &ring, Some(&path), 60, at(5, 1004)).answer,
            111
        );

        // next slot: released to everyone
        assert_eq!(non_svr_report(&f, &ring, at(6, 1005)).answer, 111);
        assert_eq!(
            stored_report(2, &f, &ring, None, 0, at(6, 1005)).answer,
            111
        );
    }

    #[test]
    fn same_slot_svr_run_holds_visible_round() {
        let (mut f, mut ring) = feed(8);
        let mut a = SvrSlot::default();
        let mut b = SvrSlot::default();
        non_svr(&mut f, &mut ring, 10, 100, at(1, 1000));
        svr(&mut f, &mut ring, &mut a, 11, 111, at(5, 1004));
        svr(&mut f, &mut ring, &mut b, 12, 112, at(5, 1004));
        assert_eq!(f.round_id, 3);
        assert_eq!(
            f.visible_round_id, 1,
            "run of SVR rounds in one slot keeps round 1 visible"
        );

        // a later non-SVR write releases everything
        non_svr(&mut f, &mut ring, 13, 113, at(5, 1004));
        assert_eq!(f.visible_round_id, 4);
    }

    #[test]
    fn svr_behind_head_adopts_head() {
        let (mut f, mut ring) = feed(8);
        let mut path = SvrSlot::default();
        non_svr(&mut f, &mut ring, 20, 200, at(1, 1000));
        let plan = svr(&mut f, &mut ring, &mut path, 15, 150, at(2, 1001));
        assert_eq!(
            plan,
            SvrPlan::AdoptHead(LatestReport {
                round_id: 1,
                answer: 200,
                observations_timestamp: 20,
                recorded_timestamp: 1000
            })
        );
        assert_eq!(f.round_id, 1, "no new round");
        assert_eq!(path.round_id, 1);
        assert_eq!(path.answer, 200);
        // the path's stale check now runs against the adopted head
        assert_eq!(
            check_svr_report(&f, &path, 20, 1, at(3, 1002), 60),
            Err(Rejection::Stale(20))
        );
    }

    #[test]
    fn stale_svr_path_falls_back_to_aged_non_svr_round() {
        let (mut f, mut ring) = feed(8);
        let mut path = SvrSlot::default();
        non_svr(&mut f, &mut ring, 10, 100, at(1, 1000));
        svr(&mut f, &mut ring, &mut path, 11, 111, at(2, 1001));
        non_svr(&mut f, &mut ring, 12, 120, at(3, 1010));
        non_svr(&mut f, &mut ring, 13, 130, at(4, 1050));

        // within the fallback window the path keeps its own selection
        assert_eq!(
            latest_report(&f, &ring, Some(&path), 30, 5, at(5, 1020)).answer,
            111
        );

        // past the window: newest non-SVR round at least `fallback` old, recorded after the slot
        let r = latest_report(&f, &ring, Some(&path), 30, 5, at(5, 1060));
        assert_eq!((r.round_id, r.answer), (3, 120));

        // a larger window only reaches back to the path's own round
        let r = latest_report(&f, &ring, Some(&path), 70, 5, at(5, 1072));
        assert_eq!((r.round_id, r.answer), (2, 111));

        // max sync iterations bound the scan from the head
        let r = latest_report(&f, &ring, Some(&path), 30, 1, at(5, 1060));
        assert_eq!(
            r.answer, 111,
            "round 4 is too recent, round 3 is out of budget"
        );

        // disabled paths read the non-SVR feed
        path.disabled = true;
        assert_eq!(
            latest_report(&f, &ring, Some(&path), 30, 5, at(5, 1020)).answer,
            130
        );
    }

    #[test]
    fn svr_historical_reads_respect_fallback_window() {
        let (mut f, mut ring) = feed(8);
        let mut path = SvrSlot::default();
        non_svr(&mut f, &mut ring, 10, 100, at(1, 1000));
        svr(&mut f, &mut ring, &mut path, 11, 111, at(2, 1001));
        non_svr(&mut f, &mut ring, 12, 120, at(3, 1010));

        // round 3 is past the path's selection and younger than the window
        assert_eq!(
            stored_report(3, &f, &ring, Some(&path), 30, at(4, 1020)),
            StoredReport::default()
        );
        assert_eq!(
            stored_report(3, &f, &ring, Some(&path), 30, at(4, 1040)).answer,
            120
        );
        assert_eq!(
            stored_report(3, &f, &ring, None, 0, at(4, 1020)).answer,
            120
        );
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
        non_svr(&mut f, &mut ring, 1, 1, at(1, 1));
        non_svr(&mut f, &mut ring, 2, 2, at(2, 2));
        assert_eq!(check_max_stored_reports(&f, 4), Ok(()));
        non_svr(&mut f, &mut ring, 3, 3, at(3, 3));
        assert_eq!(
            check_max_stored_reports(&f, 9),
            Err(MaxStoredReportsError::Locked)
        );

        let (mut f, mut ring) = feed(4);
        non_svr(&mut f, &mut ring, 1, 1, at(1, 1));
        non_svr(&mut f, &mut ring, 2, 2, at(2, 2));
        assert_eq!(
            check_max_stored_reports(&f, 2),
            Err(MaxStoredReportsError::TooLow)
        );
    }

    #[test]
    fn downscale_truncates() {
        assert_eq!(
            downscale(1_234_567_891 * E18 / 1_000_000_000, 8),
            123_456_789
        );
        assert_eq!(downscale(-15 * E18 / 10, 0), -1);
        assert_eq!(downscale(42, 18), 42);
    }
}

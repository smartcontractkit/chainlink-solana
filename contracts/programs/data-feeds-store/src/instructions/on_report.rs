use anchor_lang::prelude::*;

use crate::{
    common::ACCOUNT_VERSION,
    context::OnReport,
    data_id, df1,
    error::StoreError,
    event::*,
    logic::{self, Now},
    state::{Df1SkipReason, Feed, ReceivedReport, StoreState},
    utils::{
        has_permission, is_uninitialized, load_boxed, load_feed_into, next_account, store_feed,
        workflow_metadata, EventEmitter, RingMut,
    },
};

/// Receives a forwarder report, a Borsh-encoded `Vec<ReceivedReport>`, and applies each entry on
/// its own. An entry is skipped with an event when its data ID is invalid, its feed hasn't been
/// set or has writes disabled, its workflow isn't allowed, or it is future-dated, stale or out of
/// range; the other entries still land. When a feed links a DF1 feed, the round is also written
/// there, unless DF1 can't take it, in which case only the DF1 write is skipped.
///
/// Each report passes its data ID's feed, then the linked DF1 feed if the feed has one, even when
/// the report is then skipped. An empty system account stands for a feed that hasn't been set.
/// Its address isn't checked, because the DON's report signature covers every account key (the
/// forwarder checks their hash against the signed report).
///
/// These abort the whole transaction: malformed metadata or payload; a bad state account or
/// version; a wrong DF1 program or writer account; too few or too many accounts; an initialized
/// account that isn't this store's feed for the data ID; a missing or wrong linked DF1 feed; a
/// feed passed read-only for a report that would be stored; a linked DF1 feed passed read-only
/// when DF1 would be written; and a failing `submit` or event call. The DON's signature doesn't
/// cover writable flags, so only a faulty or malicious transmitter passes those accounts
/// read-only, and aborting lets another transmitter deliver the report. Because the signature
/// covers the account list, changing a data ID's DF1 link while a signed batch is in flight
/// aborts that batch.
pub fn handler<'info>(
    ctx: Context<'_, '_, 'info, 'info, OnReport<'info>>,
    metadata: Vec<u8>,
    report: Vec<u8>,
) -> Result<()> {
    let (workflow_name, workflow_owner) = workflow_metadata(&metadata)?;
    let reports =
        Vec::<ReceivedReport>::try_from_slice(&report).map_err(|_| StoreError::MalformedReport)?;

    let state_key = ctx.accounts.state.key();
    let state: Box<StoreState> = load_boxed(&ctx.accounts.state.to_account_info())?;
    require_eq!(
        state.version,
        ACCOUNT_VERSION,
        StoreError::InvalidAccountVersion
    );
    let clock = Clock::get()?;
    let mut batch = Batch {
        state_key,
        state: &state,
        sender: ctx.accounts.forwarder_authority.key(),
        workflow_name,
        workflow_owner,
        now: Now {
            slot: clock.slot,
            // Seconds are kept as u32, like EVM's uint32 timestamps, which lasts until 2106.
            timestamp: clock.unix_timestamp as u32,
        },
        events: EventEmitter::new(
            ctx.accounts.event_authority.to_account_info(),
            ctx.bumps.event_authority,
        ),
        df1: match (&ctx.accounts.df1_program, &ctx.accounts.df1_writer) {
            (Some(program), Some(writer)) => Some(df1::Submit::new(
                program.to_account_info(),
                writer.to_account_info(),
                state.df1_writer_bump,
            )),
            _ => None,
        },
        // One buffer serves every report. A program's 32 KiB heap is never freed, so a new buffer
        // per report would run a large batch out of heap.
        feed: bytemuck::zeroed_box(),
    };

    let mut accounts = ctx.remaining_accounts.iter();
    for report in reports.iter() {
        let feed_info = next_account(&mut accounts)?;
        batch.apply(report, feed_info, &mut accounts)?;
    }
    require!(accounts.next().is_none(), StoreError::InvalidAccountCount);
    Ok(())
}

/// The values shared by every report of one `on_report` call, loaded once.
struct Batch<'a, 'info> {
    state_key: Pubkey,
    state: &'a StoreState,
    sender: Pubkey,
    workflow_name: [u8; 10],
    workflow_owner: [u8; 20],
    now: Now,
    events: EventEmitter<'info>,
    df1: Option<df1::Submit<'info>>,
    feed: Box<Feed>,
}

impl<'info> Batch<'_, 'info> {
    /// Applies one report. The first line below that matches decides what happens to it:
    ///
    /// ```text
    ///   data ID is zero or has an SVR path ............ skip    InvalidDataIdIgnored
    ///   feed account is empty (feed not set) .......... skip    FeedNotInitializedIgnored
    ///   account isn't this store's feed for the ID .... ABORT
    ///   feed links DF1 and the next account isn't it .. ABORT
    ///   feed's writes are disabled .................... skip    FeedWritesDisabledIgnored
    ///   workflow not allowed for the data ID .......... skip    InvalidUpdatePermission
    ///   timestamp > now + forward window .............. skip    FutureReportIgnored
    ///   timestamp <= the feed's latest ................ skip    StaleReport
    ///   answer outside min/max (when set) ............. skip    AnswerOutOfMinMaxRange
    ///   feed account passed read-only ................. ABORT
    ///   feed links DF1:
    ///     write disabled, DF1 accounts not passed,
    ///     or `check_round` refuses .................... DF1 only skipped   Df1WriteSkipped
    ///     DF1 feed passed read-only ................... ABORT
    ///     otherwise `submit` (if it fails: ABORT)
    ///   store the round ............................... FeedUpdated
    /// ```
    fn apply(
        &mut self,
        report: &ReceivedReport,
        feed_info: &AccountInfo<'info>,
        accounts: &mut std::slice::Iter<'_, AccountInfo<'info>>,
    ) -> Result<()> {
        let state_key = self.state_key;
        let id = report.data_id;
        // Anyone can cause the skip events that come before the permission check, so they carry
        // the sender.
        if !data_id::is_valid_data_id(&id) {
            return self.events.emit(&InvalidDataIdIgnored {
                state: state_key,
                data_id: id,
                sender: self.sender,
            });
        }
        // A feed admin creates feeds, because the forwarder passes no account that could pay a
        // new feed's rent.
        if is_uninitialized(feed_info) {
            return self.events.emit(&FeedNotInitializedIgnored {
                state: state_key,
                data_id: id,
                sender: self.sender,
            });
        }
        load_feed_into(feed_info, &state_key, &id, &mut self.feed)?;
        // The linked DF1 feed is taken before any later skip, so the next report's accounts stay
        // in line.
        let df1_info = if self.feed.has_df1_feed() {
            let info = next_account(accounts)?;
            require_keys_eq!(*info.key, self.feed.df1_feed, StoreError::AccountMismatch);
            Some(info)
        } else {
            None
        };
        if self.feed.writes_disabled != 0 {
            return self.events.emit(&FeedWritesDisabledIgnored {
                state: state_key,
                data_id: id,
                sender: self.sender,
            });
        }

        if !has_permission(
            &self.feed.specific_workflow_metadata,
            &self.state.default_workflow_metadata,
            &self.sender,
            &self.workflow_owner,
            &self.workflow_name,
        ) {
            return self.events.emit(&InvalidUpdatePermission {
                state: state_key,
                data_id: id,
                sender: self.sender,
                workflow_owner: self.workflow_owner,
                workflow_name: self.workflow_name,
            });
        }

        // A report stamped further ahead than the forward window would become the latest round
        // and leave every later report stale until the clock caught up.
        if u64::from(report.timestamp)
            > u64::from(self.now.timestamp) + u64::from(self.state.max_observation_forward_secs)
        {
            return self.events.emit(&FutureReportIgnored {
                state: state_key,
                data_id: id,
                observations_timestamp: report.timestamp,
                recorded_timestamp: self.now.timestamp,
            });
        }
        if report.timestamp <= self.feed.latest_observations_timestamp {
            return self.events.emit(&StaleReport {
                state: state_key,
                data_id: id,
                observations_timestamp: report.timestamp,
                latest_observations_timestamp: self.feed.latest_observations_timestamp,
            });
        }
        if self.feed.min_max_configured != 0
            && !(self.feed.min_answer <= report.answer && report.answer <= self.feed.max_answer)
        {
            return self.events.emit(&AnswerOutOfMinMaxRange {
                state: state_key,
                data_id: id,
                min_answer: self.feed.min_answer,
                max_answer: self.feed.max_answer,
                answer: report.answer,
            });
        }
        // The feed must be writable only now: a skipped report may pass its feed read-only.
        require!(feed_info.is_writable, StoreError::AccountNotWritable);
        if let Some(info) = df1_info {
            self.write_df1(info, report)?;
        }

        let round_id = {
            let mut data = feed_info.try_borrow_mut_data()?;
            let mut ring = RingMut::new(&mut data[..]);
            logic::store_round(
                &mut self.feed,
                &mut ring,
                report.timestamp,
                report.answer,
                self.now,
            )
        };
        store_feed(feed_info, &self.feed)?;
        self.events.emit(&FeedUpdated {
            state: state_key,
            data_id: id,
            round_id,
            observations_timestamp: report.timestamp,
            answer: report.answer,
        })
    }

    /// Writes the round to the data ID's linked DF1 feed, or skips the DF1 write with a
    /// `Df1WriteSkipped` event if the DF1 feed can't take it.
    fn write_df1(&mut self, info: &AccountInfo<'info>, report: &ReceivedReport) -> Result<()> {
        let reason = if self.feed.df1_write_disabled != 0 {
            Df1SkipReason::Disabled
        } else if let Some(submit) = self.df1.as_mut() {
            // A failed borrow aborts rather than becoming a skip, so the report can never be
            // marked delivered without its DF1 write. The borrow ends with this statement, before
            // `submit` is called.
            let checked = df1::check_round(
                info.owner,
                &info.try_borrow_data()?,
                submit.program(),
                submit.writer(),
                report.timestamp,
                report.answer,
            );
            match checked {
                // There is no event for a DF1 write: the `submit` call already shows in the
                // transaction's inner instructions, and Solana allows at most 64 instructions per
                // transaction, inner ones and events included.
                Ok(answer) => {
                    // A DF1 feed passed read-only aborts rather than skips. The DON doesn't sign
                    // writable flags, so a skip would let a transmitter keep rounds from DF1. This
                    // is only checked when DF1 is written, so while the link is disabled, or while
                    // OCR2 is still the DF1 feed's writer, the DF1 feed can be passed read-only.
                    // Passing it writable would write-lock it: Solana would not run this
                    // transaction at the same time as OCR2's writes to that account.
                    require!(info.is_writable, StoreError::AccountNotWritable);
                    return submit.call(&self.state_key, info, report.timestamp, answer);
                }
                Err(reason) => reason,
            }
        } else {
            Df1SkipReason::NotPassed
        };
        self.events.emit(&Df1WriteSkipped {
            state: self.state_key,
            data_id: report.data_id,
            df1_feed: *info.key,
            reason,
        })
    }
}

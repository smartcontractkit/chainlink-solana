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

/// Receives a forwarder report: Borsh `Vec<ReceivedReport>`. Each report is applied independently:
/// a report with an invalid data ID, for a feed that hasn't been set or whose writes are disabled,
/// not permitted, future-dated, stale or out of range is skipped with an event while the rest still
/// land. A feed linked to a DF1 feed also writes the round there, unless DF1 can't take it (then only
/// DF1 is skipped).
///
/// A report passes one account per data ID, its feed, plus the linked DF1 feed. What still aborts
/// the whole transaction: a malformed payload or metadata, an account that is not the data ID's feed
/// or linked DF1 feed, and a feed not passed writable.
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
        // one buffer for every report: the bump allocator never frees, so a fresh one per report
        // would run a batch out of the 32KiB heap
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
    fn apply(
        &mut self,
        report: &ReceivedReport,
        feed_info: &AccountInfo<'info>,
        accounts: &mut std::slice::Iter<'_, AccountInfo<'info>>,
    ) -> Result<()> {
        let state_key = self.state_key;
        let id = report.data_id;
        // The skip events before the permission check carry the sender: anyone can cause them.
        if !data_id::is_valid_data_id(&id) {
            return self.events.emit(&InvalidDataIdIgnored {
                state: state_key,
                data_id: id,
                sender: self.sender,
            });
        }
        // Feeds are set by a feed admin: the forwarder passes no account that could pay a new
        // feed's rent.
        if is_uninitialized(feed_info) {
            return self.events.emit(&FeedNotInitializedIgnored {
                state: state_key,
                data_id: id,
                sender: self.sender,
            });
        }
        load_feed_into(feed_info, &state_key, &id, &mut self.feed)?;
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
        require!(feed_info.is_writable, StoreError::AccountNotWritable);

        if let Some(info) = df1_info {
            let disabled = self.feed.df1_write_disabled != 0;
            self.write_df1(&id, disabled, info, report)?;
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

    /// Write a round to the data ID's DF1 feed, or skip DF1 with an event if it can't take it.
    fn write_df1(
        &mut self,
        id: &[u8; 16],
        disabled: bool,
        info: &AccountInfo<'info>,
        report: &ReceivedReport,
    ) -> Result<()> {
        let state_key = self.state_key;
        let checked = if disabled {
            Err(Df1SkipReason::Disabled)
        } else {
            match &self.df1 {
                None => Err(Df1SkipReason::NotPassed),
                Some(submit) => df1::check_round(
                    info,
                    submit.program(),
                    submit.writer(),
                    report.timestamp,
                    report.answer,
                ),
            }
        };
        match checked {
            // no event: the `submit` call is in the transaction, and a transaction carries at most 64
            // instructions, events and inner calls included
            Ok(answer) => {
                self.df1
                    .as_mut()
                    .unwrap()
                    .call(&state_key, info, report.timestamp, answer)
            }
            Err(reason) => self.events.emit(&Df1WriteSkipped {
                state: state_key,
                data_id: *id,
                df1_feed: *info.key,
                reason,
            }),
        }
    }
}

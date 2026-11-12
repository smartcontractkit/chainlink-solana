use std::mem::size_of;

use anchor_lang::{prelude::*, Discriminator};
use data_feeds_proxy::{
    cpi::accounts::{WriterOnly, WriterPayer},
    state::{Feed, NewRound},
    utils::decode_feed_into,
};

use crate::{
    common::{ANCHOR_DISCRIMINATOR, FEED_CONFIG_SEED, FEED_WRITER_SEED, STORE_AUTHORITY_SEED},
    context::OnReport,
    data_id, df1,
    error::StoreError,
    event::*,
    instructions::proxy_settings::proxy_feed_address,
    state::{Df1SkipReason, FeedConfig, FeedCreationSkipReason, ReceivedReport, StoreState},
    utils::{
        create_pda_account, feed_config_address, has_permission, is_uninitialized, load_boxed,
        load_config_into, new_config, next_account, store, workflow_metadata, EventEmitter,
    },
};

/// Receives a forwarder report: Borsh `Vec<ReceivedReport>`. Each report is applied independently:
/// a report with an invalid data ID, for a feed whose writes are disabled, not permitted,
/// future-dated, stale or out of range is skipped with an event while the rest still land. A
/// permitted report for a data ID without a feed creates it, paid from the vault, as the EVM store
/// creates a feed on its first write. A feed linked to a DF1 feed also writes the round there, unless
/// DF1 can't take it (then only DF1 is skipped).
///
/// Rounds are written to the proxy in one call after every report is checked. What still aborts the
/// whole transaction: a malformed payload or metadata, an account that is not the data ID's proxy
/// feed, feed config or linked DF1 feed, and a proxy feed not passed writable.
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
        crate::common::ACCOUNT_VERSION,
        StoreError::InvalidAccountVersion
    );
    let clock = Clock::get()?;
    let mut batch = Batch {
        state_key,
        state: &state,
        sender: ctx.accounts.forwarder_authority.key(),
        now: clock.unix_timestamp as u32,
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
        // Buffers reused for every report: the bump allocator never frees, so per-report
        // allocations would run a batch out of the 32KiB heap.
        config: bytemuck::zeroed_box(),
        feed: bytemuck::zeroed_box(),
        rounds: Vec::with_capacity(reports.len()),
        feeds: Vec::with_capacity(reports.len()),
        pending: Vec::with_capacity(reports.len()),
        vault_budget: None,
        // sized up front: growing a Vec reallocates, and the bump allocator keeps every old buffer
        new_ids: Vec::with_capacity(reports.len()),
        new_bumps: Vec::with_capacity(reports.len()),
        new_feeds: Vec::with_capacity(reports.len()),
    };

    let mut accounts = ctx.remaining_accounts.iter();
    for report in reports.iter() {
        let feed_info = next_account(&mut accounts)?;
        let config_info = next_account(&mut accounts)?;
        batch.apply(
            &ctx,
            report,
            feed_info,
            config_info,
            &mut accounts,
            &workflow_name,
            &workflow_owner,
        )?;
    }
    require!(accounts.next().is_none(), StoreError::InvalidAccountCount);

    if batch.rounds.is_empty() {
        return Ok(());
    }
    let writer_seeds: &[&[u8]] = &[
        FEED_WRITER_SEED,
        state_key.as_ref(),
        &[state.feed_writer_bump],
    ];
    // one call creates every new feed of the batch, the vault paying
    if !batch.new_ids.is_empty() {
        batch
            .events
            .emit_with(FeedsCreated::DISCRIMINATOR, |data| {
                state_key.serialize(data)?;
                batch.new_ids.serialize(data)?;
                state.default_max_stored_reports.serialize(data)?;
                Ok(())
            })?;
        let (Some(vault), Some(system_program)) =
            (&ctx.accounts.vault, &ctx.accounts.system_program)
        else {
            unreachable!("feeds are only planned with the vault passed");
        };
        data_feeds_proxy::cpi::create_feeds(
            CpiContext::new_with_signer(
                ctx.accounts.proxy_program.to_account_info(),
                WriterPayer {
                    writer: ctx.accounts.feed_writer.to_account_info(),
                    state: ctx.accounts.proxy_state.to_account_info(),
                    payer: vault.to_account_info(),
                    system_program: system_program.to_account_info(),
                },
                &[
                    writer_seeds,
                    &[
                        STORE_AUTHORITY_SEED,
                        state_key.as_ref(),
                        &[state.store_authority_bump],
                    ],
                ],
            )
            .with_remaining_accounts(std::mem::take(&mut batch.new_feeds)),
            std::mem::take(&mut batch.new_ids),
            std::mem::take(&mut batch.new_bumps),
            state.default_max_stored_reports,
        )?;
    }
    // one call writes every round of the batch
    data_feeds_proxy::cpi::write_rounds(
        CpiContext::new_with_signer(
            ctx.accounts.proxy_program.to_account_info(),
            WriterOnly {
                writer: ctx.accounts.feed_writer.to_account_info(),
                state: ctx.accounts.proxy_state.to_account_info(),
            },
            &[writer_seeds],
        )
        .with_remaining_accounts(std::mem::take(&mut batch.feeds)),
        std::mem::take(&mut batch.rounds),
    )
}

/// What a batch has written so far to one proxy feed: rounds are written after the loop, so a
/// second report for the same feed checks against the first.
struct Pending {
    feed: Pubkey,
    latest_observations_timestamp: u32,
    round_id: u64,
}

struct Batch<'a, 'info> {
    state_key: Pubkey,
    state: &'a StoreState,
    sender: Pubkey,
    now: u32,
    events: EventEmitter<'info>,
    df1: Option<df1::Submit<'info>>,
    config: Box<FeedConfig>,
    feed: Box<Feed>,
    rounds: Vec<NewRound>,
    feeds: Vec<AccountInfo<'info>>,
    pending: Vec<Pending>,
    /// Lamports the vault can still spend on this batch's new accounts, read on first use
    vault_budget: Option<u64>,
    /// Feeds to create in one call after the loop: the bump allocator never frees, so a call per
    /// new feed would run a batch of new feeds out of heap
    new_ids: Vec<[u8; 16]>,
    new_bumps: Vec<u8>,
    new_feeds: Vec<AccountInfo<'info>>,
}

impl<'info> Batch<'_, 'info> {
    #[allow(clippy::too_many_arguments)]
    fn apply(
        &mut self,
        ctx: &Context<'_, '_, 'info, 'info, OnReport<'info>>,
        report: &ReceivedReport,
        feed_info: &AccountInfo<'info>,
        config_info: &AccountInfo<'info>,
        accounts: &mut std::slice::Iter<'_, AccountInfo<'info>>,
        workflow_name: &[u8; 10],
        workflow_owner: &[u8; 20],
    ) -> Result<()> {
        let state_key = self.state_key;
        let id = report.data_id;
        if !data_id::is_valid_data_id(&id) {
            return self.events.emit(&InvalidDataIdIgnored {
                state: state_key,
                data_id: id,
                sender: self.sender,
            });
        }

        // The feed config: a sender can't leave out an existing one (it may hold the data ID's own
        // workflows), so one that doesn't exist must be passed at its address.
        let config_exists = !is_uninitialized(config_info);
        let mut df1_info = None;
        if config_exists {
            load_config_into(config_info, &state_key, &id, &mut self.config)?;
            if self.config.has_df1_feed() {
                let info = next_account(accounts)?;
                require_keys_eq!(*info.key, self.config.df1_feed, StoreError::AccountMismatch);
                df1_info = Some(info);
            }
            if self.config.writes_disabled != 0 {
                return self.events.emit(&FeedWritesDisabledIgnored {
                    state: state_key,
                    data_id: id,
                    sender: self.sender,
                });
            }
        } else {
            require_keys_eq!(
                feed_config_address(&state_key, &id).0,
                *config_info.key,
                StoreError::AccountMismatch
            );
            new_config(&state_key, &id, &mut self.config);
        }

        if !has_permission(
            &self.config.specific_workflow_metadata,
            &self.state.default_workflow_metadata,
            &self.sender,
            workflow_owner,
            workflow_name,
        ) {
            return self.events.emit(&InvalidUpdatePermission {
                state: state_key,
                data_id: id,
                sender: self.sender,
                workflow_owner: *workflow_owner,
                workflow_name: *workflow_name,
            });
        }

        // The proxy feed's latest round, or this batch's pending one.
        let feed_exists = !is_uninitialized(feed_info);
        let (latest_observations_timestamp, round_id) =
            match self.pending.iter().find(|p| p.feed == *feed_info.key) {
                Some(p) => (p.latest_observations_timestamp, p.round_id),
                None if feed_exists => {
                    require_keys_eq!(
                        *feed_info.owner,
                        self.state.proxy_program,
                        StoreError::AccountMismatch
                    );
                    decode_feed_into(
                        &feed_info.try_borrow_data()?,
                        &self.state.proxy_state,
                        &data_id::to_seed(&id),
                        &mut self.feed,
                    )?;
                    (self.feed.latest_observations_timestamp, self.feed.round_id)
                }
                None => (0, 0),
            };

        if u64::from(report.timestamp)
            > u64::from(self.now) + u64::from(self.state.max_observation_forward_secs)
        {
            return self.events.emit(&FutureReportIgnored {
                state: state_key,
                data_id: id,
                observations_timestamp: report.timestamp,
                recorded_timestamp: self.now,
            });
        }
        if report.timestamp <= latest_observations_timestamp {
            return self.events.emit(&StaleReport {
                state: state_key,
                data_id: id,
                observations_timestamp: report.timestamp,
                latest_observations_timestamp,
            });
        }
        if self.config.min_max_configured != 0
            && !(self.config.min_answer <= report.answer && report.answer <= self.config.max_answer)
        {
            return self.events.emit(&AnswerOutOfMinMaxRange {
                state: state_key,
                data_id: id,
                min_answer: self.config.min_answer,
                max_answer: self.config.max_answer,
                answer: report.answer,
            });
        }
        require!(feed_info.is_writable, StoreError::AccountNotWritable);

        // the first written report creates what the data ID lacks: its proxy feed, and its config
        // when a feed admin created only the feed (e.g. by setting its description)
        let feed_missing = !feed_exists && !self.pending.iter().any(|p| p.feed == *feed_info.key);
        let config_missing = !config_exists && is_uninitialized(config_info);
        if feed_missing || config_missing {
            if let Some(reason) = self.create_accounts(
                ctx,
                &id,
                feed_info,
                config_info,
                feed_missing,
                config_missing,
            )? {
                return self.events.emit(&FeedCreationSkipped {
                    state: state_key,
                    data_id: id,
                    reason,
                });
            }
        }

        if let Some(info) = df1_info {
            self.write_df1(&id, info, report)?;
        }

        let round_id = round_id + 1;
        self.rounds.push(NewRound {
            observations_timestamp: report.timestamp,
            answer: report.answer,
        });
        self.feeds.push(feed_info.clone());
        match self.pending.iter_mut().find(|p| p.feed == *feed_info.key) {
            Some(p) => {
                p.latest_observations_timestamp = report.timestamp;
                p.round_id = round_id;
            }
            None => self.pending.push(Pending {
                feed: *feed_info.key,
                latest_observations_timestamp: report.timestamp,
                round_id,
            }),
        }
        self.events.emit(&FeedUpdated {
            state: state_key,
            data_id: id,
            round_id,
            observations_timestamp: report.timestamp,
            answer: report.answer,
        })
    }

    /// Create a data ID's missing feed config now and plan its missing proxy feed (created after the
    /// loop with the batch's other new feeds), paid from the vault. A wrong feed address aborts, as
    /// for any wrong account; a vault that wasn't passed or can't pay returns why the report is
    /// skipped.
    fn create_accounts(
        &mut self,
        ctx: &Context<'_, '_, 'info, 'info, OnReport<'info>>,
        id: &[u8; 16],
        feed_info: &AccountInfo<'info>,
        config_info: &AccountInfo<'info>,
        feed_missing: bool,
        config_missing: bool,
    ) -> Result<Option<FeedCreationSkipReason>> {
        let mut bump = 0;
        if feed_missing {
            let (address, feed_bump) =
                proxy_feed_address(&self.state.proxy_program, &self.state.proxy_state, id);
            require_keys_eq!(address, *feed_info.key, StoreError::AccountMismatch);
            bump = feed_bump;
        }
        let (Some(vault), Some(system_program)) =
            (&ctx.accounts.vault, &ctx.accounts.system_program)
        else {
            return Ok(Some(FeedCreationSkipReason::VaultNotPassed));
        };
        let rent = Rent::get()?;
        let config_len = ANCHOR_DISCRIMINATOR + size_of::<FeedConfig>();
        let mut needed = 0;
        if feed_missing {
            needed += rent.minimum_balance(data_feeds_proxy::state::feed_account_len(
                self.state.default_max_stored_reports,
            ));
        }
        if config_missing {
            needed += rent.minimum_balance(config_len);
        }
        // the vault stays rent exempt itself
        let budget = *self
            .vault_budget
            .get_or_insert_with(|| vault.lamports().saturating_sub(rent.minimum_balance(0)));
        if budget < needed {
            return Ok(Some(FeedCreationSkipReason::InsufficientVaultBalance));
        }
        self.vault_budget = Some(budget - needed);

        let state_key = self.state_key;
        if config_missing {
            let seed = data_id::to_seed(id);
            let config_bump = feed_config_address(&state_key, id).1;
            create_pda_account(
                config_len,
                &[
                    &[
                        STORE_AUTHORITY_SEED,
                        state_key.as_ref(),
                        &[self.state.store_authority_bump],
                    ],
                    &[FEED_CONFIG_SEED, state_key.as_ref(), &seed, &[config_bump]],
                ],
                &vault.to_account_info(),
                config_info,
                &system_program.to_account_info(),
            )?;
            store(config_info, &*self.config)?;
        }
        if feed_missing {
            self.new_ids.push(*id);
            self.new_bumps.push(bump);
            self.new_feeds.push(feed_info.clone());
        }
        Ok(None)
    }

    /// Write a round to the data ID's DF1 feed, or skip DF1 with an event if it can't take it.
    fn write_df1(
        &mut self,
        id: &[u8; 16],
        info: &AccountInfo<'info>,
        report: &ReceivedReport,
    ) -> Result<()> {
        let state_key = self.state_key;
        let checked = if self.config.df1_write_disabled != 0 {
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

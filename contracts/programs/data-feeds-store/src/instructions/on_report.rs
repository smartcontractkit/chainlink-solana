use anchor_lang::{
    prelude::*,
    solana_program::{
        instruction::{AccountMeta, Instruction},
        program::{get_return_data, invoke_signed},
    },
};

use crate::{
    common::{ON_REPORT_CALLBACK_DISCRIMINATOR, STORED_DECIMALS, STORE_AUTHORITY_SEED},
    context::OnReport,
    data_id,
    error::StoreError,
    event::*,
    logic::{self, Now, Rejection, SvrPlan, SvrSlot},
    state::{DataIdConfig, DataIdSettings, FeedData, OnReportCallbackArgs, ReceivedReport, StoreState},
    utils::{
        has_permission, load_boxed, load_data_id_config, load_feed, next_account, store,
        workflow_metadata, RingMut,
    },
};

/// Receives a forwarder report: Borsh `Vec<ReceivedReport>`. Each report is applied independently,
/// a report that is not permitted, future-dated, stale, out of range or rejected by its callback is
/// skipped with an event while the rest still land.
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
    let (max_forward_secs, store_authority_bump) = (
        state.max_observation_forward_secs,
        state.store_authority_bump,
    );
    let sender = ctx.accounts.forwarder_authority.key();
    let clock = Clock::get()?;
    let now = Now {
        slot: clock.slot,
        timestamp: clock.unix_timestamp as u32,
    };

    let callback = Callback {
        state: state_key,
        store_authority: ctx.accounts.store_authority.to_account_info(),
        bump: store_authority_bump,
    };

    let mut accounts = ctx.remaining_accounts.iter();
    for report in reports.iter() {
        // an SVR path passes its config before the shared feed, a non-SVR data ID only its feed
        let svr = data_id::is_svr(&report.data_id);
        let config = if svr {
            let info = next_account(&mut accounts)?;
            Some((info, load_data_id_config(info, &state_key, &report.data_id)?))
        } else {
            None
        };
        let feed_info = next_account(&mut accounts)?;
        let feed = load_feed(
            feed_info,
            &state_key,
            &data_id::to_non_svr(&report.data_id),
        )?;
        let settings = match &config {
            Some((_, config)) => &config.settings,
            None => &feed.settings,
        };

        // callback program and accounts must be passed exactly as configured
        let callback_infos = if settings.has_callback() {
            let program = next_account(&mut accounts)?;
            require_keys_eq!(
                *program.key,
                settings.callback_program,
                StoreError::AccountMismatch
            );
            let mut infos = vec![program.clone()];
            for expected in settings.callback_accounts() {
                let info = next_account(&mut accounts)?;
                require_keys_eq!(*info.key, *expected, StoreError::AccountMismatch);
                infos.push(info.clone());
            }
            infos
        } else {
            vec![]
        };

        if !has_permission(
            &settings.specific_workflow_metadata,
            &state.default_workflow_metadata,
            &sender,
            &workflow_owner,
            &workflow_name,
        ) {
            emit!(InvalidUpdatePermission {
                state: state_key,
                data_id: report.data_id,
                sender,
                workflow_owner,
                workflow_name,
            });
            continue;
        }

        let processed = Processed {
            state: state_key,
            report,
            now,
            max_forward_secs,
            callback: &callback,
            callback_infos: &callback_infos,
        };
        match config {
            Some((config_info, config)) => processed.svr(config_info, feed_info, feed, config)?,
            None => processed.non_svr(feed_info, feed)?,
        }
    }

    require!(accounts.next().is_none(), StoreError::InvalidAccountCount);
    Ok(())
}

struct Callback<'info> {
    state: Pubkey,
    store_authority: AccountInfo<'info>,
    bump: u8,
}

impl<'info> Callback<'info> {
    /// Call the data ID's callback, if any. Returns false only when the callback did not accept
    /// the report and its config drops rejected reports. A callback that fails aborts the whole
    /// transaction, Solana cannot contain a failed cross-program call.
    fn call(
        &self,
        config: &DataIdSettings,
        infos: &[AccountInfo<'info>],
        args: OnReportCallbackArgs,
    ) -> Result<bool> {
        if !config.has_callback() {
            return Ok(true);
        }

        let (program, callback_accounts) = infos.split_first().unwrap();
        let mut metas = vec![AccountMeta::new_readonly(*self.store_authority.key, true)];
        for (i, info) in callback_accounts.iter().enumerate() {
            metas.push(if config.callback_account_writable(i) {
                AccountMeta::new(*info.key, false)
            } else {
                AccountMeta::new_readonly(*info.key, false)
            });
        }

        let mut data = ON_REPORT_CALLBACK_DISCRIMINATOR.to_vec();
        args.serialize(&mut data)?;

        let mut account_infos = vec![self.store_authority.clone()];
        account_infos.extend(callback_accounts.iter().cloned());
        account_infos.push(program.clone());

        invoke_signed(
            &Instruction {
                program_id: config.callback_program,
                accounts: metas,
                data,
            },
            &account_infos,
            &[&[STORE_AUTHORITY_SEED, self.state.as_ref(), &[self.bump]]],
        )?;

        let accepted = match get_return_data() {
            Some((program_id, data)) if program_id == config.callback_program => {
                bool::try_from_slice(&data).unwrap_or(false)
            }
            _ => false,
        };
        Ok(accepted || config.callback_revert_on_failure == 0)
    }
}

struct Processed<'a, 'info> {
    state: Pubkey,
    report: &'a ReceivedReport,
    now: Now,
    max_forward_secs: u32,
    callback: &'a Callback<'info>,
    callback_infos: &'a [AccountInfo<'info>],
}

impl<'info> Processed<'_, 'info> {
    fn rejected(&self, data_id: [u8; 16], rejection: Rejection, svr: bool) {
        let report = self.report;
        match rejection {
            Rejection::FutureReport => emit!(FutureReportIgnored {
                state: self.state,
                data_id,
                observations_timestamp: report.timestamp,
                recorded_timestamp: self.now.timestamp,
            }),
            Rejection::Stale(latest) if svr => emit!(StaleSvrReport {
                state: self.state,
                svr_data_id: data_id,
                observations_timestamp: report.timestamp,
                latest_observations_timestamp: latest,
            }),
            Rejection::Stale(latest) => emit!(StaleReport {
                state: self.state,
                non_svr_data_id: data_id,
                observations_timestamp: report.timestamp,
                latest_observations_timestamp: latest,
            }),
            Rejection::OutOfRange(min_answer, max_answer) => emit!(AnswerOutOfMinMaxRange {
                state: self.state,
                data_id,
                min_answer,
                max_answer,
                answer: report.answer,
            }),
        }
    }

    fn non_svr(&self, feed_info: &AccountInfo<'info>, mut feed: Box<FeedData>) -> Result<()> {
        let report = self.report;
        let non_svr_data_id = report.data_id;

        let round_id = match logic::check_non_svr_report(
            &feed,
            report.timestamp,
            report.answer,
            self.now,
            self.max_forward_secs,
        ) {
            Ok(round_id) => round_id,
            Err(rejection) => {
                self.rejected(non_svr_data_id, rejection, false);
                return Ok(());
            }
        };

        // nothing is written until the callback accepts
        let accepted = self.callback.call(
            &feed.settings,
            self.callback_infos,
            OnReportCallbackArgs {
                data_id: non_svr_data_id,
                svr_path_id: [0, 0],
                round_id,
                answer: report.answer,
                decimals: STORED_DECIMALS,
                observations_timestamp: report.timestamp,
                recorded_timestamp: self.now.timestamp,
                is_freshest: true,
            },
        )?;
        if !accepted {
            emit!(ReportCallbackRejected {
                state: self.state,
                data_id: non_svr_data_id,
                round_id,
                answer: report.answer,
            });
            return Ok(());
        }

        {
            let mut data = feed_info.try_borrow_mut_data()?;
            let mut ring = RingMut::new(&mut data[..]);
            logic::store_report(
                &mut feed,
                &mut ring,
                report.timestamp,
                report.answer,
                self.now,
                false,
            );
        }
        store(feed_info, &*feed)?;

        emit!(FeedUpdated {
            state: self.state,
            non_svr_data_id,
            round_id,
            observations_timestamp: report.timestamp,
            answer: report.answer,
        });
        Ok(())
    }

    fn svr(
        &self,
        config_info: &AccountInfo<'info>,
        feed_info: &AccountInfo<'info>,
        mut feed: Box<FeedData>,
        mut config: Box<DataIdConfig>,
    ) -> Result<()> {
        let report = self.report;
        let svr_data_id = report.data_id;
        let non_svr_data_id = data_id::to_non_svr(&svr_data_id);

        let mut slot = SvrSlot {
            disabled: config.svr_disabled != 0,
            round_id: config.svr_round_id,
            answer: config.svr_answer,
            observations_timestamp: config.svr_observations_timestamp,
            recorded_timestamp: config.svr_recorded_timestamp,
            recorded_slot: config.svr_recorded_slot,
        };

        let plan = match logic::check_svr_report(
            &feed,
            &slot,
            report.timestamp,
            report.answer,
            self.now,
            self.max_forward_secs,
        ) {
            Ok(plan) => plan,
            Err(rejection) => {
                self.rejected(svr_data_id, rejection, true);
                return Ok(());
            }
        };

        let args = match plan {
            SvrPlan::Append { round_id } => OnReportCallbackArgs {
                data_id: svr_data_id,
                svr_path_id: data_id::svr_path_id(&svr_data_id),
                round_id,
                answer: report.answer,
                decimals: STORED_DECIMALS,
                observations_timestamp: report.timestamp,
                recorded_timestamp: self.now.timestamp,
                is_freshest: true,
            },
            // the head is already stored, so this selection adds no round to the shared history
            SvrPlan::AdoptHead(head) => OnReportCallbackArgs {
                data_id: svr_data_id,
                svr_path_id: data_id::svr_path_id(&svr_data_id),
                round_id: head.round_id,
                answer: head.answer,
                decimals: STORED_DECIMALS,
                observations_timestamp: head.observations_timestamp,
                recorded_timestamp: head.recorded_timestamp,
                is_freshest: false,
            },
        };
        if !self
            .callback
            .call(&config.settings, self.callback_infos, args.clone())?
        {
            emit!(ReportCallbackRejected {
                state: self.state,
                data_id: svr_data_id,
                round_id: args.round_id,
                answer: args.answer,
            });
            return Ok(());
        }

        {
            let mut data = feed_info.try_borrow_mut_data()?;
            let mut ring = RingMut::new(&mut data[..]);
            logic::apply_svr_plan(
                &mut feed,
                &mut ring,
                &mut slot,
                plan,
                report.timestamp,
                report.answer,
                self.now,
            );
        }

        config.svr_round_id = slot.round_id;
        config.svr_answer = slot.answer;
        config.svr_observations_timestamp = slot.observations_timestamp;
        config.svr_recorded_timestamp = slot.recorded_timestamp;
        config.svr_recorded_slot = slot.recorded_slot;
        store(config_info, &*config)?;

        match plan {
            SvrPlan::Append { round_id } => {
                store(feed_info, &*feed)?;
                emit!(FeedUpdated {
                    state: self.state,
                    non_svr_data_id,
                    round_id,
                    observations_timestamp: report.timestamp,
                    answer: report.answer,
                });
                emit!(SvrFeedUpdated {
                    state: self.state,
                    svr_data_id,
                    round_id,
                    observations_timestamp: report.timestamp,
                    answer: report.answer,
                });
            }
            SvrPlan::AdoptHead(head) => emit!(SvrReportSynced {
                state: self.state,
                svr_data_id,
                round_id: head.round_id,
                observations_timestamp: head.observations_timestamp,
                answer: head.answer,
            }),
        }
        Ok(())
    }
}

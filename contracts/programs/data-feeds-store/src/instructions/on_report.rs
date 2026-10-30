use anchor_lang::{
    prelude::*,
    solana_program::{
        instruction::{AccountMeta, Instruction},
        program::{get_return_data, invoke_signed},
    },
};

use crate::{
    common::{
        ACCOUNT_VERSION, CALLBACK_AUTHORITY_SEED, MAX_CALLBACK_ACCOUNTS,
        ON_REPORT_CALLBACK_DISCRIMINATOR, STORED_DECIMALS,
    },
    context::OnReport,
    data_id,
    error::StoreError,
    event::*,
    logic::{self, Now, Rejection},
    state::{DataIdSettings, FeedData, OnReportCallbackArgs, ReceivedReport, StoreState},
    utils::{
        has_permission, is_uninitialized, load_boxed, load_feed_into, next_account, store,
        workflow_metadata, EventEmitter, RingMut,
    },
};

/// Receives a forwarder report: Borsh `Vec<ReceivedReport>`. Each report is applied independently:
/// a report with an invalid data ID, for an uninitialized feed, not permitted, future-dated, stale,
/// out of range or rejected by its callback is skipped with an event while the rest still land.
/// What still aborts the whole transaction: a malformed payload or metadata, a feed account that
/// is not the data ID's feed of this store, callback accounts not passed as configured, and a
/// callback that fails (Solana cannot contain a failed cross-program call).
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
    let max_forward_secs = state.max_observation_forward_secs;
    let sender = ctx.accounts.forwarder_authority.key();
    let clock = Clock::get()?;
    let now = Now {
        slot: clock.slot,
        timestamp: clock.unix_timestamp as u32,
    };
    let events = EventEmitter::new(
        ctx.accounts.event_authority.to_account_info(),
        ctx.bumps.event_authority,
    );
    let mut authorities = CallbackAuthorities::default();
    // Buffers reused for every report: the bump allocator never frees, so per-report allocations
    // would run a batch out of the 32KiB heap.
    let mut feed: Box<FeedData> = bytemuck::zeroed_box();
    let mut callback = CallbackScratch::default();

    let mut accounts = ctx.remaining_accounts.iter();
    for report in reports.iter() {
        let feed_info = next_account(&mut accounts)?;

        // The store keeps no feed for these, so no callback accounts follow the feed account.
        if !data_id::is_valid_data_id(&report.data_id) {
            events.emit(&InvalidDataIdIgnored {
                state: state_key,
                data_id: report.data_id,
            })?;
            continue;
        }
        if is_uninitialized(feed_info) {
            events.emit(&FeedNotInitializedIgnored {
                state: state_key,
                data_id: report.data_id,
            })?;
            continue;
        }
        load_feed_into(feed_info, &state_key, &report.data_id, &mut feed)?;

        // callback program, its authority and accounts must be passed exactly as configured
        callback.infos.clear();
        if feed.settings.has_callback() {
            let program = next_account(&mut accounts)?;
            require_keys_eq!(
                *program.key,
                feed.settings.callback_program,
                StoreError::AccountMismatch
            );
            let authority = next_account(&mut accounts)?;
            require_keys_eq!(
                *authority.key,
                authorities.get(&state_key, &feed.settings)?,
                StoreError::AccountMismatch
            );
            callback.infos.push(program.clone());
            callback.infos.push(authority.clone());
            for expected in feed.settings.callback_accounts() {
                let info = next_account(&mut accounts)?;
                require_keys_eq!(*info.key, *expected, StoreError::AccountMismatch);
                callback.infos.push(info.clone());
            }
        }

        if !has_permission(
            &feed.settings.specific_workflow_metadata,
            &state.default_workflow_metadata,
            &sender,
            &workflow_owner,
            &workflow_name,
        ) {
            events.emit(&InvalidUpdatePermission {
                state: state_key,
                data_id: report.data_id,
                sender,
                workflow_owner,
                workflow_name,
            })?;
            continue;
        }

        Processed {
            state: state_key,
            report,
            now,
            max_forward_secs,
            events: &events,
        }
        .apply(feed_info, &mut feed, &mut callback)?;
    }

    require!(accounts.next().is_none(), StoreError::InvalidAccountCount);
    Ok(())
}

/// Callback authority PDAs of the reports' callback programs, derived once per program: a batch
/// usually has one callback program (the DF1 store) for every report.
#[derive(Default)]
struct CallbackAuthorities {
    last: Option<(Pubkey, Pubkey)>,
}

impl CallbackAuthorities {
    fn get(&mut self, state: &Pubkey, settings: &DataIdSettings) -> Result<Pubkey> {
        let program = settings.callback_program;
        if let Some((cached_program, authority)) = self.last {
            if cached_program == program {
                return Ok(authority);
            }
        }
        let authority = Pubkey::create_program_address(
            &[
                CALLBACK_AUTHORITY_SEED,
                state.as_ref(),
                program.as_ref(),
                &[settings.callback_authority_bump],
            ],
            &crate::ID,
        )
        .map_err(|_| StoreError::AccountMismatch)?;
        self.last = Some((program, authority));
        Ok(authority)
    }
}

/// The callback's accounts as passed (program, its callback authority, configured accounts) and
/// the instruction and account list reused to call it, for every report of a batch.
struct CallbackScratch<'info> {
    infos: Vec<AccountInfo<'info>>,
    call_infos: Vec<AccountInfo<'info>>,
    ix: Instruction,
}

impl Default for CallbackScratch<'_> {
    fn default() -> Self {
        Self {
            infos: Vec::with_capacity(2 + MAX_CALLBACK_ACCOUNTS),
            call_infos: Vec::with_capacity(2 + MAX_CALLBACK_ACCOUNTS),
            ix: Instruction {
                program_id: Pubkey::default(),
                accounts: Vec::with_capacity(1 + MAX_CALLBACK_ACCOUNTS),
                data: Vec::with_capacity(128),
            },
        }
    }
}

/// Call the data ID's callback, if any, signed by the callback program's own authority PDA.
/// Returns false only when the callback did not accept the report and its settings drop rejected
/// reports. A callback that fails aborts the whole transaction, Solana cannot contain a failed
/// cross-program call.
fn call_callback<'info>(
    state: &Pubkey,
    settings: &DataIdSettings,
    scratch: &mut CallbackScratch<'info>,
    args: OnReportCallbackArgs,
) -> Result<bool> {
    if !settings.has_callback() {
        return Ok(true);
    }

    // scratch.infos: [program, authority, configured accounts...]
    let CallbackScratch {
        infos,
        call_infos,
        ix,
    } = scratch;
    let (program, rest) = infos.split_first().unwrap();
    let (authority, callback_accounts) = rest.split_first().unwrap();

    ix.program_id = settings.callback_program;
    ix.accounts.clear();
    ix.accounts
        .push(AccountMeta::new_readonly(*authority.key, true));
    for (i, info) in callback_accounts.iter().enumerate() {
        ix.accounts.push(if settings.callback_account_writable(i) {
            AccountMeta::new(*info.key, false)
        } else {
            AccountMeta::new_readonly(*info.key, false)
        });
    }
    ix.data.clear();
    ix.data.extend_from_slice(&ON_REPORT_CALLBACK_DISCRIMINATOR);
    args.serialize(&mut ix.data)?;

    call_infos.clear();
    call_infos.push(authority.clone());
    call_infos.extend(callback_accounts.iter().cloned());
    call_infos.push(program.clone());

    invoke_signed(
        ix,
        call_infos,
        &[&[
            CALLBACK_AUTHORITY_SEED,
            state.as_ref(),
            settings.callback_program.as_ref(),
            &[settings.callback_authority_bump],
        ]],
    )?;

    let accepted = match get_return_data() {
        Some((program_id, data)) if program_id == settings.callback_program => {
            bool::try_from_slice(&data).unwrap_or(false)
        }
        _ => false,
    };
    Ok(accepted || settings.callback_revert_on_failure == 0)
}

struct Processed<'a, 'info> {
    state: Pubkey,
    report: &'a ReceivedReport,
    now: Now,
    max_forward_secs: u32,
    events: &'a EventEmitter<'info>,
}

impl<'info> Processed<'_, 'info> {
    fn rejected(&self, rejection: Rejection) -> Result<()> {
        let report = self.report;
        match rejection {
            Rejection::FutureReport => self.events.emit(&FutureReportIgnored {
                state: self.state,
                data_id: report.data_id,
                observations_timestamp: report.timestamp,
                recorded_timestamp: self.now.timestamp,
            }),
            Rejection::Stale(latest) => self.events.emit(&StaleReport {
                state: self.state,
                data_id: report.data_id,
                observations_timestamp: report.timestamp,
                latest_observations_timestamp: latest,
            }),
            Rejection::OutOfRange(min_answer, max_answer) => {
                self.events.emit(&AnswerOutOfMinMaxRange {
                    state: self.state,
                    data_id: report.data_id,
                    min_answer,
                    max_answer,
                    answer: report.answer,
                })
            }
        }
    }

    fn apply(
        &self,
        feed_info: &AccountInfo<'info>,
        feed: &mut FeedData,
        callback: &mut CallbackScratch<'info>,
    ) -> Result<()> {
        let report = self.report;

        let round_id = match logic::check_report(
            feed,
            report.timestamp,
            report.answer,
            self.now,
            self.max_forward_secs,
        ) {
            Ok(round_id) => round_id,
            Err(rejection) => return self.rejected(rejection),
        };

        // nothing is written until the callback accepts
        let accepted = call_callback(
            &self.state,
            &feed.settings,
            callback,
            OnReportCallbackArgs {
                data_id: report.data_id,
                round_id,
                answer: report.answer,
                decimals: STORED_DECIMALS,
                observations_timestamp: report.timestamp,
                recorded_timestamp: self.now.timestamp,
            },
        )?;
        if !accepted {
            return self.events.emit(&ReportCallbackRejected {
                state: self.state,
                data_id: report.data_id,
                round_id,
                answer: report.answer,
            });
        }

        {
            let mut data = feed_info.try_borrow_mut_data()?;
            let mut ring = RingMut::new(&mut data[..]);
            logic::store_report(feed, &mut ring, report.timestamp, report.answer, self.now);
        }
        store(feed_info, feed)?;

        self.events.emit(&FeedUpdated {
            state: self.state,
            data_id: report.data_id,
            round_id,
            observations_timestamp: report.timestamp,
            answer: report.answer,
        })
    }
}

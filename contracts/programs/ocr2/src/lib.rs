use anchor_lang::prelude::*;
use anchor_lang::solana_program::sysvar::fees::Fees;
use anchor_spl::token;

use arrayref::{array_ref, array_refs};

#[cfg(feature = "mainnet")]
declare_id!("My11111111111111111111111111111111111111111");
#[cfg(feature = "testnet")]
declare_id!("My11111111111111111111111111111111111111111");
#[cfg(feature = "devnet")]
declare_id!("My11111111111111111111111111111111111111111");
#[cfg(not(any(feature = "mainnet", feature = "testnet", feature = "devnet")))]
declare_id!("CF13pnKGJ1WJZeEgVAtFdUi4MMndXm9hneiHs8azUaZt");

mod context;
pub mod event;
mod state;

use crate::context::*;
use crate::state::{Config, LeftoverPayment, Oracle, SigningKey, State, MAX_ORACLES};

use std::collections::BTreeSet;
use std::convert::TryInto;
use std::mem::size_of;

use access_controller::AccessController;
use store::Transmission;

#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct NewOracle {
    pub signer: [u8; 20],
    pub transmitter: Pubkey,
}

#[program]
pub mod ocr2 {
    use super::*;
    pub fn initialize(
        ctx: Context<Initialize>,
        nonce: u8,
        min_answer: i128,
        max_answer: i128,
    ) -> ProgramResult {
        let mut state = ctx.accounts.state.load_init()?;
        state.version = 1;
        state.nonce = nonce;
        state.transmissions = ctx.accounts.transmissions.key();

        let config = &mut state.config;

        config.owner = ctx.accounts.owner.key();

        config.token_mint = ctx.accounts.token_mint.key();
        config.token_vault = ctx.accounts.token_vault.key();
        config.requester_access_controller = ctx.accounts.requester_access_controller.key();
        config.billing_access_controller = ctx.accounts.billing_access_controller.key();

        config.min_answer = min_answer;
        config.max_answer = max_answer;

        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn close(ctx: Context<Close>) -> ProgramResult {
        // NOTE: Close is handled by anchor on exit due to the `close` attribute
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn transfer_ownership(
        ctx: Context<TransferOwnership>,
        proposed_owner: Pubkey,
    ) -> ProgramResult {
        require!(proposed_owner != Pubkey::default(), InvalidInput);
        let state = &mut *ctx.accounts.state.load_mut()?;
        state.config.proposed_owner = proposed_owner;
        Ok(())
    }

    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> ProgramResult {
        let state = &mut *ctx.accounts.state.load_mut()?;
        require!(
            ctx.accounts.authority.key == &state.config.proposed_owner,
            Unauthorized
        );
        state.config.owner = std::mem::take(&mut state.config.proposed_owner);
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn begin_offchain_config(
        ctx: Context<SetConfig>,
        offchain_config_version: u64,
    ) -> ProgramResult {
        let state = &mut *ctx.accounts.state.load_mut()?;
        let config = &mut state.config;
        // disallow begin if we already started writing
        require!(config.pending_offchain_config.version == 0, InvalidInput);
        require!(config.pending_offchain_config.is_empty(), InvalidInput);
        require!(offchain_config_version != 0, InvalidInput);

        config.pending_offchain_config.version = offchain_config_version;
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn write_offchain_config(
        ctx: Context<SetConfig>,
        offchain_config: Vec<u8>,
    ) -> ProgramResult {
        let state = &mut *ctx.accounts.state.load_mut()?;
        let config = &mut state.config;
        require!(
            offchain_config.len() < config.pending_offchain_config.remaining_capacity(),
            InvalidInput
        );
        require!(config.pending_offchain_config.version != 0, InvalidInput);
        config.pending_offchain_config.extend(&offchain_config);
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn commit_offchain_config(ctx: Context<SetConfig>) -> ProgramResult {
        let state = &mut *ctx.accounts.state.load_mut()?;
        let config = &mut state.config;

        // Require that at least some data was written
        require!(config.pending_offchain_config.version > 0, InvalidInput);
        require!(!config.pending_offchain_config.is_empty(), InvalidInput);

        // move staging area onto actual config
        config.offchain_config = config.pending_offchain_config;
        // reset staging area
        config.pending_offchain_config.clear();
        config.pending_offchain_config.version = 0;

        // TODO: how does this interact with paying off oracles?

        // recalculate digest
        // TODO: share with setConfig?
        let slot = Clock::get()?.slot;
        // let previous_config_block_number = config.latest_config_block_number;
        config.latest_config_block_number = slot;
        config.config_count += 1;
        let config_digest = config.config_digest_from_data(&crate::id(), &state.oracles);
        config.latest_config_digest = config_digest;

        // Generate an event
        let signers = state
            .oracles
            .iter()
            .map(|oracle| oracle.signer.key)
            .collect();
        emit!(event::SetConfig {
            config_digest,
            f: config.f,
            signers
        });

        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn reset_pending_offchain_config(ctx: Context<SetConfig>) -> ProgramResult {
        let state = &mut *ctx.accounts.state.load_mut()?;
        let config = &mut state.config;

        // Require that at least some data was written
        require!(
            config.pending_offchain_config.version > 0
                || !config.pending_offchain_config.is_empty(),
            InvalidInput
        );

        // reset staging area
        config.pending_offchain_config.clear();
        config.pending_offchain_config.version = 0;
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn set_config(
        ctx: Context<SetConfig>,
        new_oracles: Vec<NewOracle>,
        f: u8,
    ) -> ProgramResult {
        let len = new_oracles.len();
        require!(f != 0, InvalidInput);
        require!(len <= MAX_ORACLES, TooManyOracles);
        let n = len as u8; // safe since it's less than MAX_ORACLES
        require!(3 * f < n, InvalidInput);

        let State {
            ref mut config,
            ref mut oracles,
            ref mut leftover_payments,
            ..
        } = &mut *ctx.accounts.state.load_mut()?;

        // Ensure no leftover payments
        let leftovers = leftover_payments
            .iter()
            .any(|leftover| leftover.amount != 0);
        require!(!leftovers, PaymentsRemaining);

        leftover_payments.clear();

        // Move current balances to leftover payments
        for oracle in oracles.iter() {
            leftover_payments.push(LeftoverPayment {
                payee: oracle.payee,
                amount: calculate_owed_payment(config, oracle)?,
            })
        }

        // Clear out old oracles
        oracles.clear();

        // Insert new oracles into the state
        for oracle in new_oracles.into_iter() {
            oracles.push(Oracle {
                signer: SigningKey { key: oracle.signer },
                transmitter: oracle.transmitter,
                from_round_id: config.latest_aggregator_round_id,
                ..Default::default()
            })
        }

        // Sort oracles so we can use binary search to locate the signer
        oracles.sort_unstable_by_key(|oracle| oracle.signer.key);
        // check for signer duplicates, we can compare successive keys since the array is now sorted
        let duplicate_signer = oracles
            .windows(2)
            .any(|pair| pair[0].signer.key == pair[1].signer.key);
        require!(!duplicate_signer, DuplicateSigner);

        let mut transmitters = BTreeSet::new();
        // check for transmitter duplicates
        for oracle in oracles.iter() {
            let inserted = transmitters.insert(oracle.transmitter);
            require!(inserted, DuplicateTransmitter);
        }

        // Update config
        config.f = f;

        let slot = Clock::get()?.slot;
        config.latest_config_block_number = slot;
        config.config_count += 1;
        let config_digest = config.config_digest_from_data(&crate::id(), oracles);
        config.latest_config_digest = config_digest;
        // Reset epoch and round
        config.epoch = 0;
        config.round = 0;

        // Generate an event
        let signers = oracles.iter().map(|oracle| oracle.signer.key).collect();
        emit!(event::SetConfig {
            config_digest,
            f,
            signers
        });

        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn set_requester_access_controller(ctx: Context<SetAccessController>) -> ProgramResult {
        let mut state = ctx.accounts.state.load_mut()?;
        state.config.requester_access_controller = ctx.accounts.access_controller.key();
        Ok(())
    }

    #[access_control(has_requester_access(&ctx.accounts.state, &ctx.accounts.access_controller, &ctx.accounts.authority))]
    pub fn request_new_round(ctx: Context<RequestNewRound>) -> ProgramResult {
        let config = ctx.accounts.state.load()?.config;

        emit!(event::RoundRequested {
            requester: ctx.accounts.authority.key(),
            config_digest: config.latest_config_digest,
            round: config.round,
            epoch: config.epoch,
        });
        // NOTE: can't really return round_id + 1, assume it on the client side
        Ok(())
    }

    #[inline(never)]
    pub fn transmit<'info>(
        program_id: &Pubkey,
        accounts: &[AccountInfo<'info>],
        data: &[u8],
    ) -> ProgramResult {
        // Based on https://github.com/project-serum/anchor/blob/d1edf2653f13f908a095081ec95d4b2c85a0b2d2/lang/syn/src/codegen/program/handlers.rs#L598-L625
        // Use a raw instruction to skip data decoding, but keep using Anchor contexts.

        // Deserialize accounts.
        let mut remaining_accounts: &[AccountInfo] = accounts;
        let mut accounts = Transmit::try_accounts(program_id, &mut remaining_accounts, data)?;

        // Construct a context
        let ctx = Context::new(program_id, &mut accounts, remaining_accounts);

        transmit_impl(ctx, data)?;

        // Exit routine
        accounts.exit(program_id)
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn set_billing_access_controller(ctx: Context<SetAccessController>) -> ProgramResult {
        let mut state = ctx.accounts.state.load_mut()?;
        state.config.billing_access_controller = ctx.accounts.access_controller.key();
        Ok(())
    }

    #[access_control(has_billing_access(&ctx.accounts.state, &ctx.accounts.access_controller, &ctx.accounts.authority))]
    pub fn set_billing(
        ctx: Context<SetBilling>,
        observation_payment_gjuels: u32,
        transmission_payment_gjuels: u32,
    ) -> ProgramResult {
        let mut state = ctx.accounts.state.load_mut()?;
        state.config.billing.observation_payment_gjuels = observation_payment_gjuels;
        state.config.billing.transmission_payment_gjuels = transmission_payment_gjuels;
        emit!(event::SetBilling {
            observation_payment_gjuels,
            transmission_payment_gjuels,
        });
        Ok(())
    }

    #[access_control(has_billing_access(&ctx.accounts.state, &ctx.accounts.access_controller, &ctx.accounts.authority))]
    pub fn withdraw_funds(ctx: Context<WithdrawFunds>, amount: u64) -> ProgramResult {
        let state = &ctx.accounts.state.load()?;

        let link_due =
            calculate_total_link_due(&state.config, &state.oracles, &state.leftover_payments)?;

        let balance = token::accessor::amount(&ctx.accounts.token_vault.to_account_info())?;
        let available = balance.saturating_sub(link_due);

        token::transfer(
            ctx.accounts.transfer_ctx().with_signer(&[&[
                b"vault".as_ref(),
                ctx.accounts.state.key().as_ref(),
                &[state.nonce],
            ]]),
            amount.min(available),
        )?;
        Ok(())
    }

    pub fn withdraw_payment(ctx: Context<WithdrawPayment>) -> ProgramResult {
        let State {
            ref config,
            ref mut oracles,
            ref nonce,
            ..
        } = &mut *ctx.accounts.state.load_mut()?;

        // Validate that the token account is actually for LINK
        require!(ctx.accounts.payee.mint == config.token_mint, InvalidInput);

        let key = ctx.accounts.payee.key();

        let oracle = oracles
            .iter_mut()
            .find(|oracle| oracle.payee == key)
            .ok_or(ErrorCode::Unauthorized)?;

        // Validate that the instruction was signed by the same authority as the token account
        require!(
            ctx.accounts.payee.owner == ctx.accounts.authority.key(),
            Unauthorized
        );

        // -- Pay oracle

        let amount = calculate_owed_payment(config, oracle)?;
        // Reset reward and gas reimbursement
        oracle.payment = 0;
        oracle.from_round_id = config.latest_aggregator_round_id;

        if amount == 0 {
            return Ok(());
        }

        // transfer funds
        token::transfer(
            ctx.accounts.transfer_ctx().with_signer(&[&[
                b"vault".as_ref(),
                ctx.accounts.state.key().as_ref(),
                &[*nonce],
            ]]),
            amount,
        )?; // consider using a custom transfer that calls invoke_signed_unchecked instead

        Ok(())
    }

    #[access_control(has_billing_access(&ctx.accounts.state, &ctx.accounts.access_controller, &ctx.accounts.authority))]
    pub fn pay_remaining<'info>(
        ctx: Context<'_, '_, '_, 'info, PayOracles<'info>>,
    ) -> ProgramResult {
        let State {
            ref mut leftover_payments,
            ref nonce,
            ..
        } = &mut *ctx.accounts.state.load_mut()?;

        require!(
            ctx.remaining_accounts.len() == leftover_payments.len(),
            InvalidInput
        );

        let payments: Vec<(u64, CpiContext<'_, '_, '_, 'info, token::Transfer<'info>>)> =
            leftover_payments
                .iter_mut()
                .zip(ctx.remaining_accounts)
                .map(|(leftover, account)| {
                    // Ensure specified accounts match the ones inside leftover_payments
                    require!(&leftover.payee == account.key, InvalidInput);

                    let cpi = CpiContext::new(
                        ctx.accounts.token_program.to_account_info(),
                        token::Transfer {
                            from: ctx.accounts.token_vault.to_account_info(),
                            to: account.to_account_info(),
                            authority: ctx.accounts.vault_authority.to_account_info(),
                        },
                    );

                    Ok((leftover.amount, cpi))
                })
                .collect::<Result<_>>()?;

        // Clear leftover payments
        leftover_payments.clear();

        let nonce = *nonce;

        for (amount, cpi) in payments {
            if amount == 0 {
                continue;
            }

            token::transfer(
                cpi.with_signer(&[&[
                    b"vault".as_ref(),
                    ctx.accounts.state.key().as_ref(),
                    &[nonce],
                ]]),
                amount,
            )?;
        }

        Ok(())
    }

    #[access_control(has_billing_access(&ctx.accounts.state, &ctx.accounts.access_controller, &ctx.accounts.authority))]
    pub fn pay_oracles<'info>(ctx: Context<'_, '_, '_, 'info, PayOracles<'info>>) -> ProgramResult {
        let State {
            ref config,
            ref mut oracles,
            ref nonce,
            ..
        } = &mut *ctx.accounts.state.load_mut()?;

        require!(ctx.remaining_accounts.len() == oracles.len(), InvalidInput);

        let payments: Vec<(u64, CpiContext<'_, '_, '_, 'info, token::Transfer<'info>>)> = oracles
            .iter_mut()
            .zip(ctx.remaining_accounts)
            .map(|(oracle, payee)| {
                // Ensure specified accounts match the ones inside leftover_payments
                require!(&oracle.payee == payee.key, InvalidInput);

                let amount = calculate_owed_payment(config, oracle)?;
                // Reset reward and gas reimbursement
                oracle.payment = 0;
                oracle.from_round_id = config.latest_aggregator_round_id;

                let cpi = CpiContext::new(
                    ctx.accounts.token_program.to_account_info(),
                    token::Transfer {
                        from: ctx.accounts.token_vault.to_account_info(),
                        to: payee.to_account_info(),
                        authority: ctx.accounts.vault_authority.to_account_info(),
                    },
                );

                Ok((amount, cpi))
            })
            .collect::<Result<_>>()?;

        let nonce = *nonce;

        for (amount, cpi) in payments {
            if amount == 0 {
                continue;
            }

            token::transfer(
                cpi.with_signer(&[&[
                    b"vault".as_ref(),
                    ctx.accounts.state.key().as_ref(),
                    &[nonce],
                ]]),
                amount,
            )?;
        }

        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn set_payees(ctx: Context<SetPayees>, payees: Vec<Pubkey>) -> ProgramResult {
        let State {
            ref config,
            ref mut oracles,
            ..
        } = &mut *ctx.accounts.state.load_mut()?;

        // Need to provide a payee for each oracle
        require!(oracles.len() == payees.len(), PayeeOracleMismatch);

        // Verify that the remaining accounts are valid token accounts.
        for account in ctx.remaining_accounts {
            let account = Account::<'_, token::TokenAccount>::try_from(account)?;
            require!(account.mint == config.token_mint, InvalidTokenAccount);
        }

        for (oracle, payee) in oracles.iter_mut().zip(payees.into_iter()) {
            // Can't set if already set before
            require!(oracle.payee == Pubkey::default(), PayeeAlreadySet);
            oracle.payee = payee;
        }

        Ok(())
    }

    pub fn transfer_payeeship(ctx: Context<TransferPayeeship>) -> ProgramResult {
        // Can't transfer to self
        require!(
            ctx.accounts.payee.key() != ctx.accounts.proposed_payee.key(),
            InvalidInput
        );

        let State {
            ref config,
            ref mut oracles,
            ..
        } = &mut *ctx.accounts.state.load_mut()?;

        // Validate that the token account is actually for LINK
        require!(
            ctx.accounts.proposed_payee.mint == config.token_mint,
            InvalidInput
        );

        let oracle = oracles
            .iter_mut()
            .find(|oracle| &oracle.transmitter == ctx.accounts.transmitter.key)
            .ok_or(ErrorCode::InvalidInput)?;

        // Validate that the instruction was signed by the same authority as the token account
        require!(oracle.payee == ctx.accounts.payee.key(), InvalidInput);
        require!(
            ctx.accounts.payee.owner == ctx.accounts.authority.key(),
            Unauthorized
        );

        oracle.proposed_payee = ctx.accounts.proposed_payee.key();
        Ok(())
    }

    pub fn accept_payeeship(ctx: Context<AcceptPayeeship>) -> ProgramResult {
        let State {
            ref mut oracles, ..
        } = &mut *ctx.accounts.state.load_mut()?;

        let oracle = oracles
            .iter_mut()
            .find(|oracle| &oracle.transmitter == ctx.accounts.transmitter.key)
            .ok_or(ErrorCode::InvalidInput)?;

        // Validate that the instruction was signed by the same authority as the token account
        require!(
            oracle.proposed_payee == ctx.accounts.proposed_payee.key(),
            InvalidInput
        );
        require!(
            ctx.accounts.proposed_payee.owner == ctx.accounts.authority.key(),
            Unauthorized
        );

        oracle.payee = std::mem::take(&mut oracle.proposed_payee);
        Ok(())
    }
}

#[inline(always)]
fn transmit_impl<'info>(ctx: Context<Transmit<'info>>, data: &[u8]) -> ProgramResult {
    let (nonce, data) = data.split_first().ok_or(ErrorCode::InvalidInput)?;

    use anchor_lang::solana_program::{hash, keccak, secp256k1_recover::*};

    // 32 byte digest, 32 bytes (27 byte padding, 4 byte epoch, 1 byte round), 32 byte extra hash entropy
    const CONTEXT_LEN: usize = 96;
    const RAW_REPORT_LEN: usize = CONTEXT_LEN + Report::LEN;

    require!(data.len() > RAW_REPORT_LEN, InvalidInput);

    let (raw_report, raw_signatures) = data.split_at(RAW_REPORT_LEN);
    let raw_report: [u8; RAW_REPORT_LEN] = raw_report.try_into().unwrap();

    // Parse the report context
    #[allow(clippy::ptr_offset_with_cast)] // complains about arrayref internals
    let (report_context, raw_report) = array_refs![&raw_report, CONTEXT_LEN, Report::LEN];
    let (config_digest, _padding, epoch, round, _extra_hash) =
        array_refs![report_context, 32, 27, 4, 1, 32];
    let epoch = u32::from_be_bytes(*epoch);
    let round = round[0];

    let mut state = ctx.accounts.state.load_mut()?;
    let config = &state.config;

    // Either newer epoch, or same epoch but higher round ID
    require!((config.epoch, config.round) < (epoch, round), StaleReport);

    // validate transmitter
    let oracle_idx = state
        .oracles
        .iter()
        .position(|oracle| &oracle.transmitter == ctx.accounts.transmitter.key)
        .ok_or(ErrorCode::UnauthorizedTransmitter)?;

    require!(
        config.latest_config_digest == *config_digest,
        DigestMismatch
    );

    // 64 byte signature + 1 byte recovery id
    const SIGNATURE_LEN: usize = SECP256K1_SIGNATURE_LENGTH + 1;
    // raw_signatures is exactly sized
    require!(raw_signatures.len() % SIGNATURE_LEN == 0, InvalidInput);
    let signature_count = raw_signatures.len() / SIGNATURE_LEN;
    require!(
        signature_count == usize::from(config.f) + 1,
        WrongNumberOfSignatures
    );
    let raw_signatures = raw_signatures
        .chunks(SIGNATURE_LEN)
        .map(|raw| raw.split_last());

    // Verify signatures attached to report
    let hash = hash::hashv(&[&[raw_report.len() as u8], raw_report, report_context]).to_bytes();

    // this fits MAX_ORACLES
    let mut uniques: u32 = 0;

    for data in raw_signatures {
        let (recovery_id, signature) = data.ok_or(ErrorCode::InvalidInput)?;

        let signer =
            secp256k1_recover(&hash, *recovery_id, signature).map_err(|err| match err {
                Secp256k1RecoverError::InvalidHash => ErrorCode::InvalidInput,
                Secp256k1RecoverError::InvalidRecoveryId => ErrorCode::InvalidInput,
                Secp256k1RecoverError::InvalidSignature => ErrorCode::Unauthorized,
            })?;

        // convert to a raw 20 byte Ethereum address
        let address = &keccak::hash(&signer.0).to_bytes()[12..];

        let index = state
            .oracles
            .binary_search_by_key(&address, |oracle| &oracle.signer.key)
            .map_err(|_| ErrorCode::UnauthorizedSigner)?;

        uniques |= 1 << index;
    }

    require!(
        uniques.count_ones() as usize == signature_count,
        DuplicateSigner
    );

    // -- report():

    let report = Report::unpack(raw_report)?;

    require!(config.f < report.observer_count, InvalidInput);

    require!(
        report.median >= state.config.min_answer && report.median <= state.config.max_answer,
        MedianOutOfRange
    );

    state.config.epoch = epoch;
    state.config.round = round;
    state.config.latest_aggregator_round_id = state
        .config
        .latest_aggregator_round_id
        .checked_add(1)
        .ok_or(ErrorCode::Overflow)?; // this should never occur, but let's check for it anyway
    state.config.latest_transmitter = ctx.accounts.transmitter.key();

    // calculate and pay reimbursement
    let reimbursement = calculate_reimbursement(report.juels_per_lamport, signature_count)?;
    let amount = reimbursement + u64::from(state.config.billing.transmission_payment_gjuels);
    state.oracles[oracle_idx].payment += amount;

    emit!(event::NewTransmission {
        round_id: state.config.latest_aggregator_round_id,
        config_digest: state.config.latest_config_digest,
        answer: report.median,
        transmitter: oracle_idx as u8, // has to fit in u8 because MAX_ORACLES < 255
        observations_timestamp: report.observations_timestamp,
        observer_count: report.observer_count,
        observers: report.observers,
        juels_per_lamport: report.juels_per_lamport,
        reimbursement,
    });

    let round = Transmission {
        answer: report.median,
        timestamp: report.observations_timestamp as u64,
    };

    // store and validate answer
    require!(ctx.accounts.store.owner == &store::ID, InvalidInput);
    require!(ctx.accounts.store.is_writable, InvalidInput);

    let cpi_ctx = CpiContext::new(
        ctx.accounts.store_program.to_account_info(),
        store::cpi::accounts::Submit {
            store: ctx.accounts.store.to_account_info(),
            feed: ctx.accounts.transmissions.to_account_info(),
            authority: ctx.accounts.store_authority.to_account_info(),
        },
    );

    drop(state);

    let seeds = &[
        b"store",
        ctx.accounts.state.to_account_info().key.as_ref(),
        &[*nonce],
    ];

    store::cpi::submit(cpi_ctx.with_signer(&[&seeds[..]]), round)?;

    // TODO: use _unchecked to save some instructions

    Ok(())
}

struct Report {
    pub median: i128,
    pub observer_count: u8,
    pub observers: [u8; MAX_ORACLES], // observer index
    pub observations_timestamp: u32,
    pub juels_per_lamport: u64,
}

impl Report {
    // (uint32, u8, bytes32, int128, uint128)
    pub const LEN: usize =
        size_of::<u32>() + size_of::<u8>() + 32 + size_of::<i128>() + size_of::<u64>();

    pub fn unpack(raw_report: &[u8]) -> Result<Self> {
        require!(raw_report.len() == Self::LEN, InvalidInput);

        let data = array_ref![raw_report, 0, Report::LEN];
        let (observations_timestamp, observer_count, observers, median, juels_per_lamport) =
            array_refs![data, 4, 1, 32, 16, 8];

        let observations_timestamp = u32::from_be_bytes(*observations_timestamp);
        let observer_count = observer_count[0];
        let observers = observers[..MAX_ORACLES].try_into().unwrap();
        let median = i128::from_be_bytes(*median);
        let juels_per_lamport = u64::from_be_bytes(*juels_per_lamport);

        Ok(Self {
            median,
            observer_count,
            observers,
            observations_timestamp,
            juels_per_lamport,
        })
    }
}

fn calculate_reimbursement(juels_per_lamport: u64, _signature_count: usize) -> Result<u64> {
    const SIGNERS: u64 = 1;
    let fees = Fees::get()?;
    let lamports_per_signature = fees.fee_calculator.lamports_per_signature;
    let lamports = lamports_per_signature * SIGNERS;
    let juels = lamports * juels_per_lamport;
    Ok(juels)
}

fn calculate_owed_payment(config: &Config, oracle: &Oracle) -> Result<u64> {
    let rounds = config
        .latest_aggregator_round_id
        .checked_sub(oracle.from_round_id)
        .ok_or(ErrorCode::Overflow)?;

    let amount = u64::from(config.billing.observation_payment_gjuels)
        .checked_mul(rounds.into())
        .ok_or(ErrorCode::Overflow)?
        .checked_add(oracle.payment)
        .ok_or(ErrorCode::Overflow)?;

    Ok(amount)
}

fn calculate_total_link_due(
    config: &Config,
    oracles: &[Oracle],
    leftover_payments: &[LeftoverPayment],
) -> Result<u64> {
    let (rounds, reimbursements) = oracles
        .iter()
        .try_fold((0, 0), |(rounds, reimbursements): (u32, u64), oracle| {
            let count = config
                .latest_aggregator_round_id
                .checked_sub(oracle.from_round_id)?;

            Some((
                rounds.checked_add(count)?,
                reimbursements.checked_add(oracle.payment)?,
            ))
        })
        .ok_or(ErrorCode::Overflow)?;

    let leftover_payments = leftover_payments
        .iter()
        .map(|leftover| leftover.amount)
        .sum();

    let amount = u64::from(config.billing.observation_payment_gjuels)
        .checked_mul(u64::from(rounds))
        .ok_or(ErrorCode::Overflow)?
        .checked_add(reimbursements)
        .ok_or(ErrorCode::Overflow)?
        .checked_add(leftover_payments)
        .ok_or(ErrorCode::Overflow)?;

    Ok(amount)
}

// -- Access control modifiers

// Only owner access
fn owner(state_loader: &AccountLoader<State>, signer: &AccountInfo) -> ProgramResult {
    let config = state_loader.load()?.config;
    require!(signer.key.eq(&config.owner), Unauthorized);
    Ok(())
}

fn has_billing_access(
    state: &AccountLoader<State>,
    controller: &AccountLoader<AccessController>,
    authority: &AccountInfo,
) -> ProgramResult {
    let config = state.load()?.config;

    require!(
        config.billing_access_controller == controller.key(),
        InvalidInput
    );

    let is_owner = config.owner == authority.key();

    let has_access = is_owner
        || access_controller::has_access(controller, authority.key)
            .map_err(|_| ErrorCode::InvalidInput)?;

    require!(has_access, Unauthorized);
    Ok(())
}

fn has_requester_access(
    state: &AccountLoader<State>,
    controller: &AccountLoader<AccessController>,
    authority: &AccountInfo,
) -> ProgramResult {
    let config = state.load()?.config;

    require!(
        config.requester_access_controller == controller.key(),
        InvalidInput
    );

    let is_owner = config.owner == authority.key();

    let has_access = is_owner
        || access_controller::has_access(controller, authority.key)
            .map_err(|_| ErrorCode::InvalidInput)?;

    require!(has_access, Unauthorized);
    Ok(())
}

#[error]
pub enum ErrorCode {
    #[msg("Unauthorized")]
    Unauthorized = 0,

    #[msg("Invalid input")]
    InvalidInput = 1,

    #[msg("Too many oracles")]
    TooManyOracles = 2,

    #[msg("Stale report")]
    StaleReport = 3,

    #[msg("Digest mismatch")]
    DigestMismatch = 4,

    #[msg("Wrong number of signatures")]
    WrongNumberOfSignatures = 5,

    #[msg("Overflow")]
    Overflow = 6,

    #[msg("Median out of range")]
    MedianOutOfRange = 7,

    #[msg("Duplicate signer")]
    DuplicateSigner,

    #[msg("Duplicate transmitter")]
    DuplicateTransmitter,

    #[msg("Payee already set")]
    PayeeAlreadySet,

    #[msg("Leftover payments remaining, please call payRemaining first")]
    PaymentsRemaining,

    #[msg("Payee and Oracle lenght mismatch")]
    PayeeOracleMismatch,

    #[msg("Invalid Token Account")]
    InvalidTokenAccount,

    #[msg("Oracle signer key not found")]
    UnauthorizedSigner,

    #[msg("Oracle transmitter key not found")]
    UnauthorizedTransmitter,
}

pub mod query {
    use super::*;

    #[account]
    pub struct LatestConfig {
        pub config_count: u32,
        pub config_digest: [u8; 32],
        pub block_number: u64,
    }

    #[account]
    pub struct LinkAvailableForPayment {
        pub available_balance: u64,
    }

    #[account]
    pub struct OracleObservationCount {
        pub count: u32,
    }

    pub fn latest_config_details(account: &AccountInfo) -> Result<LatestConfig> {
        let loader = AccountLoader::<State>::try_from(account)?;
        let state = loader.load()?;
        let config = state.config;
        Ok(LatestConfig {
            config_count: config.config_count,
            config_digest: config.latest_config_digest,
            block_number: config.latest_config_block_number,
        })
    }

    // Returns the total link available for payment from a specific token vault
    //
    // This allows oracles to check that sufficient LINK balance is available
    pub fn link_available_for_payment(
        account: &AccountInfo,
        token_vault: &AccountInfo,
    ) -> Result<LinkAvailableForPayment> {
        let loader = AccountLoader::<State>::try_from(account)?;
        let state = loader.load()?;

        let balance = token::accessor::amount(token_vault)?;

        let link_due =
            calculate_total_link_due(&state.config, &state.oracles, &state.leftover_payments)?;

        let available_balance = balance.saturating_sub(link_due);

        Ok(LinkAvailableForPayment { available_balance })
    }

    // Returns the total number of observation counts for a specific transmitter
    //
    // This is the number of observations oracle is due to be reimbursed for.
    pub fn oracle_observation_count(
        account: &AccountInfo,
        transmitter: &AccountInfo,
    ) -> Result<OracleObservationCount> {
        let loader = AccountLoader::<State>::try_from(account)?;
        let state = loader.load()?;

        let oracle = &state
            .oracles
            .iter()
            .find(|oracle| &oracle.transmitter == transmitter.key)
            .ok_or(ErrorCode::InvalidInput)?;

        let count = state
            .config
            .latest_aggregator_round_id
            .saturating_sub(oracle.from_round_id);

        Ok(OracleObservationCount { count })
    }
}

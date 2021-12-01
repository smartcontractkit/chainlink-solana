use anchor_lang::prelude::*;
use anchor_lang::solana_program::sysvar::fees::Fees;
use anchor_spl::token;

use arrayref::{array_ref, array_refs};

declare_id!("3PWYU38zBeeh1fKDYx9diGPK6TVyFrtUpFT6UHiAEoJ5");

mod context;
pub mod event;
mod state;

use crate::context::*;
use crate::state::{
    config_digest_from_data, Config, LeftoverPayment, Oracle, SigningKey, State, Transmission,
    MAX_ORACLES,
};

use std::convert::{TryFrom, TryInto};

use deviation_flagging_validator as validator;

// TODO: use a custom serialize / deserialize to pack more tightly
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
        decimals: u8,
        description: String,
    ) -> ProgramResult {
        let mut state = ctx.accounts.state.load_init()?;
        state.config.version = 1;
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
        config.decimals = decimals;

        let description = description.as_bytes();
        require!(description.len() < 32, InvalidInput);
        config.description[..description.len()].copy_from_slice(description);

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
    pub fn set_config(
        ctx: Context<SetConfig>,
        new_oracles: Vec<NewOracle>,
        f: u8,
        onchain_config: Vec<u8>,
        offchain_config_version: u64,
        offchain_config: Vec<u8>,
    ) -> ProgramResult {
        let slot = Clock::get()?.slot;

        let len = new_oracles.len();
        require!(f != 0, InvalidInput);
        require!(len <= MAX_ORACLES, TooManyOracles);
        let n = len as u8; // safe since it's less than MAX_ORACLES
        require!(3 * f < n, InvalidInput); // TODO custom error

        let State {
            ref mut config,
            ref mut oracles,
            ref mut leftover_payments,
            ref mut leftover_payments_len,
            ..
        } = &mut *ctx.accounts.state.load_mut()?;

        // Ensure no leftover payments
        let leftovers = leftover_payments
            .iter()
            .any(|leftover| leftover.amount != 0);
        require!(!leftovers, PaymentsRemaining);

        // Move current balances to leftover payments
        for (i, oracle) in oracles[..len].iter().enumerate() {
            leftover_payments[i] = LeftoverPayment {
                payee: oracle.payee,
                amount: calculate_owed_payment(config, oracle)?,
            };
        }
        *leftover_payments_len = config.n;

        // Clear out old oracles
        *oracles = [Oracle::default(); MAX_ORACLES];

        // Insert new oracles into the state
        for (i, oracle) in new_oracles.into_iter().enumerate() {
            oracles[i].signer = SigningKey { key: oracle.signer };
            oracles[i].transmitter = oracle.transmitter;
        }
        config.n = n;

        // Sort oracles so we can use binary search to locate the signer
        oracles[..len].sort_unstable_by_key(|oracle| oracle.signer.key);
        // check for signer duplicates, we can compare successive keys since the array is now sorted
        let duplicate_signer = oracles[..len]
            .windows(2)
            .any(|pair| pair[0].signer.key == pair[1].signer.key);
        require!(!duplicate_signer, ErrorCode::DuplicateSigner);

        // TODO: check for transmitter duplicates

        // Update config
        config.f = f;
        let previous_config_block_number = config.latest_config_block_number;
        config.latest_config_block_number = slot;
        config.config_count += 1;
        config.latest_config_digest = config_digest_from_data(
            &crate::id(),
            config.config_count,
            &oracles[..len],
            f,
            &onchain_config,
            offchain_config_version,
            &offchain_config,
        );
        // Reset epoch and round
        config.epoch = 0;
        config.round = 0;

        // TODO: emit full events
        emit!(event::SetConfig {
            previous_config_block_number,
            latest_config_digest: config.latest_config_digest,
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

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn set_validator_config(
        ctx: Context<SetValidatorConfig>,
        flagging_threshold: u32,
    ) -> ProgramResult {
        let mut state = ctx.accounts.state.load_mut()?;
        state.config.validator = ctx.accounts.validator.key();
        state.config.flagging_threshold = flagging_threshold;
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
    pub fn set_billing(ctx: Context<SetBilling>, observation_payment: u32) -> ProgramResult {
        let mut state = ctx.accounts.state.load_mut()?;
        state.config.billing.observation_payment = observation_payment;
        Ok(())
    }

    #[access_control(has_billing_access(&ctx.accounts.state, &ctx.accounts.access_controller, &ctx.accounts.authority))]
    pub fn withdraw_funds(ctx: Context<WithdrawFunds>, amount: u64) -> ProgramResult {
        let state = &ctx.accounts.state.load()?;

        let link_due = calculate_total_link_due(
            &state.config,
            &state.oracles[..state.config.n as usize],
            &state.leftover_payments[..state.leftover_payments_len as usize],
        )?;

        let balance = token::accessor::amount(&ctx.accounts.token_vault.to_account_info())?;
        let available = balance.saturating_sub(link_due);

        token::transfer(
            ctx.accounts.into_transfer().with_signer(&[&[
                b"vault".as_ref(),
                ctx.accounts.state.key().as_ref(),
                &[state.nonce],
            ]]),
            amount.min(available),
        )?;
        // anchor_lang::solana_program::log::sol_log_compute_units(); // 184994, 185707 if unchecked, about 800 savings
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

        let oracle = oracles[..config.n as usize]
            .iter_mut()
            .find(|oracle| oracle.payee == key)
            .ok_or(ErrorCode::Unauthorized)?;

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
            ctx.accounts.into_transfer().with_signer(&[&[
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
            ref mut leftover_payments_len,
            ref nonce,
            ..
        } = &mut *ctx.accounts.state.load_mut()?;

        require!(
            ctx.remaining_accounts.len() == *leftover_payments_len as usize,
            InvalidInput
        );

        let payments: Vec<(u64, CpiContext<'_, '_, '_, 'info, token::Transfer<'info>>)> =
            leftover_payments[..*leftover_payments_len as usize]
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
        *leftover_payments_len = 0;
        *leftover_payments = [LeftoverPayment::default(); MAX_ORACLES];

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

        require!(
            ctx.remaining_accounts.len() == config.n as usize,
            InvalidInput
        );

        let payments: Vec<(u64, CpiContext<'_, '_, '_, 'info, token::Transfer<'info>>)> = oracles
            [..config.n as usize]
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
        require!(config.n as usize == payees.len(), PayeeOracleMismatch);

        // Verify that the remaining accounts are valid token accounts.
        for account in ctx.remaining_accounts {
            let account = Account::<'_, token::TokenAccount>::try_from(account)?;
            require!(account.mint == config.token_mint, InvalidTokenAccount);
        }

        for (oracle, payee) in oracles[..config.n as usize]
            .iter_mut()
            .zip(payees.into_iter())
        {
            // Can't set if already set before
            require!(oracle.payee == Pubkey::default(), PayeeAlreadySet);
            oracle.payee = payee;
        }

        Ok(())
    }

    // TODO: proposer could pay for creating associated token account?
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

        let oracle = oracles[..config.n as usize]
            .iter_mut()
            .find(|oracle| &oracle.transmitter == ctx.accounts.transmitter.key)
            .ok_or(ErrorCode::InvalidInput)?;

        // Validate that the instruction was signed by the same authority as the token account
        require!(oracle.payee == ctx.accounts.payee.key(), InvalidInput);
        let token_authority = token::accessor::authority(&ctx.accounts.payee.to_account_info())?;
        require!(
            token_authority == ctx.accounts.authority.key(),
            Unauthorized
        );

        oracle.proposed_payee = ctx.accounts.proposed_payee.key();
        Ok(())
    }

    pub fn accept_payeeship(ctx: Context<AcceptPayeeship>) -> ProgramResult {
        let State {
            ref config,
            ref mut oracles,
            ..
        } = &mut *ctx.accounts.state.load_mut()?;

        let oracle = oracles[..config.n as usize]
            .iter_mut()
            .find(|oracle| &oracle.transmitter == ctx.accounts.transmitter.key)
            .ok_or(ErrorCode::InvalidInput)?;

        // Validate that the instruction was signed by the same authority as the token account
        require!(
            oracle.proposed_payee == ctx.accounts.proposed_payee.key(),
            InvalidInput
        );
        let token_authority =
            token::accessor::authority(&ctx.accounts.proposed_payee.to_account_info())?;
        require!(
            token_authority == ctx.accounts.authority.key(),
            Unauthorized
        );

        oracle.payee = std::mem::take(&mut oracle.proposed_payee);
        Ok(())
    }
}

#[inline(always)]
fn transmit_impl<'info>(ctx: Context<Transmit<'info>>, data: &[u8]) -> ProgramResult {
    let (nonce, data) = data.split_first().ok_or(ErrorCode::InvalidInput)?;

    anchor_lang::solana_program::log::sol_log_compute_units();
    use anchor_lang::solana_program::{hash, keccak, secp256k1_recover::*};

    const CONTEXT_LEN: usize = 96;
    const RAW_REPORT_LEN: usize = CONTEXT_LEN + Report::LEN;

    require!(data.len() > RAW_REPORT_LEN, InvalidInput);

    let (raw_report, raw_signatures) = data.split_at(RAW_REPORT_LEN);
    let raw_report: [u8; RAW_REPORT_LEN] = raw_report.try_into().unwrap();

    // Parse the report context
    let (report_context, raw_report) = array_refs![&raw_report, CONTEXT_LEN, Report::LEN];
    let (config_digest, _padding, epoch, round, _extra_hash) =
        array_refs![report_context, 32, 27, 4, 1, 32];
    let epoch = u32::from_be_bytes(*epoch);
    let round = round[0];

    let mut state = ctx.accounts.state.load_mut()?;
    let config = &state.config;

    // Either newer epoch, or same epoch but higher round ID
    require!(
        config.epoch < epoch || (config.epoch == epoch && config.round < round),
        StaleReport
    );

    // validate transmitter
    let oracle_idx = state.oracles[..state.config.n as usize]
        .iter()
        .position(|oracle| &oracle.transmitter == ctx.accounts.transmitter.key)
        .ok_or(ErrorCode::Unauthorized)?;

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
        signature_count == 3 * usize::from(config.f) + 1,
        WrongNumberOfSignatures
    );
    let raw_signatures = raw_signatures
        .chunks(SIGNATURE_LEN)
        .map(|raw| raw.split_last());

    // Verify signatures attached to report
    let hash = hash::hashv(&[raw_report, report_context]).to_bytes();

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

        let index = state.oracles[..state.config.n as usize]
            .binary_search_by_key(&address, |oracle| &oracle.signer.key)
            .map_err(|_| ErrorCode::Unauthorized)?;

        uniques |= 1 << index;
    }

    require!(
        uniques.count_ones() as usize == signature_count,
        DuplicateSigner
    );

    // -- report():

    let report = Report::unpack(raw_report)?;

    // TODO: maybe still validate observers?
    // require!(usize::from(config.f) < len, InvalidInput);

    require!(
        report.median >= state.config.min_answer && report.median <= state.config.max_answer,
        MedianOutOfRange
    );

    state.config.epoch = epoch;
    state.config.round = round;
    state.config.latest_aggregator_round_id += 1;

    let mut transmissions = ctx.accounts.transmissions.load_mut()?;
    transmissions.store_round(Transmission {
        answer: report.median,
        timestamp: report.observations_timestamp,
    });

    // calculate and pay reimbursement
    let reimbursement = calculate_reimbursement(report.juels_per_lamport, signature_count)?;
    state.oracles[oracle_idx].payment += reimbursement;

    // validate answer
    if state.config.validator != Pubkey::default() {
        let round_id = state.config.latest_aggregator_round_id;
        let previous_round_id = round_id - 1;
        let previous_answer = transmissions
            .fetch_round(previous_round_id)
            .map(|transmission| transmission.answer)
            .unwrap_or(0);
        let flagging_threshold = state.config.flagging_threshold;

        let cpi_ctx = CpiContext::new(
            ctx.accounts.validator_program.clone(),
            validator::cpi::accounts::Validate {
                state: ctx.accounts.validator.to_account_info(),
                authority: ctx.accounts.validator_authority.to_account_info(),
                access_controller: ctx.accounts.validator_access_controller.to_account_info(),
                address: ctx.accounts.state.to_account_info(),
            },
        );

        drop(state);

        let seeds = &[
            b"validator",
            ctx.accounts.state.to_account_info().key.as_ref(),
            &[*nonce],
        ];

        let _ = validator::cpi::validate(
            cpi_ctx.with_signer(&[&seeds[..]]),
            flagging_threshold,
            previous_round_id,
            previous_answer,
            round_id,
            report.median,
        ); // ignore result, validate should not stop transmit()

        // TODO: use _unchecked to save some instructions
    }

    Ok(())
}

/// (sender_id, signature, recovery_id)
// type Signature<'a> = (u8, &'a [u8], u8);
//
// fn decode_signatures(raw_signatures: &[u8]) -> Result<impl Iterator<Item = Option<Signature>>> {
//     use anchor_lang::solana_program::secp256k1_recover::*;
//     // 1 byte sender ID + 64 byte signature + 1 byte recovery id
//     const SIGNATURE_LEN: usize = 1 + SECP256K1_SIGNATURE_LENGTH + 1;
//
//     // raw_signatures is exactly sized
//     require!(raw_signatures.len() % SIGNATURE_LEN == 0, InvalidInput);
//
//     let iterator = raw_signatures.chunks(SIGNATURE_LEN).map(|raw| match raw {
//         [sender_id, signature @ .., recovery_id] => Some((*sender_id, signature, *recovery_id)),
//         _ => None,
//     });
//
//     Ok(iterator)
// }

struct Report {
    pub median: i128,
    pub observers: [u8; MAX_ORACLES], // observer index
    pub observations_timestamp: u32,
    pub juels_per_lamport: u128,
}

impl Report {
    pub const LEN: usize = 4 + 32 + 16 + 16;

    pub fn unpack(raw_report: &[u8]) -> Result<Self> {
        // (uint32, bytes32, int128, uint128)
        require!(raw_report.len() == Self::LEN, InvalidInput);

        let data = array_ref![raw_report, 0, Report::LEN];
        let (observations_timestamp, observers, median, juels_per_lamport) =
            array_refs![data, 4, 32, 16, 16];

        let observations_timestamp = u32::from_be_bytes(*observations_timestamp);
        let observers = observers[..MAX_ORACLES].try_into().unwrap();
        let median = i128::from_be_bytes(*median);
        let juels_per_lamport = u128::from_be_bytes(*juels_per_lamport);

        Ok(Self {
            median,
            observers,
            observations_timestamp,
            juels_per_lamport,
        })
    }
}

fn calculate_reimbursement(juels_per_lamport: u128, signature_count: usize) -> Result<u64> {
    const SIGNERS: u64 = 1; // TODO: probably needs to include signing the validator call
    let fees = Fees::get()?;
    let lamports_per_signature = fees.fee_calculator.lamports_per_signature;
    // num of signatures + const based on how many signers we have
    let signature_count = signature_count as u64 + SIGNERS;
    let lamports = lamports_per_signature * signature_count;
    // TODO: use u64 instead of u128 for juels_per_lamport
    let juels = lamports * u64::try_from(juels_per_lamport).map_err(|_| ErrorCode::Overflow)?;
    Ok(juels)
}

fn calculate_owed_payment(config: &Config, oracle: &Oracle) -> Result<u64> {
    let rounds = config.latest_aggregator_round_id - oracle.from_round_id;
    let amount = u64::from(config.billing.observation_payment)
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
    let (rounds, reimbursements) =
        oracles
            .iter()
            .fold((0, 0), |(rounds, reimbursements), oracle| {
                (
                    rounds + (config.latest_aggregator_round_id - oracle.from_round_id),
                    reimbursements + oracle.payment,
                )
            });

    let leftover_payments = leftover_payments
        .iter()
        .map(|leftover| leftover.amount)
        .sum();

    let amount = u64::from(config.billing.observation_payment)
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
fn owner(state_loader: &Loader<State>, signer: &AccountInfo) -> ProgramResult {
    let config = state_loader.load()?.config;
    require!(signer.key.eq(&config.owner), Unauthorized);
    Ok(())
}

fn has_billing_access(
    state: &Loader<State>,
    controller: &AccountInfo,
    authority: &AccountInfo,
) -> ProgramResult {
    let config = state.load()?.config;

    require!(
        config.billing_access_controller == controller.key(),
        InvalidInput
    );

    let is_owner = config.owner == authority.key();

    let has_access = is_owner
        || access_controller::has_access(
            &Loader::try_from(&access_controller::ID, controller)?,
            authority.key,
        )
        .map_err(|_| ErrorCode::InvalidInput)?;

    require!(has_access, Unauthorized);
    Ok(())
}

fn has_requester_access(
    state: &Loader<State>,
    controller: &AccountInfo,
    authority: &AccountInfo,
) -> ProgramResult {
    let config = state.load()?.config;

    require!(
        config.requester_access_controller == controller.key(),
        InvalidInput
    );

    let is_owner = config.owner == authority.key();

    let has_access = is_owner
        || access_controller::has_access(
            &Loader::try_from(&access_controller::ID, controller)?,
            authority.key,
        )
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

    #[msg("Payee already set")]
    PayeeAlreadySet,

    #[msg("Leftover payments remaining, please call payRemaining first")]
    PaymentsRemaining,

    #[msg("Payee and Oracle lenght mismatch")]
    PayeeOracleMismatch,

    #[msg("Invalid Token Account")]
    InvalidTokenAccount,
}

pub mod query {
    use super::*;
    use std::cell::Ref;

    pub struct ConfigDetails {
        pub config_count: u32,
        pub config_digest: [u8; 32],
        pub block_number: u64,
    }

    pub fn latest_config_details(account: &AccountInfo) -> Result<ConfigDetails> {
        let loader = Loader::try_from_unchecked(&crate::id(), account)?;
        let state: Ref<State> = loader.load()?;
        let config = state.config;
        Ok(ConfigDetails {
            config_count: config.config_count,
            config_digest: config.latest_config_digest,
            block_number: config.latest_config_block_number,
        })
    }
    // TODO:
    // link_available_for_payment
    // oracle_observation_count
}

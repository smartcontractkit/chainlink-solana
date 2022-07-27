use anchor_lang::prelude::*;
use anchor_spl::token;

use arrayref::{array_ref, array_refs};
use state::{Billing, Proposal, ProposedOracle};

declare_id!("cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ");

mod context;
pub mod event;
mod state;

use crate::context::*;
use crate::state::{Config, Oracle, SigningKey, State, DIGEST_SIZE, MAX_ORACLES};

use std::collections::BTreeSet;
use std::convert::TryInto;
use std::mem::size_of;

use access_controller::AccessController;
use store::NewTransmission;

#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct NewOracle {
    pub signer: [u8; 20],
    pub transmitter: Pubkey,
}

#[program]
pub mod ocr2 {
    use super::*;
    pub fn initialize(ctx: Context<Initialize>, min_answer: i128, max_answer: i128) -> Result<()> {
        let mut state = ctx.accounts.state.load_init()?;
        state.version = 1;

        state.vault_nonce = *ctx.bumps.get("vault_authority").unwrap();
        state.feed = ctx.accounts.feed.key();

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
    pub fn close<'info>(ctx: Context<'_, '_, '_, 'info, Close<'info>>) -> Result<()> {
        // Pay out any remaining balances
        pay_oracles_impl(
            ctx.accounts.state.clone(),
            ctx.accounts.token_program.to_account_info(),
            ctx.accounts.token_vault.to_account_info(),
            ctx.accounts.vault_authority.clone(),
            ctx.remaining_accounts,
        )?;

        // NOTE: Close is handled by anchor on exit due to the `close` attribute
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn transfer_ownership(
        ctx: Context<TransferOwnership>,
        proposed_owner: Pubkey,
    ) -> Result<()> {
        require!(proposed_owner != Pubkey::default(), InvalidInput);
        let mut state = ctx.accounts.state.load_mut()?;
        state.config.proposed_owner = proposed_owner;
        Ok(())
    }

    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        let mut state = ctx.accounts.state.load_mut()?;
        require!(
            ctx.accounts.authority.key == &state.config.proposed_owner,
            Unauthorized
        );
        state.config.owner = std::mem::take(&mut state.config.proposed_owner);
        Ok(())
    }

    pub fn create_proposal(
        ctx: Context<CreateProposal>,
        offchain_config_version: u64,
    ) -> Result<()> {
        let mut proposal = ctx.accounts.proposal.load_init()?;

        proposal.version = 1;
        proposal.owner = ctx.accounts.authority.key();

        require!(offchain_config_version != 0, InvalidInput);
        proposal.offchain_config.version = offchain_config_version;
        Ok(())
    }

    #[access_control(proposal_owner(&ctx.accounts.proposal, &ctx.accounts.authority))]
    pub fn write_offchain_config(
        ctx: Context<ProposeConfig>,
        offchain_config: Vec<u8>,
    ) -> Result<()> {
        let mut proposal = ctx.accounts.proposal.load_mut()?;
        require!(proposal.state != Proposal::FINALIZED, InvalidInput);

        require!(
            offchain_config.len() < proposal.offchain_config.remaining_capacity(),
            InvalidInput
        );
        proposal.offchain_config.extend(&offchain_config);
        Ok(())
    }

    #[access_control(proposal_owner(&ctx.accounts.proposal, &ctx.accounts.authority))]
    pub fn finalize_proposal(ctx: Context<ProposeConfig>) -> Result<()> {
        let mut proposal = ctx.accounts.proposal.load_mut()?;
        require!(proposal.state != Proposal::FINALIZED, InvalidInput);

        // Require that at least some data was written via setOffchainConfig
        require!(proposal.offchain_config.version > 0, InvalidInput);
        require!(!proposal.offchain_config.is_empty(), InvalidInput);
        // setConfig must have been called
        require!(!proposal.oracles.is_empty(), InvalidInput);
        // setPayees must have been called
        let valid_payees = proposal
            .oracles
            .iter()
            .all(|oracle| oracle.payee != Pubkey::default());
        require!(valid_payees, InvalidInput);

        proposal.state = Proposal::FINALIZED;

        Ok(())
    }

    #[access_control(proposal_owner(&ctx.accounts.proposal, &ctx.accounts.authority))]
    pub fn close_proposal(ctx: Context<CloseProposal>) -> Result<()> {
        // NOTE: Close is handled by anchor on exit due to the `close` attribute
        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn accept_proposal<'info>(
        ctx: Context<'_, '_, '_, 'info, AcceptProposal<'info>>,
        digest: Vec<u8>,
    ) -> Result<()> {
        require!(digest.len() == DIGEST_SIZE, InvalidInput);

        // NOTE: if multisig supported multi instruction transactions, this could be [pay_oracles, accept_proposal]
        pay_oracles_impl(
            ctx.accounts.state.clone(),
            ctx.accounts.token_program.to_account_info(),
            ctx.accounts.token_vault.to_account_info(),
            ctx.accounts.vault_authority.clone(),
            ctx.remaining_accounts,
        )?;

        let mut state = ctx.accounts.state.load_mut()?;
        let proposal = ctx.accounts.proposal.load()?;

        // Proposal has to be finalized
        require!(proposal.state == Proposal::FINALIZED, InvalidInput);
        // Digest has to match
        require!(proposal.digest().as_ref() == digest, DigestMismatch);

        // The proposal payees have to use the same mint as the aggregator
        require!(
            proposal.token_mint == state.config.token_mint,
            InvalidTokenAccount
        );

        state.oracles.clear();

        // Move staging area onto actual config
        state.offchain_config = proposal.offchain_config;
        state.config.f = proposal.f;

        // Insert new oracles into the state
        let from_round_id = state.config.latest_aggregator_round_id;
        for oracle in proposal.oracles.iter() {
            state.oracles.push(Oracle {
                signer: oracle.signer,
                transmitter: oracle.transmitter,
                payee: oracle.payee,
                from_round_id,
                ..Default::default()
            })
        }
        // NOTE: proposal already sorts the oracles by signer key

        // Recalculate digest
        let slot = Clock::get()?.slot;
        // let previous_config_block_number = config.latest_config_block_number;
        state.config.latest_config_block_number = slot;
        state.config.config_count += 1;
        let config_digest = state.config.config_digest_from_data(
            &crate::id(),
            &ctx.accounts.state.key(),
            &state.offchain_config,
            &state.oracles,
        );
        state.config.latest_config_digest = config_digest;

        // NOTE: proposal is closed afterwards and the rent deposit is reclaimed

        // Generate an event
        let signers = state
            .oracles
            .iter()
            .map(|oracle| oracle.signer.key)
            .collect();
        emit!(event::SetConfig {
            config_digest,
            f: state.config.f,
            signers
        });

        Ok(())
    }

    #[access_control(proposal_owner(&ctx.accounts.proposal, &ctx.accounts.authority))]
    pub fn propose_config(
        ctx: Context<ProposeConfig>,
        new_oracles: Vec<NewOracle>,
        f: u8,
    ) -> Result<()> {
        let len = new_oracles.len();
        require!(f != 0, InvalidInput);
        require!(len <= MAX_ORACLES, TooManyOracles);
        require!(3 * usize::from(f) < len, InvalidInput);

        let mut proposal = ctx.accounts.proposal.load_mut()?;
        require!(proposal.state != Proposal::FINALIZED, InvalidInput);
        // begin_proposal must be called first
        require!(proposal.offchain_config.version != 0, InvalidInput);

        // Clear out old oracles
        proposal.oracles.clear();

        // Insert new oracles into the state
        for oracle in new_oracles.into_iter() {
            proposal.oracles.push(ProposedOracle {
                signer: SigningKey { key: oracle.signer },
                transmitter: oracle.transmitter,
                payee: Pubkey::default(),
                _padding: 0,
            })
        }

        // Sort oracles so we can use binary search to locate the signer
        proposal
            .oracles
            .sort_unstable_by_key(|oracle| oracle.signer.key);
        // check for signer duplicates, we can compare successive keys since the array is now sorted
        let duplicate_signer = proposal
            .oracles
            .windows(2)
            .any(|pair| pair[0].signer.key == pair[1].signer.key);
        require!(!duplicate_signer, DuplicateSigner);

        let mut transmitters = BTreeSet::new();
        // check for transmitter duplicates
        for oracle in proposal.oracles.iter() {
            let inserted = transmitters.insert(oracle.transmitter);
            require!(inserted, DuplicateTransmitter);
        }

        // Store the new f value
        proposal.f = f;

        Ok(())
    }

    #[access_control(proposal_owner(&ctx.accounts.proposal, &ctx.accounts.authority))]
    pub fn propose_payees(ctx: Context<ProposeConfig>, token_mint: Pubkey) -> Result<()> {
        let mut proposal = ctx.accounts.proposal.load_mut()?;
        require!(proposal.state != Proposal::FINALIZED, InvalidInput);

        let payees = ctx.remaining_accounts;

        // Need to provide a payee for each oracle
        require!(proposal.oracles.len() == payees.len(), PayeeOracleMismatch);

        // Verify that the remaining accounts are valid token accounts.
        for account in payees {
            let account = Account::<'_, token::TokenAccount>::try_from(account)?;
            require!(account.mint == token_mint, InvalidTokenAccount);
        }

        for (oracle, payee) in proposal.oracles.iter_mut().zip(payees.iter()) {
            oracle.payee = payee.key();
        }
        proposal.token_mint = token_mint;

        Ok(())
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn set_requester_access_controller(ctx: Context<SetAccessController>) -> Result<()> {
        let mut state = ctx.accounts.state.load_mut()?;
        state.config.requester_access_controller = ctx.accounts.access_controller.key();
        Ok(())
    }

    #[access_control(has_requester_access(&ctx.accounts.state, &ctx.accounts.access_controller, &ctx.accounts.authority))]
    pub fn request_new_round(ctx: Context<RequestNewRound>) -> Result<()> {
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
    ) -> Result<()> {
        // Based on https://github.com/project-serum/anchor/blob/2390a4f16791b40c63efe621ffbd558e354d5303/lang/syn/src/codegen/program/handlers.rs#L696-L737
        // Use a raw instruction to skip data decoding, but keep using Anchor contexts.

        let mut bumps = std::collections::BTreeMap::new();
        // Deserialize accounts.
        let mut remaining_accounts: &[AccountInfo] = accounts;
        let mut accounts =
            Transmit::try_accounts(program_id, &mut remaining_accounts, data, &mut bumps)?;

        // Construct a context
        let ctx = Context::new(program_id, &mut accounts, remaining_accounts, bumps);

        transmit_impl(ctx, data)?;

        // Exit routine
        accounts.exit(program_id)
    }

    #[access_control(owner(&ctx.accounts.state, &ctx.accounts.authority))]
    pub fn set_billing_access_controller(ctx: Context<SetAccessController>) -> Result<()> {
        let mut state = ctx.accounts.state.load_mut()?;
        state.config.billing_access_controller = ctx.accounts.access_controller.key();
        Ok(())
    }

    #[access_control(has_billing_access(&ctx.accounts.state, &ctx.accounts.access_controller, &ctx.accounts.authority))]
    pub fn set_billing<'info>(
        ctx: Context<'_, '_, '_, 'info, SetBilling<'info>>,
        observation_payment_gjuels: u32,
        transmission_payment_gjuels: u32,
    ) -> Result<()> {
        // First... pay out the oracles with the original config
        pay_oracles_impl(
            ctx.accounts.state.clone(),
            ctx.accounts.token_program.to_account_info(),
            ctx.accounts.token_vault.to_account_info(),
            ctx.accounts.vault_authority.clone(),
            ctx.remaining_accounts,
        )?;

        // Then update the config
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
    pub fn withdraw_funds(ctx: Context<WithdrawFunds>, amount_gjuels: u64) -> Result<()> {
        let state = &ctx.accounts.state.load()?;

        let link_due = calculate_total_link_due_gjuels(&state.config, &state.oracles)?;

        let balance_gjuels = token::accessor::amount(&ctx.accounts.token_vault.to_account_info())?;
        let available = balance_gjuels.saturating_sub(link_due);

        token::transfer(
            ctx.accounts.transfer_ctx().with_signer(&[&[
                b"vault".as_ref(),
                ctx.accounts.state.key().as_ref(),
                &[state.vault_nonce],
            ]]),
            amount_gjuels.min(available),
        )?;
        Ok(())
    }

    pub fn withdraw_payment(ctx: Context<WithdrawPayment>) -> Result<()> {
        let mut state = ctx.accounts.state.load_mut()?;

        let vault_nonce = state.vault_nonce;
        let latest_round_id = state.config.latest_aggregator_round_id;
        let billing = state.config.billing;

        // Validate that the token account is actually for LINK
        require!(
            ctx.accounts.payee.mint == state.config.token_mint,
            InvalidInput
        );

        let key = ctx.accounts.payee.key();

        let oracle = state
            .oracles
            .iter_mut()
            .find(|oracle| oracle.payee == key)
            .ok_or(ErrorCode::Unauthorized)?;

        // Validate that the instruction was signed by the same authority as the token account
        require!(
            ctx.accounts.payee.owner == ctx.accounts.authority.key(),
            Unauthorized
        );

        // -- Pay oracle

        let amount_gjuels = calculate_owed_payment_gjuels(&billing, oracle, latest_round_id)?;
        // Reset reward and gas reimbursement
        oracle.payment_gjuels = 0;
        oracle.from_round_id = latest_round_id;

        if amount_gjuels == 0 {
            return Ok(());
        }

        // transfer funds
        token::transfer(
            ctx.accounts.transfer_ctx().with_signer(&[&[
                b"vault".as_ref(),
                ctx.accounts.state.key().as_ref(),
                &[vault_nonce],
            ]]),
            amount_gjuels,
        )?; // consider using a custom transfer that calls invoke_signed_unchecked instead

        Ok(())
    }

    #[access_control(has_billing_access(&ctx.accounts.state, &ctx.accounts.access_controller, &ctx.accounts.authority))]
    pub fn pay_oracles<'info>(ctx: Context<'_, '_, '_, 'info, SetBilling<'info>>) -> Result<()> {
        pay_oracles_impl(
            ctx.accounts.state.clone(),
            ctx.accounts.token_program.to_account_info(),
            ctx.accounts.token_vault.to_account_info(),
            ctx.accounts.vault_authority.clone(),
            ctx.remaining_accounts,
        )
    }

    pub fn transfer_payeeship(ctx: Context<TransferPayeeship>) -> Result<()> {
        // Can't transfer to self
        require!(
            ctx.accounts.payee.key() != ctx.accounts.proposed_payee.key(),
            InvalidInput
        );

        let mut state = ctx.accounts.state.load_mut()?;

        // Validate that the token account is actually for LINK
        require!(
            ctx.accounts.proposed_payee.mint == state.config.token_mint,
            InvalidInput
        );

        let oracle = state
            .oracles
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

    pub fn accept_payeeship(ctx: Context<AcceptPayeeship>) -> Result<()> {
        let mut state = ctx.accounts.state.load_mut()?;

        let oracle = state
            .oracles
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

fn pay_oracles_impl<'info>(
    state: AccountLoader<'info, State>,
    token_program: AccountInfo<'info>,
    token_vault: AccountInfo<'info>,
    vault_authority: AccountInfo<'info>,
    remaining_accounts: &[AccountInfo<'info>],
) -> Result<()> {
    let state_id = state.key();
    let mut state = state.load_mut()?;

    require!(
        remaining_accounts.len() == state.oracles.len(),
        InvalidInput
    );

    let vault_nonce = state.vault_nonce;
    let latest_round_id = state.config.latest_aggregator_round_id;
    let billing = state.config.billing;

    let payments_gjuels: Vec<(u64, CpiContext<'_, '_, '_, 'info, token::Transfer<'info>>)> = state
        .oracles
        .iter_mut()
        .zip(remaining_accounts)
        .map(|(oracle, payee)| {
            // Ensure specified accounts match the ones inside oracles
            require!(&oracle.payee == payee.key, InvalidInput);

            let amount_gjuels = calculate_owed_payment_gjuels(&billing, oracle, latest_round_id)?;
            // Reset reward and gas reimbursement
            oracle.payment_gjuels = 0;
            oracle.from_round_id = latest_round_id;

            let cpi = CpiContext::new(
                token_program.clone(),
                token::Transfer {
                    from: token_vault.clone(),
                    to: payee.to_account_info(),
                    authority: vault_authority.clone(),
                },
            );

            Ok((amount_gjuels, cpi))
        })
        .collect::<Result<_>>()?;

    // No account can be borrowed during CPI...
    drop(state);

    for (amount_gjuels, cpi) in payments_gjuels {
        if amount_gjuels == 0 {
            continue;
        }

        token::transfer(
            cpi.with_signer(&[&[b"vault".as_ref(), state_id.as_ref(), &[vault_nonce]]]),
            amount_gjuels,
        )?;
    }

    Ok(())
}

#[inline(always)]
fn transmit_impl<'info>(ctx: Context<Transmit<'info>>, data: &[u8]) -> Result<()> {
    let (store_nonce, data) = data.split_first().ok_or(ErrorCode::InvalidInput)?;

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
    let reimbursement_gjuels =
        calculate_reimbursement_gjuels(report.juels_per_lamport, signature_count)?; // gjuels
    let amount_gjuels = reimbursement_gjuels
        .saturating_add(u64::from(state.config.billing.transmission_payment_gjuels));
    state.oracles[oracle_idx].payment_gjuels = state.oracles[oracle_idx]
        .payment_gjuels
        .saturating_add(amount_gjuels);

    emit!(event::NewTransmission {
        round_id: state.config.latest_aggregator_round_id,
        config_digest: state.config.latest_config_digest,
        answer: report.median,
        transmitter: oracle_idx as u8, // has to fit in u8 because MAX_ORACLES < 255
        observations_timestamp: report.observations_timestamp,
        observer_count: report.observer_count,
        observers: report.observers,
        juels_per_lamport: report.juels_per_lamport,
        reimbursement_gjuels,
    });

    let round = NewTransmission {
        answer: report.median,
        timestamp: report.observations_timestamp as u64,
    };

    let cpi_ctx = CpiContext::new(
        ctx.accounts.store_program.to_account_info(),
        store::cpi::accounts::Submit {
            feed: ctx.accounts.feed.to_account_info(),
            authority: ctx.accounts.store_authority.to_account_info(),
        },
    );

    drop(state);

    let seeds = &[
        b"store",
        ctx.accounts.state.to_account_info().key.as_ref(),
        &[*store_nonce],
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

fn calculate_reimbursement_gjuels(juels_per_lamport: u64, _signature_count: usize) -> Result<u64> {
    const SIGNERS: u64 = 1;
    const GIGA: u128 = 10u128.pow(9);
    const LAMPORTS_PER_SIGNATURE: u64 = 5_000; // constant, originally retrieved from deprecated sysvar fees
    let lamports = LAMPORTS_PER_SIGNATURE * SIGNERS;
    let juels = u128::from(lamports) * u128::from(juels_per_lamport);
    let gjuels = juels / GIGA; // return value as gjuels

    // convert from u128 to u64 with staturating logic to max u64
    Ok(gjuels.try_into().unwrap_or(u64::MAX))
}

fn calculate_owed_payment_gjuels(
    config: &Billing,
    oracle: &Oracle,
    latest_round_id: u32,
) -> Result<u64> {
    let rounds = latest_round_id
        .checked_sub(oracle.from_round_id)
        .ok_or(ErrorCode::Overflow)?;

    let amount_gjuels = u64::from(config.observation_payment_gjuels)
        .checked_mul(rounds.into())
        .ok_or(ErrorCode::Overflow)?
        .checked_add(oracle.payment_gjuels)
        .ok_or(ErrorCode::Overflow)?;

    Ok(amount_gjuels)
}

fn calculate_total_link_due_gjuels(config: &Config, oracles: &[Oracle]) -> Result<u64> {
    let (rounds, reimbursements) = oracles
        .iter()
        .try_fold((0, 0), |(rounds, reimbursements): (u32, u64), oracle| {
            let count = config
                .latest_aggregator_round_id
                .checked_sub(oracle.from_round_id)?;

            Some((
                rounds.checked_add(count)?,
                reimbursements.checked_add(oracle.payment_gjuels)?,
            ))
        })
        .ok_or(ErrorCode::Overflow)?;

    let amount_gjuels = u64::from(config.billing.observation_payment_gjuels)
        .checked_mul(u64::from(rounds))
        .ok_or(ErrorCode::Overflow)?
        .checked_add(reimbursements)
        .ok_or(ErrorCode::Overflow)?;

    Ok(amount_gjuels)
}

// -- Access control modifiers

// Only owner access
fn owner(state_loader: &AccountLoader<State>, signer: &AccountInfo) -> Result<()> {
    let config = state_loader.load()?.config;
    require!(signer.key.eq(&config.owner), Unauthorized);
    Ok(())
}

fn proposal_owner(proposal_loader: &AccountLoader<Proposal>, signer: &AccountInfo) -> Result<()> {
    let proposal = proposal_loader.load()?;
    require!(signer.key.eq(&proposal.owner), Unauthorized);
    Ok(())
}

fn has_billing_access(
    state: &AccountLoader<State>,
    controller: &AccountLoader<AccessController>,
    authority: &AccountInfo,
) -> Result<()> {
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
) -> Result<()> {
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

#[error_code]
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

    #[msg("Payee and Oracle length mismatch")]
    PayeeOracleMismatch,

    #[msg("Invalid Token Account")]
    InvalidTokenAccount,

    #[msg("Oracle signer key not found")]
    UnauthorizedSigner,

    #[msg("Oracle transmitter key not found")]
    UnauthorizedTransmitter,
}

pub mod query {
    use super::ErrorCode;
    use super::*;

    #[account]
    pub struct LatestConfig {
        pub config_count: u32,
        pub config_digest: [u8; DIGEST_SIZE],
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

        let balance_gjuels = token::accessor::amount(token_vault)?;

        let link_due = calculate_total_link_due_gjuels(&state.config, &state.oracles)?;

        let available_balance = balance_gjuels.saturating_sub(link_due);

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

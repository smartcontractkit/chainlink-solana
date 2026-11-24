use std::mem::size_of;

use anchor_lang::{prelude::*, Discriminator};
use anchor_spl::token_interface;

use crate::{
    common::{
        ANCHOR_DISCRIMINATOR, DF1_WRITER_SEED, MAX_ACCOUNT_DATA_INCREASE, MIN_MAX_STORED_REPORTS,
        STATE_SEED, STORE_AUTHORITY_SEED, STORE_STATE_VERSION,
    },
    context::*,
    error::StoreError,
    event::*,
    state::{feed_account_len, StoreConfig, StoreState, WorkflowMetadata},
    utils::{create_pda_account, is_uninitialized, validate_workflows, verify_feed_admin},
};

/// Creates a store: its state account at the PDA `["state", store ID]`, with the upgrade authority
/// as owner and paying the rent, and the settings in `config`. The settings are checked before the
/// account is created.
pub fn initialize(ctx: Context<Initialize>, store_id: u64, config: StoreConfig) -> Result<()> {
    let StoreConfig {
        feed_admins,
        default_max_stored_reports,
        max_observation_forward_secs,
        df1_program,
    } = config;
    // The program creates every feed account, and an account a program creates can be at most
    // 10 KiB, so the default ring size must fit a feed in that.
    require!(
        default_max_stored_reports >= MIN_MAX_STORED_REPORTS
            && feed_account_len(default_max_stored_reports) <= MAX_ACCOUNT_DATA_INCREASE,
        StoreError::InvalidMaxStoredReports
    );
    require_keys_neq!(df1_program, Pubkey::default(), StoreError::InvalidAddress);
    // This program's ID stands for "not passed" among a report's optional DF1 accounts, so it
    // can't also be the DF1 program.
    require_keys_neq!(df1_program, crate::ID, StoreError::InvalidAddress);

    let state_key = ctx.accounts.state.key();
    let mut state: Box<StoreState> = bytemuck::zeroed_box();
    state.version = STORE_STATE_VERSION;
    state.owner = ctx.accounts.owner.key();
    state.default_max_stored_reports = default_max_stored_reports;
    state.max_observation_forward_secs = max_observation_forward_secs;
    state.df1_program = df1_program;
    let bump =
        |seed: &[u8]| Pubkey::find_program_address(&[seed, state_key.as_ref()], &crate::ID).1;
    state.store_authority_bump = bump(STORE_AUTHORITY_SEED);
    state.df1_writer_bump = bump(DF1_WRITER_SEED);

    require!(
        feed_admins.len() <= state.feed_admins.capacity(),
        StoreError::FeedAdminsFull
    );
    let mut previous = Pubkey::default();
    for admin in feed_admins.iter() {
        // The admins must be strictly increasing, so the list is sorted for binary search. Since
        // `previous` starts at the default key, this also refuses the default key.
        require!(previous < *admin, StoreError::InvalidAddress);
        state.feed_admins.push(*admin);
        previous = *admin;
    }

    // A second `initialize` for the same store ID finds the account already created.
    let info = ctx.accounts.state.to_account_info();
    require!(is_uninitialized(&info), StoreError::StoreAlreadyInitialized);
    let space = ANCHOR_DISCRIMINATOR + size_of::<StoreState>();
    create_pda_account(
        space,
        &[STATE_SEED, &store_id.to_le_bytes(), &[ctx.bumps.state]],
        &ctx.accounts.owner.to_account_info(),
        &info,
        &ctx.accounts.system_program.to_account_info(),
    )?;
    {
        let mut data = info.try_borrow_mut_data()?;
        data[..ANCHOR_DISCRIMINATOR].copy_from_slice(StoreState::DISCRIMINATOR);
        data[ANCHOR_DISCRIMINATOR..space].copy_from_slice(bytemuck::bytes_of(&*state));
    }

    emit_cpi!(StoreInitialized {
        state: state_key,
        owner: state.owner,
        default_max_stored_reports,
        max_observation_forward_secs,
        df1_program,
        feed_admins,
    });
    Ok(())
}

/// Proposes a new owner, who takes over only after accepting.
pub fn transfer_ownership(ctx: Context<OwnerOnly>, proposed_owner: Pubkey) -> Result<()> {
    let state = &mut ctx.accounts.state.load_mut()?;
    require_keys_neq!(
        proposed_owner,
        state.owner,
        StoreError::InvalidProposedOwner
    );
    state.proposed_owner = proposed_owner;
    emit_cpi!(OwnershipTransferRequested {
        state: ctx.accounts.state.key(),
        from: state.owner,
        to: proposed_owner,
    });
    Ok(())
}

/// Makes the proposed owner the owner and clears the proposal.
pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
    let state = &mut ctx.accounts.state.load_mut()?;
    let previous = state.owner;
    state.owner = std::mem::take(&mut state.proposed_owner);
    emit_cpi!(OwnershipTransferred {
        state: ctx.accounts.state.key(),
        from: previous,
        to: state.owner,
    });
    Ok(())
}

/// Grants or revokes a key's feed admin permission, keeping the admin list sorted.
pub fn set_feed_admin(ctx: Context<OwnerOnly>, feed_admin: Pubkey, is_admin: bool) -> Result<()> {
    require_keys_neq!(feed_admin, Pubkey::default(), StoreError::InvalidAddress);
    let state = &mut ctx.accounts.state.load_mut()?;
    match (state.feed_admins.binary_search(&feed_admin), is_admin) {
        (Err(index), true) => {
            require!(
                state.feed_admins.remaining_capacity() > 0,
                StoreError::FeedAdminsFull
            );
            state.feed_admins.insert(index, feed_admin);
        }
        (Ok(index), false) => {
            state.feed_admins.remove(index);
            // The layout is public, so the removed admin must not stay visible in the entry the
            // list no longer uses.
            let freed = state.feed_admins.len();
            state.feed_admins.xs[freed] = Pubkey::default();
        }
        // The key already has the requested status, so nothing changes.
        _ => {}
    }
    emit_cpi!(FeedAdminSet {
        state: ctx.accounts.state.key(),
        feed_admin,
        is_admin,
    });
    Ok(())
}

/// Replaces the store's default workflow list. Unlike on EVM, the list can be emptied. Only feeds
/// with their own workflow list can then be written, so a compromised default workflow can be
/// revoked in one call without naming a replacement.
pub fn set_default_feed_metadata(
    ctx: Context<FeedAdminState>,
    workflow_metadata: Vec<WorkflowMetadata>,
) -> Result<()> {
    validate_workflows(&workflow_metadata)?;

    let state = &mut ctx.accounts.state.load_mut()?;
    verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;

    // The list is zeroed first, so a shorter list leaves no removed workflows in the public
    // layout.
    state.default_workflow_metadata = bytemuck::Zeroable::zeroed();
    state.default_workflow_metadata.extend(&workflow_metadata);

    emit_cpi!(DefaultFeedMetadataSet {
        state: ctx.accounts.state.key(),
        workflow_metadata,
    });
    Ok(())
}

/// Sets how many seconds ahead of the chain's clock a report's observation timestamp may be (see
/// `StoreState::max_observation_forward_secs`).
pub fn set_max_observation_forward_duration(
    ctx: Context<FeedAdminState>,
    max_observation_forward_secs: u32,
) -> Result<()> {
    let state = &mut ctx.accounts.state.load_mut()?;
    verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;
    state.max_observation_forward_secs = max_observation_forward_secs;
    emit_cpi!(MaxObservationForwardDurationSecondsUpdated {
        state: ctx.accounts.state.key(),
        new_duration: max_observation_forward_secs,
    });
    Ok(())
}

/// Moves tokens out of a token account of the store authority PDA, to recover tokens sent to the
/// store by mistake.
pub fn recover_tokens(ctx: Context<RecoverTokens>, amount: u64) -> Result<()> {
    require!(
        amount <= ctx.accounts.from.amount,
        StoreError::InsufficientBalance
    );

    let state_key = ctx.accounts.state.key();
    let bump = ctx.accounts.state.load()?.store_authority_bump;
    let seeds: &[&[u8]] = &[STORE_AUTHORITY_SEED, state_key.as_ref(), &[bump]];

    token_interface::transfer_checked(
        CpiContext::new_with_signer(
            ctx.accounts.token_program.to_account_info(),
            token_interface::TransferChecked {
                from: ctx.accounts.from.to_account_info(),
                mint: ctx.accounts.mint.to_account_info(),
                to: ctx.accounts.to.to_account_info(),
                authority: ctx.accounts.store_authority.to_account_info(),
            },
            &[seeds],
        ),
        amount,
        ctx.accounts.mint.decimals,
    )?;

    emit_cpi!(TokenRecovered {
        state: state_key,
        mint: ctx.accounts.mint.key(),
        to: ctx.accounts.to.key(),
        amount,
    });
    Ok(())
}

/// Moves SOL out of the store authority PDA, to recover SOL sent to the store by mistake.
pub fn recover_native(ctx: Context<RecoverNative>, amount: u64) -> Result<()> {
    // The transfer must take all of the SOL or leave the store authority rent-exempt, because the
    // runtime refuses a balance in between.
    let balance = ctx.accounts.store_authority.lamports();
    let left = balance
        .checked_sub(amount)
        .ok_or(StoreError::InsufficientBalance)?;
    require!(
        left == 0 || left >= Rent::get()?.minimum_balance(0),
        StoreError::InsufficientBalance
    );

    let state_key = ctx.accounts.state.key();
    let bump = ctx.accounts.state.load()?.store_authority_bump;
    let seeds: &[&[u8]] = &[STORE_AUTHORITY_SEED, state_key.as_ref(), &[bump]];

    anchor_lang::system_program::transfer(
        CpiContext::new_with_signer(
            ctx.accounts.system_program.to_account_info(),
            anchor_lang::system_program::Transfer {
                from: ctx.accounts.store_authority.to_account_info(),
                to: ctx.accounts.to.to_account_info(),
            },
            &[seeds],
        ),
        amount,
    )?;

    emit_cpi!(TokenRecovered {
        state: state_key,
        mint: Pubkey::default(),
        to: ctx.accounts.to.key(),
        amount,
    });
    Ok(())
}

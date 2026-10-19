use anchor_lang::prelude::*;
use anchor_spl::token_interface;

use crate::{
    common::{
        ACCOUNT_VERSION, MAX_ACCOUNT_DATA_INCREASE, MAX_WORKFLOW_METADATAS, MIN_MAX_STORED_REPORTS,
        STORE_AUTHORITY_SEED,
    },
    context::*,
    error::StoreError,
    event::*,
    state::{feed_account_len, WorkflowMetadata},
    utils::{validate_workflow_metadata, verify_feed_admin},
};

pub fn initialize(
    ctx: Context<Initialize>,
    feed_admins: Vec<Pubkey>,
    default_max_stored_reports: u64,
    max_observation_forward_secs: u32,
) -> Result<()> {
    require!(
        default_max_stored_reports >= MIN_MAX_STORED_REPORTS
            && feed_account_len(default_max_stored_reports) <= MAX_ACCOUNT_DATA_INCREASE,
        StoreError::InvalidMaxStoredReports
    );

    let state_key = ctx.accounts.state.key();
    let state = &mut ctx.accounts.state.load_init()?;
    state.version = ACCOUNT_VERSION;
    state.owner = ctx.accounts.owner.key();
    state.forwarder_id = ctx.accounts.forwarder_program.key();
    state.default_max_stored_reports = default_max_stored_reports;
    state.max_observation_forward_secs = max_observation_forward_secs;
    state.store_authority_bump =
        Pubkey::find_program_address(&[STORE_AUTHORITY_SEED, state_key.as_ref()], &crate::ID).1;

    require!(
        feed_admins.len() <= state.feed_admins.capacity(),
        StoreError::FeedAdminsFull
    );
    let mut previous = Pubkey::default();
    for admin in feed_admins {
        // sorted for binary search
        require!(previous < admin, StoreError::InvalidAddress);
        state.feed_admins.push(admin);
        emit!(FeedAdminSet {
            state: state_key,
            feed_admin: admin,
            is_admin: true,
        });
        previous = admin;
    }

    emit!(StoreInitialized {
        state: state_key,
        forwarder_id: state.forwarder_id,
        default_max_stored_reports,
        max_observation_forward_secs,
    });
    Ok(())
}

pub fn transfer_ownership(ctx: Context<OwnerOnly>, proposed_owner: Pubkey) -> Result<()> {
    let state = &mut ctx.accounts.state.load_mut()?;
    require_keys_neq!(
        proposed_owner,
        state.owner,
        StoreError::InvalidProposedOwner
    );
    state.proposed_owner = proposed_owner;
    emit!(OwnershipTransferRequested {
        state: ctx.accounts.state.key(),
        from: state.owner,
        to: proposed_owner,
    });
    Ok(())
}

pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
    let state = &mut ctx.accounts.state.load_mut()?;
    let previous = state.owner;
    state.owner = std::mem::take(&mut state.proposed_owner);
    emit!(OwnershipTransferred {
        state: ctx.accounts.state.key(),
        from: previous,
        to: state.owner,
    });
    Ok(())
}

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
        }
        // already in the requested state
        _ => {}
    }
    emit!(FeedAdminSet {
        state: ctx.accounts.state.key(),
        feed_admin,
        is_admin,
    });
    Ok(())
}

pub fn update_forwarder_id(ctx: Context<UpdateForwarder>) -> Result<()> {
    let state = &mut ctx.accounts.state.load_mut()?;
    let previous = state.forwarder_id;
    state.forwarder_id = ctx.accounts.forwarder_program.key();
    emit!(ForwarderUpdated {
        state: ctx.accounts.state.key(),
        previous_forwarder: previous,
        new_forwarder: state.forwarder_id,
    });
    Ok(())
}

pub fn set_default_feed_metadata(
    ctx: Context<FeedAdminState>,
    workflow_metadata: Vec<WorkflowMetadata>,
) -> Result<()> {
    require!(!workflow_metadata.is_empty(), StoreError::EmptyConfig);
    require!(
        workflow_metadata.len() <= MAX_WORKFLOW_METADATAS,
        StoreError::MaxEntriesExceeded
    );

    let state = &mut ctx.accounts.state.load_mut()?;
    verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;

    state.default_workflow_metadata.clear();
    for metadata in workflow_metadata.iter() {
        validate_workflow_metadata(metadata)?;
        state.default_workflow_metadata.push(*metadata);
    }

    emit!(DefaultFeedMetadataSet {
        state: ctx.accounts.state.key(),
        workflow_metadata,
    });
    Ok(())
}

pub fn set_max_observation_forward_duration(
    ctx: Context<FeedAdminState>,
    max_observation_forward_secs: u32,
) -> Result<()> {
    let state = &mut ctx.accounts.state.load_mut()?;
    verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;
    state.max_observation_forward_secs = max_observation_forward_secs;
    emit!(MaxObservationForwardDurationSecondsUpdated {
        state: ctx.accounts.state.key(),
        new_duration: max_observation_forward_secs,
    });
    Ok(())
}

/// Recover SPL tokens held by token accounts of the store authority.
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

    emit!(TokenRecovered {
        state: state_key,
        mint: ctx.accounts.mint.key(),
        to: ctx.accounts.to.key(),
        amount,
    });
    Ok(())
}

/// Recover native SOL sent to the store authority.
pub fn recover_native(ctx: Context<RecoverNative>, amount: u64) -> Result<()> {
    require!(
        amount <= ctx.accounts.store_authority.lamports(),
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

    emit!(TokenRecovered {
        state: state_key,
        mint: Pubkey::default(),
        to: ctx.accounts.to.key(),
        amount,
    });
    Ok(())
}

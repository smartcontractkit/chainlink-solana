use anchor_lang::prelude::*;

use crate::{
    common::{ACCOUNT_VERSION, DATA_ID_SEED, MAX_CALLBACK_ACCOUNTS, MAX_WORKFLOW_METADATAS},
    context::FeedAdminAccounts,
    data_id,
    error::StoreError,
    event::*,
    state::{CallbackConfigArgs, DataIdConfig, WorkflowMetadata},
    utils::{
        close_account, create_pda_account, load_data_id_config, store, update_settings,
        validate_workflow_metadata, verify_feed_admin,
    },
};

/// Create the config accounts of SVR paths. Reports for an SVR path are only applied once it has a
/// config. Non-SVR data IDs keep their settings in their feed account and have none. Accounts that
/// already exist are left untouched.
pub fn init_data_ids<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require_eq!(
        data_ids.len(),
        ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );

    let state_key = ctx.accounts.state.key();
    for (id, info) in data_ids.iter().zip(ctx.remaining_accounts.iter()) {
        require!(data_id::is_valid_svr(id), StoreError::InvalidDataId);

        let (address, bump) =
            Pubkey::find_program_address(&[DATA_ID_SEED, state_key.as_ref(), id], &crate::ID);
        require_keys_eq!(address, *info.key, StoreError::AccountMismatch);

        if !info.data_is_empty() {
            load_data_id_config(info, &state_key, id)?;
            continue;
        }

        create_pda_account(
            8 + std::mem::size_of::<DataIdConfig>(),
            &[DATA_ID_SEED, state_key.as_ref(), id, &[bump]],
            &ctx.accounts.feed_admin.to_account_info(),
            info,
            &ctx.accounts.system_program.to_account_info(),
        )?;

        let mut config: Box<DataIdConfig> = bytemuck::zeroed_box();
        config.version = ACCOUNT_VERSION;
        config.data_id = *id;
        config.state = state_key;
        store(info, &*config)?;

        emit!(DataIdInitialized {
            state: state_key,
            data_id: *id,
        });
    }
    Ok(())
}

/// Close SVR path config accounts, returning their rent to the feed admin.
pub fn close_data_ids<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require_eq!(
        data_ids.len(),
        ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );

    let state_key = ctx.accounts.state.key();
    for (id, info) in data_ids.iter().zip(ctx.remaining_accounts.iter()) {
        load_data_id_config(info, &state_key, id)?;
        close_account(info, &ctx.accounts.feed_admin.to_account_info())?;
        emit!(DataIdClosed {
            state: state_key,
            data_id: *id,
        });
    }
    Ok(())
}

/// Set the workflows allowed to write the given data IDs, replacing any previous feed-specific
/// metadata. Data IDs without feed-specific metadata use the store's default metadata.
pub fn set_specific_feed_metadata<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
    workflow_metadata: Vec<WorkflowMetadata>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require!(
        !data_ids.is_empty() && !workflow_metadata.is_empty(),
        StoreError::EmptyConfig
    );
    require!(
        workflow_metadata.len() <= MAX_WORKFLOW_METADATAS,
        StoreError::MaxEntriesExceeded
    );
    require_eq!(
        data_ids.len(),
        ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );
    for metadata in workflow_metadata.iter() {
        validate_workflow_metadata(metadata)?;
    }

    let state_key = ctx.accounts.state.key();
    for (id, info) in data_ids.iter().zip(ctx.remaining_accounts.iter()) {
        let replaced = update_settings(info, &state_key, id, |settings| {
            let replaced = !settings.specific_workflow_metadata.is_empty();
            settings.specific_workflow_metadata.clear();
            settings.specific_workflow_metadata.extend(&workflow_metadata);
            Ok(replaced)
        })?;

        if replaced {
            emit!(SpecificFeedMetadataRemoved {
                state: state_key,
                data_id: *id,
            });
        }

        emit!(SpecificFeedMetadataSet {
            state: state_key,
            data_id: *id,
            workflow_metadata: workflow_metadata.clone(),
        });
    }
    Ok(())
}

/// Remove feed-specific workflow metadata, the data IDs fall back to the default metadata.
pub fn remove_specific_feed_metadata<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require_eq!(
        data_ids.len(),
        ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );

    let state_key = ctx.accounts.state.key();
    for (id, info) in data_ids.iter().zip(ctx.remaining_accounts.iter()) {
        update_settings(info, &state_key, id, |settings| {
            require!(
                !settings.specific_workflow_metadata.is_empty(),
                StoreError::FeedNotConfigured
            );
            settings.specific_workflow_metadata.clear();
            Ok(())
        })?;

        emit!(SpecificFeedMetadataRemoved {
            state: state_key,
            data_id: *id,
        });
    }
    Ok(())
}

/// Enable or disable SVR paths. Readers of a disabled path are served the non-SVR feed.
pub fn set_svr_disabled<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    svr_data_ids: Vec<[u8; 16]>,
    is_disabled: Vec<bool>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require!(
        svr_data_ids.len() == is_disabled.len()
            && svr_data_ids.len() == ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );

    let state_key = ctx.accounts.state.key();
    for ((id, disabled), info) in svr_data_ids
        .iter()
        .zip(is_disabled.iter().copied())
        .zip(ctx.remaining_accounts.iter())
    {
        require!(data_id::is_valid_svr(id), StoreError::InvalidDataId);
        let mut config = load_data_id_config(info, &state_key, id)?;
        config.svr_disabled = disabled as u8;
        store(info, &*config)?;

        emit!(SvrFeedDisabled {
            state: state_key,
            svr_data_id: *id,
            is_disabled: disabled,
        });
    }
    Ok(())
}

/// Configure report callbacks. Remaining accounts are, per data ID in order, its feed (non-SVR) or
/// data ID config (SVR path), followed by the callback program account of every enabled
/// (non-default program) callback in order.
pub fn set_report_callback_configs<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    data_ids: Vec<[u8; 16]>,
    callbacks: Vec<CallbackConfigArgs>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require_eq!(
        data_ids.len(),
        callbacks.len(),
        StoreError::ArrayLengthMismatch
    );

    let enabled = callbacks
        .iter()
        .filter(|c| c.program != Pubkey::default())
        .count();
    require_eq!(
        ctx.remaining_accounts.len(),
        data_ids.len() + enabled,
        StoreError::InvalidAccountCount
    );
    let (settings_infos, program_infos) = ctx.remaining_accounts.split_at(data_ids.len());
    let mut program_infos = program_infos.iter();

    let state_key = ctx.accounts.state.key();
    for ((id, callback), info) in data_ids
        .iter()
        .zip(callbacks.iter())
        .zip(settings_infos.iter())
    {
        require!(!data_id::is_zero(id), StoreError::InvalidDataId);
        // A callback that could never run is rejected here instead of failing every report.
        if callback.program != Pubkey::default() {
            let program = program_infos.next().unwrap();
            require!(
                program.key() == callback.program && program.executable,
                StoreError::ReportCallbackNoContract
            );
            require!(
                callback.accounts.len() <= MAX_CALLBACK_ACCOUNTS,
                StoreError::MaxEntriesExceeded
            );
        }

        let (previous_program, current_program) =
            update_settings(info, &state_key, id, |settings| {
                let previous_program = settings.callback_program;
                settings.callback_program = callback.program;
                settings.callback_accounts = [Pubkey::default(); MAX_CALLBACK_ACCOUNTS];
                settings.callback_accounts_len = 0;
                settings.callback_writable_mask = 0;
                settings.callback_revert_on_failure = 0;
                if callback.program != Pubkey::default() {
                    settings.callback_revert_on_failure = callback.revert_on_failure as u8;
                    settings.callback_accounts_len = callback.accounts.len() as u8;
                    for (i, account) in callback.accounts.iter().enumerate() {
                        settings.callback_accounts[i] = account.pubkey;
                        if account.is_writable {
                            settings.callback_writable_mask |= 1 << i;
                        }
                    }
                }
                Ok((previous_program, settings.callback_program))
            })?;

        emit!(ReportCallbackConfigSet {
            state: state_key,
            data_id: *id,
            previous_program,
            current_program,
            accounts: callback.accounts.clone(),
            revert_on_failure: callback.revert_on_failure,
        });
    }
    Ok(())
}

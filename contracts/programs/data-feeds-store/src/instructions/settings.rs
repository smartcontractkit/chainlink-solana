use anchor_lang::prelude::*;

use crate::{
    common::{MAX_CALLBACK_ACCOUNTS, MAX_WORKFLOW_METADATAS},
    context::FeedAdminAccounts,
    data_id,
    error::StoreError,
    event::*,
    state::{CallbackConfigArgs, WorkflowMetadata},
    utils::{update_settings, validate_workflow_metadata, verify_feed_admin},
};

/// Set the workflows allowed to write the given data IDs, replacing any previous feed-specific
/// metadata. Data IDs without feed-specific metadata use the store's default metadata. Remaining
/// accounts are the feeds of the data IDs in order.
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
            settings
                .specific_workflow_metadata
                .extend(&workflow_metadata);
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

/// Configure report callbacks. Remaining accounts are the feeds of the data IDs in order, followed
/// by the callback program account of every enabled (non-default program) callback in order.
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
        require!(data_id::is_valid_data_id(id), StoreError::InvalidDataId);
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

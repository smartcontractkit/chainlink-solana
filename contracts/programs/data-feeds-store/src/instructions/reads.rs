use anchor_lang::prelude::*;

use crate::{
    context::PermissionRead,
    data_id,
    error::StoreError,
    state::{FeedConfig, StoreState, WorkflowMetadata},
    utils::{has_permission, load_boxed, load_config_into},
};

/// Whether a workflow may write a data ID, through its feed-specific metadata when configured and
/// the store's default metadata otherwise (also when no feed config is passed). Writes disabled
/// for the data ID are not considered.
pub fn check_feed_permission(
    ctx: Context<PermissionRead>,
    data_id: [u8; 16],
    workflow_metadata: WorkflowMetadata,
) -> Result<bool> {
    // a data ID the store doesn't serve can't be written, whatever the metadata says
    require!(
        data_id::is_valid_data_id(&data_id),
        StoreError::InvalidDataId
    );
    let state_info = ctx.accounts.state.to_account_info();
    let state: Box<StoreState> = load_boxed(&state_info)?;
    let mut config: Box<FeedConfig> = bytemuck::zeroed_box();
    if let Some(info) = &ctx.accounts.feed_config {
        load_config_into(
            &info.to_account_info(),
            state_info.key,
            &data_id,
            &mut config,
        )?;
    }
    Ok(has_permission(
        &config.specific_workflow_metadata,
        &state.default_workflow_metadata,
        &workflow_metadata.allowed_sender,
        &workflow_metadata.allowed_workflow_owner,
        &workflow_metadata.allowed_workflow_name,
    ))
}

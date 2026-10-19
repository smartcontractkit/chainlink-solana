use anchor_lang::prelude::*;

use crate::{
    common::{
        ACCOUNT_VERSION, ANCHOR_DISCRIMINATOR, FEED_DESCRIPTION_SEED, SVR_PATH_DESCRIPTION_SEED,
    },
    context::FeedAdminAccounts,
    data_id,
    error::StoreError,
    event::*,
    state::{FeedDescription, SvrPathDescription},
    utils::{create_pda_account, encode_description, verify_feed_admin},
};

/// Create the description PDA if needed, returning whether it already existed.
fn ensure_account<'info>(
    ctx: &Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    info: &AccountInfo<'info>,
    seed: &[u8],
    key: &[u8],
    space: usize,
) -> Result<bool> {
    let state_key = ctx.accounts.state.key();
    let (address, bump) =
        Pubkey::find_program_address(&[seed, state_key.as_ref(), key], &crate::ID);
    require_keys_eq!(address, *info.key, StoreError::AccountMismatch);

    if !info.data_is_empty() {
        return Ok(true);
    }
    create_pda_account(
        space,
        &[seed, state_key.as_ref(), key, &[bump]],
        &ctx.accounts.feed_admin.to_account_info(),
        info,
        &ctx.accounts.system_program.to_account_info(),
    )?;
    Ok(false)
}

/// Set descriptions shared by every instance and SVR path of a feed, e.g. "ETH/USD".
pub fn set_feed_descriptions<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    indexed_feed_ids: Vec<[u8; 16]>,
    descriptions: Vec<String>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require!(
        indexed_feed_ids.len() == descriptions.len()
            && indexed_feed_ids.len() == ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );

    let state_key = ctx.accounts.state.key();
    for ((id, description), info) in indexed_feed_ids
        .iter()
        .zip(descriptions.iter())
        .zip(ctx.remaining_accounts.iter())
    {
        require!(
            !data_id::is_zero(id) && data_id::is_valid_indexed_feed_id(id),
            StoreError::InvalidDataId
        );
        let encoded = encode_description(description)?;
        ensure_account(
            &ctx,
            info,
            FEED_DESCRIPTION_SEED,
            id,
            ANCHOR_DISCRIMINATOR + FeedDescription::INIT_SPACE,
        )?;

        FeedDescription {
            version: ACCOUNT_VERSION,
            state: state_key,
            indexed_feed_id: *id,
            description: encoded,
        }
        .try_serialize(&mut &mut info.try_borrow_mut_data()?[..])?;

        emit!(FeedDescriptionSet {
            state: state_key,
            indexed_feed_id: *id,
            description: description.clone(),
        });
    }
    Ok(())
}

/// Set descriptions of SVR paths, appended to feed descriptions for SVR data IDs.
pub fn set_svr_path_descriptions<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    svr_path_ids: Vec<[u8; 2]>,
    descriptions: Vec<String>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require!(
        svr_path_ids.len() == descriptions.len()
            && svr_path_ids.len() == ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );

    let state_key = ctx.accounts.state.key();
    for ((path_id, description), info) in svr_path_ids
        .iter()
        .zip(descriptions.iter())
        .zip(ctx.remaining_accounts.iter())
    {
        require!(*path_id != [0, 0], StoreError::ZeroSvrPathId);
        let encoded = encode_description(description)?;
        ensure_account(
            &ctx,
            info,
            SVR_PATH_DESCRIPTION_SEED,
            path_id,
            ANCHOR_DISCRIMINATOR + SvrPathDescription::INIT_SPACE,
        )?;

        SvrPathDescription {
            version: ACCOUNT_VERSION,
            state: state_key,
            svr_path_id: *path_id,
            description: encoded,
        }
        .try_serialize(&mut &mut info.try_borrow_mut_data()?[..])?;

        emit!(SvrPathDescriptionSet {
            state: state_key,
            svr_path_id: *path_id,
            description: description.clone(),
        });
    }
    Ok(())
}

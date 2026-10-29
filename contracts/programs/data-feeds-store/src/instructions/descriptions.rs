use anchor_lang::prelude::*;

use crate::{
    common::{ACCOUNT_VERSION, ANCHOR_DISCRIMINATOR, FEED_DESCRIPTION_SEED},
    context::FeedAdminAccounts,
    data_id,
    error::StoreError,
    event::*,
    state::FeedDescription,
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

/// Set descriptions shared by every instance of a feed, e.g. "ETH/USD".
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
            data_id::is_valid_indexed_feed_id(id),
            StoreError::InvalidDataId
        );
        let encoded = encode_description(description)?;
        let seed = data_id::to_seed(id);
        ensure_account(
            &ctx,
            info,
            FEED_DESCRIPTION_SEED,
            &seed,
            ANCHOR_DISCRIMINATOR + FeedDescription::INIT_SPACE,
        )?;

        FeedDescription {
            version: ACCOUNT_VERSION,
            state: state_key,
            indexed_feed_id: seed,
            description: encoded,
        }
        .try_serialize(&mut &mut info.try_borrow_mut_data()?[..])?;

        emit_cpi!(FeedDescriptionSet {
            state: state_key,
            indexed_feed_id: *id,
            description: description.clone(),
        });
    }
    Ok(())
}

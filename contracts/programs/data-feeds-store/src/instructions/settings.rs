//! Feed admin settings kept in feed configs (remaining accounts: the data IDs' feed config PDAs, in
//! order). A config that doesn't exist yet is created, the feed admin paying, so a data ID can be
//! configured before its first report.

use anchor_lang::{prelude::*, Discriminator};

use crate::{
    common::MAX_WORKFLOW_METADATAS,
    context::FeedAdminConfigs,
    df1,
    error::StoreError,
    event::*,
    state::{FeedConfig, WorkflowMetadata},
    utils::{
        load_or_create_config, store, validate_data_id, validate_workflow_metadata,
        verify_feed_admin, EventEmitter,
    },
};

/// Run `f` on each data ID's config, loading or creating it and writing it back.
fn update_configs<'info>(
    ctx: &Context<'_, '_, 'info, 'info, FeedAdminConfigs<'info>>,
    data_ids: &[[u8; 16]],
    infos: &'info [AccountInfo<'info>],
    mut f: impl FnMut(usize, &[u8; 16], &mut FeedConfig, &EventEmitter<'info>) -> Result<()>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require_eq!(data_ids.len(), infos.len(), StoreError::ArrayLengthMismatch);
    let state_key = ctx.accounts.state.key();
    let payer = ctx.accounts.feed_admin.to_account_info();
    let system_program = ctx.accounts.system_program.to_account_info();
    // one buffer and one event instruction for the whole batch: the bump allocator never frees
    let events = EventEmitter::new(
        ctx.accounts.event_authority.to_account_info(),
        ctx.bumps.event_authority,
    );
    let mut config: Box<FeedConfig> = bytemuck::zeroed_box();
    for (i, (id, info)) in data_ids.iter().zip(infos.iter()).enumerate() {
        validate_data_id(id)?;
        load_or_create_config(info, &state_key, id, &payer, &system_program, &mut config)?;
        f(i, id, &mut config, &events)?;
        store(info, &*config)?;
    }
    Ok(())
}

/// Set the workflows allowed to write the given data IDs, replacing any previous feed-specific
/// metadata. Data IDs without feed-specific metadata use the store's default metadata.
pub fn set_specific_feed_metadata<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminConfigs<'info>>,
    data_ids: Vec<[u8; 16]>,
    workflow_metadata: Vec<WorkflowMetadata>,
) -> Result<()> {
    require!(
        !data_ids.is_empty() && !workflow_metadata.is_empty(),
        StoreError::EmptyConfig
    );
    require!(
        workflow_metadata.len() <= MAX_WORKFLOW_METADATAS,
        StoreError::MaxEntriesExceeded
    );
    for metadata in workflow_metadata.iter() {
        validate_workflow_metadata(metadata)?;
    }
    let state_key = ctx.accounts.state.key();
    update_configs(
        &ctx,
        &data_ids,
        ctx.remaining_accounts,
        |_, id, config, events| {
            if !config.specific_workflow_metadata.is_empty() {
                events.emit(&SpecificFeedMetadataRemoved {
                    state: state_key,
                    data_id: *id,
                })?;
            }
            config.specific_workflow_metadata.clear();
            config.specific_workflow_metadata.extend(&workflow_metadata);
            // SpecificFeedMetadataSet, written from the borrowed list instead of a copy per data ID
            events.emit_with(SpecificFeedMetadataSet::DISCRIMINATOR, |data| {
                state_key.serialize(data)?;
                id.serialize(data)?;
                workflow_metadata.as_slice().serialize(data)?;
                Ok(())
            })
        },
    )
}

/// Remove feed-specific workflow metadata, the data IDs fall back to the default metadata.
pub fn remove_specific_feed_metadata<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminConfigs<'info>>,
    data_ids: Vec<[u8; 16]>,
) -> Result<()> {
    let state_key = ctx.accounts.state.key();
    update_configs(
        &ctx,
        &data_ids,
        ctx.remaining_accounts,
        |_, id, config, events| {
            require!(
                !config.specific_workflow_metadata.is_empty(),
                StoreError::FeedNotConfigured
            );
            config.specific_workflow_metadata.clear();
            events.emit(&SpecificFeedMetadataRemoved {
                state: state_key,
                data_id: *id,
            })
        },
    )
}

/// Set the accepted answer range of feeds, the (i128::MIN, i128::MAX) pair clears the range.
pub fn set_min_max_answers<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminConfigs<'info>>,
    data_ids: Vec<[u8; 16]>,
    min_answers: Vec<i128>,
    max_answers: Vec<i128>,
) -> Result<()> {
    require!(
        data_ids.len() == min_answers.len() && data_ids.len() == max_answers.len(),
        StoreError::ArrayLengthMismatch
    );
    let state_key = ctx.accounts.state.key();
    update_configs(
        &ctx,
        &data_ids,
        ctx.remaining_accounts,
        |i, id, config, events| {
            let (min_answer, max_answer) = (min_answers[i], max_answers[i]);
            if min_answer == i128::MIN && max_answer == i128::MAX {
                // unbounded pair, same as never configured
                config.min_max_configured = 0;
                config.min_answer = 0;
                config.max_answer = 0;
            } else {
                require!(min_answer <= max_answer, StoreError::InvalidMinMaxRange);
                config.min_max_configured = 1;
                config.min_answer = min_answer;
                config.max_answer = max_answer;
            }
            events.emit(&MinMaxAnswerSet {
                state: state_key,
                data_id: *id,
                min_answer,
                max_answer,
            })
        },
    )
}

/// Disable or enable writes of feeds: while disabled, every workflow's reports for the data ID are
/// skipped (default and feed-specific alike) and reads keep serving the last round. Disabling a data
/// ID whose feed doesn't exist yet keeps its first report from creating it.
pub fn set_feed_writes_disabled<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminConfigs<'info>>,
    data_ids: Vec<[u8; 16]>,
    disabled: bool,
) -> Result<()> {
    let state_key = ctx.accounts.state.key();
    update_configs(
        &ctx,
        &data_ids,
        ctx.remaining_accounts,
        |_, id, config, events| {
            config.writes_disabled = disabled as u8;
            events.emit(&FeedWritesDisabledSet {
                state: state_key,
                data_id: *id,
                disabled,
            })
        },
    )
}

/// Link data IDs to the DF1 feeds this store keeps written with their rounds (default pubkey
/// unlinks), and enable or disable that write. Remaining accounts: the feed configs, then each
/// linked DF1 feed account in order, which must be a DF1 feed `submit` can write. A DF1 feed should
/// be linked from one data ID only; the deployment tooling checks that across configs.
pub fn set_df1_feeds<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminConfigs<'info>>,
    data_ids: Vec<[u8; 16]>,
    df1_feeds: Vec<Pubkey>,
    write_disabled: Vec<bool>,
) -> Result<()> {
    require!(
        data_ids.len() == df1_feeds.len() && data_ids.len() == write_disabled.len(),
        StoreError::ArrayLengthMismatch
    );
    let linked = df1_feeds
        .iter()
        .filter(|feed| **feed != Pubkey::default())
        .count();
    require_eq!(
        ctx.remaining_accounts.len(),
        data_ids.len() + linked,
        StoreError::InvalidAccountCount
    );
    let (configs, df1_infos) = ctx.remaining_accounts.split_at(data_ids.len());
    let mut df1_infos = df1_infos.iter();
    let (state_key, df1_program) = {
        let state = ctx.accounts.state.load()?;
        (ctx.accounts.state.key(), state.df1_program)
    };
    update_configs(&ctx, &data_ids, configs, |i, id, config, events| {
        let df1_feed = df1_feeds[i];
        if df1_feed != Pubkey::default() {
            let info = df1_infos.next().unwrap();
            require_keys_eq!(*info.key, df1_feed, StoreError::AccountMismatch);
            require!(
                df1::check_layout(info, &df1_program).is_ok(),
                StoreError::InvalidDf1Feed
            );
        }
        config.df1_feed = df1_feed;
        config.df1_write_disabled = write_disabled[i] as u8;
        events.emit(&Df1FeedSet {
            state: state_key,
            data_id: *id,
            df1_feed,
            write_disabled: write_disabled[i],
        })
    })
}

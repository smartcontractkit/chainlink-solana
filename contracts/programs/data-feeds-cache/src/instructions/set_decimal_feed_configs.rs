use crate::{
    common::ANCHOR_DISCRIMINATOR,
    context::SetDecimalFeedConfigs,
    error::DataCacheError,
    event::DecimalFeedConfigSet,
    state::WritePermissionFlag,
    utils::{
        create_account, create_report_hash, get_decimals, permission_flag, sorted_insert,
        validate_feed_config_inputs, verify_feed_admin,
    },
    FeedConfig, WorkflowMetadata, ZERO_DATA_ID,
};
use anchor_lang::{prelude::*, Discriminator};

pub fn handler<'info>(
    ctx: Context<'_, '_, 'info, 'info, SetDecimalFeedConfigs<'info>>,
    data_ids: Vec<[u8; 16]>,
    descriptions: Vec<[u8; 32]>,
    workflow_metadatas: Vec<WorkflowMetadata>,
) -> Result<()> {
    // check feed admin here
    let state = &mut ctx.accounts.state.load()?;
    verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;

    validate_feed_config_inputs(&data_ids, &descriptions, &workflow_metadatas)?;

    // check the remaining accounts length has sufficient feed config and permission accounts
    let minimum_len = data_ids.len() + data_ids.len() * workflow_metadatas.len();

    // you have an unknown of defunct permission accounts as well, so as long as the amount is >= we're good
    require_gte!(
        ctx.remaining_accounts.len(),
        minimum_len,
        DataCacheError::InvalidAccountCount
    );

    // require ctx.remaining_accounts are in the correct order [ [...feed_config] [...permission_flags] ]
    let feed_config_account_infos = &ctx.remaining_accounts[..data_ids.len()];
    let index = data_ids.len() + data_ids.len() * workflow_metadatas.len();
    let permission_flag_account_infos = &ctx.remaining_accounts[data_ids.len()..index];
    let delete_permission_account_infos = &ctx.remaining_accounts[index..];

    let cache_state_key = ctx.accounts.state.key();

    let mut delete_permission_accounts: Vec<Pubkey> = Vec::new();

    for (i, curr_data_id) in data_ids.iter().enumerate() {
        require!(*curr_data_id != ZERO_DATA_ID, DataCacheError::InvalidDataId);

        // derive the PDA
        // get the existing config feed, see if it's empty or not
        let (curr_feed_config, feed_config_bump) = Pubkey::find_program_address(
            &[b"feed_config", cache_state_key.as_ref(), curr_data_id],
            &crate::ID,
        );

        // the feed config accounts should be in order
        require_keys_eq!(
            *feed_config_account_infos[i].key,
            curr_feed_config,
            DataCacheError::AccountMismatch
        );

        let feed_config_exists = !feed_config_account_infos[i].data_is_empty();

        let feed_config_loader = if feed_config_exists {
            AccountLoader::<FeedConfig>::try_from(&feed_config_account_infos[i])?
        } else {
            let space = ANCHOR_DISCRIMINATOR + FeedConfig::INIT_SPACE;
            // initialize it
            let seeds: &[&[u8]] = &[
                b"feed_config",
                cache_state_key.as_ref(),
                curr_data_id,
                &[feed_config_bump],
            ];

            create_account(
                space,
                seeds,
                ctx.accounts.feed_admin.clone(),
                feed_config_account_infos[i].clone(),
                crate::ID,
                ctx.accounts.system_program.to_account_info(),
            )?;

            // avoid double borrow to write discriminator
            {
                let mut dst = feed_config_account_infos[i].try_borrow_mut_data()?;
                dst[..ANCHOR_DISCRIMINATOR].copy_from_slice(&FeedConfig::discriminator());
            }

            AccountLoader::<FeedConfig>::try_from(&feed_config_account_infos[i])?
        };

        // load_mut instead of load_init because we write the discriminator above
        let mut feed_config = feed_config_loader.load_mut()?;

        // sorted
        let mut temp_candidates_deletion: Vec<Pubkey> = Vec::new();

        // so these are the permission accounts you need to delete later
        if feed_config_exists {
            // go over current workflows
            for metadata in feed_config.workflow_metadata.iter() {
                // these entries are not to be deleted yet... we'll find out at the end if we need to delete them
                let (permission_flag, _) =
                    permission_flag(metadata, curr_data_id, cache_state_key.as_ref());

                sorted_insert(&mut temp_candidates_deletion, permission_flag);
            }
        }

        // let mut new_workflow_metadata = Vec::default();

        feed_config.workflow_metadata.clear();

        // go over new workflows to be added
        // inner loop iterates over the workflow metadata
        for (j, metadata) in workflow_metadatas.iter().enumerate() {
            let report_hash = create_report_hash(
                curr_data_id,
                &metadata.allowed_sender,
                &metadata.allowed_workflow_owner,
                &metadata.allowed_workflow_name,
            );
            let (curr_permission_flag, bump) = Pubkey::find_program_address(
                &[
                    b"permission_flag",
                    ctx.accounts.state.key().as_ref(),
                    &report_hash,
                ],
                &crate::ID,
            );

            // ex: data_ids: [1, 2]
            // workflow metadatas [5, 6, 7]
            // ctx remaining accounts:
            // [1-feed-config]  |- feed_config_accounts
            // [2-feed-config]  |
            // [flag-1-5] [flag-1-6] [flag-1-7]  |- permission_flag_accounts
            // [flag-2-5] [flag-2-6] [flag-2-7]  |

            let permission_flag_account_info =
                &permission_flag_account_infos[i * workflow_metadatas.len() + j];

            // check that it is in the remaining accounts
            require_keys_eq!(
                curr_permission_flag,
                *permission_flag_account_info.key,
                DataCacheError::AccountMismatch
            );

            // create permission_flag if needed
            if permission_flag_account_info.data_is_empty() {
                let seeds: &[&[u8]] = &[
                    b"permission_flag",
                    cache_state_key.as_ref(),
                    &report_hash,
                    &[bump],
                ];

                create_account(
                    ANCHOR_DISCRIMINATOR,
                    seeds,
                    ctx.accounts.feed_admin.clone(),
                    permission_flag_account_info.clone(),
                    crate::ID,
                    ctx.accounts.system_program.to_account_info(),
                )?;

                let mut dst = permission_flag_account_info.try_borrow_mut_data()?;
                dst[..ANCHOR_DISCRIMINATOR].copy_from_slice(&WritePermissionFlag::discriminator());
            }

            // ensure the flag has expected schema
            WritePermissionFlag::try_deserialize(
                &mut &permission_flag_account_info.data.borrow()[..],
            )?;

            // permission flag are removed from deletion set because it's still in use
            if let Ok(index) = temp_candidates_deletion.binary_search(&curr_permission_flag) {
                temp_candidates_deletion.remove(index);
            }

            feed_config.workflow_metadata.push(*metadata);
        }

        feed_config.description = descriptions[i];

        emit!(DecimalFeedConfigSet {
            state: ctx.accounts.state.key(),
            data_id: *curr_data_id,
            decimals: get_decimals(curr_data_id),
            description: descriptions[i],
            workflow_metadatas: workflow_metadatas.clone(),
        });

        delete_permission_accounts.append(&mut temp_candidates_deletion)
    }

    require_eq!(
        delete_permission_accounts.len(),
        delete_permission_account_infos.len(),
        DataCacheError::ArrayLengthMismatch
    );

    for (i, permission_account) in delete_permission_accounts.iter().enumerate() {
        let curr_permission_account_info = &delete_permission_account_infos[i];

        require_keys_eq!(
            *permission_account,
            *curr_permission_account_info.key,
            DataCacheError::AccountMismatch
        );

        let account: Account<WritePermissionFlag> =
            Account::try_from(curr_permission_account_info)?;
        account.close(ctx.accounts.feed_admin.to_account_info())?;
    }

    Ok(())
}

use crate::{
    context::PreviewDecimalFeedConfigs,
    error::DataCacheError,
    utils::{permission_flag, sorted_insert, validate_feed_config_inputs},
    FeedConfig, WorkflowMetadata, ZERO_DATA_ID,
};
use anchor_lang::prelude::*;

pub fn handler<'info>(
    ctx: Context<'_, '_, 'info, 'info, PreviewDecimalFeedConfigs<'info>>,
    data_ids: Vec<[u8; 16]>,
    descriptions: Vec<[u8; 32]>,
    workflow_metadatas: Vec<WorkflowMetadata>,
) -> Result<Vec<Pubkey>> {
    validate_feed_config_inputs(&data_ids, &descriptions, &workflow_metadatas)?;

    // check the remaining accounts length has sufficient feed config and permission accounts
    let expected_len = data_ids.len() + data_ids.len() * workflow_metadatas.len();

    require_eq!(
        ctx.remaining_accounts.len(),
        expected_len,
        DataCacheError::InvalidAccountCount
    );

    // require ctx.remaining_accounts are in the correct order [ [...feed_config] [...permission_flags] ]
    let feed_config_account_infos = &ctx.remaining_accounts[..data_ids.len()];
    let permission_flag_account_infos = &ctx.remaining_accounts[data_ids.len()..];

    let cache_state_key = ctx.accounts.state.key();

    let mut delete_permission_accounts: Vec<Pubkey> = Vec::new();

    for (i, curr_data_id) in data_ids.iter().enumerate() {
        require!(*curr_data_id != ZERO_DATA_ID, DataCacheError::InvalidDataId);

        let (curr_feed_config, _) = Pubkey::find_program_address(
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

        // sorted
        let mut temp_candidates_deletion: Vec<Pubkey> = Vec::new();

        if feed_config_exists {
            let feed_config_loader =
                AccountLoader::<FeedConfig>::try_from(&feed_config_account_infos[i])?;

            let feed_config = feed_config_loader.load()?;

            for metadata in feed_config.workflow_metadata.iter() {
                // these entries are not to be deleted yet... we'll find out at the end if we need to delete them
                let (permission_flag, _) =
                    permission_flag(metadata, curr_data_id, cache_state_key.as_ref());

                sorted_insert(&mut temp_candidates_deletion, permission_flag)
            }
        }

        for (j, metadata) in workflow_metadatas.iter().enumerate() {
            let (curr_permission_flag, _) =
                permission_flag(metadata, curr_data_id, cache_state_key.as_ref());

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

            // permission flag are removed from deletion set because it's still in use
            if let Ok(index) = temp_candidates_deletion.binary_search(&curr_permission_flag) {
                temp_candidates_deletion.remove(index);
            }
        }

        // add items
        delete_permission_accounts.append(&mut temp_candidates_deletion);
    }

    // order has to be exactly the same
    Ok(delete_permission_accounts)
}

use anchor_lang::prelude::*;

declare_id!("3kX63udXtYcsdj2737Wi2KGd2PhqiKPgAFAxstrjtRUa");

mod context;
mod state;
mod common;
mod error;
mod event;

use context::*;
use state::{AdminList, FeedConfig, WorkflowMetadata};
use error::{DataCacheError, AuthError};
use common::{ZERO_DATA_ID, ZERO_ADDRESS, MAX_WORKFLOW_METADATAS};
use event::{DecimalFeedConfigSet};

use anchor_lang::solana_program::hash;
#[program]
pub mod data_feeds_cache {

    use anchor_lang::{solana_program::{program::invoke_signed, system_instruction}, Discriminator};
    use keystone_forwarder::AuthError;
    use state::WorkflowMetadata;

    use crate::{common::ANCHOR_DISCRIMINATOR, state::WritePermissionFlag};

    use super::*;

    // todo, add feed admins? who will also be a zero_copy array
    pub fn initialize(ctx: Context<Initialize>, feed_admins: Vec<Pubkey>) -> Result<()> {
        let state = &mut ctx.accounts.state.load_init()?;
        state.owner = ctx.accounts.owner.key();

        let mut prev_admin = Pubkey::default();
        for admin in feed_admins.iter() {
            require!(&prev_admin <= admin, DataCacheError::AddressesMustStrictlyIncrease);
            state.feed_admins.push(admin.clone());
            prev_admin = *admin;
        }

        // todo: can probably remove
        state.proposed_owner = Pubkey::default();

        Ok(())
    }

    pub fn set_feed_admin(ctx: Context<SetFeedAdmin>, admin: Pubkey, is_admin: bool) -> Result<()> {
        let state = ctx.accounts.state.load()?;
        require!(ctx.accounts.owner.key == &state.owner, AuthError::Unauthorized);


        // set feed admin . make sure it's sorted and what not
        Ok(())
    }

    pub fn transfer_ownership(ctx: Context<TransferOwnership>, proposed_owner: Pubkey) -> Result<()> {

        Ok(())
    }

    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        
        Ok(())
    }


    pub fn init_legacy_feeds_config(ctx: Context<InitLegacyFeedsConfig>, data_ids: Vec<[u8; 16]>, legacy_feeds: Vec<Pubkey>) -> Result<()> {
          // data_ids are sorted,
        // len(data_ids) == len(legacy_feeds)

        Ok(())
    }

    pub fn set_legacy_feeds_config(ctx: Context<UpdateLegacyFeedsConfig>, data_ids: Vec<[u8; 16]>, legacy_feeds: Vec<Pubkey>, legacy_store: Pubkey) -> Result<()> {
        // data_ids are sorted,
        // len(data_ids) == len(legacy_feeds)
        Ok(())
    }

    pub fn close_legacy_feeds_config(ctx: Context<InitLegacyFeedsConfig>) -> Result<()> {
        Ok(())
    }


    // in general, you always add before removing for making things
    // migration-friendly.
    // so we can add the new feed_configs, keep track of the permissions to be deleted 
    // in a follow up transaction
    pub fn set_decimal_feed_configs<'info>(ctx: Context<'_, '_, 'info, 'info, SetDecimalFeedConfigs<'info>>, data_ids: Vec<[u8; 16]>, descriptions: Vec<[u8; 32]>, workflow_metadatas: Vec<WorkflowMetadata>) -> Result<()> {
        // check feed admin here 
        let state = &mut ctx.accounts.state.load()?;
        verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;

        require!(workflow_metadatas.len() <= MAX_WORKFLOW_METADATAS, DataCacheError::MaxWorkflowsExceeded);

        require!(workflow_metadatas.len() != 0 && descriptions.len() != 0, DataCacheError::EmptyConfig);

        require!(data_ids.len() == descriptions.len(), DataCacheError::ArrayLengthMismatch);

        // check the remaining accounts length has sufficient feed config and permission accounts
        let expected_len = data_ids.len() + data_ids.len()*workflow_metadatas.len();

        require!(ctx.remaining_accounts.len() == expected_len, DataCacheError::MissingAccounts);

        for metadata in workflow_metadatas.iter() {
            require!(metadata.allowed_sender != Pubkey::default(), DataCacheError::InvalidAddress);
            require!(!metadata.allowed_workflow_name.is_empty(), DataCacheError::InvalidWorkflowName);
            require!(metadata.allowed_workflow_owner != ZERO_ADDRESS, DataCacheError::InvalidAddress);
        }

        // require ctx.remaining_accounts are in the correct order [ [...feed_config] [...permission_flags] ]
        let feed_config_account_infos = &ctx.remaining_accounts[..data_ids.len()];
        let permission_flag_account_infos = &ctx.remaining_accounts[data_ids.len()..];

        for (i, curr_data_id) in data_ids.iter().enumerate() {

            require!(*curr_data_id != ZERO_DATA_ID, DataCacheError::InvalidDataId);

            // derive the PDA
            // get the existing config feed , see if it's empty or not
            let (curr_feed_config, feed_config_bump) = Pubkey::find_program_address(&[b"feed_config", curr_data_id], &crate::ID);

            // the feed config accounts should be in order
            require!(feed_config_account_infos[i].key() == curr_feed_config, DataCacheError::AccountMismatch);

            let feed_config_exists = feed_config_account_infos[i].data_len() != 0;

            // todo: make string a [u8; 32]
            let feed_config_loader = if feed_config_exists {
                AccountLoader::<FeedConfig>::try_from(&feed_config_account_infos[i])?
            } else {
                let rent = Rent::get()?.minimum_balance(ANCHOR_DISCRIMINATOR + FeedConfig::INIT_SPACE);
                // initialize it
                let seeds: &[&[u8]] = &[b"feed_config", curr_data_id, &[feed_config_bump]];

                invoke_signed(
                    &system_instruction::create_account(
                        ctx.accounts.feed_admin.key,
                        &curr_feed_config,
                        rent,
                        (ANCHOR_DISCRIMINATOR + FeedConfig::INIT_SPACE) as u64,
                        ctx.program_id,
                    ),
                    &[
                        ctx.accounts.feed_admin.to_account_info(),
                        feed_config_account_infos[i].clone(),
                        ctx.accounts.system_program.to_account_info(),
                    ],
                    &[seeds],
                )?;

                AccountLoader::<FeedConfig>::try_from(&feed_config_account_infos[i])?
            };

            let mut feed_config = if feed_config_exists {
                feed_config_loader.load_mut()?
            } else {
                feed_config_loader.load_init()?
            };

            let mut stale_permission_flag_accounts: Vec<Pubkey> = Vec::new();

            // so these are the permission accounts you need to delete later
            if feed_config_exists {
                // check that the stale accounts are empty
                require!(feed_config.stale_permission_accounts.is_empty(), DataCacheError::UnclosedPermissionFlags);

                for (i, metadata) in feed_config.workflow_metadata.iter().enumerate() {
                    // these entries are not to be deleted yet... we'll find out at the end if we need to delete them

                    // todo: should you store the nonce? or permission account in metadata? i don't think it is necessary
                    let (permission_flag, _) = Pubkey::find_program_address(
                        &[b"permission", &create_report_hash(curr_data_id, &metadata.allowed_sender, &metadata.allowed_workflow_owner, &metadata.allowed_workflow_name)],
                        &crate::ID,
                    );

                    stale_permission_flag_accounts.push(permission_flag);
                }
            }


            // let mut new_workflow_metadata = Vec::default();

            feed_config.workflow_metadata.clear();

            // inner loop iterates over the workflow metadata
            for (j, metadata) in workflow_metadatas.iter().enumerate() {

                let report_hash = create_report_hash(curr_data_id, &metadata.allowed_sender, &metadata.allowed_workflow_owner, &metadata.allowed_workflow_name);
                let (curr_permission_flag, bump) = Pubkey::find_program_address(
                    &[b"permission_flag", &report_hash],
                    &crate::ID,
                );

                // ex: data_ids: [1, 2]
                // workflow metdatas [5, 6, 7]
                // ctx remaining accounts: 
                // [1-feed-config]
                // [2-feed-config] 
                // [flag-1-5] [flag-1-6] [flag-1-7] 
                // [flag-2-5] [flag-2-6] [flag-2-7]

                let permission_flag_account_info = &permission_flag_account_infos[data_ids.len() + i*workflow_metadatas.len() + j];

                // check that it is in the remaining accounts
                require!(&curr_permission_flag == permission_flag_account_info.key, DataCacheError::AccountMismatch);

                // create permission_flag if needed
                if permission_flag_account_info.data_is_empty() {
                    let rent = Rent::get()?.minimum_balance(ANCHOR_DISCRIMINATOR);

                    let seeds: &[&[u8]] = &[b"permission_flag", &report_hash, &[bump]];

                    invoke_signed(
                        &system_instruction::create_account(
                            ctx.accounts.feed_admin.key,
                            &curr_permission_flag,
                            rent,
                            ANCHOR_DISCRIMINATOR as u64,
                            ctx.program_id,
                        ),
                        &[
                            ctx.accounts.feed_admin.to_account_info(),
                            permission_flag_account_info.clone(),
                            ctx.accounts.system_program.to_account_info(),
                        ],
                        &[seeds],
                    )?;

                    let mut dst = permission_flag_account_info.try_borrow_mut_data()?;
                    dst[..ANCHOR_DISCRIMINATOR].copy_from_slice(&WritePermissionFlag::discriminator());
                } 

                // ensure the flag has expected schema
                WritePermissionFlag::try_deserialize(&mut &permission_flag_account_info.data.borrow()[..])?;

                // permission flag are removed from stale set because it's still in use
                if let Some(index) =  stale_permission_flag_accounts.iter().position(|&a| a == curr_permission_flag) {
                    stale_permission_flag_accounts.remove(index);
                }

                feed_config.workflow_metadata.push(metadata.clone());
                // new_workflow_metadata.push(metadata.clone());
            }

            feed_config.description = descriptions[i].clone();

            feed_config.stale_permission_accounts.clear();
            stale_permission_flag_accounts.iter().for_each(|f| {
                feed_config.stale_permission_accounts.push(f.clone());
            });

            // can also move to inside the for loop
            // feed_config.workflow_metadata.clear();
            // workflow_metadatas.iter().for_each(|w| {
            //     feed_config.workflow_metadata.push(w.clone());
            // });


            emit!(DecimalFeedConfigSet {
                data_id: curr_data_id.clone(),
                decimals: get_decimals(curr_data_id),
                description: descriptions[i].clone(),
                workflow_metadatas: workflow_metadatas.clone(),
                stale_permission_flags: stale_permission_flag_accounts
            });

        };
        // for each data id 

            // for each workflow_metadata

            // check the feed_config account. 
            // get the report hash, and add it to a set

            // no need to delete the feed_config since we'll overwrite it

            // for each workflow metadata
                // fill in the feed_config account
                // write permission enabled, and if it exists in that earlier set, remove it

                // but you need to know the accounts you need to delete beforehand
                // so this function needs to be split up

                // needs to be deployment-friendly for adding or removing workflow for data id

        // it gets the report_hash from the feed_config 
        // then it checks the permission of the report_hash
        Ok(())
    }

    pub fn on_report(ctx: Context<OnReport>) -> Result<()> {

        Ok(())
    }

    // pub fn set_decimal_feed_configs(ctx: Context<SetDecimalFeedConfigs>, data_ids: Vec<[u8; 2]>, descriptions: Vec<String>, workflow_metadatas: Vec<WorkflowMetadata>) -> Result<()> {
    //     Ok(())
    // }

    // // on-chain sdk helper reads from multiple data accounts
    // // off-chain would read the individual data accounts associated with each one
    // // are these even a priority for BNY though? 
    // pub fn get_feed_metadata(ctx: Context<GetFeedMetadata>, data_ids: Vec<[u8; 2]>, start_index: usize, max_count: usize) -> Result<()> {
    //     Ok(())
    // }





}

fn verify_feed_admin(admin: &Signer, admin_list: &AdminList) -> Result<()> {
    let is_admin = admin_list.binary_search(admin.key).is_ok();
    require!(is_admin, AuthError::Unauthorized);

    Ok(())
}

fn create_report_hash(data_id: &[u8], sender: &Pubkey, owner: &[u8], name: &[u8]) -> [u8; 32] {
    hash::hash(&[data_id, &sender.to_bytes(), owner, name].concat()).to_bytes()
}

fn get_decimals(data_id: &[u8; 16]) -> u8 {
    let report_type = data_id[7];

    if report_type >= 0x20 && report_type <= 0x60 {
        report_type - 32
    } else {
        0
    }
}
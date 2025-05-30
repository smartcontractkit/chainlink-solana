
use anchor_lang::prelude::*;
use anchor_lang::__private::CLOSED_ACCOUNT_DISCRIMINATOR;
use std::{io::Write, ops::DerefMut};
use std::io::Cursor;

declare_id!("3kX63udXtYcsdj2737Wi2KGd2PhqiKPgAFAxstrjtRUa");

mod context;
mod state;
mod common;
mod error;
mod event;

use context::*;
use state::{AdminList, FeedConfig, LegacyFeedEntry, LegacyFeedsConfig, ReceivedDecimalReport, WorkflowMetadata};
use error::{DataCacheError, AuthError};
use common::{ZERO_DATA_ID, ZERO_ADDRESS, MAX_WORKFLOW_METADATAS};
use event::{DecimalFeedConfigSet};

use anchor_lang::solana_program::hash;
#[program]
pub mod data_feeds_cache {

    use anchor_lang::{solana_program::{program::invoke_signed, system_instruction}, Discriminator};
    use state::{ReceivedDecimalReport, WorkflowMetadata};

    use crate::{common::ANCHOR_DISCRIMINATOR, state::{DecimalReport, LegacyFeedEntry, WritePermissionFlag}};

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
        let mut state = ctx.accounts.state.load_mut()?;

        match state.feed_admins.binary_search_by(|a| { a.cmp(&admin) }) {
            Ok(i) => {
                if !is_admin {
                    state.feed_admins.remove(i);
                }
            }, 
            Err(i) => {
                if is_admin {
                    state.feed_admins.insert(i, admin);
                }
            }
        };
        Ok(())
    }

    pub fn transfer_ownership(ctx: Context<TransferOwnership>, proposed_owner: Pubkey) -> Result<()> {
        let state = &mut ctx.accounts.state.load_mut()?;
        state.proposed_owner = proposed_owner;

        Ok(())
    }

    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        let state = &mut ctx.accounts.state.load_mut()?;

        state.owner = state.proposed_owner;
        state.proposed_owner = Pubkey::default();

        Ok(())
    }

    pub fn init_decimal_reports<'info>(ctx: Context<'_, '_, 'info, 'info, InitDecimalReports<'info>>, data_ids: Vec<[u8; 16]>) -> Result<()> {
         // check feed admin here 
         let state = &mut ctx.accounts.state.load()?;
         verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;

        let state_key = ctx.accounts.state.key();

        let data_ids_account_infos = ctx.remaining_accounts;

        require!(data_ids.len() == data_ids_account_infos.len(), DataCacheError::ArrayLengthMismatch);

        for (i, data_id) in data_ids.iter().enumerate() {

            let curr_report_account_info = &data_ids_account_infos[i];

            let (decimal_report, bump) = Pubkey::find_program_address(
                &[b"decimal_report", state_key.as_ref(), data_id],
                &crate::ID,
            );

            require!(&decimal_report == curr_report_account_info.key, DataCacheError::AccountMismatch);

            if curr_report_account_info.data_is_empty() {
                let rent = Rent::get()?.minimum_balance(ANCHOR_DISCRIMINATOR + DecimalReport::INIT_SPACE);

                let seeds: &[&[u8]] = &[b"decimal_report", state_key.as_ref(), data_id, &[bump]];

                invoke_signed(
                    &system_instruction::create_account(
                        ctx.accounts.feed_admin.key,
                        &decimal_report,
                        rent,
                        (ANCHOR_DISCRIMINATOR + DecimalReport::INIT_SPACE) as u64,
                        ctx.program_id,
                    ),
                    &[
                        ctx.accounts.feed_admin.to_account_info(),
                        curr_report_account_info.clone(),
                        ctx.accounts.system_program.to_account_info(),
                    ],
                    &[seeds],
                )?;

                let mut dst = curr_report_account_info.try_borrow_mut_data()?;
                dst[..ANCHOR_DISCRIMINATOR].copy_from_slice(&DecimalReport::discriminator());
            }

            // todo: probably optional... just makes sure the account is the right type
            DecimalReport::try_deserialize(&mut &curr_report_account_info.data.borrow()[..])?;
        }

        Ok(())
    }

    // todo: should i write out the entire logic for this? it should
    // probably query the contract... and feed ids of each one as a precaution?
    pub fn init_legacy_feeds_config(ctx: Context<InitLegacyFeedsConfig>, data_ids: Vec<[u8; 16]>) -> Result<()> {
        set_legacy_feeds_config(
            true, &mut ctx.accounts.legacy_feeds_config, ctx.accounts.legacy_store.key(), ctx.remaining_accounts, data_ids
        )
    }

    pub fn update_legacy_feeds_config(ctx: Context<UpdateLegacyFeedsConfig>, data_ids: Vec<[u8; 16]>) -> Result<()> {
        set_legacy_feeds_config(
            false, &mut ctx.accounts.legacy_feeds_config, ctx.accounts.legacy_store.key(), ctx.remaining_accounts, data_ids
        )
    }

    pub fn close_legacy_feeds_config(_ctx: Context<CloseLegacyFeedsConfig>) -> Result<()> {
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

        let cache_state_key = ctx.accounts.state.key();

        for (i, curr_data_id) in data_ids.iter().enumerate() {

            require!(*curr_data_id != ZERO_DATA_ID, DataCacheError::InvalidDataId);

            // derive the PDA
            // get the existing config feed , see if it's empty or not
            let (curr_feed_config, feed_config_bump) = Pubkey::find_program_address(
                &[b"feed_config", cache_state_key.as_ref(), curr_data_id], 
                &crate::ID
            );

            // the feed config accounts should be in order
            require!(feed_config_account_infos[i].key() == curr_feed_config, DataCacheError::AccountMismatch);

            let feed_config_exists = feed_config_account_infos[i].data_len() != 0;

            // todo: make string a [u8; 32]
            let feed_config_loader = if feed_config_exists {
                AccountLoader::<FeedConfig>::try_from(&feed_config_account_infos[i])?
            } else {
                let rent = Rent::get()?.minimum_balance(ANCHOR_DISCRIMINATOR + FeedConfig::INIT_SPACE);
                // initialize it
                let seeds: &[&[u8]] = &[b"feed_config", cache_state_key.as_ref(), curr_data_id, &[feed_config_bump]];

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

                // avoid double borrow to write discriminator
                {
                    let mut dst = feed_config_account_infos[i].try_borrow_mut_data()?;
                    dst[..ANCHOR_DISCRIMINATOR].copy_from_slice(&FeedConfig::discriminator());
                }

                AccountLoader::<FeedConfig>::try_from(&feed_config_account_infos[i])?
            };


            // load_mut instead of load_mut because we write the discriminator above
            let mut feed_config = feed_config_loader.load_mut()?;

            let mut stale_permission_flag_accounts: Vec<Pubkey> = Vec::new();

            // so these are the permission accounts you need to delete later
            if feed_config_exists {
                // check that the stale accounts are empty
                require!(feed_config.stale_permission_accounts.is_empty(), DataCacheError::UnclosedPermissionFlags);

                for (i, metadata) in feed_config.workflow_metadata.iter().enumerate() {
                    // these entries are not to be deleted yet... we'll find out at the end if we need to delete them

                    let derived_report_hash = create_report_hash(
                        curr_data_id, 
                        &metadata.allowed_sender, 
                        &metadata.allowed_workflow_owner, 
                        &metadata.allowed_workflow_name
                    );
                    // todo: should you store the nonce? or permission account in metadata? i don't think it is necessary
                    let (permission_flag, _) = Pubkey::find_program_address(
                        &[b"permission_flag", cache_state_key.as_ref(), &derived_report_hash],
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
                    &[b"permission_flag", ctx.accounts.state.key().as_ref(), &report_hash],
                    &crate::ID,
                );

                // ex: data_ids: [1, 2]
                // workflow metdatas [5, 6, 7]
                // ctx remaining accounts: 
                // [1-feed-config]  |- feed_config_accounts
                // [2-feed-config]  |
                // [flag-1-5] [flag-1-6] [flag-1-7]  |- permission_flag_accounts
                // [flag-2-5] [flag-2-6] [flag-2-7]  |

                let permission_flag_account_info = &permission_flag_account_infos[i*workflow_metadatas.len() + j];

                // check that it is in the remaining accounts
                require!(&curr_permission_flag == permission_flag_account_info.key, DataCacheError::AccountMismatch);

                // create permission_flag if needed
                if permission_flag_account_info.data_is_empty() {
                    let rent = Rent::get()?.minimum_balance(ANCHOR_DISCRIMINATOR);

                    let seeds: &[&[u8]] = &[b"permission_flag", cache_state_key.as_ref(), &report_hash, &[bump]];

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

    pub fn close_stale_permission_accounts<'info>(ctx: Context<'_, '_, 'info, 'info, CloseStalePermissionAccounts<'info>>, data_ids: Vec<[u8; 16]>) -> Result<()> {
        let feed_config_account_infos = &ctx.remaining_accounts[..data_ids.len()];
        let stale_permission_flag_account_infos = &ctx.remaining_accounts[data_ids.len()..];
       
       let mut flag_idx = 0;
       // for each of the data ids , read the account. should be in order.
       for (i, curr_data_id) in data_ids.iter().enumerate() {
            require!(*curr_data_id != ZERO_DATA_ID, DataCacheError::InvalidDataId);

            // derive the PDA
            // get the existing config feed , see if it's empty or not
            let (curr_feed_config_key, _feed_config_bump) = Pubkey::find_program_address(
                &[b"feed_config", ctx.accounts.state.key().as_ref(), curr_data_id], 
                &crate::ID
            );

            // the feed config accounts should be in order
            require!(feed_config_account_infos[i].key() == curr_feed_config_key, DataCacheError::AccountMismatch);

            let loader = AccountLoader::<FeedConfig>::try_from(&feed_config_account_infos[i])?;

            let mut curr_feed_config = loader.load_mut()?;
            // close stale accounts
            for (i, stale_account_key) in curr_feed_config.stale_permission_accounts.iter().enumerate() {
                let curr_stale_account_info = stale_permission_flag_account_infos[flag_idx].clone();
                require!(stale_account_key == curr_stale_account_info.key, DataCacheError::AccountMismatch);
                close_account(curr_stale_account_info, ctx.accounts.feed_admin.to_account_info())?;
                flag_idx += 1;
            }

            curr_feed_config.stale_permission_accounts.clear();
       }

        Ok(())
    }

    // // todo: change report and metadata to &[u8]
    pub fn on_report(ctx: Context<OnReport>, metadata: Vec<u8>, report: Vec<u8>) -> Result<()> {
        // todo: check if legacy_store and legacy_feed_config are both there

        // first assume we don't have legacy_store or legacy_feed_config

        let (workflow_name, workflow_owner) = get_workflow_metadata(&metadata)?;

        let received_decimal_reports = Vec::<ReceivedDecimalReport>::try_from_slice(&report[..])?;


        let report_account_infos = &ctx.remaining_accounts[..received_decimal_reports.len()];
        // todo: adjust indexing if we have legacy store
        let permission_flag_account_infos = &ctx.remaining_accounts[received_decimal_reports.len()..]; 

        require!(
            report_account_infos.len() == received_decimal_reports.len() &&
            permission_flag_account_infos.len() == received_decimal_reports.len(), 
            DataCacheError::ArrayLengthMismatch
        );


        for (i, received_decimal_report) in received_decimal_reports.iter().enumerate() {
            let ReceivedDecimalReport { data_id, answer, timestamp } = received_decimal_report;

            // panic!("len {:?} , data_id {:?} , forwarder authority {:?} , workflow owner {:?}, workflow name {:?}", 
            // received_decimal_reports.len(), data_id,  &ctx.accounts.forwarder_authority.key(), workflow_owner, workflow_name
            
            // );
            // 1. check that sender has permission to write

                let report_hash = create_report_hash(
                data_id, 
                &ctx.accounts.forwarder_authority.key(), 
                workflow_owner, 
                workflow_name
            );

            let (curr_permission_flag, _) = Pubkey::find_program_address(
                &[b"permission_flag", ctx.accounts.cache_state.key().as_ref(), &report_hash],
                &crate::ID,
            );

            require!(&curr_permission_flag == permission_flag_account_infos[i].key, DataCacheError::AccountMismatch);
            
            // verifies the permission account exists
            WritePermissionFlag::try_deserialize(
                &mut &permission_flag_account_infos[i].data.borrow()[..]
            ).map_err(|_| DataCacheError::InvalidUpdatePermission)?;

            // 2. check report account is valid
            let (curr_report, _) = Pubkey::find_program_address(
                &[b"decimal_report", ctx.accounts.cache_state.key().as_ref(), data_id],
                &crate::ID,
            );

            require!(&curr_report == report_account_infos[i].key, DataCacheError::AccountMismatch);

            let mut dst = report_account_infos[i].try_borrow_mut_data()?;

            let updated_report = DecimalReport{
                answer: answer.clone(), 
                timestamp: timestamp.clone()
            };

            updated_report.serialize(&mut &mut dst[ANCHOR_DISCRIMINATOR..])?;

            // todo: add event here

        };

        Ok(())
    }


    // pub fn on_report(ctx: Context<TestOption>) -> Result<()> {
    //     match &ctx.accounts.legacy_store {
    //         Some(x) => {
    //             let y = x.key();
    //             // panic!("its some!, {:?}", x.key());
    //             // panic!("its some! {:}", y);
    //         },
    //         None => {
    //             panic!("its none!")
    //         },
    //     }

    //     // match ctx.accounts.cache_state {
    //     //     Some(_) => {
    //     //         panic!("its some");
    //     //     },
    //     //     None => {
    //     //         panic!("its none");
    //     //     }

    //     // };

    //     // panic!("len {:?} and key {:?} ", ctx.remaining_accounts.len(), ctx.remaining_accounts[0].key());

    //     Ok(())
    // }

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

fn close_account(account: AccountInfo, destination: AccountInfo) -> Result<()> {
    **destination.lamports.borrow_mut() = destination
        .lamports()
        .checked_add(account.lamports())
        .unwrap();
    **account.lamports.borrow_mut() = 0;

    let mut data = account.try_borrow_mut_data()?;
    for byte in data.deref_mut().iter_mut() {
        *byte = 0;
    }

    let dst: &mut [u8] = &mut data;
    let mut cursor = Cursor::new(dst);
    cursor.write_all(&CLOSED_ACCOUNT_DISCRIMINATOR).unwrap();

    Ok(())
}

// workflow_cid           offset  0, size 32
// workflow_name          offset  32, size 10
// workflow_owner         offset  42, size 20
// report_id              offset  62, size  2
fn get_workflow_metadata(metadata: &[u8]) -> Result<(&[u8], &[u8])> {
    let workflow_name = metadata.get(32..42).ok_or(DataCacheError::OutOfBounds)?;
    let workflow_owner = metadata.get(42..62).ok_or(DataCacheError::OutOfBounds)?;

    Ok((workflow_name, workflow_owner))
}

fn set_legacy_feeds_config(init: bool, config: &mut AccountLoader<LegacyFeedsConfig>, legacy_store: Pubkey, legacy_feeds: &[AccountInfo], data_ids: Vec<[u8; 16]>) -> Result<()> {
    require!(data_ids.len() == legacy_feeds.len(), DataCacheError::ArrayLengthMismatch);

    let mut legacy_feeds_config = if init {
        config.load_init()?
    } else {
        config.load_mut()?
    };

    // reset the array
    legacy_feeds_config.id_to_feed.clear();

    legacy_feeds_config.legacy_store = legacy_store;

    let mut prev_data_id = [0_u8; 16];
    for (i, data_id) in data_ids.iter().enumerate() {
        require!(prev_data_id < *data_id, DataCacheError::IdsMustStrictlyIncrease);

        legacy_feeds_config.id_to_feed.push(LegacyFeedEntry { data_id: data_id.clone(), legacy_feed: legacy_feeds[i].key() });

        prev_data_id = *data_id; 
    };

    Ok(())
}
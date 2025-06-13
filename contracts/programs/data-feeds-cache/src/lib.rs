use anchor_lang::__private::CLOSED_ACCOUNT_DISCRIMINATOR;
use anchor_lang::prelude::*;
use std::io::Cursor;
use std::{io::Write, ops::DerefMut};

declare_id!("3kX63udXtYcsdj2737Wi2KGd2PhqiKPgAFAxstrjtRUa");

mod common;
mod context;
mod error;
mod event;
pub mod state;

use anchor_lang::solana_program::hash;
use common::{ANCHOR_DISCRIMINATOR, MAX_WORKFLOW_METADATAS, ZERO_ADDRESS, ZERO_DATA_ID};
use context::*;
use error::{AuthError, DataCacheError};
use event::{DecimalFeedConfigSet, LegacyFeedsReported};
use state::{
    AccountList, CacheTransmission, DecimalReport, FeedConfig, LegacyFeedEntry, LegacyFeedsConfig,
    ReceivedDecimalReport, WorkflowMetadata, WritePermissionFlag,
};

// derived from hash::hash("global:cache_submit".as_bytes()).to_bytes()[..8].to_vec()
pub const SUBMIT_DISCRIMINATOR: [u8; 8] = [173, 69, 171, 96, 179, 143, 243, 226];

#[program]
pub mod data_feeds_cache {

    use anchor_lang::{
        prelude::borsh::BorshSerialize,
        solana_program::{instruction::Instruction, program::invoke_signed, system_instruction},
        Discriminator,
    };

    use crate::event::{
        CacheInitialized, DecimalReportClosed, DecimalReportInitialized, DecimalReportUpdate, FeedAdminUpdated, InvalidUpdatePermission, LegacyFeedsConfigInitialized, LegacyFeedsConfigUpdated, OwnershipAcceptance, OwnershipTransfer, StaleDecimalReport
    };

    use super::*;

    pub fn initialize(ctx: Context<Initialize>, feed_admins: Vec<Pubkey>) -> Result<()> {
        let state = &mut ctx.accounts.state.load_init()?;
        state.owner = ctx.accounts.owner.key();

        let mut prev_admin = Pubkey::default();
        for admin in feed_admins.iter() {
            require!(
                &prev_admin < admin,
                DataCacheError::AddressesMustStrictlyIncrease
            );
            state.feed_admins.push(*admin);
            prev_admin = *admin;
        }

        let (_, authority_nonce) = Pubkey::find_program_address(
            &[b"legacy_writer", ctx.accounts.state.key().as_ref()],
            &crate::ID,
        );
        state.legacy_writer_nonce = authority_nonce;

        emit!({
            CacheInitialized{
                state: ctx.accounts.state.key()
            }
        });

        Ok(())
    }

    pub fn set_feed_admin(ctx: Context<SetFeedAdmin>, admin: Pubkey, is_admin: bool) -> Result<()> {
        let mut state = ctx.accounts.state.load_mut()?;
        let mut changed = false;

        match state.feed_admins.binary_search_by(|a| a.cmp(&admin)) {
            Ok(i) => {
                if !is_admin {
                    state.feed_admins.remove(i);
                    changed = true;
                }
            }
            Err(i) => {
                if is_admin {
                    state.feed_admins.insert(i, admin);
                    changed = true;
                }
            }
        }

        if changed {
            emit!(FeedAdminUpdated { admin, is_admin });
        }

        Ok(())
    }

    pub fn transfer_ownership(
        ctx: Context<TransferOwnership>,
        proposed_owner: Pubkey,
    ) -> Result<()> {
        let state = &mut ctx.accounts.state.load_mut()?;
        state.proposed_owner = proposed_owner;

        emit!(OwnershipTransfer {
            current_owner: state.owner,
            proposed_owner: proposed_owner
        });

        Ok(())
    }

    pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
        let state = &mut ctx.accounts.state.load_mut()?;

        emit!(OwnershipAcceptance {
            previous_owner: state.owner,
            new_owner: state.proposed_owner
        });

        state.owner = std::mem::take(&mut state.proposed_owner);

        Ok(())
    }

    pub fn close_decimal_reports(
        ctx: Context<CloseDecimalReports>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        let state = &ctx.accounts.state.load()?;
        verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;

        let state_key = ctx.accounts.state.key();

        let data_ids_account_infos = ctx.remaining_accounts;

        require_eq!(
            data_ids.len(),
            data_ids_account_infos.len(),
            DataCacheError::ArrayLengthMismatch
        );

        for (i, data_id) in data_ids.iter().enumerate() {
            let curr_report_account_info = &data_ids_account_infos[i];

            let (decimal_report, _) = Pubkey::find_program_address(
                &[b"decimal_report", state_key.as_ref(), data_id],
                &crate::ID,
            );

            require_keys_eq!(
                decimal_report,
                *curr_report_account_info.key,
                DataCacheError::AccountMismatch
            );

            close_account(
                curr_report_account_info.clone(),
                ctx.accounts.feed_admin.to_account_info(),
            )?;

            emit!(DecimalReportClosed { data_id: *data_id });
        }

        Ok(())
    }

    pub fn init_decimal_reports<'info>(
        ctx: Context<'_, '_, 'info, 'info, InitDecimalReports<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {
        // check feed admin here
        let state = &ctx.accounts.state.load()?;
        verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;

        let state_key = ctx.accounts.state.key();

        let data_ids_account_infos = ctx.remaining_accounts;

        require_eq!(
            data_ids.len(),
            data_ids_account_infos.len(),
            DataCacheError::ArrayLengthMismatch
        );

        for (i, data_id) in data_ids.iter().enumerate() {
            let curr_report_account_info = &data_ids_account_infos[i];

            let (decimal_report, bump) = Pubkey::find_program_address(
                &[b"decimal_report", state_key.as_ref(), data_id],
                &crate::ID,
            );

            require_keys_eq!(
                decimal_report,
                *curr_report_account_info.key,
                DataCacheError::AccountMismatch
            );

            // only initialize if required
            if curr_report_account_info.data_is_empty() {
                let rent =
                    Rent::get()?.minimum_balance(ANCHOR_DISCRIMINATOR + DecimalReport::INIT_SPACE);

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

                emit!(DecimalReportInitialized { data_id: *data_id });
            }

            // todo: probably optional... just makes sure the account is the right type
            DecimalReport::try_deserialize(&mut &curr_report_account_info.data.borrow()[..])?;
        }

        Ok(())
    }

    pub fn init_legacy_feeds_config(
        ctx: Context<InitLegacyFeedsConfig>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<()> {

        emit!(LegacyFeedsConfigInitialized { 
            config: ctx.accounts.legacy_feeds_config.key()
        });

        set_legacy_feeds_config(
            true,
            &mut ctx.accounts.legacy_feeds_config,
            ctx.accounts.legacy_store.key(),
            ctx.remaining_accounts,
            &data_ids,
            &vec![0_u8; data_ids.len()],
        )
    }

    pub fn update_legacy_feeds_config(
        ctx: Context<UpdateLegacyFeedsConfig>,
        data_ids: Vec<[u8; 16]>,
        write_disabled: Vec<bool>,
    ) -> Result<()> {
        
        emit!(LegacyFeedsConfigUpdated {
            config: ctx.accounts.legacy_feeds_config.key()
        });

        let write_disabled: Vec<u8> = write_disabled
            .iter()
            .copied() // &bool → bool
            .map(|f| f as u8)
            .collect();

        set_legacy_feeds_config(
            false,
            &mut ctx.accounts.legacy_feeds_config,
            ctx.accounts.legacy_store.key(),
            ctx.remaining_accounts,
            &data_ids,
            &write_disabled,
        )
    }

    pub fn close_legacy_feeds_config(_ctx: Context<CloseLegacyFeedsConfig>) -> Result<()> {
        Ok(())
    }

    // previews the permission accounts to be closed when
    // calling set_decimal_feed_configs
    // used for tx simulation only
    // no state changes
    pub fn preview_decimal_feed_configs<'info>(
        ctx: Context<'_, '_, 'info, 'info, PreviewDecimalFeedConfigs<'info>>,
        data_ids: Vec<[u8; 16]>,
        descriptions: Vec<[u8; 32]>,
        workflow_metadatas: Vec<WorkflowMetadata>,
    ) -> Result<Vec<Pubkey>> {
        require_gte!(
            MAX_WORKFLOW_METADATAS,
            workflow_metadatas.len(),
            DataCacheError::MaxWorkflowsExceeded
        );

        require!(
            !workflow_metadatas.is_empty() && !descriptions.is_empty(),
            DataCacheError::EmptyConfig
        );

        require_eq!(
            data_ids.len(),
            descriptions.len(),
            DataCacheError::ArrayLengthMismatch
        );

        // check the remaining accounts length has sufficient feed config and permission accounts
        let expected_len = data_ids.len() + data_ids.len() * workflow_metadatas.len();

        require_eq!(
            ctx.remaining_accounts.len(),
            expected_len,
            DataCacheError::MissingAccounts
        );

        for metadata in workflow_metadatas.iter() {
            require_keys_neq!(
                metadata.allowed_sender,
                Pubkey::default(),
                DataCacheError::InvalidAddress
            );
            require!(
                !metadata.allowed_workflow_name.is_empty(),
                DataCacheError::InvalidWorkflowName
            );
            require!(
                metadata.allowed_workflow_owner != ZERO_ADDRESS,
                DataCacheError::InvalidAddress
            );
        }

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

            let feed_config_exists = feed_config_account_infos[i].data_len() != 0;

            // sorted
            let mut temp_candidates_deletion: Vec<Pubkey> = Vec::new();

            if feed_config_exists {
                let feed_config_loader =
                    AccountLoader::<FeedConfig>::try_from(&feed_config_account_infos[i])?;

                let feed_config = feed_config_loader.load()?;

                for metadata in feed_config.workflow_metadata.iter() {
                    // these entries are not to be deleted yet... we'll find out at the end if we need to delete them

                    let derived_report_hash = create_report_hash(
                        curr_data_id,
                        &metadata.allowed_sender,
                        &metadata.allowed_workflow_owner,
                        &metadata.allowed_workflow_name,
                    );

                    let (permission_flag, _) = Pubkey::find_program_address(
                        &[
                            b"permission_flag",
                            cache_state_key.as_ref(),
                            &derived_report_hash,
                        ],
                        &crate::ID,
                    );

                    sorted_insert(&mut temp_candidates_deletion, permission_flag)
                }
            }

            for (j, metadata) in workflow_metadatas.iter().enumerate() {
                let report_hash = create_report_hash(
                    curr_data_id,
                    &metadata.allowed_sender,
                    &metadata.allowed_workflow_owner,
                    &metadata.allowed_workflow_name,
                );

                let (curr_permission_flag, _) = Pubkey::find_program_address(
                    &[
                        b"permission_flag",
                        ctx.accounts.state.key().as_ref(),
                        &report_hash,
                    ],
                    &crate::ID,
                );

                // ex: data_ids: [1, 2]
                // workflow metdatas [5, 6, 7]
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

    pub fn set_decimal_feed_configs<'info>(
        ctx: Context<'_, '_, 'info, 'info, SetDecimalFeedConfigs<'info>>,
        data_ids: Vec<[u8; 16]>,
        descriptions: Vec<[u8; 32]>,
        workflow_metadatas: Vec<WorkflowMetadata>,
    ) -> Result<()> {
        // check feed admin here
        let state = &mut ctx.accounts.state.load()?;
        verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;

        require_gte!(
            MAX_WORKFLOW_METADATAS,
            workflow_metadatas.len(),
            DataCacheError::MaxWorkflowsExceeded
        );

        require!(
            !workflow_metadatas.is_empty() && !descriptions.is_empty(),
            DataCacheError::EmptyConfig
        );

        require_eq!(
            data_ids.len(),
            descriptions.len(),
            DataCacheError::ArrayLengthMismatch
        );

        // check the remaining accounts length has sufficient feed config and permission accounts
        let minimum_len = data_ids.len() + data_ids.len() * workflow_metadatas.len();

        // you have an unknown of defunct permission accounts as well, so as long as the amount is >= we're good
        require_gte!(
            ctx.remaining_accounts.len(),
            minimum_len,
            DataCacheError::MissingAccounts
        );

        for metadata in workflow_metadatas.iter() {
            require_keys_neq!(
                metadata.allowed_sender,
                Pubkey::default(),
                DataCacheError::InvalidAddress
            );
            require!(
                !metadata.allowed_workflow_name.is_empty(),
                DataCacheError::InvalidWorkflowName
            );
            require!(
                metadata.allowed_workflow_owner != ZERO_ADDRESS,
                DataCacheError::InvalidAddress
            );
        }

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
            // get the existing config feed , see if it's empty or not
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

            let feed_config_exists = feed_config_account_infos[i].data_len() != 0;

            // todo: make string a [u8; 32]
            let feed_config_loader = if feed_config_exists {
                AccountLoader::<FeedConfig>::try_from(&feed_config_account_infos[i])?
            } else {
                let rent =
                    Rent::get()?.minimum_balance(ANCHOR_DISCRIMINATOR + FeedConfig::INIT_SPACE);
                // initialize it
                let seeds: &[&[u8]] = &[
                    b"feed_config",
                    cache_state_key.as_ref(),
                    curr_data_id,
                    &[feed_config_bump],
                ];

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

            // sorted
            let mut temp_candidates_deletion: Vec<Pubkey> = Vec::new();

            // so these are the permission accounts you need to delete later
            if feed_config_exists {
                // go over current workflows
                for metadata in feed_config.workflow_metadata.iter() {
                    // these entries are not to be deleted yet... we'll find out at the end if we need to delete them

                    let derived_report_hash = create_report_hash(
                        curr_data_id,
                        &metadata.allowed_sender,
                        &metadata.allowed_workflow_owner,
                        &metadata.allowed_workflow_name,
                    );
                    // todo: should you store the nonce? or permission account in metadata? i don't think it is necessary
                    let (permission_flag, _) = Pubkey::find_program_address(
                        &[
                            b"permission_flag",
                            cache_state_key.as_ref(),
                            &derived_report_hash,
                        ],
                        &crate::ID,
                    );

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
                // workflow metdatas [5, 6, 7]
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
                    let rent = Rent::get()?.minimum_balance(ANCHOR_DISCRIMINATOR);

                    let seeds: &[&[u8]] = &[
                        b"permission_flag",
                        cache_state_key.as_ref(),
                        &report_hash,
                        &[bump],
                    ];

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
                    dst[..ANCHOR_DISCRIMINATOR]
                        .copy_from_slice(&WritePermissionFlag::discriminator());
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
                data_id: *curr_data_id,
                decimals: get_decimals(curr_data_id),
                description: descriptions[i],
                workflow_metadatas: workflow_metadatas.clone(),
            });

            delete_permission_accounts.append(&mut temp_candidates_deletion)
        }

        for (i, permission_account) in delete_permission_accounts.iter().enumerate() {
            let curr_permission_account_info = &delete_permission_account_infos[i];

            require_keys_eq!(
                *permission_account,
                *curr_permission_account_info.key,
                DataCacheError::AccountMismatch
            );

            close_account(
                curr_permission_account_info.clone(),
                ctx.accounts.feed_admin.to_account_info(),
            )?;
        }

        Ok(())
    }

    // // todo: change report and metadata to &[u8]
    pub fn on_report<'info>(
        ctx: Context<'_, '_, '_, 'info, OnReport<'info>>,
        metadata: Vec<u8>,
        report: Vec<u8>,
    ) -> Result<()> {
        // todo: check if legacy_store and legacy_feed_config are both there
        // let legacy_feed_config_included = ctx.accounts.legacy_feeds_config.is_some();
        // let legacy_store_included = ctx.accounts.legacy_store.is_some();

        let legacy_feeds_config = if let Some(loader) = &ctx.accounts.legacy_feeds_config {
            let loader = loader.load()?;
            // check legacy feed entries are sorted by data id
            require!(
                loader
                    .id_to_feed
                    .windows(2)
                    .all(|w| w[0].data_id < w[1].data_id),
                DataCacheError::IdsMustStrictlyIncrease
            );

            // if included, check that the legacy store passed in via account context the same as the one in the config
            if let Some(legacy_store) = &ctx.accounts.legacy_store {
                require_keys_eq!(
                    *legacy_store.key,
                    loader.legacy_store,
                    DataCacheError::AccountMismatch
                );
            };

            Some(loader)
        } else {
            None
        };

        // first assume we don't have legacy_store or legacy_feed_config

        let (workflow_name, workflow_owner) = get_workflow_metadata(&metadata)?;

        let received_decimal_reports = Vec::<ReceivedDecimalReport>::try_from_slice(&report)
            .map_err(|_| DataCacheError::MalformedReport)?;

        let len = received_decimal_reports.len();

        let report_account_infos = &ctx.remaining_accounts[..len];
        let permission_flag_account_infos = &ctx.remaining_accounts[len..2 * len];
        let legacy_feed_account_infos = &ctx.remaining_accounts[2 * len..];

        // sorted by key
        let legacy_accounts_sorted = legacy_feed_account_infos
            .windows(2)
            .all(|w| w[0].key.lt(w[1].key));

        require!(
            legacy_accounts_sorted,
            DataCacheError::AddressesMustStrictlyIncrease
        );

        require_eq!(
            report_account_infos.len(),
            received_decimal_reports.len(),
            DataCacheError::ArrayLengthMismatch
        );

        require_eq!(
            permission_flag_account_infos.len(),
            received_decimal_reports.len(),
            DataCacheError::ArrayLengthMismatch
        );

        let mut candidate_legacy_writes: Vec<(&LegacyFeedEntry, &ReceivedDecimalReport)> =
            Vec::new();

        for (i, received_decimal_report) in received_decimal_reports.iter().enumerate() {
            // 1. check that sender has permission to write
            let report_hash = create_report_hash(
                &received_decimal_report.data_id,
                ctx.accounts.forwarder_authority.key,
                workflow_owner,
                workflow_name,
            );

            let (curr_permission_flag, _) = Pubkey::find_program_address(
                &[
                    b"permission_flag",
                    ctx.accounts.cache_state.key().as_ref(),
                    &report_hash,
                ],
                &crate::ID,
            );

            require_keys_eq!(
                curr_permission_flag,
                *permission_flag_account_infos[i].key,
                DataCacheError::AccountMismatch
            );

            // verifies the permission account exists
            if WritePermissionFlag::try_deserialize(
                &mut &permission_flag_account_infos[i].data.borrow()[..],
            )
            .is_err()
            {
                emit!(InvalidUpdatePermission {
                    data_id: received_decimal_report.data_id,
                    sender: ctx.accounts.forwarder_authority.key(),
                    workflow_owner: workflow_owner
                        .try_into()
                        .map_err(|_| DataCacheError::InvalidLength)?,
                    workflow_name: workflow_name
                        .try_into()
                        .map_err(|_| DataCacheError::InvalidLength)?,
                });

                continue;
            }

            // 2. check report account is valid
            let (curr_report, _) = Pubkey::find_program_address(
                &[
                    b"decimal_report",
                    ctx.accounts.cache_state.key().as_ref(),
                    &received_decimal_report.data_id,
                ],
                &crate::ID,
            );

            require_keys_eq!(
                curr_report,
                *report_account_infos[i].key,
                DataCacheError::AccountMismatch
            );

            // update report

            let latest_report =
                DecimalReport::try_deserialize(&mut &report_account_infos[i].data.borrow()[..])?;

            // dont update if the received report is stale
            if received_decimal_report.timestamp <= latest_report.timestamp {
                emit!(StaleDecimalReport {
                    data_id: received_decimal_report.data_id,
                    received_timestamp: received_decimal_report.timestamp,
                    latest_timestamp: latest_report.timestamp
                });

                continue;
            }

            let mut dst = report_account_infos[i].try_borrow_mut_data()?;

            let updated_report = DecimalReport {
                answer: received_decimal_report.answer,
                timestamp: received_decimal_report.timestamp,
            };

            updated_report.serialize(&mut &mut dst[ANCHOR_DISCRIMINATOR..])?;

            emit!(DecimalReportUpdate {
                answer: received_decimal_report.answer,
                timestamp: received_decimal_report.timestamp,
                data_id: received_decimal_report.data_id
            });

            // todo: add dfc update event here

            // 3. check if the report is also associated with a legacy feed
            // a. search config by data_id to get the account key
            // b. search passed in legacy_feed_account_infos by key

            if let Some(config) = &legacy_feeds_config {
                // a given legacy feed will only write under conditions
                // I. legacy feed config is provided
                // II. data id is associated with a legacy feed in the config
                // III. the legacy store is provided
                // IV. legacy writer is provided
                // V. writes are not disabled for that legacy feed
                // VI. the legacy feed is provided in account context

                // condition I and II: if the data id is associated with a legacy feed
                if let Some(entry) = config
                    .id_to_feed
                    .binary_search_by(|e| e.data_id.cmp(&received_decimal_report.data_id))
                    .ok()
                    .and_then(|index| config.id_to_feed.get(index))
                {
                    candidate_legacy_writes.push((entry, received_decimal_report));
                }
            }
        }

        // or add generic dfc event out here

        // seperate out write disabled entries
        let write_disabled_entries: Vec<(&LegacyFeedEntry, &ReceivedDecimalReport)> =
            candidate_legacy_writes
                .iter()
                .filter(|e| e.0.write_disabled != 0)
                .cloned()
                .collect();

        let write_enabled_entries: Vec<(&LegacyFeedEntry, &ReceivedDecimalReport)> =
            candidate_legacy_writes
                .iter()
                .filter(|e| e.0.write_disabled == 0)
                .cloned()
                .collect();

        let mut write_occured = false;

        // condition III & condition IV
        if let (Some(legacy_store), Some(legacy_writer)) =
            (&ctx.accounts.legacy_store, &ctx.accounts.legacy_writer)
        {
            // use legacy_store and legacy_writer here

            let mut ordered_legacy_feed_account_infos: Vec<&AccountInfo> = Vec::new();

            // condition V
            for entry in write_enabled_entries.iter() {
                // condition VI: error if legacy feed account not supplied in account context
                let account = legacy_feed_account_infos
                    .binary_search_by(|a| a.key.cmp(&entry.0.legacy_feed))
                    .map(|i| &legacy_feed_account_infos[i])
                    .map_err(|_| DataCacheError::MissingLegacyFeedAccount)?;

                ordered_legacy_feed_account_infos.push(account);
            }

            // write to store program
            if !write_enabled_entries.is_empty() {
                let metas: Vec<AccountMeta> = std::iter::once(AccountMeta {
                    pubkey: legacy_writer.key(),
                    is_signer: true,
                    is_writable: false,
                })
                .chain(
                    ordered_legacy_feed_account_infos
                        .iter()
                        .map(|acc| AccountMeta {
                            pubkey: *acc.key,
                            is_signer: false,
                            is_writable: true,
                        }),
                )
                .collect();

                let account_infos: Vec<AccountInfo<'info>> =
                    std::iter::once(legacy_writer.to_account_info())
                        .chain(
                            ordered_legacy_feed_account_infos
                                .iter()
                                .map(|val| val.to_account_info()),
                        )
                        .collect();

                // payload begins with the Anchor discriminator
                let mut payload = SUBMIT_DISCRIMINATOR.to_vec();

                let transmissions: Vec<CacheTransmission> = write_enabled_entries
                    .iter()
                    .map(|e| CacheTransmission {
                        timestamp: e.1.timestamp,
                        answer: e.1.answer,
                    })
                    .collect();

                payload.extend(transmissions.try_to_vec()?);

                let cache_state_key = ctx.accounts.cache_state.key();

                let ix = Instruction::new_with_bytes(legacy_store.key(), &payload, metas);
                let signer_seeds = &[
                    b"legacy_writer",
                    cache_state_key.as_ref(),
                    &[ctx.accounts.cache_state.load()?.legacy_writer_nonce],
                ];

                invoke_signed(&ix, &account_infos, &[signer_seeds])
                    .map_err(|_| DataCacheError::FailedLegacyWrite)?;

                write_occured = true;
            }
        }

        // emit legacy event only if there were candidates identified by the feed config
        if !candidate_legacy_writes.is_empty() {
            let (feeds_skipped, feeds_written) = if write_occured {
                (write_disabled_entries, write_enabled_entries)
            } else {
                (candidate_legacy_writes, vec![])
            };

            emit!(LegacyFeedsReported {
                feeds_skipped: feeds_skipped.iter().map(|e| e.0.data_id).collect(),
                feeds_written: feeds_written.iter().map(|e| e.0.data_id).collect(),
            });
        }

        Ok(())
    }

    pub fn query_feed_metadata(
        ctx: Context<QueryFeedMetadata>,
        _data_id: [u8; 16],
        start_index: u8,
        max_count: u8,
    ) -> Result<Vec<WorkflowMetadata>> {
        let feed_config = ctx.accounts.feed_config.load()?;

        require!(
            !feed_config.workflow_metadata.is_empty(),
            DataCacheError::FeedNotConfigured
        );

        let len = feed_config.workflow_metadata.len();

        let start_index: usize = start_index.into();
        let max_count: usize = max_count.into();

        if start_index >= len {
            return Ok(Vec::new());
        }

        // max count 0 means take start_index and everything after it

        let mut end_index = start_index + max_count;
        end_index = if end_index > len || max_count == 0 {
            len
        } else {
            end_index
        };

        Ok(feed_config.workflow_metadata[start_index..end_index].to_vec())
    }

    pub fn query_values<'info>(
        ctx: Context<'_, '_, 'info, 'info, QueryValues<'info>>,
        data_ids: Vec<[u8; 16]>,
    ) -> Result<Vec<DecimalReport>> {
        let mut reports = Vec::new();

        for (i, data_id) in data_ids.iter().enumerate() {
            let (decimal_report, _) = Pubkey::find_program_address(
                &[
                    b"decimal_report",
                    ctx.accounts.cache_state.key().as_ref(),
                    data_id,
                ],
                &crate::ID,
            );

            let report_account_info = &ctx.remaining_accounts[i];

            require_keys_eq!(
                decimal_report,
                *report_account_info.key,
                DataCacheError::AccountMismatch
            );

            let r = Account::<DecimalReport>::try_from(report_account_info)?;

            reports.push(DecimalReport {
                timestamp: r.timestamp,
                answer: r.answer,
            });
        }

        Ok(reports)
    }
}

fn verify_feed_admin(admin: &Signer, admin_list: &AccountList) -> Result<()> {
    let is_admin = admin_list.binary_search(admin.key).is_ok();
    require!(is_admin, AuthError::Unauthorized);

    Ok(())
}

fn create_report_hash(data_id: &[u8], sender: &Pubkey, owner: &[u8], name: &[u8]) -> [u8; 32] {
    hash::hash(&[data_id, &sender.to_bytes(), owner, name].concat()).to_bytes()
}

fn get_decimals(data_id: &[u8; 16]) -> u8 {
    let report_type = data_id[7];

    if (0x20..=0x60).contains(&report_type) {
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

fn set_legacy_feeds_config(
    init: bool,
    config: &mut AccountLoader<LegacyFeedsConfig>,
    legacy_store: Pubkey,
    legacy_feeds: &[AccountInfo],
    data_ids: &[[u8; 16]],
    write_disabled: &[u8],
) -> Result<()> {
    require!(
        data_ids.len() == legacy_feeds.len() && data_ids.len() == write_disabled.len(),
        DataCacheError::ArrayLengthMismatch
    );

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
        require!(
            prev_data_id < *data_id,
            DataCacheError::IdsMustStrictlyIncrease
        );

        legacy_feeds_config.id_to_feed.push(LegacyFeedEntry {
            data_id: *data_id,
            legacy_feed: legacy_feeds[i].key(),
            write_disabled: write_disabled[i],
        });

        prev_data_id = *data_id;
    }

    Ok(())
}

fn sorted_insert<T: Ord>(vec: &mut Vec<T>, value: T) {
    match vec.binary_search(&value) {
        Ok(pos) | Err(pos) => vec.insert(pos, value),
    }
}

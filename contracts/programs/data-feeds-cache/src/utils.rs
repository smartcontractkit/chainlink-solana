use std::cell::RefMut;

use anchor_lang::{
    prelude::*,
    solana_program::{hash, program::invoke_signed, system_instruction},
};

use crate::{
    common::{MAX_WORKFLOW_METADATAS, ZERO_ADDRESS},
    error::{AuthError, DataCacheError},
    state::{AccountList, LegacyFeedEntry, LegacyFeedsConfig, WorkflowMetadata},
};

// already checked it's empty
pub fn create_account<'info>(
    space: usize,
    seeds: &[&[u8]],
    payer: Signer<'info>,
    to_account: AccountInfo<'info>,
    program_id: Pubkey,
    system_program: AccountInfo<'info>,
) -> Result<()> {
    let rent = Rent::get()?.minimum_balance(space);

    let current_lamports = to_account.lamports();
    if current_lamports == 0 {
        invoke_signed(
            &system_instruction::create_account(
                payer.key,
                to_account.key,
                rent,
                space as u64,
                &program_id,
            ),
            &[
                payer.to_account_info(),
                to_account.clone(),
                system_program.clone(),
            ],
            &[seeds],
        )?;
    } else {
        // do extra stuff
        let required_lamports = rent.saturating_sub(current_lamports);
        // transfer remaining lamports
        if required_lamports > 0 {
            let cpi_accounts = anchor_lang::system_program::Transfer {
                from: payer.to_account_info(),
                to: to_account.clone(),
            };
            let cpi_context =
                anchor_lang::context::CpiContext::new(system_program.clone(), cpi_accounts);
            anchor_lang::system_program::transfer(cpi_context, required_lamports)?;
        }
        // allocate space
        let cpi_accounts = anchor_lang::system_program::Allocate {
            account_to_allocate: to_account.clone(),
        };
        let cpi_context =
            anchor_lang::context::CpiContext::new(system_program.clone(), cpi_accounts);
        anchor_lang::system_program::allocate(cpi_context.with_signer(&[seeds]), space as u64)?;

        // Assign ownership to program
        let cpi_accounts = anchor_lang::system_program::Assign {
            account_to_assign: to_account.clone(),
        };
        let cpi_context =
            anchor_lang::context::CpiContext::new(system_program.clone(), cpi_accounts);
        anchor_lang::system_program::assign(cpi_context.with_signer(&[seeds]), &program_id)?;
    }
    Ok(())
}

pub fn verify_feed_admin(admin: &Signer, admin_list: &AccountList) -> Result<()> {
    let is_admin = admin_list.binary_search(admin.key).is_ok();
    require!(is_admin, AuthError::Unauthorized);

    Ok(())
}

pub fn create_report_hash(data_id: &[u8], sender: &Pubkey, owner: &[u8], name: &[u8]) -> [u8; 32] {
    hash::hash(&[data_id, &sender.to_bytes(), owner, name].concat()).to_bytes()
}

pub fn get_decimals(data_id: &[u8; 16]) -> u8 {
    let report_type = data_id[7];

    if (0x20..=0x60).contains(&report_type) {
        report_type - 32
    } else {
        0
    }
}

// workflow_cid           offset  0, size 32
// workflow_name          offset  32, size 10
// workflow_owner         offset  42, size 20
// report_id              offset  62, size  2
pub fn get_workflow_metadata(metadata: &[u8]) -> Result<(&[u8], &[u8])> {
    let workflow_name = metadata.get(32..42).ok_or(DataCacheError::OutOfBounds)?;
    let workflow_owner = metadata.get(42..62).ok_or(DataCacheError::OutOfBounds)?;

    Ok((workflow_name, workflow_owner))
}

pub fn set_legacy_feeds_config(
    mut legacy_feeds_config: RefMut<LegacyFeedsConfig>,
    legacy_store: Pubkey,
    legacy_feeds: &[AccountInfo],
    data_ids: &[[u8; 16]],
    write_disabled: &[u8],
) -> Result<()> {
    require!(
        data_ids.len() == legacy_feeds.len() && data_ids.len() == write_disabled.len(),
        DataCacheError::ArrayLengthMismatch
    );

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

pub fn sorted_insert<T: Ord>(vec: &mut Vec<T>, value: T) {
    match vec.binary_search(&value) {
        Ok(pos) | Err(pos) => vec.insert(pos, value),
    }
}

pub fn validate_feed_config_inputs(
    data_ids: &Vec<[u8; 16]>,
    descriptions: &Vec<[u8; 32]>,
    workflow_metadatas: &Vec<WorkflowMetadata>,
) -> Result<()> {
    require_gte!(
        MAX_WORKFLOW_METADATAS,
        workflow_metadatas.len(),
        DataCacheError::MaxWorkflowsExceeded
    );

    require!(!descriptions.is_empty(), DataCacheError::EmptyConfig);

    require_eq!(
        data_ids.len(),
        descriptions.len(),
        DataCacheError::ArrayLengthMismatch
    );

    for d in descriptions.iter() {
        if workflow_metadatas.is_empty() {
            require!(*d == [0; 32], DataCacheError::EmptyDescriptionEnforced);
        } else {
            require!(*d != [0; 32], DataCacheError::InvalidDescription);
        }
    }

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

    Ok(())
}

pub fn permission_flag(
    metadata: &WorkflowMetadata,
    data_id: &[u8; 16],
    cache_state: &[u8],
) -> (Pubkey, u8) {
    let derived_report_hash = create_report_hash(
        data_id,
        &metadata.allowed_sender,
        &metadata.allowed_workflow_owner,
        &metadata.allowed_workflow_name,
    );

    Pubkey::find_program_address(
        &[b"permission_flag", cache_state, &derived_report_hash],
        &crate::ID,
    )
}

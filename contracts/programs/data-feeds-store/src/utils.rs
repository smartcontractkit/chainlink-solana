use std::{cell::RefCell, mem::size_of};

use anchor_lang::{
    prelude::*,
    solana_program::{
        instruction::{AccountMeta, Instruction},
        program::invoke_signed,
        system_instruction,
    },
    Discriminator,
};
use bytemuck::Pod;

use crate::{
    common::{ACCOUNT_VERSION, ANCHOR_DISCRIMINATOR, FEED_CONFIG_SEED, ZERO_ADDRESS, ZERO_NAME},
    data_id,
    error::StoreError,
    state::{AccountList, FeedConfig, WorkflowMetadata},
};

/// Create a program owned PDA, topping up an account someone pre-funded (a plain `create_account`
/// would fail on it). `signers` are the PDA's seeds, and the payer's when the payer is a PDA too.
pub fn create_pda_account<'info>(
    space: usize,
    signers: &[&[&[u8]]],
    payer: &AccountInfo<'info>,
    to_account: &AccountInfo<'info>,
    system_program: &AccountInfo<'info>,
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
                &crate::ID,
            ),
            &[payer.clone(), to_account.clone(), system_program.clone()],
            signers,
        )?;
        return Ok(());
    }

    let required_lamports = rent.saturating_sub(current_lamports);
    if required_lamports > 0 {
        invoke_signed(
            &system_instruction::transfer(payer.key, to_account.key, required_lamports),
            &[payer.clone(), to_account.clone(), system_program.clone()],
            signers,
        )?;
    }
    invoke_signed(
        &system_instruction::allocate(to_account.key, space as u64),
        &[to_account.clone(), system_program.clone()],
        signers,
    )?;
    invoke_signed(
        &system_instruction::assign(to_account.key, &crate::ID),
        &[to_account.clone(), system_program.clone()],
        signers,
    )?;
    Ok(())
}

/// Like reading a zero-copy account, but by value on the heap, checking owner and discriminator.
/// For accounts too large for the 4KiB stack frame. Alignment agnostic.
pub fn load_boxed<T: Pod + Discriminator>(info: &AccountInfo) -> Result<Box<T>> {
    require_keys_eq!(*info.owner, crate::ID, StoreError::InvalidAccount);
    let data = info.try_borrow_data()?;
    require!(
        data.len() >= ANCHOR_DISCRIMINATOR + size_of::<T>()
            && &data[..ANCHOR_DISCRIMINATOR] == T::DISCRIMINATOR,
        StoreError::InvalidAccount
    );
    let mut value: Box<T> = bytemuck::zeroed_box();
    bytemuck::bytes_of_mut(&mut *value)
        .copy_from_slice(&data[ANCHOR_DISCRIMINATOR..ANCHOR_DISCRIMINATOR + size_of::<T>()]);
    Ok(value)
}

/// Write a zero-copy account's discriminator and data.
pub fn store<T: Pod + Discriminator>(info: &AccountInfo, value: &T) -> Result<()> {
    require!(info.is_writable, StoreError::AccountNotWritable);
    let mut data = info.try_borrow_mut_data()?;
    data[..ANCHOR_DISCRIMINATOR].copy_from_slice(T::DISCRIMINATOR);
    data[ANCHOR_DISCRIMINATOR..ANCHOR_DISCRIMINATOR + size_of::<T>()]
        .copy_from_slice(bytemuck::bytes_of(value));
    Ok(())
}

/// Whether an account was never created.
pub fn is_uninitialized(info: &AccountInfo) -> bool {
    *info.owner == System::id() && info.data_is_empty()
}

/// The feed config address of a data ID and its bump.
pub fn feed_config_address(state: &Pubkey, data_id: &[u8; 16]) -> (Pubkey, u8) {
    Pubkey::find_program_address(
        &[FEED_CONFIG_SEED, state.as_ref(), &data_id::to_seed(data_id)],
        &crate::ID,
    )
}

/// Load a feed config of `state` and `data_id` into an existing buffer, checking its version. Batches
/// reuse one buffer: the program's bump allocator never frees.
pub fn load_config_into(
    info: &AccountInfo,
    state: &Pubkey,
    data_id: &[u8; 16],
    config: &mut FeedConfig,
) -> Result<()> {
    require_keys_eq!(*info.owner, crate::ID, StoreError::InvalidAccount);
    {
        let data = info.try_borrow_data()?;
        require!(
            data.len() >= ANCHOR_DISCRIMINATOR + size_of::<FeedConfig>()
                && &data[..ANCHOR_DISCRIMINATOR] == FeedConfig::DISCRIMINATOR,
            StoreError::InvalidAccount
        );
        bytemuck::bytes_of_mut(config).copy_from_slice(
            &data[ANCHOR_DISCRIMINATOR..ANCHOR_DISCRIMINATOR + size_of::<FeedConfig>()],
        );
    }
    require_eq!(
        config.version,
        ACCOUNT_VERSION,
        StoreError::InvalidAccountVersion
    );
    require!(
        config.state == *state && config.data_id == data_id::to_seed(data_id),
        StoreError::AccountMismatch
    );
    Ok(())
}

/// A new, empty feed config of `state` and `data_id`.
pub fn new_config(state: &Pubkey, data_id: &[u8; 16], config: &mut FeedConfig) {
    bytemuck::bytes_of_mut(config).fill(0);
    config.version = ACCOUNT_VERSION;
    config.data_id = data_id::to_seed(data_id);
    config.state = *state;
}

/// Load a data ID's feed config, creating it at its PDA first when it doesn't exist, `payer` (a
/// signer of this instruction) paying the rent.
pub fn load_or_create_config<'info>(
    info: &AccountInfo<'info>,
    state: &Pubkey,
    data_id: &[u8; 16],
    payer: &AccountInfo<'info>,
    system_program: &AccountInfo<'info>,
    config: &mut FeedConfig,
) -> Result<()> {
    if !is_uninitialized(info) {
        return load_config_into(info, state, data_id, config);
    }
    let (address, bump) = feed_config_address(state, data_id);
    require_keys_eq!(address, *info.key, StoreError::AccountMismatch);
    let seed = data_id::to_seed(data_id);
    create_pda_account(
        ANCHOR_DISCRIMINATOR + size_of::<FeedConfig>(),
        &[&[FEED_CONFIG_SEED, state.as_ref(), &seed, &[bump]]],
        payer,
        info,
        system_program,
    )?;
    new_config(state, data_id, config);
    store(info, config)
}

/// Emits CPI events from code that has no `Context` (`emit_cpi!` needs one): the same self-invoke
/// signed by the event authority PDA. It reuses one instruction for every event, serializing the
/// event straight into it, so a batch of reports doesn't allocate per event beyond the copy the
/// runtime's invoke makes.
pub struct EventEmitter<'info> {
    authority: AccountInfo<'info>,
    bump: u8,
    ix: RefCell<Instruction>,
}

impl<'info> EventEmitter<'info> {
    pub fn new(authority: AccountInfo<'info>, bump: u8) -> Self {
        let ix = Instruction {
            program_id: crate::ID,
            accounts: vec![AccountMeta::new_readonly(*authority.key, true)],
            data: Vec::with_capacity(256),
        };
        Self {
            authority,
            bump,
            ix: RefCell::new(ix),
        }
    }

    pub fn emit<E: AnchorSerialize + Discriminator>(&self, event: &E) -> Result<()> {
        self.emit_with(E::DISCRIMINATOR, |data| Ok(event.serialize(data)?))
    }

    /// Emit an event whose fields `write` serializes in order, so large fields can be written from
    /// borrowed data instead of an owned copy per event. The bytes must match the event struct's
    /// Borsh encoding (see the tests).
    pub fn emit_with(
        &self,
        discriminator: &[u8],
        write: impl FnOnce(&mut Vec<u8>) -> Result<()>,
    ) -> Result<()> {
        let mut ix = self.ix.borrow_mut();
        ix.data.clear();
        ix.data
            .extend_from_slice(anchor_lang::event::EVENT_IX_TAG_LE);
        ix.data.extend_from_slice(discriminator);
        write(&mut ix.data)?;
        invoke_signed(
            &ix,
            std::slice::from_ref(&self.authority),
            &[&[b"__event_authority", &[self.bump]]],
        )
        .map_err(Into::into)
    }
}

pub fn verify_feed_admin(admin: &Signer, admins: &AccountList) -> Result<()> {
    require!(
        admins.binary_search(admin.key).is_ok(),
        StoreError::Unauthorized
    );
    Ok(())
}

pub fn validate_data_id(id: &[u8; 16]) -> Result<()> {
    require!(data_id::is_valid_data_id(id), StoreError::InvalidDataId);
    Ok(())
}

pub fn validate_workflow_metadata(metadata: &WorkflowMetadata) -> Result<()> {
    require_keys_neq!(
        metadata.allowed_sender,
        Pubkey::default(),
        StoreError::InvalidAddress
    );
    require!(
        metadata.allowed_workflow_owner != ZERO_ADDRESS,
        StoreError::InvalidAddress
    );
    require!(
        metadata.allowed_workflow_name != ZERO_NAME,
        StoreError::InvalidWorkflowName
    );
    Ok(())
}

/// Whether a sender and workflow may write a data ID: feed-specific metadata when configured,
/// otherwise the store's default metadata.
pub fn has_permission(
    specific: &[WorkflowMetadata],
    default: &[WorkflowMetadata],
    sender: &Pubkey,
    workflow_owner: &[u8; 20],
    workflow_name: &[u8; 10],
) -> bool {
    let metadata = if specific.is_empty() {
        default
    } else {
        specific
    };
    metadata.iter().any(|m| {
        m.allowed_sender == *sender
            && m.allowed_workflow_owner == *workflow_owner
            && m.allowed_workflow_name == *workflow_name
    })
}

// workflow_cid           offset  0, size 32
// workflow_name          offset 32, size 10
// workflow_owner         offset 42, size 20
// report_id              offset 62, size  2
pub fn workflow_metadata(metadata: &[u8]) -> Result<([u8; 10], [u8; 20])> {
    let name = metadata.get(32..42).ok_or(StoreError::MalformedReport)?;
    let owner = metadata.get(42..62).ok_or(StoreError::MalformedReport)?;
    Ok((name.try_into().unwrap(), owner.try_into().unwrap()))
}

pub fn next_account<'a, 'info>(
    accounts: &mut std::slice::Iter<'a, AccountInfo<'info>>,
) -> Result<&'a AccountInfo<'info>> {
    accounts
        .next()
        .ok_or_else(|| error!(StoreError::InvalidAccountCount))
}

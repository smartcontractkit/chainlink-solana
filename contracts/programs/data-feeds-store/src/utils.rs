use std::mem::size_of;

use anchor_lang::{
    prelude::*,
    solana_program::{program::invoke_signed, system_instruction},
    Discriminator,
};
use bytemuck::Pod;

use crate::{
    common::{ANCHOR_DISCRIMINATOR, DESCRIPTION_LEN, ZERO_ADDRESS, ZERO_NAME},
    error::{AuthError, StoreError},
    logic::Ring,
    state::{
        feed_account_len, AccountList, DataIdSettings, FeedData, StoredReport, WorkflowMetadata,
        FEED_RING_OFFSET,
    },
};

/// Create a program owned PDA, topping up an account that was pre-funded.
pub fn create_pda_account<'info>(
    space: usize,
    seeds: &[&[u8]],
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
            &[seeds],
        )?;
        return Ok(());
    }

    let required_lamports = rent.saturating_sub(current_lamports);
    if required_lamports > 0 {
        anchor_lang::system_program::transfer(
            CpiContext::new(
                system_program.clone(),
                anchor_lang::system_program::Transfer {
                    from: payer.clone(),
                    to: to_account.clone(),
                },
            ),
            required_lamports,
        )?;
    }
    anchor_lang::system_program::allocate(
        CpiContext::new(
            system_program.clone(),
            anchor_lang::system_program::Allocate {
                account_to_allocate: to_account.clone(),
            },
        )
        .with_signer(&[seeds]),
        space as u64,
    )?;
    anchor_lang::system_program::assign(
        CpiContext::new(
            system_program.clone(),
            anchor_lang::system_program::Assign {
                account_to_assign: to_account.clone(),
            },
        )
        .with_signer(&[seeds]),
        &crate::ID,
    )?;
    Ok(())
}

/// Close a program owned account, sending its lamports to `receiver`.
pub fn close_account<'info>(
    account: &AccountInfo<'info>,
    receiver: &AccountInfo<'info>,
) -> Result<()> {
    require!(account.is_writable, StoreError::AccountNotWritable);
    let lamports = account.lamports();
    **receiver.try_borrow_mut_lamports()? += lamports;
    **account.try_borrow_mut_lamports()? = 0;
    account.assign(&System::id());
    account.resize(0)?;
    Ok(())
}

/// Read a zero-copy account by value, checking owner and discriminator. Alignment agnostic.
pub fn load<T: Pod + Discriminator>(info: &AccountInfo) -> Result<T> {
    require_keys_eq!(*info.owner, crate::ID, StoreError::InvalidAccount);
    let data = info.try_borrow_data()?;
    require!(
        data.len() >= ANCHOR_DISCRIMINATOR + size_of::<T>()
            && &data[..ANCHOR_DISCRIMINATOR] == T::DISCRIMINATOR,
        StoreError::InvalidAccount
    );
    Ok(bytemuck::pod_read_unaligned(
        &data[ANCHOR_DISCRIMINATOR..ANCHOR_DISCRIMINATOR + size_of::<T>()],
    ))
}

/// Like [`load`] but on the heap, for accounts too large for the 4KiB stack frame.
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

/// Load a feed header belonging to `state` and `data_id`, checking its ring fits.
pub fn load_feed(info: &AccountInfo, state: &Pubkey, data_id: &[u8; 16]) -> Result<Box<FeedData>> {
    let feed: Box<FeedData> = load_boxed(info)?;
    require!(
        feed.state == *state
            && feed.data_id == *data_id
            && info.data_len() >= feed_account_len(feed.max_stored_reports),
        StoreError::AccountMismatch
    );
    Ok(feed)
}

/// Settings of a data ID, kept in its feed account.
pub fn load_settings(
    info: &AccountInfo,
    state: &Pubkey,
    data_id: &[u8; 16],
) -> Result<DataIdSettings> {
    Ok(load_feed(info, state, data_id)?.settings)
}

/// Update the settings of a data ID in its feed account.
pub fn update_settings<R>(
    info: &AccountInfo,
    state: &Pubkey,
    data_id: &[u8; 16],
    f: impl FnOnce(&mut DataIdSettings) -> Result<R>,
) -> Result<R> {
    let mut feed = load_feed(info, state, data_id)?;
    let result = f(&mut feed.settings)?;
    store(info, &*feed)?;
    Ok(result)
}

/// Circular buffer over a feed account's data (the whole account data, header included).
pub struct RingMut<'a> {
    data: &'a mut [u8],
}

impl<'a> RingMut<'a> {
    pub fn new(data: &'a mut [u8]) -> Self {
        Self { data }
    }
}

fn ring_offset(slot: u64) -> usize {
    FEED_RING_OFFSET + slot as usize * size_of::<StoredReport>()
}

impl Ring for RingMut<'_> {
    fn get(&self, slot: u64) -> StoredReport {
        let offset = ring_offset(slot);
        bytemuck::pod_read_unaligned(&self.data[offset..offset + size_of::<StoredReport>()])
    }

    fn set(&mut self, slot: u64, report: StoredReport) {
        let offset = ring_offset(slot);
        self.data[offset..offset + size_of::<StoredReport>()]
            .copy_from_slice(bytemuck::bytes_of(&report));
    }
}

/// Read-only circular buffer over a feed account's data, for reads.
pub struct RingRef<'a> {
    data: &'a [u8],
}

impl<'a> RingRef<'a> {
    pub fn new(data: &'a [u8]) -> Self {
        Self { data }
    }
}

impl Ring for RingRef<'_> {
    fn get(&self, slot: u64) -> StoredReport {
        let offset = ring_offset(slot);
        bytemuck::pod_read_unaligned(&self.data[offset..offset + size_of::<StoredReport>()])
    }

    fn set(&mut self, _slot: u64, _report: StoredReport) {
        unreachable!("read-only ring")
    }
}

pub fn verify_feed_admin(admin: &Signer, admins: &AccountList) -> Result<()> {
    require!(
        admins.binary_search(admin.key).is_ok(),
        AuthError::Unauthorized
    );
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

pub fn encode_description(description: &str) -> Result<[u8; DESCRIPTION_LEN]> {
    let bytes = description.as_bytes();
    require!(
        bytes.len() <= DESCRIPTION_LEN,
        StoreError::DescriptionTooLong
    );
    let mut out = [0u8; DESCRIPTION_LEN];
    out[..bytes.len()].copy_from_slice(bytes);
    Ok(out)
}

pub fn decode_description(description: &[u8; DESCRIPTION_LEN]) -> String {
    let end = description
        .iter()
        .position(|b| *b == 0)
        .unwrap_or(DESCRIPTION_LEN);
    String::from_utf8_lossy(&description[..end]).into_owned()
}

pub fn next_account<'a, 'info>(
    accounts: &mut std::slice::Iter<'a, AccountInfo<'info>>,
) -> Result<&'a AccountInfo<'info>> {
    accounts
        .next()
        .ok_or_else(|| error!(StoreError::InvalidAccountCount))
}

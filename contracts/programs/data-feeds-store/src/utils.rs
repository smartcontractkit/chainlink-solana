use std::{cell::RefCell, mem::size_of};

use anchor_lang::{
    prelude::*,
    solana_program::{
        instruction::{AccountMeta, Instruction},
        program::invoke_signed,
    },
    Discriminator,
};
use bytemuck::Pod;

use crate::{
    common::{
        ACCOUNT_VERSION, ANCHOR_DISCRIMINATOR, DESCRIPTION_LEN, DF1_LINK_SEED, FEED_SEED,
        MAX_WORKFLOW_METADATAS, ZERO_ADDRESS, ZERO_NAME,
    },
    data_id,
    error::StoreError,
    logic::Ring,
    state::{
        feed_account_len, AccountList, Feed, StoredReport, WorkflowMetadata, FEED_RING_OFFSET,
    },
};

/// Creates an account of `space` bytes at a PDA of this program, with `payer` paying the rent. If
/// someone already sent SOL to the address, a plain `create_account` would fail, so it tops the
/// balance up, then allocates and assigns the account instead. `payer` must already sign this
/// instruction.
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
        return anchor_lang::system_program::create_account(
            CpiContext::new(
                system_program.clone(),
                anchor_lang::system_program::CreateAccount {
                    from: payer.clone(),
                    to: to_account.clone(),
                },
            )
            .with_signer(&[seeds]),
            rent,
            space as u64,
            &crate::ID,
        );
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

/// Copies an account of this program onto the heap and returns the copy. It first checks that
/// this program owns the account and that the data starts with the type's tag. The account is
/// copied to the heap because a Solana stack frame is only 4 KiB and the store state alone takes
/// 3,232 bytes, and copying also works whatever the alignment of the account data.
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

/// Returns true if no account has been created at this address yet. Such an address is owned by
/// the system program and has no data, though someone may have sent SOL to it.
pub fn is_uninitialized(info: &AccountInfo) -> bool {
    *info.owner == System::id() && info.data_is_empty()
}

/// Returns the address of a data ID's feed in a store, together with the PDA's bump.
pub fn feed_address(state: &Pubkey, data_id: &[u8; 16]) -> (Pubkey, u8) {
    Pubkey::find_program_address(
        &[FEED_SEED, state.as_ref(), &data_id::to_seed(data_id)],
        &crate::ID,
    )
}

/// Returns the address of a DF1 feed's link marker in a store, together with the PDA's bump.
pub fn df1_link_address(state: &Pubkey, df1_feed: &Pubkey) -> (Pubkey, u8) {
    Pubkey::find_program_address(
        &[DF1_LINK_SEED, state.as_ref(), df1_feed.as_ref()],
        &crate::ID,
    )
}

/// Copies a feed header out of a feed account's data. It checks the tag, the layout version, that
/// the feed belongs to `state` and `data_id`, and that the account is long enough for its ring.
/// Both the program and readers that decode accounts use it. It doesn't check the account's
/// owner, so callers must check that this program owns the account.
pub fn decode_feed_into(
    data: &[u8],
    state: &Pubkey,
    data_id: &[u8; 16],
    feed: &mut Feed,
) -> Result<()> {
    require!(
        data.len() >= ANCHOR_DISCRIMINATOR + size_of::<Feed>()
            && &data[..ANCHOR_DISCRIMINATOR] == Feed::DISCRIMINATOR,
        StoreError::InvalidAccount
    );
    bytemuck::bytes_of_mut(feed)
        .copy_from_slice(&data[ANCHOR_DISCRIMINATOR..ANCHOR_DISCRIMINATOR + size_of::<Feed>()]);
    require_eq!(
        feed.version,
        ACCOUNT_VERSION,
        StoreError::InvalidAccountVersion
    );
    require!(
        feed.state == *state
            && feed.data_id == data_id::to_seed(data_id)
            && data.len() >= feed_account_len(feed.max_stored_reports),
        StoreError::AccountMismatch
    );
    Ok(())
}

/// Copies a feed header of this program into a buffer the caller owns. It checks that this
/// program owns the account, then everything [`decode_feed_into`] checks. A batch reuses one
/// buffer for every feed, because a program's heap is never freed.
pub fn load_feed_into(
    info: &AccountInfo,
    state: &Pubkey,
    data_id: &[u8; 16],
    feed: &mut Feed,
) -> Result<()> {
    require_keys_eq!(*info.owner, crate::ID, StoreError::InvalidAccount);
    decode_feed_into(&info.try_borrow_data()?, state, data_id, feed)
}

/// Writes a feed header, with its tag, back to the start of the feed account. It fails if the
/// account wasn't passed writable.
pub fn store_feed(info: &AccountInfo, feed: &Feed) -> Result<()> {
    require!(info.is_writable, StoreError::AccountNotWritable);
    let mut data = info.try_borrow_mut_data()?;
    data[..ANCHOR_DISCRIMINATOR].copy_from_slice(Feed::DISCRIMINATOR);
    data[ANCHOR_DISCRIMINATOR..ANCHOR_DISCRIMINATOR + size_of::<Feed>()]
        .copy_from_slice(bytemuck::bytes_of(feed));
    Ok(())
}

/// Returns the byte offset of a ring index in a feed account.
fn ring_offset(index: u64) -> usize {
    FEED_RING_OFFSET + index as usize * size_of::<StoredReport>()
}

/// The ring of a feed account, for reading and writing. It wraps the whole account data, header
/// included, and finds index i at byte `FEED_RING_OFFSET + 32 * i`.
pub struct RingMut<'a> {
    data: &'a mut [u8],
}

impl<'a> RingMut<'a> {
    /// Wraps a feed account's whole data.
    pub fn new(data: &'a mut [u8]) -> Self {
        Self { data }
    }
}

impl Ring for RingMut<'_> {
    fn get(&self, index: u64) -> StoredReport {
        let offset = ring_offset(index);
        bytemuck::pod_read_unaligned(&self.data[offset..offset + size_of::<StoredReport>()])
    }

    fn set(&mut self, index: u64, report: StoredReport) {
        let offset = ring_offset(index);
        self.data[offset..offset + size_of::<StoredReport>()]
            .copy_from_slice(bytemuck::bytes_of(&report));
    }
}

/// The ring of a feed account, for reading only.
pub struct RingRef<'a> {
    data: &'a [u8],
}

impl<'a> RingRef<'a> {
    /// Wraps a feed account's whole data.
    pub fn new(data: &'a [u8]) -> Self {
        Self { data }
    }
}

impl Ring for RingRef<'_> {
    fn get(&self, index: u64) -> StoredReport {
        let offset = ring_offset(index);
        bytemuck::pod_read_unaligned(&self.data[offset..offset + size_of::<StoredReport>()])
    }

    fn set(&mut self, _index: u64, _report: StoredReport) {
        unreachable!("read-only ring")
    }
}

/// Encodes a description as a feed stores it. The description must be at most 64 UTF-8 bytes
/// with no zero byte, and it is zero padded to 64 bytes.
pub fn encode_description(description: &str) -> Result<[u8; DESCRIPTION_LEN]> {
    let bytes = description.as_bytes();
    require!(
        bytes.len() <= DESCRIPTION_LEN,
        StoreError::DescriptionTooLong
    );
    // The description is stored zero padded, so a zero byte would cut it short when read back.
    require!(!bytes.contains(&0), StoreError::InvalidDescription);
    let mut out = [0u8; DESCRIPTION_LEN];
    out[..bytes.len()].copy_from_slice(bytes);
    Ok(out)
}

/// Decodes a stored description, which ends at its first zero byte.
pub fn decode_description(description: &[u8; DESCRIPTION_LEN]) -> String {
    let end = description
        .iter()
        .position(|b| *b == 0)
        .unwrap_or(DESCRIPTION_LEN);
    String::from_utf8_lossy(&description[..end]).into_owned()
}

/// Emits CPI events from code that has no Anchor `Context`, which `emit_cpi!` needs. It makes the
/// same call to this program, signed by the event authority PDA. It reuses one instruction buffer
/// and serializes each event straight into it, so the only allocation per event is the copy that
/// `invoke_signed` makes on the heap.
pub struct EventEmitter<'info> {
    authority: AccountInfo<'info>,
    bump: u8,
    ix: RefCell<Instruction>,
}

impl<'info> EventEmitter<'info> {
    /// Prepares to emit events signed by `authority`, the event authority PDA with bump `bump`.
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

    /// Emits one event.
    pub fn emit<E: AnchorSerialize + Discriminator>(&self, event: &E) -> Result<()> {
        self.emit_with(E::DISCRIMINATOR, |data| Ok(event.serialize(data)?))
    }

    /// Emits an event whose fields `write` serializes in order, so large fields can be written
    /// from borrowed data instead of an owned copy per event. The bytes must match the event
    /// struct's Borsh encoding, which the tests in `event.rs` check.
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

/// Fails with `Unauthorized` unless the signer is one of the store's feed admins.
pub fn verify_feed_admin(admin: &Signer, admins: &AccountList) -> Result<()> {
    require!(
        admins.binary_search(admin.key).is_ok(),
        StoreError::Unauthorized
    );
    Ok(())
}

/// Fails with `InvalidDataId` for a data ID the store doesn't serve: one that is zero or has an
/// SVR path ID.
pub fn validate_data_id(id: &[u8; 16]) -> Result<()> {
    require!(data_id::is_valid_data_id(id), StoreError::InvalidDataId);
    Ok(())
}

/// Checks a workflow list. It may hold at most `MAX_WORKFLOW_METADATAS` entries, and each entry
/// must pass [`validate_workflow_metadata`].
pub fn validate_workflows(workflow_metadata: &[WorkflowMetadata]) -> Result<()> {
    require!(
        workflow_metadata.len() <= MAX_WORKFLOW_METADATAS,
        StoreError::MaxEntriesExceeded
    );
    for metadata in workflow_metadata.iter() {
        validate_workflow_metadata(metadata)?;
    }
    Ok(())
}

/// Refuses a workflow entry whose sender is the default key, or whose workflow owner or name is
/// all zeros. EVM refuses the same entries.
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

/// Returns whether a sender and workflow may write a data ID. The feed's own workflow list decides
/// when it has one, and the store's default list decides otherwise.
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

/// Returns the workflow name and owner from the forwarder's metadata, the only parts the store
/// uses. The metadata is laid out as follows:
///
/// ```text
/// workflow_cid           offset  0, size 32
/// workflow_name          offset 32, size 10
/// workflow_owner         offset 42, size 20
/// report_id              offset 62, size  2
/// ```
pub fn workflow_metadata(metadata: &[u8]) -> Result<([u8; 10], [u8; 20])> {
    let name = metadata.get(32..42).ok_or(StoreError::MalformedReport)?;
    let owner = metadata.get(42..62).ok_or(StoreError::MalformedReport)?;
    Ok((name.try_into().unwrap(), owner.try_into().unwrap()))
}

/// Returns the next remaining account, or fails with `InvalidAccountCount` if the caller passed
/// too few. Remaining accounts are a plain list the caller builds, so running out is the caller's
/// mistake, which is reported as a named error rather than a panic.
pub fn next_account<'a, 'info>(
    accounts: &mut std::slice::Iter<'a, AccountInfo<'info>>,
) -> Result<&'a AccountInfo<'info>> {
    accounts
        .next()
        .ok_or_else(|| error!(StoreError::InvalidAccountCount))
}

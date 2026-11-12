use std::mem::size_of;

use anchor_lang::{
    prelude::*,
    solana_program::{program::invoke_signed, system_instruction},
    Discriminator,
};

use crate::{
    common::{ACCOUNT_VERSION, ANCHOR_DISCRIMINATOR, DESCRIPTION_LEN},
    error::ProxyError,
    logic::Ring,
    state::{feed_account_len, Feed, StoredReport, FEED_RING_OFFSET},
};

/// Create a program owned PDA, topping up an account someone pre-funded (a plain
/// `create_account` would fail on it). `payer` must already be a signer of this instruction.
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

/// Decode a feed header from a feed account's data, checking the discriminator, version, proxy
/// state, data ID and that its ring fits. Shared by the program and readers decoding accounts.
pub fn decode_feed_into(
    data: &[u8],
    state: &Pubkey,
    data_id_seed: &[u8; 32],
    feed: &mut Feed,
) -> Result<()> {
    require!(
        data.len() >= ANCHOR_DISCRIMINATOR + size_of::<Feed>()
            && &data[..ANCHOR_DISCRIMINATOR] == Feed::DISCRIMINATOR,
        ProxyError::InvalidAccount
    );
    bytemuck::bytes_of_mut(feed)
        .copy_from_slice(&data[ANCHOR_DISCRIMINATOR..ANCHOR_DISCRIMINATOR + size_of::<Feed>()]);
    require_eq!(
        feed.version,
        ACCOUNT_VERSION,
        ProxyError::InvalidAccountVersion
    );
    require!(
        feed.state == *state
            && feed.data_id == *data_id_seed
            && data.len() >= feed_account_len(feed.max_stored_reports),
        ProxyError::AccountMismatch
    );
    Ok(())
}

/// Load a feed header of this program into an existing buffer: the program's bump allocator never
/// frees, so batches reuse one buffer.
pub fn load_feed_into(
    info: &AccountInfo,
    state: &Pubkey,
    data_id_seed: &[u8; 32],
    feed: &mut Feed,
) -> Result<()> {
    require_keys_eq!(*info.owner, crate::ID, ProxyError::InvalidAccount);
    decode_feed_into(&info.try_borrow_data()?, state, data_id_seed, feed)
}

/// Load a feed header by its own data ID, for writes that identify feeds by account.
pub fn load_any_feed_into(info: &AccountInfo, state: &Pubkey, feed: &mut Feed) -> Result<()> {
    require_keys_eq!(*info.owner, crate::ID, ProxyError::InvalidAccount);
    let data = info.try_borrow_data()?;
    require!(
        data.len() >= ANCHOR_DISCRIMINATOR + size_of::<Feed>(),
        ProxyError::InvalidAccount
    );
    let mut seed = [0u8; 32];
    seed.copy_from_slice(&data[ANCHOR_DISCRIMINATOR + 16..ANCHOR_DISCRIMINATOR + 48]);
    decode_feed_into(&data, state, &seed, feed)
}

/// Write a feed header back.
pub fn store_feed(info: &AccountInfo, feed: &Feed) -> Result<()> {
    require!(info.is_writable, ProxyError::AccountNotWritable);
    let mut data = info.try_borrow_mut_data()?;
    data[..ANCHOR_DISCRIMINATOR].copy_from_slice(Feed::DISCRIMINATOR);
    data[ANCHOR_DISCRIMINATOR..ANCHOR_DISCRIMINATOR + size_of::<Feed>()]
        .copy_from_slice(bytemuck::bytes_of(feed));
    Ok(())
}

/// Whether an account was never created.
pub fn is_uninitialized(info: &AccountInfo) -> bool {
    *info.owner == System::id() && info.data_is_empty()
}

fn ring_offset(slot: u64) -> usize {
    FEED_RING_OFFSET + slot as usize * size_of::<StoredReport>()
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

/// Read-only circular buffer over a feed account's data.
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

pub fn encode_description(description: &str) -> Result<[u8; DESCRIPTION_LEN]> {
    let bytes = description.as_bytes();
    require!(
        bytes.len() <= DESCRIPTION_LEN,
        ProxyError::DescriptionTooLong
    );
    // stored zero padded, so a NUL would cut the description short on read
    require!(!bytes.contains(&0), ProxyError::InvalidDescription);
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

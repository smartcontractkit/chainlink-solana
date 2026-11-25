use anchor_lang::{prelude::*, solana_program::bpf_loader_upgradeable};
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};

use crate::common::{DF1_WRITER_SEED, STATE_SEED, STORE_AUTHORITY_SEED, STORE_STATE_VERSION};
use crate::error::StoreError;
#[cfg(feature = "idl-build")]
use crate::state::Feed;
use crate::state::StoreState;

// Every context that changes state emits CPI events. `#[event_cpi]` adds the event authority PDA
// and this program as the context's last two accounts. Every context except `Initialize` checks
// the state account's layout version; `OnReport` does so in its handler instead.

/// The accounts of `initialize`. Only the program's upgrade authority, the key allowed to redeploy
/// the program, can create a store, so every store under this program ID was created by its
/// deployer. `initialize` creates the state account itself, at a PDA of the store ID, so no
/// half-created account ever waits for it: an account owned by this program but still all zeros
/// could be taken over by anyone through Anchor's built-in IDL instructions.
#[event_cpi]
#[derive(Accounts)]
#[instruction(store_id: u64)]
pub struct Initialize<'info> {
    /// The program's upgrade authority. It becomes the store's first owner and pays the state
    /// account's rent.
    #[account(mut)]
    pub owner: Signer<'info>,

    /// CHECK: The new store's state account, at the PDA `["state", store ID]`, which the handler
    /// creates. Anchor checks the address; the handler checks that no account exists there yet.
    #[account(mut, seeds = [STATE_SEED, &store_id.to_le_bytes()], bump)]
    pub state: UncheckedAccount<'info>,

    /// The program's ProgramData account, which records the program's upgrade authority.
    #[account(
        seeds = [crate::ID.as_ref()],
        bump,
        seeds::program = bpf_loader_upgradeable::ID,
        constraint = program_data.upgrade_authority_address == Some(owner.key()) @ StoreError::Unauthorized
    )]
    pub program_data: Account<'info, ProgramData>,

    pub system_program: Program<'info, System>,
}

/// The accounts of the owner's instructions. The signer must be the store's owner.
#[event_cpi]
#[derive(Accounts)]
pub struct OwnerOnly<'info> {
    #[account(address = state.load()?.owner @ StoreError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(mut, constraint = state.load()?.version == STORE_STATE_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,
}

/// The accounts of `accept_ownership`. The signer must be the proposed owner.
#[event_cpi]
#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(address = state.load()?.proposed_owner @ StoreError::Unauthorized)]
    pub new_owner: Signer<'info>,

    #[account(mut, constraint = state.load()?.version == STORE_STATE_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,
}

/// The accounts of the feed admin instructions that change only the store state. The handler
/// checks that the signer is a feed admin.
#[event_cpi]
#[derive(Accounts)]
pub struct FeedAdminState<'info> {
    pub feed_admin: Signer<'info>,

    #[account(mut, constraint = state.load()?.version == STORE_STATE_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,
}

/// The accounts of the feed admin instructions that change feeds. The feeds come as remaining
/// accounts. The handler checks that the signer is a feed admin. The signer pays for any feed
/// created or grown and for any DF1 link marker created.
#[event_cpi]
#[derive(Accounts)]
pub struct FeedAdminAccounts<'info> {
    #[account(mut)]
    pub feed_admin: Signer<'info>,

    #[account(constraint = state.load()?.version == STORE_STATE_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

    pub system_program: Program<'info, System>,
}

/// The accounts of `on_report`, in the order the forwarder passes them. The DON's report
/// signature covers every account key here, because the forwarder checks a hash of the keys
/// against the signed report. It does not cover which accounts are passed writable.
///
/// ```text
/// fixed      forwarder_state, forwarder_authority (signer), state,
///            df1_program, df1_writer      this program's ID in place of either: DF1 is skipped
///            event_authority, program     added by `#[event_cpi]`
/// remaining  one group per report, in report order, with nothing after the last group:
///              feed of the data ID        an empty account if the feed isn't set;
///                                         writable if the round will be stored
///              linked DF1 feed            only if that feed links one;
///                                         writable if DF1 will be written
///
/// Reports for A, B and C, where only B's feed links the DF1 feed D:
///            ... program | feed A | feed B | D | feed C
/// ```
#[event_cpi]
#[derive(Accounts)]
pub struct OnReport<'info> {
    /// CHECK: The forwarder's state account. The forwarder always passes it first, and the store
    /// does not read it.
    pub forwarder_state: UncheckedAccount<'info>,

    /// The sender of the reports. A report is only applied when this is the allowed sender of a
    /// workflow permitted for its data ID. That sender is a forwarder's authority PDA, which the
    /// forwarder only signs with after verifying a report, so supporting more forwarders (one per
    /// zone, for example) only takes more workflow entries.
    pub forwarder_authority: Signer<'info>,

    /// The store state. The handler checks its version once and reuses it for every report.
    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: The DF1 program, needed when a report's feed links a DF1 feed. To leave it out, the
    /// caller passes this program's ID instead, and DF1 writes are then skipped as `NotPassed`.
    #[account(address = state.load()?.df1_program @ StoreError::AccountMismatch)]
    pub df1_program: Option<UncheckedAccount<'info>>,

    /// CHECK: The store's `df1_writer` PDA, needed when a report's feed links a DF1 feed. To leave
    /// it out, the caller passes this program's ID instead, as for `df1_program`.
    #[account(seeds = [DF1_WRITER_SEED, state.key().as_ref()], bump = state.load()?.df1_writer_bump)]
    pub df1_writer: Option<UncheckedAccount<'info>>,
}

/// The accounts of the read instructions: the store state and the feed of the data ID being read.
#[derive(Accounts)]
pub struct FeedRead<'info> {
    #[account(constraint = state.load()?.version == STORE_STATE_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

    // Anchor includes typed accounts in the IDL. Keep that public type for IDL builds, while
    // runtime reads use an unchecked account so an uninitialized feed reaches the handler.
    /// The feed of the data ID. The handler checks that it belongs to this state and this data ID,
    /// returning `NoDataPresent` for a correctly derived but uninitialized feed.
    #[cfg(feature = "idl-build")]
    pub feed: AccountLoader<'info, Feed>,
    /// CHECK: The handler checks the feed's owner, layout, state and data ID.
    #[cfg(not(feature = "idl-build"))]
    pub feed: UncheckedAccount<'info>,
}

/// The accounts of `check_feed_permission`.
#[derive(Accounts)]
pub struct PermissionRead<'info> {
    #[account(constraint = state.load()?.version == STORE_STATE_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: The data ID's feed PDA, which may not exist yet. The handler checks its address when
    /// it doesn't exist, and its contents when it does.
    pub feed: UncheckedAccount<'info>,
}

/// The accounts of `recover_tokens`. The owner moves tokens from a token account of the store
/// authority PDA to any token account of the same mint.
#[event_cpi]
#[derive(Accounts)]
pub struct RecoverTokens<'info> {
    #[account(address = state.load()?.owner @ StoreError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(constraint = state.load()?.version == STORE_STATE_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: The store authority PDA, which owns the token account being recovered from.
    #[account(
        seeds = [STORE_AUTHORITY_SEED, state.key().as_ref()],
        bump = state.load()?.store_authority_bump,
    )]
    pub store_authority: UncheckedAccount<'info>,

    #[account(mut, token::authority = store_authority, token::mint = mint, token::token_program = token_program)]
    pub from: InterfaceAccount<'info, TokenAccount>,

    #[account(mut, token::mint = mint, token::token_program = token_program)]
    pub to: InterfaceAccount<'info, TokenAccount>,

    pub mint: InterfaceAccount<'info, Mint>,

    pub token_program: Interface<'info, TokenInterface>,
}

/// The accounts of `recover_native`. The owner moves SOL from the store authority PDA to any
/// account.
#[event_cpi]
#[derive(Accounts)]
pub struct RecoverNative<'info> {
    #[account(address = state.load()?.owner @ StoreError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(constraint = state.load()?.version == STORE_STATE_VERSION @ StoreError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, StoreState>,

    /// CHECK: The store authority PDA, which holds the SOL being recovered.
    #[account(
        mut,
        seeds = [STORE_AUTHORITY_SEED, state.key().as_ref()],
        bump = state.load()?.store_authority_bump,
    )]
    pub store_authority: UncheckedAccount<'info>,

    /// CHECK: Any account can receive SOL.
    #[account(mut)]
    pub to: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,
}

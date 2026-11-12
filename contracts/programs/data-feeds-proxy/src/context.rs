use anchor_lang::{prelude::*, solana_program::bpf_loader_upgradeable};

use crate::common::{ACCOUNT_VERSION, ANCHOR_DISCRIMINATOR};
use crate::error::ProxyError;
use crate::state::{Feed, ProxyState};

// Owner contexts emit CPI events (`#[event_cpi]` adds the event authority PDA and this program as
// their last accounts). Every context checks the state's layout version.

/// Only the program's upgrade authority creates proxies, so every proxy under this program ID is
/// ours. The state account is created beforehand (system `create_account`, owned by this program)
/// so that `initialize` needs no other signer, e.g. when the authority is an MCMS timelock.
#[event_cpi]
#[derive(Accounts)]
pub struct Initialize<'info> {
    /// The program's upgrade authority, the proxy's first owner
    pub owner: Signer<'info>,

    #[account(
        zero,
        constraint = state.to_account_info().data_len() == ANCHOR_DISCRIMINATOR + ProxyState::INIT_SPACE
            @ ProxyError::InvalidAccount
    )]
    pub state: AccountLoader<'info, ProxyState>,

    #[account(
        seeds = [crate::ID.as_ref()],
        bump,
        seeds::program = bpf_loader_upgradeable::ID,
        constraint = program_data.upgrade_authority_address == Some(owner.key()) @ ProxyError::Unauthorized
    )]
    pub program_data: Account<'info, ProgramData>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct OwnerOnly<'info> {
    #[account(address = state.load()?.owner @ ProxyError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(mut, constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,
}

#[event_cpi]
#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(address = state.load()?.proposed_owner @ ProxyError::Unauthorized)]
    pub new_owner: Signer<'info>,

    #[account(mut, constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,
}

/// The owner sets a feed's minimum read decimals, as on Stellar's proxy.
#[event_cpi]
#[derive(Accounts)]
pub struct SetMinDecimals<'info> {
    #[account(address = state.load()?.owner @ ProxyError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,

    /// Feed of the data ID, matched against the state and the data ID in the handler
    #[account(mut)]
    pub feed: AccountLoader<'info, Feed>,
}

/// The trusted store changes feeds passed as remaining accounts.
#[derive(Accounts)]
pub struct WriterOnly<'info> {
    #[account(address = state.load()?.writer @ ProxyError::Unauthorized)]
    pub writer: Signer<'info>,

    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,
}

/// The trusted store creates or grows feeds passed as remaining accounts, `payer` paying the rent.
#[derive(Accounts)]
pub struct WriterPayer<'info> {
    #[account(address = state.load()?.writer @ ProxyError::Unauthorized)]
    pub writer: Signer<'info>,

    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,

    #[account(mut)]
    pub payer: Signer<'info>,

    pub system_program: Program<'info, System>,
}

/// Reads of one feed.
#[derive(Accounts)]
pub struct FeedRead<'info> {
    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,

    /// Feed of the data ID, matched against the state and the data ID in the handler
    pub feed: AccountLoader<'info, Feed>,
}

#[derive(Accounts)]
pub struct StateRead<'info> {
    #[account(constraint = state.load()?.version == ACCOUNT_VERSION @ ProxyError::InvalidAccountVersion)]
    pub state: AccountLoader<'info, ProxyState>,
}

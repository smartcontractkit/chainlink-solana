use anchor_lang::prelude::*;
use keystone_forwarder::ID as FORWARDER_ID;
use keystone_forwarder::ForwarderState;

use crate::common::ANCHOR_DISCRIMINATOR;
use crate::program::DataFeedsCache;
use crate::state::CacheState;
use crate::state::LegacyFeedsConfig;
use crate::error::AuthError;

// SetDecimalsFeedConfig

// RemoveFeedConfig

// bunch of query methods... maybe just one? we'll see

// #[derive(Accounts)]
// pub struct GetFeedMetadata {
//     // feed config accounts are passed in via ctx.remaining_accounts 
//     // they are verified in the instruction
// }

#[derive(Accounts)]
pub struct Initialize<'info> {
    #[account(mut)]
    pub owner: Signer<'info>,

    #[account(
        init,
        payer = owner,
        space = ANCHOR_DISCRIMINATOR + CacheState::INIT_SPACE + 5000,
    )]
    pub state: AccountLoader<'info, CacheState>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct SetFeedAdmin<'info> {
    #[account(address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(mut)]
    pub state: AccountLoader<'info, CacheState>,
}

#[derive(Accounts)]
pub struct TransferOwnership<'info> {
    #[account(address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(mut)]
    pub state: AccountLoader<'info, CacheState>,
}

#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(address = state.load()?.proposed_owner @ AuthError::Unauthorized)]
    pub new_owner: Signer<'info>,

    #[account(mut)]
    pub state: AccountLoader<'info, CacheState>,
}


#[derive(Accounts)]
pub struct InitLegacyFeedsConfig<'info> {
    #[account(mut, address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    pub state: AccountLoader<'info, CacheState>,

    #[account(
        init,
        payer = owner,
        space = ANCHOR_DISCRIMINATOR, // todo add legacy feeds config size
        seeds = [b"legacy_feeds_config", state.key().as_ref()],
        bump
    )]
    pub legacy_feeds_config: AccountLoader<'info, LegacyFeedsConfig>,

    pub system_program: Program<'info, System>,
}

// todo: the offchain code will need to wrap add/remove data_ids mappings
#[derive(Accounts)]
pub struct UpdateLegacyFeedsConfig<'info> {
    #[account(mut, address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    pub state: AccountLoader<'info, CacheState>,

    #[account(
        mut,
        seeds = [b"legacy_feeds_config", state.key().as_ref()],
        bump
    )]
    pub legacy_feeds_config: AccountLoader<'info, LegacyFeedsConfig>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct CloseLegacyFeedsConfig<'info> {
    #[account(mut, address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    pub state: AccountLoader<'info, CacheState>,


    #[account(
        mut,
        seeds = [b"legacy_feeds_config", state.key().as_ref()],
        bump,
        close = owner
    )]
    pub legacy_feeds_config: AccountLoader<'info, LegacyFeedsConfig>,
}

// you may need to close old config accounts, init new ones
// use realloc if possible
#[derive(Accounts)]
pub struct SetDecimalFeedConfigs<'info> {
    // todo: inline check if it is an admin
    #[account(mut)]
    pub feed_admin: Signer<'info>,

    pub state: AccountLoader<'info, CacheState>,

    pub system_program: Program<'info, System>,

    // dynamic list of writePermissions. create if not created already, or overwrite as well
    
        
      // N accounts, N = # of data ids 
    //   #[account(
    //     mut,
    //     seeds = [
    //         b"feed_config",
    //         data_id,
    //     ],
    //     bump
    //   )]
    //   pub feed_config: UncheckedAccount<'info>

    // N X M accounts, N = # of data_ids, M = # of workflows
    // #[account(
    //     mut,
    //     seeds = [
    //         b"permission_flag", 
    //         report_hash,
    //     ],
    //     bump
    // )]
    // pub permission: UncheckedAccount<'info>

      // it's more of a usability question. you could stuff many of these ixs into one transaction,
      // but you'd duplicate the signer and state variables...
}


#[derive(Accounts)]
pub struct OnReport<'info> {
    // inline: verified via the permission account
    #[account(owner = FORWARDER_ID)]
    pub forwarder_state: Account<'info, ForwarderState>,

    #[account(seeds = [b"forwarder", forwarder_state.key().as_ref()], bump = forwarder_state.authority_nonce, seeds::program = FORWARDER_ID)]
    pub forwarder_authority: Signer<'info>,

    // omit if you don't want to write to the store
    // inline: check that the store equals the one set in the legacy_feeds_config
    #[account(executable)]
    pub legacy_store: Option<UncheckedAccount<'info>>,

    // we will always include legacy_feeds_config
    // UNLESS the legacy_feeds_config account is closed 
    // a.k.a we don't write to legacy feeds because customers
    // have migrated off of them
    #[account(
        seeds = [b"legacy_feeds_config"], // todo: add the current state
        bump
    )]
    pub legacy_feeds_config: Option<AccountLoader<'info, LegacyFeedsConfig>>

    // remaining accounts (N data ids, M legacy feeds)

    // N accounts
    // #[account(
    //     mut,
    //     seeds = [
    //         b"decimal_report", 
    //         data_id,
    //     ],
    //     bump
    // )]
    // pub report: UncheckedAccount<'info>

    // N accounts
    // #[account(
    //     mut,
    //     seeds = [
    //         b"permission", 
    //         report_hash,
    //     ],
    //     bump
    // )]
    // pub permission: UncheckedAccount<'info>

    // M transmission feed accounts
    // pub legacy_feed: UncheckedAccount<'info>

}
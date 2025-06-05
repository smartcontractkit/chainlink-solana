use anchor_lang::prelude::*;
use keystone_forwarder::ForwarderState;
use keystone_forwarder::ID as FORWARDER_ID;
use store::Transmissions;

use crate::common::ANCHOR_DISCRIMINATOR;
use crate::error::AuthError;
use crate::program::DataFeedsCache;
use crate::state::CacheState;
use crate::state::LegacyFeedsConfig;

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
        space = ANCHOR_DISCRIMINATOR + CacheState::INIT_SPACE,
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

    #[account(executable)]
    /// CHECK: We don't use Program<> here since it can be any program that obeys the interface, "executable" is enough
    pub legacy_store: UncheckedAccount<'info>,

    #[account(
        init,
        payer = owner,
        space = ANCHOR_DISCRIMINATOR + LegacyFeedsConfig::INIT_SPACE, // todo add legacy feeds config size
        seeds = [b"legacy_feeds_config", state.key().as_ref()],
        bump
    )]
    pub legacy_feeds_config: AccountLoader<'info, LegacyFeedsConfig>,

    pub system_program: Program<'info, System>,
    // in ctx.remaining_accounts N legacy feeds (to match N legacy data ids)
    // we do not enforce an account type because the account struct is subject to change
    // and knowing its schema is not the responsibility of the cache program but the store
    // we just need to know what the account address is for verification purposes
    // pub legacy_feed: UncheckedAccount<'info>
}

// todo: the offchain code will need to wrap add/remove data_ids mappings
#[derive(Accounts)]
pub struct UpdateLegacyFeedsConfig<'info> {
    #[account(mut, address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    pub state: AccountLoader<'info, CacheState>,

    #[account(executable)]
    /// CHECK: We don't use Program<> here since it can be any program that obeys the interface, "executable" is enough
    pub legacy_store: UncheckedAccount<'info>,

    #[account(
        mut,
        seeds = [b"legacy_feeds_config", state.key().as_ref()],
        bump
    )]
    pub legacy_feeds_config: AccountLoader<'info, LegacyFeedsConfig>,
    // in ctx.remaining_accounts N legacy feeds (to match N legacy data ids)
    // we do not enforce an account type because the account struct is subject to change
    // and knowing its schema is not the responsibility of the cache program but the store
    // we just need to know what the account address is for verification purposes
    // pub legacy_feed: UncheckedAccount<'info>
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

#[derive(Accounts)]
pub struct InitDecimalReports<'info> {
    #[account(mut)]
    pub feed_admin: Signer<'info>,

    pub state: AccountLoader<'info, CacheState>,

    pub system_program: Program<'info, System>,
    // N data report accounts
    // #[account(
    //     init,
    //     seeds = [
    //         b"decimal_report",
    //         cache_state.key().as_ref()
    //         data_id,
    //     ],
    //     bump
    // )]
    // pub report: UncheckedAccount<'info>
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
    //         state.key().as_ref()
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
    //         state.key().as_ref()
    //         report_hash,
    //     ],
    //     bump
    // )]
    // pub permission_flag: UncheckedAccount<'info>

    // it's more of a usability question. you could stuff many of these ixs into one transaction,
    // but you'd duplicate the signer and state variables...
}

#[derive(Accounts)]
pub struct CloseStalePermissionAccounts<'info> {
    #[account(mut)]
    pub feed_admin: Signer<'info>,

    pub state: AccountLoader<'info, CacheState>,
    // N accounts, N = # of data ids
    //   #[account(
    //     mut,
    //     seeds = [
    //         b"feed_config",
    //         state.key().as_ref()
    //         data_id,
    //     ],
    //     bump
    //   )]
    //   pub feed_config: UncheckedAccount<'info>

    // M accounts (which need to be deleted)
    // #[account(
    //     mut,
    //     seeds = [
    //         b"permission_flag",
    //         state.key().as_ref()
    //         report_hash,
    //     ],
    //     bump
    // )]
    // pub permission_flag: UncheckedAccount<'info>
}

// oh yeah, they have to be remaining accounts...

#[derive(Accounts)]
pub struct TestOption<'info> {
    // also verified inline (via the permission account)
    #[account(owner = FORWARDER_ID)]
    pub forwarder_state: Account<'info, ForwarderState>,

    #[account(seeds = [b"forwarder", forwarder_state.key().as_ref()], bump = forwarder_state.authority_nonce, seeds::program = FORWARDER_ID)]
    pub forwarder_authority: Signer<'info>,

    #[account(executable)]
    pub legacy_store: Option<UncheckedAccount<'info>>,

    #[account()]
    pub cache_state: Option<AccountLoader<'info, CacheState>>,
    // add remaining accounts
}

#[derive(Accounts)]
pub struct OnReport<'info> {
    // #[account(owner = FORWARDER_ID)]
    // checking the owner of the state is optional and not necessary
    // because the forwarder state is uniquely associated with the
    // forwarder authority which is verified in the instruction
    // warning: the FORWARDER_ID deployed in an environment may be different
    // than the one in source control. you need to view the docs to determine
    // what the actual deployed program id is.
    pub forwarder_state: Account<'info, ForwarderState>,

    #[account(seeds = [b"forwarder", forwarder_state.key().as_ref()], bump = forwarder_state.authority_nonce, seeds::program = FORWARDER_ID)]
    pub forwarder_authority: Signer<'info>,

    #[account()]
    pub cache_state: AccountLoader<'info, CacheState>,

    // some data cache instances may not care about the legacy feeds so they
    // will omit them both and legacy feed logic will be skipped

    // omit if you don't want to write to the store
    // inline: check that the store equals the one set in the legacy_feeds_config
    #[account(executable)]
    pub legacy_store: Option<UncheckedAccount<'info>>,

    // behavior if we don't include legacy store ...
    // legacy_store | legacy feeds config
    // N                N     - skips logic altogether
    // N                Y     - if it is in a report, don't write but log about feeds that would be affected?
    // Y                N     - ERROR state
    // Y                Y     - if it is in a report, writes to the legacy feed

    // we can emit an event that says if it's writing or not
    #[account(
        seeds = [b"legacy_feeds_config", cache_state.key().as_ref()], // todo: add the current state
        bump
    )]
    pub legacy_feeds_config: Option<AccountLoader<'info, LegacyFeedsConfig>>,

    /// CHECK: This is a PDA
    #[account(seeds = [b"legacy_writer", cache_state.key().as_ref()], bump = cache_state.load()?.legacy_writer_nonce)]
    pub legacy_writer: Option<UncheckedAccount<'info>>,

    pub system_program: Program<'info, System>,
    // remaining accounts (N data ids, M legacy feeds)

    // N accounts
    // #[account(
    //     mut,
    //     seeds = [
    //         b"decimal_report",
    //         cache_state.key().as_ref()
    //         data_id,
    //     ],
    //     bump
    // )]
    // pub report: UncheckedAccount<'info>

    // N accounts
    // #[account(
    //     mut,
    //     seeds = [
    //         b"permission_flag",
    //         cache_state.key().as_ref()
    //         report_hash,
    //     ],
    //     bump
    // )]
    // pub permission_flag: UncheckedAccount<'info>

    // M transmission feed accounts
    // should be sorted
    //
    // included if and only if both legacy_store and legacy_feeds_config is included.
    // if only 1 or 0 or the legacy_store / legacy_feeds_config accounts are included
    // this should not be included.
    //
    // note: not all of the legacy feed accounts supplied may be written to because there is
    // a write_disabled flag per account. assume this is sorted.
    //
    // pub legacy_feed: UncheckedAccount<'info>
}

#[derive(Accounts)]
pub struct DummyCtx {}

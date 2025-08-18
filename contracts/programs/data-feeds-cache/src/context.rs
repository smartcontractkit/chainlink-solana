use anchor_lang::prelude::*;
use keystone_forwarder::ForwarderState;

use crate::common::ANCHOR_DISCRIMINATOR;
use crate::error::AuthError;
use crate::state::CacheState;
use crate::state::DecimalReport;
use crate::state::FeedConfig;
use crate::state::LegacyFeedsConfig;
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

    #[account(executable)]
    /// CHECK: We don't specify the static forwarder program id from the forwarder crate because
    /// the actual program id on chain may have been generated through a different mechanism
    pub forwarder_program: UncheckedAccount<'info>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct UpdateForwarder<'info> {
    #[account(address = state.load()?.owner @ AuthError::Unauthorized)]
    pub owner: Signer<'info>,

    #[account(mut)]
    pub state: AccountLoader<'info, CacheState>,

    #[account(executable)]
    /// CHECK: We don't specify the static forwarder program id from the forwarder crate because
    /// the actual program id on chain may have been generated through a different mechanism
    pub forwarder_program: UncheckedAccount<'info>,
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
        space = ANCHOR_DISCRIMINATOR + LegacyFeedsConfig::INIT_SPACE,
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
    //         state.key().as_ref()
    //         data_id,
    //     ],
    //     bump
    // )]
    // pub report: UncheckedAccount<'info>
}

#[derive(Accounts)]
#[instruction(data_id: [u8; 16])]
pub struct CloseDecimalReport<'info> {
    #[account(mut)]
    pub feed_admin: Signer<'info>,

    pub state: AccountLoader<'info, CacheState>,

    #[account(
        mut,
        seeds = [
            b"decimal_report",
            state.key().as_ref(),
            &data_id,
        ],
        bump,
        close = feed_admin,
    )]
    pub decimal_report: Account<'info, DecimalReport>,

    #[account(
        mut,
        seeds = [
            b"feed_config",
            state.key().as_ref(),
            &data_id,
        ],
        bump,
        close = feed_admin,
      )]
    pub feed_config: AccountLoader<'info, FeedConfig>,
}

#[derive(Accounts)]
pub struct PreviewDecimalFeedConfigs<'info> {
    pub state: AccountLoader<'info, CacheState>,
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
}

#[derive(Accounts)]
pub struct SetDecimalFeedConfigs<'info> {
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

    // Permission accounts that authorize workflows
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

    // Defunct permission accounts that need closing
    // acquired by simulating "preview_decimal_feed_configs"
    // L accounts
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

#[derive(Accounts)]
pub struct OnReport<'info> {
    // Note: the data feed cache's on_report function does not directly authenticate the forwarder state.
    // Instead, it indirectly verifies the correct state by enforcing that the forwarder_authority is authorized.
    // WARNING: the FORWARDER_ID deployed in an environment may be different
    // than the one in source control (the chainlink keystone_forwarder crate). You need to view the official chainlink docs to determine
    // the correct FORWARDER_ID to use
    pub forwarder_state: Account<'info, ForwarderState>,

    #[account(seeds = [b"forwarder", forwarder_state.key().as_ref(), crate::ID.as_ref()], bump, seeds::program = cache_state.load()?.forwarder_id)]
    pub forwarder_authority: Signer<'info>,

    #[account()]
    pub cache_state: AccountLoader<'info, CacheState>,

    // omit if you don't want to write to the store
    #[account(executable)]
    pub legacy_store: Option<UncheckedAccount<'info>>,

    // omit if you don't want to write to the store
    #[account(
        seeds = [b"legacy_feeds_config", cache_state.key().as_ref()],
        bump
    )]
    pub legacy_feeds_config: Option<AccountLoader<'info, LegacyFeedsConfig>>,

    // omit if you don't want to write to the store
    /// CHECK: This is a PDA
    #[account(seeds = [b"legacy_writer", cache_state.key().as_ref()], bump = cache_state.load()?.legacy_writer_bump)]
    pub legacy_writer: Option<UncheckedAccount<'info>>,
    // pub system_program: Program<'info, System>,
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

    // M transmission feed accounts (sorted)
    //
    // Note: not all of the legacy feed accounts supplied may be written to because there is
    // a write_disabled flag per account. Additionally, if either legacy_store, legacy_feeds_config,
    // or legacy_writer is omitted no legacy feeds will be written to
    //
    // pub legacy_feed: UncheckedAccount<'info>
}

#[derive(Accounts)]
pub struct QueryValues<'info> {
    #[account()]
    pub cache_state: AccountLoader<'info, CacheState>,
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
}

#[derive(Accounts)]
#[instruction(data_id: [u8; 16])]
pub struct QueryFeedMetadata<'info> {
    #[account()]
    pub cache_state: AccountLoader<'info, CacheState>,

    #[account(
        seeds = [
            b"feed_config",
            cache_state.key().as_ref(),
            &data_id,
        ],
        bump
    )]
    pub feed_config: AccountLoader<'info, FeedConfig>,
}

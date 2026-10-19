use anchor_lang::prelude::*;

use crate::{
    common::{ACCOUNT_VERSION, ANCHOR_DISCRIMINATOR, PROXY_SEED, STORED_DECIMALS},
    context::FeedAdminAccounts,
    data_id,
    error::StoreError,
    event::*,
    state::{ProxyConfig, ProxyConfigArgs},
    utils::{close_account, create_pda_account, load_feed, verify_feed_admin},
};

fn load_proxy(info: &AccountInfo, state: &Pubkey, proxy_id: &[u8; 32]) -> Result<ProxyConfig> {
    require_keys_eq!(*info.owner, crate::ID, StoreError::InvalidAccount);
    let proxy = ProxyConfig::try_deserialize(&mut &info.try_borrow_data()?[..])?;
    require!(
        proxy.state == *state && proxy.proxy_id == *proxy_id,
        StoreError::AccountMismatch
    );
    Ok(proxy)
}

/// Create or update proxy (reader) configs. Remaining accounts are, per proxy in order, the proxy
/// config PDA followed by the non-SVR feed of its data ID (which may not exist yet).
pub fn update_configs_for_proxies<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    proxy_ids: Vec<[u8; 32]>,
    configs: Vec<ProxyConfigArgs>,
) -> Result<()> {
    let state = ctx.accounts.state.load()?;
    verify_feed_admin(&ctx.accounts.feed_admin, &state.feed_admins)?;
    require!(
        proxy_ids.len() == configs.len() && ctx.remaining_accounts.len() == 2 * proxy_ids.len(),
        StoreError::ArrayLengthMismatch
    );

    let state_key = ctx.accounts.state.key();
    for (i, (proxy_id, config)) in proxy_ids.iter().zip(configs.iter()).enumerate() {
        let proxy_info = &ctx.remaining_accounts[2 * i];
        let feed_info = &ctx.remaining_accounts[2 * i + 1];

        require!(
            !data_id::is_zero(&config.data_id),
            StoreError::InvalidDataId
        );

        // The retained history must span at least the fallback window, so the sync scan can always
        // reach a report old enough to serve.
        let non_svr_data_id = data_id::to_non_svr(&config.data_id);
        let max_stored_reports = if feed_info.data_is_empty() {
            require_keys_eq!(
                *feed_info.key,
                Pubkey::find_program_address(
                    &[
                        crate::common::FEED_SEED,
                        state_key.as_ref(),
                        &non_svr_data_id
                    ],
                    &crate::ID
                )
                .0,
                StoreError::AccountMismatch
            );
            state.default_max_stored_reports
        } else {
            load_feed(feed_info, &state_key, &non_svr_data_id)?.max_stored_reports
        };
        require!(
            u64::from(config.fallback_time) <= max_stored_reports,
            StoreError::FallbackTimeAboveMaxStoredReports
        );

        // The sync scan never runs for SVR data IDs without iterations.
        require!(
            !data_id::is_svr(&config.data_id) || config.max_sync_iterations != 0,
            StoreError::InvalidMaxSyncIterations
        );

        // Answers are stored with STORED_DECIMALS, so a proxy can only ever downscale.
        require!(
            config.decimals <= STORED_DECIMALS,
            StoreError::DecimalsAboveStoredDecimals
        );

        let (address, bump) =
            Pubkey::find_program_address(&[PROXY_SEED, state_key.as_ref(), proxy_id], &crate::ID);
        require_keys_eq!(address, *proxy_info.key, StoreError::AccountMismatch);

        let mut proxy = if proxy_info.data_is_empty() {
            create_pda_account(
                ANCHOR_DISCRIMINATOR + ProxyConfig::INIT_SPACE,
                &[PROXY_SEED, state_key.as_ref(), proxy_id, &[bump]],
                &ctx.accounts.feed_admin.to_account_info(),
                proxy_info,
                &ctx.accounts.system_program.to_account_info(),
            )?;
            ProxyConfig {
                version: ACCOUNT_VERSION,
                state: state_key,
                proxy_id: *proxy_id,
                data_id: [0; 16],
                decimals: 0,
                fallback_time: 0,
                max_sync_iterations: 0,
                _reserved: [0; 32],
            }
        } else {
            load_proxy(proxy_info, &state_key, proxy_id)?
        };

        proxy.data_id = config.data_id;
        proxy.decimals = config.decimals;
        proxy.fallback_time = config.fallback_time;
        proxy.max_sync_iterations = config.max_sync_iterations;
        proxy.try_serialize(&mut &mut proxy_info.try_borrow_mut_data()?[..])?;

        emit!(ProxyConfigUpdated {
            state: state_key,
            proxy: proxy_info.key(),
            proxy_id: *proxy_id,
            data_id: config.data_id,
            decimals: config.decimals,
            fallback_time: config.fallback_time,
            max_sync_iterations: config.max_sync_iterations,
        });
    }
    Ok(())
}

/// Remove proxy configs, closing their accounts.
pub fn remove_configs_for_proxies<'info>(
    ctx: Context<'_, '_, 'info, 'info, FeedAdminAccounts<'info>>,
    proxy_ids: Vec<[u8; 32]>,
) -> Result<()> {
    verify_feed_admin(
        &ctx.accounts.feed_admin,
        &ctx.accounts.state.load()?.feed_admins,
    )?;
    require_eq!(
        proxy_ids.len(),
        ctx.remaining_accounts.len(),
        StoreError::ArrayLengthMismatch
    );

    let state_key = ctx.accounts.state.key();
    for (proxy_id, info) in proxy_ids.iter().zip(ctx.remaining_accounts.iter()) {
        let proxy = load_proxy(info, &state_key, proxy_id)?;
        close_account(info, &ctx.accounts.feed_admin.to_account_info())?;

        emit!(ProxyConfigRemoved {
            state: state_key,
            proxy: info.key(),
            proxy_id: *proxy_id,
            data_id: proxy.data_id,
        });
    }
    Ok(())
}

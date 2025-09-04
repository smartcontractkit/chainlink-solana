use crate::{
    common::{ANCHOR_DISCRIMINATOR, SUBMIT_DISCRIMINATOR},
    context::OnReport,
    error::DataCacheError,
    event::{
        DecimalReportUpdated, InvalidUpdatePermission, LegacyFeedsReported, StaleDecimalReport,
    },
    invoke_signed,
    state::{DecimalReport, LegacyFeedEntry, ReceivedDecimalReport, WritePermissionFlag},
    utils::{create_report_hash, get_workflow_metadata},
    CacheTransmission,
};
use anchor_lang::{prelude::*, solana_program::instruction::Instruction};

type LegacyWriteEntry<'a> = (&'a LegacyFeedEntry, &'a ReceivedDecimalReport);

pub fn handler<'info>(
    ctx: Context<'_, '_, '_, 'info, OnReport<'info>>,
    metadata: Vec<u8>,
    report: Vec<u8>,
) -> Result<()> {
    let legacy_feeds_config = if let Some(loader) = &ctx.accounts.legacy_feeds_config {
        let loader = loader.load()?;

        // if included, check that the legacy store passed in via account context is the same as the one in the config
        if let Some(legacy_store) = &ctx.accounts.legacy_store {
            require_keys_eq!(
                *legacy_store.key,
                loader.legacy_store,
                DataCacheError::AccountMismatch
            );
        };

        Some(loader)
    } else {
        None
    };

    // first assume we don't have legacy_store or legacy_feed_config

    let (workflow_name, workflow_owner) = get_workflow_metadata(&metadata)?;

    let received_decimal_reports = Vec::<ReceivedDecimalReport>::try_from_slice(&report)
        .map_err(|_| DataCacheError::MalformedReport)?;

    let len = received_decimal_reports.len();

    let report_account_infos = &ctx.remaining_accounts[..len];
    let permission_flag_account_infos = &ctx.remaining_accounts[len..2 * len];
    let legacy_feed_account_infos = &ctx.remaining_accounts[2 * len..];

    // sorted by key
    let legacy_accounts_sorted = legacy_feed_account_infos
        .windows(2)
        .all(|w| w[0].key.lt(w[1].key));

    require!(
        legacy_accounts_sorted,
        DataCacheError::AddressesMustStrictlyIncrease
    );

    require_eq!(
        report_account_infos.len(),
        received_decimal_reports.len(),
        DataCacheError::ArrayLengthMismatch
    );

    require_eq!(
        permission_flag_account_infos.len(),
        received_decimal_reports.len(),
        DataCacheError::ArrayLengthMismatch
    );

    let mut candidate_legacy_writes: Vec<(&LegacyFeedEntry, &ReceivedDecimalReport)> = Vec::new();

    for (i, received_decimal_report) in received_decimal_reports.iter().enumerate() {
        // 1. check that sender has permission to write
        let report_hash = create_report_hash(
            &received_decimal_report.data_id,
            ctx.accounts.forwarder_authority.key,
            workflow_owner,
            workflow_name,
        );

        let (curr_permission_flag, _) = Pubkey::find_program_address(
            &[
                b"permission_flag",
                ctx.accounts.cache_state.key().as_ref(),
                &report_hash,
            ],
            &crate::ID,
        );

        require_keys_eq!(
            curr_permission_flag,
            *permission_flag_account_infos[i].key,
            DataCacheError::AccountMismatch
        );

        // verifies the permission account exists
        if WritePermissionFlag::try_deserialize(
            &mut &permission_flag_account_infos[i].data.borrow()[..],
        )
        .is_err()
        {
            emit!(InvalidUpdatePermission {
                state: ctx.accounts.cache_state.key(),
                data_id: received_decimal_report.data_id,
                sender: ctx.accounts.forwarder_authority.key(),
                workflow_owner: workflow_owner
                    .try_into()
                    .map_err(|_| DataCacheError::InvalidLength)?,
                workflow_name: workflow_name
                    .try_into()
                    .map_err(|_| DataCacheError::InvalidLength)?,
            });

            continue;
        }

        // 2. check report account is valid
        let (curr_report, _) = Pubkey::find_program_address(
            &[
                b"decimal_report",
                ctx.accounts.cache_state.key().as_ref(),
                &received_decimal_report.data_id,
            ],
            &crate::ID,
        );

        require_keys_eq!(
            curr_report,
            *report_account_infos[i].key,
            DataCacheError::AccountMismatch
        );

        // update report

        let latest_report =
            DecimalReport::try_deserialize(&mut &report_account_infos[i].data.borrow()[..])?;

        // don't update if the received report is stale
        if received_decimal_report.timestamp <= latest_report.timestamp {
            emit!(StaleDecimalReport {
                state: ctx.accounts.cache_state.key(),
                data_id: received_decimal_report.data_id,
                received_timestamp: received_decimal_report.timestamp,
                latest_timestamp: latest_report.timestamp
            });

            continue;
        }

        let mut dst = report_account_infos[i].try_borrow_mut_data()?;

        let updated_report = DecimalReport {
            answer: received_decimal_report.answer,
            timestamp: received_decimal_report.timestamp,
        };

        updated_report.serialize(&mut &mut dst[ANCHOR_DISCRIMINATOR..])?;

        emit!(DecimalReportUpdated {
            state: ctx.accounts.cache_state.key(),
            answer: received_decimal_report.answer,
            timestamp: received_decimal_report.timestamp,
            data_id: received_decimal_report.data_id
        });

        // 3. check if the report is also associated with a legacy feed
        // a. search config by data_id to get the account key
        // b. search passed in legacy_feed_account_infos by key

        if let Some(config) = &legacy_feeds_config {
            // a given legacy feed will only write under conditions
            // I. legacy feed config is provided
            // II. data id is associated with a legacy feed in the config
            // III. the legacy store is provided
            // IV. legacy writer is provided
            // V. writes are not disabled for that legacy feed
            // VI. the legacy feed is provided in account context

            // condition I and II: if the data id is associated with a legacy feed
            if let Some(entry) = config
                .id_to_feed
                .binary_search_by(|e| e.data_id.cmp(&received_decimal_report.data_id))
                .ok()
                .and_then(|index| config.id_to_feed.get(index))
            {
                candidate_legacy_writes.push((entry, received_decimal_report));
            }
        }
    }

    // or add generic dfc event out here
    // seperate out write disabled entries
    let (write_disabled_entries, write_enabled_entries): (
        Vec<LegacyWriteEntry>,
        Vec<LegacyWriteEntry>,
    ) = candidate_legacy_writes
        .iter()
        .partition(|e| e.0.write_disabled != 0);

    let mut write_occurred = false;

    // condition III & condition IV
    if let (Some(legacy_store), Some(legacy_writer)) =
        (&ctx.accounts.legacy_store, &ctx.accounts.legacy_writer)
    {
        // use legacy_store and legacy_writer here

        let mut ordered_legacy_feed_account_infos: Vec<&AccountInfo> = Vec::new();

        // condition V
        for entry in write_enabled_entries.iter() {
            // condition VI: error if legacy feed account not supplied in account context
            let account = legacy_feed_account_infos
                .binary_search_by(|a| a.key.cmp(&entry.0.legacy_feed))
                .map(|i| &legacy_feed_account_infos[i])
                .map_err(|_| DataCacheError::MissingLegacyFeedAccount)?;

            ordered_legacy_feed_account_infos.push(account);
        }

        // write to store program
        if !write_enabled_entries.is_empty() {
            let metas: Vec<AccountMeta> = std::iter::once(AccountMeta {
                pubkey: legacy_writer.key(),
                is_signer: true,
                is_writable: false,
            })
            .chain(
                ordered_legacy_feed_account_infos
                    .iter()
                    .map(|acc| AccountMeta {
                        pubkey: *acc.key,
                        is_signer: false,
                        is_writable: true,
                    }),
            )
            .collect();

            let account_infos: Vec<AccountInfo<'info>> =
                std::iter::once(legacy_writer.to_account_info())
                    .chain(
                        ordered_legacy_feed_account_infos
                            .iter()
                            .map(|val| val.to_account_info()),
                    )
                    .collect();

            // payload begins with the Anchor discriminator
            let mut payload = SUBMIT_DISCRIMINATOR.to_vec();

            let transmissions: Vec<CacheTransmission> = write_enabled_entries
                .iter()
                .map(|e| CacheTransmission {
                    timestamp: e.1.timestamp,
                    answer: e.1.answer,
                })
                .collect();

            payload.extend(transmissions.try_to_vec()?);

            let cache_state_key = ctx.accounts.cache_state.key();

            let ix = Instruction::new_with_bytes(legacy_store.key(), &payload, metas);
            let signer_seeds = &[
                b"legacy_writer",
                cache_state_key.as_ref(),
                &[ctx.accounts.cache_state.load()?.legacy_writer_bump],
            ];

            invoke_signed(&ix, &account_infos, &[signer_seeds])
                .map_err(|_| DataCacheError::FailedLegacyWrite)?;

            write_occurred = true;
        }
    }

    // emit legacy event only if there were candidates identified by the feed config
    if !candidate_legacy_writes.is_empty() {
        let (feeds_skipped, feeds_written) = if write_occurred {
            (write_disabled_entries, write_enabled_entries)
        } else {
            (candidate_legacy_writes, vec![])
        };

        emit!(LegacyFeedsReported {
            state: ctx.accounts.cache_state.key(),
            feeds_skipped: feeds_skipped.iter().map(|e| e.0.data_id).collect(),
            feeds_written: feeds_written.iter().map(|e| e.0.data_id).collect(),
        });
    }

    Ok(())
}

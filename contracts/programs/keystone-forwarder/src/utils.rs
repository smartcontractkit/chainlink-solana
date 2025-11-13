use crate::common::{METADATA_LENGTH, REPORT_CONTEXT_LEN, SIGNATURE_LEN};
use crate::error::ForwarderError;
use crate::state::OraclesConfig;
use anchor_lang::prelude::{borsh::BorshDeserialize, *};
use anchor_lang::solana_program::hash;
use anchor_lang::solana_program::hash::Hash;

use super::verify_signatures;

pub fn get_config_id(don_id: u32, config_version: u32) -> u64 {
    ((don_id as u64) << 32) | (config_version as u64)
}

pub fn report_size_ok(data: &[u8]) -> bool {
    if !data.is_empty() {
        let num_signatures = data[0] as usize;
        data.len() > 1 + num_signatures * SIGNATURE_LEN + METADATA_LENGTH + REPORT_CONTEXT_LEN
    } else {
        false
    }
}

pub fn extract_and_verify_signatures(data: &[u8], oracles_config: &OraclesConfig) -> Result<usize> {
    require!(report_size_ok(&data), ForwarderError::InvalidReport);
    let num_signatures = data[0] as usize;

    // get config
    require_gte!(
        num_signatures,
        (oracles_config.f + 1) as usize,
        ForwarderError::InvalidSignatureCount
    );
    let data = &data[1..];
    let total_signature_len: usize = SIGNATURE_LEN * num_signatures;

    let signatures: &[u8] = &data[..total_signature_len];
    // raw_report | report context
    let data = &data[total_signature_len..];

    // Build the preimage the same way the OCR keyring does:
    // SHA256( [u8(len(raw_report))] || raw_report || ctx)
    let mut preimage = vec![0u8; 1 + data.len()];

    let raw_report_len = data.len() - REPORT_CONTEXT_LEN;
    // OCR keyring also does not error on overflow
    let raw_report_len_u8: u8 = raw_report_len as u8;

    preimage[0] = raw_report_len_u8;
    preimage[1..].copy_from_slice(data);

    let hashed_report = hash::hash(&preimage).to_bytes();

    verify_signatures(&hashed_report, signatures, &oracles_config, num_signatures)?;

    Ok(raw_report_len)
}

pub fn verify_account_hash<'info>(
    account_infos: Vec<AccountInfo<'info>>,
    account_hash: [u8; 32],
) -> Result<()> {
    // Flatten account pubkeys into one contiguous byte vector
    let account_key_bytes = account_infos.iter().fold(
        Vec::with_capacity(account_infos.len() * 32),
        |mut buf, x| {
            buf.extend_from_slice(&x.key().to_bytes());
            buf
        },
    );

    // Compute and verify
    let computed_account_hash = hash::hash(&account_key_bytes);
    require_eq!(
        computed_account_hash,
        Hash::from(account_hash),
        ForwarderError::InvalidAccountHash
    );

    Ok(())
}

// data = len_signatures (1) | signatures (N*65) | raw_report (M) | report_context (96)
pub fn extract_raw_report(data: &[u8]) -> &[u8] {
    let num_signatures = data[0] as usize;
    let data = &data[1..];
    let _signatures = &data[..num_signatures * SIGNATURE_LEN];
    let data = &data[num_signatures * SIGNATURE_LEN..];
    let _report_context = &data[data.len() - REPORT_CONTEXT_LEN..];

    // raw report
    &data[..data.len() - REPORT_CONTEXT_LEN]
}

// version                offset   0, size  1
// workflow_execution_id  offset   1, size 32
// timestamp              offset  33, size  4
// don_id                 offset  37, size  4
// don_config_version     offset  41, size  4
// workflow_cid           offset  45, size 32
// workflow_name          offset  77, size 10
// workflow_owner         offset  87, size 20
// report_id              offset 107, size  2

pub fn extract_config_id(raw_report: &[u8]) -> [u8; 8] {
    // don_id | don_config_version
    raw_report[37..45].try_into().expect("Expected 8 bytes")
}

pub fn extract_transmission_id(raw_report: &[u8], receiver: &Pubkey) -> [u8; 32] {
    let workflow_execution_id = &raw_report[1..33];
    let report_id = &raw_report[107..109];

    // use sha-256 (instead of keccak-256)
    hash::hash(&[&receiver.to_bytes(), workflow_execution_id, report_id].concat()).to_bytes()
}

#[derive(BorshDeserialize)]
pub struct ForwarderReport {
    pub account_hash: [u8; 32],
    pub payload: Vec<u8>,
}

use crate::common::{METADATA_LENGTH, REPORT_CONTEXT_LEN, SIGNATURE_LEN};
use anchor_lang::prelude::{borsh::BorshDeserialize, *};
use anchor_lang::solana_program::hash;

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

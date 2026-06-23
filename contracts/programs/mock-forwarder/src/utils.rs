use crate::common::{REPORT_CONTEXT_LEN, SIGNATURE_LEN};
use anchor_lang::prelude::{borsh::BorshDeserialize, *};
use anchor_lang::solana_program::hash;

pub fn report_size_ok(data: &[u8]) -> bool {
    if !data.is_empty() {
        let num_signatures = data[0] as usize;
        data.len() > 1 + num_signatures * SIGNATURE_LEN + crate::common::METADATA_LENGTH + REPORT_CONTEXT_LEN
    } else {
        false
    }
}

// data = len_signatures (1) | signatures (N*65) | raw_report (M) | report_context (96)
pub fn extract_raw_report(data: &[u8]) -> &[u8] {
    let num_signatures = data[0] as usize;
    let data = &data[1..];
    let data = &data[num_signatures * SIGNATURE_LEN..];
    &data[..data.len() - REPORT_CONTEXT_LEN]
}

// Layout matches keystone-forwarder; see `keystone-forwarder/src/utils.rs`.
pub fn extract_transmission_id(raw_report: &[u8], receiver: &Pubkey) -> [u8; 32] {
    let workflow_execution_id = &raw_report[1..33];
    let report_id = &raw_report[107..109];

    hash::hash(&[&receiver.to_bytes(), workflow_execution_id, report_id].concat()).to_bytes()
}

#[derive(BorshDeserialize)]
pub struct ForwarderReport {
    pub account_hash: [u8; 32],
    pub payload: Vec<u8>,
}

use anchor_lang::prelude::{borsh::BorshSerialize, *};

use crate::state::WorkflowMetadata;

#[event]
pub struct DecimalFeedConfigSet {
    pub data_id: [u8; 16],
    pub decimals: u8,
    pub description: [u8; 32],
    pub workflow_metadatas: Vec<WorkflowMetadata>,
    pub stale_permission_flags: Vec<Pubkey>,
}

// todo: should be per
#[event]
pub struct LegacyFeedsReported {
    pub feeds_skipped: Vec<[u8; 16]>,
    pub feeds_written: Vec<[u8; 16]>,
}

// #[derive(BorshSerialize, BorshDeserialize)]
// pub struct EmittedWorkflowMetadata {
//     pub allowed_sender: Pubkey, // Address of the sender allowed to send new reports (forwarder)
//     pub allowed_workflow_owner: [u8; 20], // ─╮ Address of the workflow owner
//     pub allowed_workflow_name: [u8; 32] // ──╯ Name of the workflow UTF-bytes encoded
// }

// impl From<&WorkflowMetadata> for EmittedWorkflowMetadata {
//     fn from(m: &WorkflowMetadata) -> Self {
//         Self {
//             allowed_sender: m.allowed_sender.clone(),
//             allowed_workflow_owner: m.allowed_workflow_owner.clone(),
//             allowed_workflow_name: m.allowed_workflow_name.clone(),
//         }
//     }
// }

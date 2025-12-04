use anchor_lang::prelude::*;

declare_id!("CGn5MQX5GK9qKqERhjnADhd6i2LiSF6XUC2ewUHND1Mw");

#[program]
pub mod mock_ccip_events {
    use super::*;

    pub fn initialize(
        _ctx: Context<Initialize>,
        sent_event: CCIPMessageSentObj,
        commit_event: CommitReportAcceptedObj,
        execute_event: ExecutionStateChangedObj,
        cctp_event: CcipCctpMessageSentEventObj,
    ) -> Result<()> {
        emit!(CCIPMessageSent {
            dest_chain_selector: sent_event.dest_chain_selector,
            message: sent_event.message,
            sequence_number: sent_event.sequence_number,
        });
        emit!(CommitReportAccepted {
            merkle_root: commit_event.merkle_root,
            price_updates: commit_event.price_updates,
        });
        emit!(ExecutionStateChanged {
            message_hash: execute_event.message_hash,
            message_id: execute_event.message_id,
            sequence_number: execute_event.sequence_number,
            source_chain_selector: execute_event.source_chain_selector,
            state: execute_event.state,
        });
        emit!(CcipCctpMessageSentEvent {
            cctp_nonce: cctp_event.cctp_nonce,
            event_address: cctp_event.event_address,
            message_sent_bytes: cctp_event.message_sent_bytes,
            msg_total_nonce: cctp_event.msg_total_nonce,
            original_sender: cctp_event.original_sender,
            remote_chain_selector: cctp_event.remote_chain_selector,
            source_domain: cctp_event.source_domain,
        });

        Ok(())
    }
}

#[derive(Accounts)]
pub struct Initialize {}

/// Events and structs copied from the CCIP contracts
/// https://github.com/smartcontractkit/chainlink-ccip/tree/main/chains/solana/contracts/programs

/// --------------------- CCIPMessageSent --------------------- ///
#[event]
pub struct CCIPMessageSent {
    pub dest_chain_selector: u64,
    pub sequence_number: u64,
    pub message: SVM2AnyRampMessage,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct CCIPMessageSentObj {
    pub dest_chain_selector: u64,
    pub sequence_number: u64,
    pub message: SVM2AnyRampMessage,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct SVM2AnyRampMessage {
    pub header: RampMessageHeader,
    pub sender: Pubkey,
    pub data: Vec<u8>,
    pub receiver: Vec<u8>,
    pub extra_args: Vec<u8>,
    pub fee_token: Pubkey,
    pub token_amounts: Vec<SVM2AnyTokenTransfer>,
    pub fee_token_amount: CrossChainAmount,
    pub fee_value_juels: CrossChainAmount,
}

#[derive(Clone, Copy, AnchorSerialize, AnchorDeserialize)]
pub struct RampMessageHeader {
    pub message_id: [u8; 32],
    pub source_chain_selector: u64,
    pub dest_chain_selector: u64,
    pub sequence_number: u64,
    pub nonce: u64,
}

#[derive(Debug, Clone, AnchorSerialize, AnchorDeserialize, Default)]
pub struct SVM2AnyTokenTransfer {
    pub source_pool_address: Pubkey,
    pub dest_token_address: Vec<u8>,
    pub extra_data: Vec<u8>,
    pub amount: CrossChainAmount,
    pub dest_exec_data: Vec<u8>,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, Default, Debug)]
pub struct CrossChainAmount {
    le_bytes: [u8; 32],
}

/// --------------------- CommitReportAccepted --------------------- ///
#[event]
pub struct CommitReportAccepted {
    pub merkle_root: Option<MerkleRoot>,
    pub price_updates: PriceUpdates,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct CommitReportAcceptedObj {
    pub merkle_root: Option<MerkleRoot>,
    pub price_updates: PriceUpdates,
}

#[derive(Default, Clone, AnchorSerialize, AnchorDeserialize)]
pub struct MerkleRoot {
    pub source_chain_selector: u64,
    pub on_ramp_address: Vec<u8>,
    pub min_seq_nr: u64,
    pub max_seq_nr: u64,
    pub merkle_root: [u8; 32],
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct PriceUpdates {
    pub token_price_updates: Vec<TokenPriceUpdate>,
    pub gas_price_updates: Vec<GasPriceUpdate>,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct TokenPriceUpdate {
    pub source_token: Pubkey,
    pub usd_per_token: [u8; 28],
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct GasPriceUpdate {
    pub dest_chain_selector: u64,
    pub usd_per_unit_gas: [u8; 28],
}

/// --------------------- ExecutionStateChanged --------------------- ///
#[event]
pub struct ExecutionStateChanged {
    pub source_chain_selector: u64,
    pub sequence_number: u64,
    pub message_id: [u8; 32],
    pub message_hash: [u8; 32],
    pub state: MessageExecutionState,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct ExecutionStateChangedObj {
    pub source_chain_selector: u64,
    pub sequence_number: u64,
    pub message_id: [u8; 32],
    pub message_hash: [u8; 32],
    pub state: MessageExecutionState,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug, PartialEq)]
pub enum MessageExecutionState {
    Untouched = 0,
    InProgress = 1,
    Success = 2,
    Failure = 3,
}

/// --------------------- CcipCctpMessageSentEvent --------------------- ///
#[event]
pub struct CcipCctpMessageSentEvent {
    pub original_sender: Pubkey,
    pub remote_chain_selector: u64,
    pub msg_total_nonce: u64,
    pub event_address: Pubkey,
    pub source_domain: u32,
    pub cctp_nonce: u64,
    pub message_sent_bytes: Vec<u8>,
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize)]
pub struct CcipCctpMessageSentEventObj {
    pub original_sender: Pubkey,
    pub remote_chain_selector: u64,
    pub msg_total_nonce: u64,
    pub event_address: Pubkey,
    pub source_domain: u32,
    pub cctp_nonce: u64,
    pub message_sent_bytes: Vec<u8>,
}

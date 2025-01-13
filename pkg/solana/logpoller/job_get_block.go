package logpoller

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// getBlockJob is a job that fetches transaction signatures from a block and loads
// the job queue with getTransactionLogsJobs for each transaction found in the block.
type getBlockJob struct {
	slotNumber uint64
	client     RPCClient
	blocks     chan Block
	done       chan struct{}
}

func newGetBlockJob(client RPCClient, blocks chan Block, slotNumber uint64) *getBlockJob {
	return &getBlockJob{
		client:     client,
		blocks:     blocks,
		slotNumber: slotNumber,
		done:       make(chan struct{}),
	}
}

func (j *getBlockJob) String() string {
	return fmt.Sprintf("getBlock for slotNumber: %d", j.slotNumber)
}

func (j *getBlockJob) Done() <-chan struct{} {
	return j.done
}

func (j *getBlockJob) Run(ctx context.Context) error {
	var excludeRewards bool
	// TODO: move min version definition to an RPC Client
	// NOTE: if max supported transaction version is changed after creation of a block that contains transactions of a new version
	// we at risk of producing duplicate events! To avoid this we'll need to do block based migration.
	version := uint64(0) // pull all tx types (legacy + v0)
	block, err := j.client.GetBlockWithOpts(
		ctx,
		j.slotNumber,
		// NOTE: any change to the filtering argmuments may affect calculation of logIndex, which to lead to events duplication.
		&rpc.GetBlockOpts{
			Encoding:   solana.EncodingBase64,
			Commitment: rpc.CommitmentFinalized,
			// get the full transaction details
			TransactionDetails:             rpc.TransactionDetailsFull,
			MaxSupportedTransactionVersion: &version,
			// exclude rewards
			Rewards: &excludeRewards,
		},
	)
	if err != nil {
		return err
	}

	detail := eventDetail{
		slotNumber: j.slotNumber,
		blockHash:  block.Blockhash,
	}

	if block.BlockHeight != nil {
		detail.blockHeight = *block.BlockHeight
	}

	events := make([]ProgramEvent, 0, len(block.Transactions))
	for idx, txWithMeta := range block.Transactions {
		detail.trxIdx = idx
		tx, err := txWithMeta.GetTransaction()
		if err != nil {
			return fmt.Errorf("failed to parse transaction %d in slot %d: %w", idx, txWithMeta.Slot, err)
		}
		if len(tx.Signatures) == 0 {
			return fmt.Errorf("expected all transactions to have at least one signature %d in slot %d", idx, txWithMeta.Slot)
		}
		detail.trxSig = tx.Signatures[0] // according to Solana docs fist signature is used as ID

		txEvents := j.messagesToEvents(txWithMeta.Meta.LogMessages, detail)
		events = append(events, txEvents...)
	}

	result := Block{
		SlotNumber: j.slotNumber,
		BlockHash:  block.Blockhash,
		Events:     events,
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case j.blocks <- result:
		close(j.done)
	}

	return nil
}

func (j *getBlockJob) messagesToEvents(messages []string, detail eventDetail) []ProgramEvent {
	// TODO: changes to parsing might cause changes in logIdx generate, we might want to find a more stable way of doing it.
	var logIdx uint
	// TODO: only parse logs produced by CL contracts, otherwise if we enable custom user calls, it will be possible to forge a msg.
	events := make([]ProgramEvent, 0, len(messages))
	for _, outputs := range parseProgramLogs(messages) {
		for i, event := range outputs.Events {
			event.SlotNumber = detail.slotNumber
			event.BlockHeight = detail.blockHeight
			event.BlockHash = detail.blockHash
			event.TransactionHash = detail.trxSig
			event.TransactionIndex = detail.trxIdx
			event.TransactionLogIndex = logIdx

			logIdx++
			outputs.Events[i] = event
		}

		events = append(events, outputs.Events...)
	}

	return events
}

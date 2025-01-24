package logpoller

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
)

// getBlockJob is a job that fetches a block with transactions, converts logs into ProgramEvents and writes them into blocks channel
type getBlockJob struct {
	slotNumber       uint64
	client           RPCClient
	blocks           chan Block
	done             chan struct{}
	parseProgramLogs func(logs []string) []ProgramOutput
	lggr             logger.SugaredLogger
}

func newGetBlockJob(client RPCClient, blocks chan Block, lggr logger.SugaredLogger, slotNumber uint64) *getBlockJob {
	return &getBlockJob{
		client:           client,
		blocks:           blocks,
		slotNumber:       slotNumber,
		done:             make(chan struct{}),
		parseProgramLogs: parseProgramLogs,
		lggr:             lggr,
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
	version := client.MaxSupportTransactionVersion
	block, err := j.client.GetBlockWithOpts(
		ctx,
		j.slotNumber,
		// NOTE: any change to the filtering arguments may affect calculation of txIndex, which could lead to events duplication.
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

	if block.BlockHeight == nil {
		return fmt.Errorf("block at slot %d returned from rpc is missing block number", j.slotNumber)
	}
	detail.blockHeight = *block.BlockHeight

	if block.BlockTime == nil {
		return fmt.Errorf("block at slot %d returned from rpc is missing block time", j.slotNumber)
	}
	detail.blockTime = *block.BlockTime

	events := make([]ProgramEvent, 0, len(block.Transactions))
	for idx, txWithMeta := range block.Transactions {
		detail.trxIdx = idx
		if txWithMeta.Transaction == nil {
			return fmt.Errorf("failed to parse transaction %d in slot %d: %w", idx, j.slotNumber, errors.New("missing transaction field"))
		}
		tx, err := txWithMeta.GetTransaction()
		if err != nil {
			return fmt.Errorf("failed to parse transaction %d in slot %d: %w", idx, j.slotNumber, err)
		}
		if len(tx.Signatures) == 0 {
			return fmt.Errorf("expected all transactions to have at least one signature %d in slot %d", idx, j.slotNumber)
		}
		if txWithMeta.Meta == nil {
			return fmt.Errorf("expected transaction to have meta. signature: %s; slot: %d; idx: %d", tx.Signatures[0], j.slotNumber, idx)
		}
		if txWithMeta.Meta.Err != nil {
			j.lggr.Debugw("Skipping all events of failed transaction", "err", txWithMeta.Meta.Err, "signature", tx.Signatures[0])
			continue
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
	var logIdx uint
	events := make([]ProgramEvent, 0, len(messages))
	for _, outputs := range j.parseProgramLogs(messages) {
		for i, event := range outputs.Events {
			event.SlotNumber = detail.slotNumber
			event.BlockHeight = detail.blockHeight
			event.BlockHash = detail.blockHash
			event.BlockTime = detail.blockTime
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

type eventDetail struct {
	slotNumber  uint64
	blockHeight uint64
	blockHash   solana.Hash
	blockTime   solana.UnixTimeSeconds
	trxIdx      int
	trxSig      solana.Signature
}

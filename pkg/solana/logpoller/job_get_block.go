package logpoller

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-common/pkg/services"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

// getBlockJob is a job that fetches a block with transactions, converts logs into ProgramEvents and writes them into blocks channel
type getBlockJob struct {
	slotNumber        uint64
	stopCh            services.StopRChan
	client            RPCClient
	blocks            chan types.Block
	done              chan struct{}
	parseProgramLogs  func(logs []string) ([]types.ProgramOutput, error)
	cpiEventExtractor *CPIEventExtractor
	lggr              logger.SugaredLogger
	metrics           *solLpMetrics
	aborted           bool
}

func newGetBlockJob(stopCh services.StopRChan, client RPCClient, blocks chan types.Block, lggr logger.SugaredLogger, slotNumber uint64, metrics *solLpMetrics, cpiEventExtractor *CPIEventExtractor) *getBlockJob {
	return &getBlockJob{
		client:            client,
		blocks:            blocks,
		slotNumber:        slotNumber,
		done:              make(chan struct{}),
		parseProgramLogs:  ParseProgramLogs,
		cpiEventExtractor: cpiEventExtractor,
		lggr:              lggr,
		stopCh:            stopCh,
		metrics:           metrics,
	}
}

func (j *getBlockJob) String() string {
	return fmt.Sprintf("getBlock for slotNumber: %d", j.slotNumber)
}

func (j *getBlockJob) Done() <-chan struct{} {
	return j.done
}

func (j *getBlockJob) Abort(ctx context.Context) error {
	j.aborted = true
	var abort types.Block
	abort.Aborted = true
	abort.SlotNumber = j.slotNumber
	select {
	case <-ctx.Done():
		return ctx.Err()
	case j.blocks <- abort:
		close(j.done)
	}

	return nil
}

func (j *getBlockJob) Run(ctx context.Context) error {
	ctx, cancel := j.stopCh.Ctx(ctx)
	defer cancel()
	if ctx.Err() != nil {
		return ctx.Err()
	}
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
		oldestAvailableSlot, err2 := j.client.GetFirstAvailableBlock(ctx)
		if err2 != nil {
			return fmt.Errorf("failed to get first available slot: %w", err2)
		}
		if oldestAvailableSlot > j.slotNumber {
			j.lggr.Warnf("slot %d is pruned away, as oldest available slot is %d. skipping this slot", j.slotNumber, oldestAvailableSlot)
			result := types.Block{
				SlotNumber: j.slotNumber,
				BlockHash:  nil,
				Events:     []types.ProgramEvent{},
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case j.blocks <- result:
				close(j.done)
			}
			return nil
		}
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

	events := make([]types.ProgramEvent, 0, len(block.Transactions))
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
		detail.trxSig = tx.Signatures[0] // according to Solana docs first signature is used as ID
		detail.err = txWithMeta.Meta.Err

		txOutcome := txSucceeded
		if txWithMeta.Meta.Err != nil {
			txOutcome = txReverted
		}

		txEvents := j.messagesToEvents(ctx, txWithMeta.Meta.LogMessages, detail, txOutcome)
		events = append(events, txEvents...)

		// Look for events corresponding to CPI filters
		if j.cpiEventExtractor != nil && j.cpiEventExtractor.HasCPIFilters() {
			cpiEvents := j.cpiEventExtractor.ExtractCPIEvents(tx, txWithMeta.Meta, detail, uint(len(txEvents)))
			events = append(events, cpiEvents...)
		}
	}

	j.lggr.Debugw("found events", "count", len(events), "slot", j.slotNumber)

	result := types.Block{
		SlotNumber: j.slotNumber,
		BlockHash:  &block.Blockhash,
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

func (j *getBlockJob) messagesToEvents(ctx context.Context, messages []string, detail eventDetail, txOutcome txOutcome) []types.ProgramEvent {
	var logIdx uint
	events := make([]types.ProgramEvent, 0, len(messages))
	outputs, err := j.parseProgramLogs(messages)
	if err != nil {
		j.lggr.Errorf("failed to parse program logs at slot %d for tx %s. Skipping tx due to error: %v", detail.slotNumber, detail.trxSig, err)
		j.metrics.IncrementTxsLogParsingError(ctx, txOutcome)
		return events
	}
	for _, outputs := range outputs {
		for i, event := range outputs.Events {
			event.SlotNumber = detail.slotNumber
			event.BlockHeight = detail.blockHeight
			event.BlockHash = detail.blockHash
			event.BlockTime = detail.blockTime
			event.TransactionHash = detail.trxSig
			event.TransactionIndex = detail.trxIdx
			event.TransactionLogIndex = logIdx
			event.Error = detail.err

			logIdx++
			outputs.Events[i] = event
		}

		if outputs.Truncated {
			j.lggr.Warnw("Encountered truncated logs", "program", outputs.Program, "detail", detail)
			j.metrics.IncrementTruncatedTxs(ctx, txOutcome)
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
	err         interface{}
}

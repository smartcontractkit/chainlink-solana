package logpoller

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
)

type ProgramEventProcessor interface {
	// Process should take a ProgramEvent and parseProgramLogs it based on log signature
	// and expected encoding. Only return errors that cannot be handled and
	// should exit further transaction processing on the running thread.
	//
	// Process should be thread safe.
	Process(ProgramEvent) error
}

type RPCClient interface {
	GetLatestBlockhash(ctx context.Context, commitment rpc.CommitmentType) (out *rpc.GetLatestBlockhashResult, err error)
	GetBlocks(ctx context.Context, startSlot uint64, endSlot *uint64, commitment rpc.CommitmentType) (out rpc.BlocksResult, err error)
	GetBlockWithOpts(context.Context, uint64, *rpc.GetBlockOpts) (*rpc.GetBlockResult, error)
	GetSignaturesForAddressWithOpts(context.Context, solana.PublicKey, *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error)
}

const (
	DefaultNextSlotPollingInterval = 1_000 * time.Millisecond
)

type EncodedLogCollector struct {
	// service state management
	services.Service
	engine *services.Engine

	// dependencies and configuration
	client       RPCClient
	parser       ProgramEventProcessor
	lggr         logger.Logger
	rpcTimeLimit time.Duration

	// internal state
	chSlot  chan uint64
	chBlock chan uint64
	chJobs  chan Job
	workers *WorkerGroup

	highestSlot       atomic.Uint64
	highestSlotLoaded atomic.Uint64
	lastSentSlot      atomic.Uint64
}

func NewEncodedLogCollector(
	client RPCClient,
	parser ProgramEventProcessor,
	lggr logger.Logger,
) *EncodedLogCollector {
	c := &EncodedLogCollector{
		client:       client,
		parser:       parser,
		chSlot:       make(chan uint64),
		chBlock:      make(chan uint64, 1),
		chJobs:       make(chan Job, 1),
		lggr:         lggr,
		rpcTimeLimit: 1 * time.Second,
	}

	c.Service, c.engine = services.Config{
		Name: "EncodedLogCollector",
		NewSubServices: func(lggr logger.Logger) []services.Service {
			c.workers = NewWorkerGroup(DefaultWorkerCount, lggr)

			return []services.Service{c.workers}
		},
		Start: c.start,
		Close: c.close,
	}.NewServiceEngine(lggr)

	return c
}

func (c *EncodedLogCollector) BackfillForAddress(ctx context.Context, address string, fromSlot uint64) error {
	pubKey, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return err
	}

	var (
		lowestSlotRead uint64
		lowestSlotSig  solana.Signature
	)

	for lowestSlotRead > fromSlot || lowestSlotRead == 0 {
		opts := rpc.GetSignaturesForAddressOpts{
			Commitment:     rpc.CommitmentFinalized,
			MinContextSlot: &fromSlot,
		}

		if lowestSlotRead > 0 {
			opts.Before = lowestSlotSig
		}

		sigs, err := c.client.GetSignaturesForAddressWithOpts(ctx, pubKey, &opts)
		if err != nil {
			return err
		}

		if len(sigs) == 0 {
			break
		}

		// signatures ordered from newest to oldest, defined in the Solana RPC docs
		for _, sig := range sigs {
			lowestSlotSig = sig.Signature

			if sig.Slot >= lowestSlotRead && lowestSlotRead != 0 {
				continue
			}

			lowestSlotRead = sig.Slot

			if err := c.workers.Do(ctx, &getTransactionsFromBlockJob{
				slotNumber: sig.Slot,
				client:     c.client,
				parser:     c.parser,
				chJobs:     c.chJobs,
			}); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *EncodedLogCollector) start(ctx context.Context) error {
	c.engine.Go(c.runSlotPolling)
	c.engine.Go(c.runSlotProcessing)
	c.engine.Go(c.runBlockProcessing)
	c.engine.Go(c.runJobProcessing)

	return nil
}

func (c *EncodedLogCollector) close() error {
	return nil
}

func (c *EncodedLogCollector) runSlotPolling(ctx context.Context) {
	for {
		timer := time.NewTimer(DefaultNextSlotPollingInterval)

		select {
		case <-ctx.Done():
			timer.Stop()

			return
		case <-timer.C:
			ctxB, cancel := context.WithTimeout(ctx, c.rpcTimeLimit)

			// not to be run as a job, but as a blocking call
			result, err := c.client.GetLatestBlockhash(ctxB, rpc.CommitmentFinalized)
			if err != nil {
				c.lggr.Error("failed to get latest blockhash", "err", err)
				cancel()

				continue
			}

			cancel()

			// if the slot is not higher than the highest slot, skip it
			if c.lastSentSlot.Load() >= result.Context.Slot {
				continue
			}

			c.lastSentSlot.Store(result.Context.Slot)

			select {
			case c.chSlot <- result.Context.Slot:
			default:
			}
		}

		timer.Stop()
	}
}

func (c *EncodedLogCollector) runSlotProcessing(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case slot := <-c.chSlot:
			if c.highestSlot.Load() >= slot {
				continue
			}

			c.highestSlot.Store(slot)

			// load blocks in slot range
			c.loadRange(ctx, c.highestSlotLoaded.Load()+1, slot)
		}
	}
}

func (c *EncodedLogCollector) runBlockProcessing(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case block := <-c.chBlock:
			if err := c.workers.Do(ctx, &getTransactionsFromBlockJob{
				slotNumber: block,
				client:     c.client,
				parser:     c.parser,
				chJobs:     c.chJobs,
			}); err != nil {
				c.lggr.Errorf("failed to add job to queue: %s", err)
			}
		}
	}
}

func (c *EncodedLogCollector) runJobProcessing(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-c.chJobs:
			if err := c.workers.Do(ctx, job); err != nil {
				c.lggr.Errorf("failed to add job to queue: %s", err)
			}
		}
	}
}

func (c *EncodedLogCollector) loadRange(ctx context.Context, start, end uint64) {
	if err := c.loadSlotBlocksRange(ctx, start, end); err != nil {
		// a retry will happen anyway on the next round of slots
		// so the error is handled by doing nothing
		c.lggr.Errorw("failed to load slot blocks range", "start", start, "end", end, "err", err)

		return
	}

	c.highestSlotLoaded.Store(end)
}

func (c *EncodedLogCollector) loadSlotBlocksRange(ctx context.Context, start, end uint64) error {
	if start >= end {
		return errors.New("the start block must come before the end block")
	}

	var (
		result rpc.BlocksResult
		err    error
	)

	rpcCtx, cancel := context.WithTimeout(ctx, c.rpcTimeLimit)
	defer cancel()

	if result, err = c.client.GetBlocks(rpcCtx, start, &end, rpc.CommitmentFinalized); err != nil {
		return err
	}

	for _, block := range result {
		select {
		case <-ctx.Done():
			return nil
		case c.chBlock <- block:
		}
	}

	return nil
}

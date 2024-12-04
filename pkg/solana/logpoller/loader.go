package logpoller

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
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
	ordered      *orderedParser
	unordered    *unorderedParser
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
		unordered:    newUnorderedParser(parser),
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
			c.ordered = newOrderedParser(parser, lggr)

			return []services.Service{c.workers, c.ordered}
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
				parser:     c.unordered,
				chJobs:     c.chJobs,
			}); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *EncodedLogCollector) start(_ context.Context) error {
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

			from := c.highestSlot.Load() + 1
			if c.highestSlot.Load() == 0 {
				from = slot
			}

			c.highestSlot.Store(slot)

			// load blocks in slot range
			c.loadRange(ctx, from, slot)
		}
	}
}

func (c *EncodedLogCollector) runBlockProcessing(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case slot := <-c.chBlock:
			if err := c.workers.Do(ctx, &getTransactionsFromBlockJob{
				slotNumber: slot,
				client:     c.client,
				parser:     c.ordered,
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
		c.ordered.ExpectBlock(block)
		select {
		case <-ctx.Done():
			return nil
		case c.chBlock <- block:
		}
	}

	return nil
}

type unorderedParser struct {
	parser ProgramEventProcessor
}

func newUnorderedParser(parser ProgramEventProcessor) *unorderedParser {
	return &unorderedParser{parser: parser}
}

func (p *unorderedParser) ExpectBlock(_ uint64)      {}
func (p *unorderedParser) ExpectTxs(_ uint64, _ int) {}
func (p *unorderedParser) Process(evt ProgramEvent) error {
	return p.parser.Process(evt)
}

type orderedParser struct {
	// service state management
	services.Service
	engine *services.Engine

	// internal state
	parser ProgramEventProcessor
	mu     sync.Mutex
	blocks []uint64
	ready  []uint64
	expect map[uint64]int
	actual map[uint64][]ProgramEvent
}

func newOrderedParser(parser ProgramEventProcessor, lggr logger.Logger) *orderedParser {
	op := &orderedParser{
		parser: parser,
		blocks: make([]uint64, 0),
		ready:  make([]uint64, 0),
		expect: make(map[uint64]int),
		actual: make(map[uint64][]ProgramEvent),
	}

	op.Service, op.engine = services.Config{
		Name:  "OrderedParser",
		Start: op.start,
		Close: op.close,
	}.NewServiceEngine(lggr)

	return op
}

func (p *orderedParser) ExpectBlock(block uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.blocks = append(p.blocks, block)

	// ensure sort ascending
	sort.Slice(p.blocks, func(i, j int) bool {
		return p.blocks[i] < p.blocks[j]
	})
}

func (p *orderedParser) ExpectTxs(block uint64, quantity int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.expect[block] = quantity
	p.actual[block] = make([]ProgramEvent, 0, quantity)
}

func (p *orderedParser) Process(event ProgramEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.addToExpectations(event); err != nil {
		// TODO: log error because this is an unrecoverable error
		return nil
	}

	return p.sendReadySlots()
}

func (p *orderedParser) start(_ context.Context) error {
	p.engine.GoTick(services.NewTicker(time.Second), p.run)

	return nil
}

func (p *orderedParser) close() error {
	return nil
}

func (p *orderedParser) addToExpectations(evt ProgramEvent) error {
	expectations, ok := p.expect[evt.SlotNumber]
	if !ok {
		return fmt.Errorf("%w: %d", errExpectationsNotSet, evt.SlotNumber)
	}

	evts, ok := p.actual[evt.SlotNumber]
	if !ok {
		return fmt.Errorf("%w: %d", errExpectationsNotSet, evt.SlotNumber)
	}

	p.actual[evt.SlotNumber] = append(evts, evt)

	if expectations == len(evts)+1 {
		p.setReady(evt.SlotNumber)
	}

	return nil
}

func (p *orderedParser) expectations(block uint64) (int, bool, error) {
	expectations, ok := p.expect[block]
	if !ok {
		return 0, false, fmt.Errorf("%w: %d", errExpectationsNotSet, block)
	}

	evts, ok := p.actual[block]
	if !ok {
		return 0, false, fmt.Errorf("%w: %d", errExpectationsNotSet, block)
	}

	return expectations, expectations == len(evts), nil
}

func (p *orderedParser) clearExpectations(block uint64) {
	delete(p.expect, block)
	delete(p.actual, block)
}

func (p *orderedParser) setReady(slot uint64) {
	p.ready = append(p.ready, slot)
}

func (p *orderedParser) run(_ context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	_ = p.sendReadySlots()
}

func (p *orderedParser) sendReadySlots() error {
	rmvIdx := make([]int, 0)

	// start at the lowest block and find ready blocks
	for idx, block := range p.blocks {
		// if no expectations are set, we are still waiting on information for the block.
		// if expectations set and not met, we are still waiting on information for the block
		// no other block data should be sent until this is resolved
		exp, met, err := p.expectations(block)
		if err != nil || !met {
			break
		}

		// if expectations are 0 -> remove and continue
		if exp == 0 {
			p.clearExpectations(block)
			rmvIdx = append(rmvIdx, idx)

			continue
		}

		// if expectations set and met -> forward, remove, and continue

		// to ensure ordered delivery, break from the loop if a ready block isn't found
		// this function should be preceded by clearEmptyBlocks
		rIdx, ok := getIdx(p.ready, block)
		if !ok {
			return nil
		}

		evts, ok := p.actual[block]
		if !ok {
			return errors.New("invalid state")
		}

		var errs error
		for _, evt := range evts {
			errs = errors.Join(errs, p.parser.Process(evt))
		}

		// need possible retry
		if errs != nil {
			return errs
		}

		p.ready = remove(p.ready, rIdx)
		rmvIdx = append(rmvIdx, idx)

		delete(p.expect, block)
		delete(p.actual, block)
	}

	for count, idx := range rmvIdx {
		p.blocks = remove(p.blocks, idx-count)
	}

	return nil
}

func getIdx[T any](slice []T, match T) (int, bool) {
	for idx, value := range slice {
		if reflect.DeepEqual(value, match) {
			return idx, true
		}
	}

	return -1, false
}

func remove[T any](slice []T, s int) []T {
	return append(slice[:s], slice[s+1:]...)
}

var (
	errExpectationsNotSet = errors.New("expectations not set")
)

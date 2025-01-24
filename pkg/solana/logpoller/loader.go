package logpoller

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/worker"
)

type Block struct {
	SlotNumber uint64
	BlockHash  solana.Hash
	Events     []ProgramEvent
}

type ProgramEventProcessor interface {
	// Process should take a ProgramEvent and parseProgramLogs it based on log signature
	// and expected encoding. Only return errors that cannot be handled and
	// should exit further transaction processing on the running thread.
	//
	// Process should be thread safe.
	Process(Block) error
}

type RPCClient interface {
	GetBlockWithOpts(context.Context, uint64, *rpc.GetBlockOpts) (*rpc.GetBlockResult, error)
	GetSignaturesForAddressWithOpts(context.Context, solana.PublicKey, *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error)
	SlotHeightWithCommitment(ctx context.Context, commitment rpc.CommitmentType) (uint64, error)
}

type WorkerGroup interface {
	Do(ctx context.Context, job worker.Job) error
}
type EncodedLogCollector struct {
	// service state management
	services.Service
	engine *services.Engine

	// dependencies and configuration
	client RPCClient
	lggr   logger.SugaredLogger

	workers *worker.Group
}

func NewEncodedLogCollector(
	client RPCClient,
	lggr logger.SugaredLogger,
) *EncodedLogCollector {
	c := &EncodedLogCollector{
		client: client,
		lggr:   lggr,
	}

	c.Service, c.engine = services.Config{
		Name: "EncodedLogCollector",
		NewSubServices: func(lggr logger.Logger) []services.Service {
			c.workers = worker.NewGroup(worker.DefaultWorkerCount, logger.Sugared(lggr))

			return []services.Service{c.workers}
		},
	}.NewServiceEngine(lggr)

	return c
}

func (c *EncodedLogCollector) getSlotsToFetch(ctx context.Context, addresses []PublicKey, fromSlot, toSlot uint64) ([]uint64, error) {
	// identify slots to fetch
	slotsForAddressJobs := make([]*getSlotsForAddressJob, len(addresses))
	slotsToFetch := make(map[uint64]struct{}, toSlot-fromSlot)
	var slotsToFetchMu sync.Mutex
	storeSlot := func(slot uint64) {
		slotsToFetchMu.Lock()
		slotsToFetch[slot] = struct{}{}
		slotsToFetchMu.Unlock()
	}
	for i, address := range addresses {
		slotsForAddressJobs[i] = newGetSlotsForAddress(c.client, c.workers, storeSlot, address, fromSlot, toSlot)
		err := c.workers.Do(ctx, slotsForAddressJobs[i])
		if err != nil {
			return nil, fmt.Errorf("could not shedule job to fetch slots for address: %w", err)
		}
	}

	for _, job := range slotsForAddressJobs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-job.Done():
		}
	}

	// it should be safe to access slotsToFetch without lock as all the jobs signalled that they are done
	result := make([]uint64, 0, len(slotsToFetch))
	for slot := range slotsToFetch {
		result = append(result, slot)
	}

	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func (c *EncodedLogCollector) scheduleBlocksFetching(ctx context.Context, slots []uint64) (<-chan Block, error) {
	blocks := make(chan Block)
	getBlockJobs := make([]*getBlockJob, len(slots))
	for i, slot := range slots {
		getBlockJobs[i] = newGetBlockJob(c.client, blocks, c.lggr, slot)
		err := c.workers.Do(ctx, getBlockJobs[i])
		if err != nil {
			return nil, fmt.Errorf("could not schedule job to fetch blocks for slot: %w", err)
		}
	}

	c.engine.Go(func(ctx context.Context) {
		for _, job := range getBlockJobs {
			select {
			case <-ctx.Done():
				return
			case <-job.Done():
				continue
			}
		}
		close(blocks)
	})

	return blocks, nil
}

func (c *EncodedLogCollector) BackfillForAddresses(ctx context.Context, addresses []PublicKey, fromSlot, toSlot uint64) (orderedBlocks <-chan Block, cleanUp func(), err error) {
	slotsToFetch, err := c.getSlotsToFetch(ctx, addresses, fromSlot, toSlot)
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to identify slots to fetch: %w", err)
	}

	c.lggr.Debugw("Got all slots that need fetching for backfill operations", "addresses", PublicKeysToString(addresses), "fromSlot", fromSlot, "toSlot", toSlot, "slotsToFetch", slotsToFetch)

	unorderedBlocks, err := c.scheduleBlocksFetching(ctx, slotsToFetch)
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to schedule blocks to fetch: %w", err)
	}
	blocksSorter, sortedBlocks := newBlocksSorter(unorderedBlocks, c.lggr, slotsToFetch)
	err = blocksSorter.Start(ctx)
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to start blocks sorter: %w", err)
	}

	cleanUp = func() {
		err := blocksSorter.Close()
		if err != nil {
			blocksSorter.lggr.Errorw("Failed to close blocks sorter", "err", err)
		}
	}

	return sortedBlocks, cleanUp, nil
}

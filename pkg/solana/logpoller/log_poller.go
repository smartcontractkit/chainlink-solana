package logpoller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
)

var (
	ErrFilterNameConflict = errors.New("filter with such name already exists")
)

type ORM interface {
	InsertFilter(ctx context.Context, filter Filter) (id int64, err error)
	SelectFilters(ctx context.Context) ([]Filter, error)
	DeleteFilters(ctx context.Context, filters map[int64]Filter) error
	MarkFilterDeleted(ctx context.Context, id int64) (err error)
	MarkFilterBackfilled(ctx context.Context, id int64) (err error)
	GetLatestBlock(ctx context.Context) (int64, error)
}

type logsLoader interface {
	BackfillForAddresses(ctx context.Context, addresses []PublicKey, fromSlot, toSlot uint64) (orderedBlocks <-chan Block, cleanUp func(), err error)
}

type filtersI interface {
	RegisterFilter(ctx context.Context, filter Filter) error
	UnregisterFilter(ctx context.Context, name string) error
	LoadFilters(ctx context.Context) error
	PruneFilters(ctx context.Context) error
	GetDistinctAddresses(ctx context.Context) ([]PublicKey, error)
	GetFiltersToBackfill() []Filter
	MarkFilterBackfilled(ctx context.Context, filterID int64) error
}

type LogPoller struct {
	services.Service
	eng *services.Engine

	lggr              logger.SugaredLogger
	orm               ORM
	lastProcessedSlot int64
	client            RPCClient
	loader            logsLoader
	filters           filtersI
	processBlocks     func(ctx context.Context, blocks []Block) error // TODO: introduced for smoke test. Remove after NONEVM-916 is merged
}

func New(lggr logger.SugaredLogger, orm ORM, client RPCClient) *LogPoller {
	lggr = logger.Sugared(logger.Named(lggr, "LogPoller"))
	lp := &LogPoller{
		orm:     orm,
		lggr:    lggr,
		filters: newFilters(lggr, orm),
		client:  client,
	}

	lp.processBlocks = lp.processBlocksImpl

	lp.Service, lp.eng = services.Config{
		Name:  "LogPollerService",
		Start: lp.start,
		NewSubServices: func(l logger.Logger) []services.Service {
			loader := NewEncodedLogCollector(client, lggr)
			lp.loader = loader
			return []services.Service{loader}
		},
	}.NewServiceEngine(lggr)
	lp.lggr = lp.eng.SugaredLogger
	return lp
}

func NewWithCustomProcessor(lggr logger.SugaredLogger, orm ORM, client RPCClient, processBlocks func(ctx context.Context, blocks []Block) error) *LogPoller {
	lp := New(lggr, orm, client)
	lp.processBlocks = processBlocks
	return lp
}

func (lp *LogPoller) start(ctx context.Context) error {
	err := lp.filters.LoadFilters(ctx)
	if err != nil {
		return fmt.Errorf("failed loading filters: %w", err)
	}
	lp.eng.GoTick(services.NewTicker(time.Second), func(ctx context.Context) {
		err := lp.run(ctx)
		if err != nil {
			lp.lggr.Errorw("log poller tick failed", "err", err)
		}
	})
	lp.eng.Go(lp.backgroundWorkerRun)
	return nil
}

// RegisterFilter - refer to filters.RegisterFilter for details.
func (lp *LogPoller) RegisterFilter(ctx context.Context, filter Filter) error {
	ctx, cancel := lp.eng.Ctx(ctx)
	defer cancel()
	return lp.filters.RegisterFilter(ctx, filter)
}

// UnregisterFilter refer to filters.UnregisterFilter for details
func (lp *LogPoller) UnregisterFilter(ctx context.Context, name string) error {
	ctx, cancel := lp.eng.Ctx(ctx)
	defer cancel()
	return lp.filters.UnregisterFilter(ctx, name)
}

func (lp *LogPoller) getLastProcessedSlot(ctx context.Context) (int64, error) {
	if lp.lastProcessedSlot != 0 {
		return lp.lastProcessedSlot, nil
	}

	latestDBBlock, err := lp.orm.GetLatestBlock(ctx)
	if err == nil {
		return latestDBBlock, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("error getting latest block from db: %w", err)
	}

	latestBlock, err := lp.client.GetSlot(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return 0, fmt.Errorf("error getting latest slot from RPC: %w", err)
	}

	if latestBlock == 0 {
		return 0, fmt.Errorf("latest finalized slot is 0 - waiting for next slot to start processing")
	}
	return int64(latestBlock) - 1, nil
}

func (lp *LogPoller) backfillFilters(ctx context.Context, filters []Filter, to int64) error {
	addressesSet := make(map[PublicKey]struct{})
	addresses := make([]PublicKey, 0, len(filters))
	minSlot := to
	for _, filter := range filters {
		if _, ok := addressesSet[filter.Address]; !ok {
			addressesSet[filter.Address] = struct{}{}
			addresses = append(addresses, filter.Address)
		}
		if filter.StartingBlock < minSlot {
			minSlot = filter.StartingBlock
		}
	}

	err := lp.processBlocksRange(ctx, addresses, minSlot, to)
	if err != nil {
		return err
	}

	for _, filter := range filters {
		filterErr := lp.filters.MarkFilterBackfilled(ctx, filter.ID)
		if filterErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to mark filter %d backfilled: %w", filter.ID, filterErr))
		}
	}

	return err
}

func (lp *LogPoller) processBlocksRange(ctx context.Context, addresses []PublicKey, from, to int64) error {
	blocks, cleanup, err := lp.loader.BackfillForAddresses(ctx, addresses, uint64(from), uint64(to))
	if err != nil {
		return fmt.Errorf("error backfilling filters: %w", err)
	}

	defer cleanup()
consumedAllBlocks:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case block, ok := <-blocks:
			if !ok {
				break consumedAllBlocks
			}

			batch := []Block{block}
			batch = appendBuffered(blocks, 16, batch)
			err = lp.processBlocks(ctx, batch)
			if err != nil {
				return fmt.Errorf("error processing blocks: %w", err)
			}
		}
	}

	return nil
}

func appendBuffered(ch <-chan Block, max int, blocks []Block) []Block {
	for {
		select {
		case block, ok := <-ch:
			if !ok {
				return blocks
			}

			blocks = append(blocks, block)
			if len(blocks) >= max {
				return blocks
			}
		default:
			return blocks
		}
	}
}

func (lp *LogPoller) processBlocksImpl(ctx context.Context, blocks []Block) error {
	// TODO: add logic implemented by NONEVM-916
	return nil
}

func (lp *LogPoller) run(ctx context.Context) error {
	lastProcessedSlot, err := lp.getLastProcessedSlot(ctx)
	if err != nil {
		return fmt.Errorf("failed getting last processed slot: %w", err)
	}

	filtersToBackfill := lp.filters.GetFiltersToBackfill()
	if len(filtersToBackfill) != 0 {
		lp.lggr.Debugw("Got new filters to backfill", "filters", filtersToBackfill)
		return lp.backfillFilters(ctx, filtersToBackfill, lastProcessedSlot)
	}

	addresses, err := lp.filters.GetDistinctAddresses(ctx)
	if err != nil {
		return fmt.Errorf("failed getting addresses: %w", err)
	}
	if len(addresses) == 0 {
		return nil
	}
	highestSlot, err := lp.client.GetSlot(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return fmt.Errorf("failed getting highest slot: %w", err)
	}

	if lastProcessedSlot > int64(highestSlot) {
		return fmt.Errorf("last processed slot %d is higher than highest RPC slot %d", lastProcessedSlot, highestSlot)
	}

	if lastProcessedSlot == int64(highestSlot) {
		lp.lggr.Debugw("RPC's latest finalized block is the same as latest processed - skipping", "lastProcessedSlot", lastProcessedSlot)
		return nil
	}

	lp.lggr.Debugw("Got new slot range to process", "from", lastProcessedSlot+1, "to", highestSlot)
	err = lp.processBlocksRange(ctx, addresses, lastProcessedSlot+1, int64(highestSlot))
	if err != nil {
		return fmt.Errorf("failed processing block range [%d, %d]: %w", lastProcessedSlot+1, highestSlot, err)
	}

	lp.lastProcessedSlot = int64(highestSlot)
	return nil
}

func (lp *LogPoller) backgroundWorkerRun(ctx context.Context) {
	pruneFilters := services.NewTicker(time.Minute)
	defer pruneFilters.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-pruneFilters.C:
			err := lp.filters.PruneFilters(ctx)
			if err != nil {
				lp.lggr.Errorw("Failed to prune filters", "err", err)
			}
		}
	}
}

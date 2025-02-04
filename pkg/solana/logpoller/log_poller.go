package logpoller

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"iter"
	"math"
	"time"

	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
)

var (
	ErrFilterNameConflict   = errors.New("filter with such name already exists")
	ErrMissingDiscriminator = errors.New("Solana log is missing discriminator")
)

type ORM interface {
	ChainID() string
	InsertFilter(ctx context.Context, filter Filter) (id int64, err error)
	SelectFilters(ctx context.Context) ([]Filter, error)
	DeleteFilters(ctx context.Context, filters map[int64]Filter) error
	MarkFilterDeleted(ctx context.Context, id int64) (err error)
	MarkFilterBackfilled(ctx context.Context, id int64) (err error)
	GetLatestBlock(ctx context.Context) (int64, error)
	InsertLogs(context.Context, []Log) (err error)
	SelectSeqNums(ctx context.Context) (map[int64]int64, error)
	FilteredLogs(ctx context.Context, queryFilter []query.Expression, limitAndSort query.LimitAndSort, queryName string) ([]Log, error)
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
	MatchingFiltersForEncodedEvent(event ProgramEvent) iter.Seq[Filter]
	DecodeSubKey(ctx context.Context, lggr logger.SugaredLogger, raw []byte, ID int64, subKeyPath []string) (any, error)
	IncrementSeqNum(filterID int64) int64
}

type Service struct {
	services.StateMachine
	services.Service
	eng *services.Engine

	lggr              logger.SugaredLogger
	orm               ORM
	lastProcessedSlot int64
	client            RPCClient
	loader            logsLoader
	filters           filtersI
	processBlocks     func(ctx context.Context, blocks []Block) error
}

func New(lggr logger.SugaredLogger, orm ORM, cl RPCClient) *Service {
	lggr = logger.Sugared(logger.Named(lggr, "LogPoller"))
	lp := &Service{
		orm:     orm,
		client:  cl,
		filters: newFilters(lggr, orm),
	}

	lp.processBlocks = lp.processBlocksImpl

	lp.Service, lp.eng = services.Config{
		Name:  "LogPollerService",
		Start: lp.start,
		NewSubServices: func(l logger.Logger) []services.Service {
			loader := NewEncodedLogCollector(cl, lggr)
			lp.loader = loader
			return []services.Service{loader}
		},
	}.NewServiceEngine(lggr)
	lp.lggr = lp.eng.SugaredLogger

	return lp
}

func NewWithCustomProcessor(lggr logger.SugaredLogger, orm ORM, client RPCClient, processBlocks func(ctx context.Context, blocks []Block) error) *Service {
	lp := New(lggr, orm, client)
	lp.processBlocks = processBlocks
	return lp
}

func (lp *Service) start(_ context.Context) error {
	lp.eng.GoTick(services.NewTicker(time.Second), func(ctx context.Context) {
		err := lp.run(ctx)
		if err != nil {
			lp.lggr.Errorw("log poller iteration failed - retrying", "err", err)
		}
	})
	lp.eng.GoTick(services.NewTicker(time.Minute), lp.backgroundWorkerRun)
	return nil
}

func makeLogIndex(txIndex int, txLogIndex uint) (int64, error) {
	if txIndex > 0 && txIndex < math.MaxInt32 && txLogIndex < math.MaxUint32 {
		return int64(txIndex<<32) | int64(txLogIndex), nil
	}
	return 0, fmt.Errorf("txIndex or txLogIndex out of range: txIndex=%d, txLogIndex=%d", txIndex, txLogIndex)
}

// Process - process stream of events coming from log ingester
func (lp *Service) Process(ctx context.Context, programEvent ProgramEvent) (err error) {
	// This should never happen, since the log collector isn't started until after the filters
	// get loaded. But just in case, return an error if they aren't so the collector knows to retry later.
	if err = lp.filters.LoadFilters(ctx); err != nil {
		return err
	}

	blockData := programEvent.BlockData

	matchingFilters := lp.filters.MatchingFiltersForEncodedEvent(programEvent)
	if matchingFilters == nil {
		return nil
	}

	var logs []Log
	for filter := range matchingFilters {
		var logIndex int64
		logIndex, err = makeLogIndex(blockData.TransactionIndex, blockData.TransactionLogIndex)
		if err != nil {
			lp.lggr.Criticalw("failed to make log index", "err", err, "tx", programEvent.TransactionHash)
			return err
		}
		if blockData.SlotNumber == math.MaxInt64 {
			err = fmt.Errorf("slot number %d out of range", blockData.SlotNumber)
			lp.lggr.Critical(err.Error())
			return err
		}
		log := Log{
			FilterID:       filter.ID,
			ChainID:        lp.orm.ChainID(),
			LogIndex:       logIndex,
			BlockHash:      Hash(blockData.BlockHash),
			BlockNumber:    int64(blockData.SlotNumber), //nolint:gosec
			BlockTimestamp: blockData.BlockTime.Time().UTC(),
			Address:        filter.Address,
			EventSig:       filter.EventSig,
			TxHash:         Signature(blockData.TransactionHash),
		}

		log.Data, err = base64.StdEncoding.DecodeString(programEvent.Data)
		if err != nil {
			return err
		}

		log.SubKeyValues = make([]IndexedValue, 0, len(filter.SubKeyPaths))
		for _, path := range filter.SubKeyPaths {
			subKeyVal, decodeSubKeyErr := lp.filters.DecodeSubKey(ctx, lp.lggr, log.Data, filter.ID, path)
			if decodeSubKeyErr != nil {
				return decodeSubKeyErr
			}
			indexedVal, newIndexedValErr := newIndexedValue(subKeyVal)
			if newIndexedValErr != nil {
				return newIndexedValErr
			}
			log.SubKeyValues = append(log.SubKeyValues, indexedVal)
		}

		log.SequenceNum = lp.filters.IncrementSeqNum(filter.ID)

		if filter.Retention > 0 {
			expiresAt := time.Now().Add(filter.Retention).UTC()
			log.ExpiresAt = &expiresAt
		}

		logs = append(logs, log)
	}
	if len(logs) == 0 {
		return nil
	}

	return lp.orm.InsertLogs(ctx, logs)
}

// RegisterFilter - refer to filters.RegisterFilter for details.
func (lp *Service) RegisterFilter(ctx context.Context, filter Filter) error {
	ctx, cancel := lp.eng.Ctx(ctx)
	defer cancel()
	return lp.filters.RegisterFilter(ctx, filter)
}

// UnregisterFilter refer to filters.UnregisterFilter for details
func (lp *Service) UnregisterFilter(ctx context.Context, name string) error {
	ctx, cancel := lp.eng.Ctx(ctx)
	defer cancel()
	return lp.filters.UnregisterFilter(ctx, name)
}

func (lp *Service) getLastProcessedSlot(ctx context.Context) (int64, error) {
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

	latestFinalizedSlot, err := lp.client.SlotHeightWithCommitment(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return 0, fmt.Errorf("error getting latest slot from RPC: %w", err)
	}

	if latestFinalizedSlot == 0 {
		return 0, fmt.Errorf("latest finalized slot is 0 - waiting for next slot to start processing")
	}
	// nolint:gosec
	// G115: integer overflow conversion uint64 -&gt; int64
	return int64(latestFinalizedSlot) - 1, nil //
}

func (lp *Service) backfillFilters(ctx context.Context, filters []Filter, to int64) error {
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

func (lp *Service) processBlocksRange(ctx context.Context, addresses []PublicKey, from, to int64) error {
	// nolint:gosec
	// G115: integer overflow conversion uint64 -&gt; int64
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
			batch = appendBuffered(blocks, blocksChBuffer, batch)
			err = lp.processBlocks(ctx, batch)
			if err != nil {
				return fmt.Errorf("error processing blocks: %w", err)
			}
		}
	}

	return nil
}

func (lp *Service) processBlocksImpl(ctx context.Context, blocks []Block) error {
	for _, block := range blocks {
		for _, event := range block.Events {
			err := lp.Process(ctx, event)
			if err != nil {
				return fmt.Errorf("error processing event for tx %s in block %d: %w", event.TransactionHash, block.SlotNumber, err)
			}
		}
	}

	return nil
}

func (lp *Service) run(ctx context.Context) error {
	err := lp.filters.LoadFilters(ctx)
	if err != nil {
		return fmt.Errorf("error loading filters: %w", err)
	}

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
	rawHighestSlot, err := lp.client.SlotHeightWithCommitment(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return fmt.Errorf("failed getting highest slot: %w", err)
	}

	// nolint:gosec
	// G115: integer overflow conversion uint64 -&gt; int64
	highestSlot := int64(rawHighestSlot)

	if lastProcessedSlot > highestSlot {
		return fmt.Errorf("last processed slot %d is higher than highest RPC slot %d", lastProcessedSlot, highestSlot)
	}

	if lastProcessedSlot == highestSlot {
		lp.lggr.Debugw("RPC's latest finalized block is the same as latest processed - skipping", "lastProcessedSlot", lastProcessedSlot)
		return nil
	}

	lp.lggr.Debugw("Got new slot range to process", "from", lastProcessedSlot+1, "to", highestSlot)
	err = lp.processBlocksRange(ctx, addresses, lastProcessedSlot+1, highestSlot)
	if err != nil {
		return fmt.Errorf("failed processing block range [%d, %d]: %w", lastProcessedSlot+1, highestSlot, err)
	}

	lp.lastProcessedSlot = highestSlot
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

func (lp *Service) backgroundWorkerRun(ctx context.Context) {
	err := lp.filters.PruneFilters(ctx)
	if err != nil {
		lp.lggr.Errorw("Failed to prune filters", "err", err)
	}
}

func (lp *Service) FilteredLogs(ctx context.Context, queryFilter []query.Expression, limitAndSort query.LimitAndSort, queryName string) ([]Log, error) {
	return lp.orm.FilteredLogs(ctx, queryFilter, limitAndSort, queryName)
}

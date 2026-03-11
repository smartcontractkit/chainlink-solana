package logpoller

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"math"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

var (
	ErrFilterNameConflict = errors.New("filter with such name already exists")
)

type ORM interface {
	ChainID() string
	HasFilter(ctx context.Context, name string) (bool, error)
	InsertFilter(ctx context.Context, filter types.Filter) (id int64, err error)
	SelectFilters(ctx context.Context) ([]types.Filter, error)
	DeleteFilters(ctx context.Context, filters map[int64]types.Filter) error
	MarkFilterDeleted(ctx context.Context, id int64) (err error)
	MarkFilterBackfilled(ctx context.Context, id int64) (err error)
	GetLatestBlock(ctx context.Context) (int64, error)
	InsertLogs(context.Context, []types.Log) (err error)
	SelectSeqNums(ctx context.Context) (map[int64]int64, error)
	FilteredLogs(ctx context.Context, queryFilter []query.Expression, limitAndSort query.LimitAndSort, queryName string) ([]types.Log, error)
	PruneLogsForFilter(ctx context.Context, filter types.Filter) (int64, error)
}

type logsLoader interface {
	BackfillForAddresses(ctx context.Context, addresses []types.PublicKey, fromSlot, toSlot uint64) (orderedBlocks <-chan types.Block, cleanUp func(), err error)
}

type filtersI interface {
	HasFilter(ctx context.Context, name string) bool
	RegisterFilter(ctx context.Context, filter types.Filter) error
	UnregisterFilter(ctx context.Context, name string) error
	LoadFilters(ctx context.Context) error
	PruneFilters(ctx context.Context) error
	PruneLogs(ctx context.Context) error
	GetDistinctAddresses(ctx context.Context) ([]types.PublicKey, error)
	GetFilters(ctx context.Context) (map[string]types.Filter, error)
	GetFiltersToBackfill() []types.Filter
	MarkFilterBackfilled(ctx context.Context, filterID int64) error
	UpdateStartingBlocks(startingBlocks int64)
	MatchingFiltersForEncodedEvent(event types.ProgramEvent) iter.Seq[types.Filter]
	DecodeSubKey(ctx context.Context, lggr logger.SugaredLogger, raw []byte, ID int64, subKeyPath []string) (any, error)
	IncrementSeqNum(filterID int64) int64
}

type ReplayInfo struct {
	mut          sync.RWMutex
	requestBlock int64
	status       types.ReplayStatus
}

// hasRequest returns true if a new request has been received (since the last request completed),
// whether or not it is pending yet
func (r *ReplayInfo) hasRequest() bool {
	return r.status == types.ReplayStatusRequested || r.status == types.ReplayStatusPending
}

type Service struct {
	services.Service
	eng *services.Engine

	lggr              logger.SugaredLogger
	orm               ORM
	lastProcessedSlot int64
	replay            ReplayInfo
	client            RPCClient
	loader            logsLoader
	filters           filtersI
	cpiEventExtractor *CPIEventExtractor
	processBlocks     func(ctx context.Context, blocks []types.Block) error
	blockTime         time.Duration
	startingLookback  time.Duration
	metrics           *solLpMetrics
}

func New(lggr logger.SugaredLogger, orm ORM, cl RPCClient, cfg config.Config, chainID string) (*Service, error) {
	lp := &Service{
		orm:    orm,
		client: cl,
	}

	if cfg.LogPollerCPIEventsEnabled() {
		lp.cpiEventExtractor = NewCPIEventExtractor(lggr)
	}

	var err error
	lp.metrics, err = NewSolLpMetrics(chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}

	lp.processBlocks = lp.processBlocksImpl

	lp.Service, lp.eng = services.Config{
		Name:  "LogPoller",
		Start: lp.start,
		NewSubServices: func(lggr logger.Logger) []services.Service {
			lp.filters = newFilters(lggr, orm, lp.cpiEventExtractor)
			loader := NewEncodedLogCollector(cl, lggr, chainID, lp.metrics, lp.cpiEventExtractor)
			lp.loader = loader
			return []services.Service{loader}
		},
	}.NewServiceEngine(lggr)
	lp.lggr = lp.eng.SugaredLogger
	lp.replay.status = types.ReplayStatusNoRequest

	lp.startingLookback = cfg.LogPollerStartingLookback()
	lp.blockTime = cfg.BlockTime()

	return lp, nil
}

func NewWithCustomProcessor(lggr logger.SugaredLogger, orm ORM, client RPCClient, cfg config.Config, chainID string, processBlocks func(ctx context.Context, blocks []types.Block) error) (*Service, error) {
	lp, err := New(lggr, orm, client, cfg, chainID)
	if err != nil {
		return nil, err
	}
	lp.processBlocks = processBlocks
	return lp, nil
}

// CPIEventsEnabled returns true if CPI event extraction is enabled for this log poller.
func (lp *Service) CPIEventsEnabled() bool {
	return lp.cpiEventExtractor != nil
}

const BackgroundWorkerInterval = 10 * time.Minute

func (lp *Service) start(_ context.Context) error {
	lp.eng.GoTick(services.NewTicker(2*lp.blockTime), func(ctx context.Context) {
		err := lp.run(ctx)
		if err != nil {
			lp.lggr.Errorw("log poller iteration failed - retrying", "err", err)
		}
	})
	lp.eng.GoTick(services.NewTicker(BackgroundWorkerInterval), lp.backgroundWorkerRun)
	return nil
}

func makeLogIndex(txIndex int, txLogIndex uint) (int64, error) {
	if txIndex >= 0 && txIndex < math.MaxInt32 && txLogIndex < math.MaxUint32 {
		return int64(txIndex<<32) | int64(txLogIndex), nil //nolint:gosec // safe: values validated prior to casting
	}
	return 0, fmt.Errorf("txIndex or txLogIndex out of range: txIndex=%d, txLogIndex=%d", txIndex, txLogIndex)
}

// Process - process stream of events coming from log ingester
func (lp *Service) Process(ctx context.Context, programEvent types.ProgramEvent) (err error) {
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

	var logs []types.Log
	for filter := range matchingFilters {
		var revertErr *string
		if blockData.Error != nil {
			if !filter.IncludeReverted {
				continue
			}
			revertErr = new(string)
			if j, err2 := json.Marshal(blockData.Error); err2 != nil {
				*revertErr = fmt.Sprintf("%v", blockData.Error)
				lp.lggr.Errorw("failed to marshal revert error", "revertErr", blockData.Error, "err", err2)
			} else {
				*revertErr = string(j)
			}
		}

		var logIndex int64
		logIndex, err = makeLogIndex(blockData.TransactionIndex, blockData.TransactionLogIndex)
		if err != nil {
			lp.lggr.Criticalw("failed to make log index", "err", err, "tx", programEvent.TransactionHash)
			return err
		}
		if blockData.SlotNumber > math.MaxInt64 {
			err = fmt.Errorf("slot number %d out of range", blockData.SlotNumber)
			lp.lggr.Critical(err.Error())
			return err
		}

		log := types.Log{
			FilterID:       filter.ID,
			ChainID:        lp.orm.ChainID(),
			LogIndex:       logIndex,
			BlockHash:      types.Hash(blockData.BlockHash),
			BlockNumber:    int64(blockData.SlotNumber),
			BlockTimestamp: blockData.BlockTime.Time().UTC(),
			Address:        filter.Address,
			EventSig:       filter.EventSig,
			TxHash:         types.Signature(blockData.TransactionHash),
			Error:          revertErr,
		}

		log.Data, err = base64.StdEncoding.DecodeString(programEvent.Data)
		if err != nil {
			return err
		}

		log.SubkeyValues = make([]types.IndexedValue, len(filter.SubkeyPaths))
		for idx, path := range filter.SubkeyPaths {
			if len(path) == 0 {
				continue
			}

			subKeyVal, decodeSubKeyErr := lp.filters.DecodeSubKey(ctx, lp.lggr, log.Data, filter.ID, path)
			if decodeSubKeyErr != nil {
				return decodeSubKeyErr
			}

			indexedVal, newIndexedValErr := types.NewIndexedValue(subKeyVal)
			if newIndexedValErr != nil {
				return newIndexedValErr
			}

			log.SubkeyValues[idx] = indexedVal
		}

		log.SequenceNum = lp.filters.IncrementSeqNum(filter.ID)

		if filter.Retention > 0 {
			expiresAt := time.Now().Add(filter.Retention).UTC()
			log.ExpiresAt = &expiresAt
		}

		lp.lggr.Infow("found matching event", "log", log, "eventName", filter.EventName)

		logs = append(logs, log)
	}
	if len(logs) == 0 {
		return nil
	}

	return lp.orm.InsertLogs(ctx, logs)
}

func (lp *Service) HasFilter(ctx context.Context, name string) bool {
	ctx, cancel := lp.eng.Ctx(ctx)
	defer cancel()

	return lp.filters.HasFilter(ctx, name)
}

// RegisterFilter - refer to filters.RegisterFilter for details.
func (lp *Service) RegisterFilter(ctx context.Context, filter types.Filter) error {
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

func (lp *Service) GetFilters(ctx context.Context) (map[string]types.Filter, error) {
	ctx, cancel := lp.eng.Ctx(ctx)
	defer cancel()
	return lp.filters.GetFilters(ctx)
}

// Replay submits a new replay request. If there was already a new replay request
// submitted since the last replay completed, it will be updated to the earlier of the
// two requested fromBlock's. The expectation is that, on the next timer tick of the
// LogPoller run loop it will backfill all filters starting from fromBlock. If there
// are new filters in the backfill queue, with an earlier StartingBlock, then they
// will get backfilled from there instead.
func (lp *Service) Replay(fromBlock int64) {
	lp.replay.mut.Lock()
	defer lp.replay.mut.Unlock()

	if lp.replay.hasRequest() && lp.replay.requestBlock <= fromBlock {
		// Already requested, no further action required
		lp.lggr.Warnf("Ignoring redundant request to replay from block %d, replay from block %d already requested",
			fromBlock, lp.replay.requestBlock)
		return
	}
	lp.filters.UpdateStartingBlocks(fromBlock)
	lp.replay.requestBlock = fromBlock
	if lp.replay.status != types.ReplayStatusPending {
		lp.replay.status = types.ReplayStatusRequested
	}
}

// ReplayStatus returns the current replay status of LogPoller:
//
// NoRequests - there have not been any replay requests yet since node startup
// Requested - a replay has been requested, but has not started yet
// Pending - a replay is currently in progress
// Complete - there was at least one replay executed since startup, but all have since completed
func (lp *Service) ReplayStatus() types.ReplayStatus {
	lp.replay.mut.RLock()
	defer lp.replay.mut.RUnlock()
	return lp.replay.status
}

func (lp *Service) GetLatestBlock(ctx context.Context) (int64, error) {
	return lp.orm.GetLatestBlock(ctx)
}

func (lp *Service) getLastProcessedSlot(ctx context.Context) (lastProcessed int64, err error) {
	lastProcessed = lp.lastProcessedSlot
	if lastProcessed > 0 {
		return lastProcessed, nil
	}

	lastProcessed, err = lp.orm.GetLatestBlock(ctx)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("error getting latest block from db: %w", err)
		}
		lastProcessed = 0
	}

	var lookbackSlot int64
	lookbackSlot, err = lp.computeLookbackWindow(ctx)
	if err != nil {
		return 0, err
	}
	if lookbackSlot > lastProcessed {
		lp.lggr.Infow("last processed slot is older than lookback window, skipping ahead", "lastProcessed", lastProcessed, "lookbackSlot", lookbackSlot)
		return lookbackSlot, nil
	}
	lp.lggr.Infow("last processed slot is still within lookback window, resuming at last processed slot", "lastProcessed", lastProcessed, "lookbackSlot", lookbackSlot)

	return lastProcessed, nil
}

// checkForReplayRequest checks whether there have been any new replay requests since it was last called,
// and if so sets the pending flag to true and returns the block number
func (lp *Service) checkForReplayRequest() bool {
	lp.replay.mut.Lock()
	defer lp.replay.mut.Unlock()

	if !lp.replay.hasRequest() {
		return false
	}

	lp.lggr.Infow("starting replay", "replayBlock", lp.replay.requestBlock)
	lp.replay.status = types.ReplayStatusPending
	return true
}

func (lp *Service) backfillFilters(ctx context.Context, filters []types.Filter, to int64) error {
	isReplay := lp.checkForReplayRequest()

	addressesSet := make(map[types.PublicKey]struct{})
	addresses := make([]types.PublicKey, 0, len(filters))
	minSlot := to

	for _, filter := range filters {
		if _, ok := addressesSet[filter.Address]; !ok {
			addressesSet[filter.Address] = struct{}{}
			addresses = append(addresses, filter.Address)
		}
		if filter.StartingBlock != 0 && filter.StartingBlock < minSlot {
			minSlot = filter.StartingBlock
		}
	}

	if minSlot < to {
		err := lp.processBlocksRange(ctx, addresses, minSlot, to)
		if err != nil {
			return err
		}

		lp.lggr.Infow("Done backfilling filters", "filters", len(filters), "from", minSlot, "to", to)
	} else {
		lp.lggr.Infow("Starting block for filters backfill is greater than the latest processed block - marking filters as backfilled and starting global processing", "filters", len(filters), "from", minSlot, "to", to)
	}

	if isReplay {
		lp.replayComplete(minSlot, to)
	}

	var err error
	for _, filter := range filters {
		filterErr := lp.filters.MarkFilterBackfilled(ctx, filter.ID)
		if filterErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to mark filter %d backfilled: %w", filter.ID, filterErr))
		}
	}

	return err
}

func (lp *Service) processBlocksRange(ctx context.Context, addresses []types.PublicKey, from, to int64) error {
	lp.lggr.Infow("Processing block range", "from", from, "to", to)
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

			batch := []types.Block{block}
			batch = appendBuffered(blocks, blocksChBuffer, batch)
			lp.lggr.Infow("processing batch of blocks", "count", len(batch), "start", batch[0].SlotNumber, "end", batch[len(batch)-1].SlotNumber)
			err = lp.processBlocks(ctx, batch)
			if err != nil {
				return fmt.Errorf("error processing blocks: %w", err)
			}

			// nolint:gosec
			// G115: integer overflow conversion uint64 -> int64
			highestInBatch := int64(batch[len(batch)-1].SlotNumber)
			if highestInBatch > lp.lastProcessedSlot {
				lp.lastProcessedSlot = highestInBatch
			}
		}
	}

	return nil
}

func (lp *Service) processBlocksImpl(ctx context.Context, blocks []types.Block) error {
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

func (lp *Service) run(ctx context.Context) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic recovered: %v", rec)
		}
	}()
	err = lp.filters.LoadFilters(ctx)
	if err != nil {
		return fmt.Errorf("error loading filters: %w", err)
	}

	lastProcessedSlot, err := lp.getLastProcessedSlot(ctx)
	if err != nil {
		return fmt.Errorf("failed getting last processed slot: %w", err)
	}

	filtersToBackfill := lp.filters.GetFiltersToBackfill()
	if len(filtersToBackfill) != 0 {
		lp.lggr.Debugw("Got new filters to backfill", "filters_len", len(filtersToBackfill))
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

// replayComplete is called when a backfill associated with a current pending replay has just completed.
// Assuming there were no new requests to replay while the backfill was happening, it updates the replay
// status to ReplayStatusComplete. If there was a request for a lower block number in the meantime, then
// the status will revert to ReplayStatusRequested
func (lp *Service) replayComplete(from, to int64) bool {
	lp.replay.mut.Lock()
	defer lp.replay.mut.Unlock()

	lp.lggr.Infow("replay complete", "from", from, "to", to)

	if lp.replay.requestBlock < from {
		// received a new request with lower block number while replaying, we'll process that next time
		lp.replay.status = types.ReplayStatusRequested
		return false
	}
	lp.replay.status = types.ReplayStatusComplete
	return true
}

func appendBuffered(ch <-chan types.Block, maxNum int, blocks []types.Block) []types.Block {
	for {
		select {
		case block, ok := <-ch:
			if !ok {
				return blocks
			}

			blocks = append(blocks, block)
			if len(blocks) >= maxNum {
				return blocks
			}
		default:
			return blocks
		}
	}
}

func (lp *Service) backgroundWorkerRun(ctx context.Context) {
	// Set deadline for log pruning to 90% of the time left until the next scheduled BackgroundWorkerRun
	logsCtx, cancel := context.WithTimeout(sqlutil.WithoutDefaultTimeout(ctx), 9*BackgroundWorkerInterval/10)
	defer cancel()

	// Filters table is much smaller, so default timeout makes the most sense for that
	err := lp.filters.PruneFilters(ctx)
	if err != nil {
		lp.lggr.Errorw("Failed to prune filters", "err", err)
	}

	err = lp.filters.PruneLogs(logsCtx)
	if err != nil {
		lp.lggr.Errorw("Failed to prune logs", "err", err)
	}
}

func (lp *Service) FilteredLogs(ctx context.Context, queryFilter []query.Expression, limitAndSort query.LimitAndSort, queryName string) ([]types.Log, error) {
	return lp.orm.FilteredLogs(ctx, queryFilter, limitAndSort, queryName)
}

func (lp *Service) computeLookbackWindow(ctx context.Context) (int64, error) {
	latestFinalizedSlot, err := lp.client.SlotHeightWithCommitment(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return 0, fmt.Errorf("error getting latest slot from RPC: %w", err)
	}

	if latestFinalizedSlot == 0 {
		return 0, fmt.Errorf("latest finalized slot is 0 - waiting for next slot to start processing")
	}

	lookback := lp.startingLookback

	// nolint:gosec
	// G115: integer overflow conversion uint64 -&gt; int64
	latestSlot := int64(latestFinalizedSlot)
	lookbackSlot := latestSlot - int64((lookback-1)/lp.blockTime) - 1 // = latestSlot - ceil(lookback/blockTime)

	firstAvailableSlot, err := lp.client.GetFirstAvailableBlock(ctx) // Despite the name, this returns slot # not block number
	if err != nil {
		// This is an optimization to avoid requesting pruned blocks. If there's an err we can just ignore
		lp.lggr.Warnf("Failed to get first available slot, starting at lookback slot %d from %v ago: %s", lookbackSlot, lookback, err.Error())
		return lookbackSlot, nil
	}

	// nolint:gosec
	// G115: integer overflow conversion uint64 -&gt; int64
	firstSlot := int64(firstAvailableSlot)
	if firstSlot > lookbackSlot {
		lookbackSlot = firstSlot
		lp.lggr.Infof("First available slot %d is more recent than slot %d from %v ago, using first available slot as lookback slot", firstSlot, lookbackSlot, lookback)
		return lookbackSlot, nil
	}
	lp.lggr.Infof("First available slot %d is older than lookback window, using lookback slot = %d from %v ago", firstSlot, lookbackSlot, lookback)
	return lookbackSlot, nil
}

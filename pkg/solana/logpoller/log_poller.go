package logpoller

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/internal"
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
	InsertLogs(context.Context, []Log) (err error)
	SelectSeqNums(ctx context.Context) (map[int64]int64, error)
}

type LogPoller struct {
	services.Service
	eng *services.Engine
	services.StateMachine

	lggr      logger.SugaredLogger
	orm       ORM
	client    internal.Loader[client.Reader]
	collector *EncodedLogCollector

	filters *filters
}

func New(lggr logger.SugaredLogger, orm ORM, cl internal.Loader[client.Reader]) *LogPoller {
	lggr = logger.Sugared(logger.Named(lggr, "LogPoller"))
	lp := &LogPoller{
		orm:     orm,
		client:  cl,
		filters: newFilters(lggr, orm),
	}

	lp.Service, lp.eng = services.Config{
		Name:  "LogPollerService",
		Start: lp.start,
	}.NewServiceEngine(lggr)
	lp.lggr = lp.eng.SugaredLogger

	return lp
}

func (lp *LogPoller) start(context.Context) error {
	cl, err := lp.client.Get()
	if err != nil {
		return err
	}
	lp.collector = NewEncodedLogCollector(cl, lp, lp.lggr)

	lp.eng.Go(lp.run)
	lp.eng.Go(lp.backgroundWorkerRun)
	return nil
}

func makeLogIndex(txIndex int, txLogIndex uint) (int64, error) {
	if txIndex > 0 && txIndex < math.MaxInt32 && txLogIndex < math.MaxUint32 {
		return int64(txIndex<<32) | int64(txLogIndex), nil
	}
	return 0, fmt.Errorf("txIndex or txLogIndex out of range: txIndex=%d, txLogIndex=%d", txIndex, txLogIndex)
}

// Process - process stream of events coming from log ingester
func (lp *LogPoller) Process(programEvent ProgramEvent) (err error) {
	ctx, cancel := utils.ContextFromChan(lp.eng.StopChan)
	defer cancel()

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
		logIndex, logIndexErr := makeLogIndex(blockData.TransactionIndex, blockData.TransactionLogIndex)
		if logIndexErr != nil {
			lp.lggr.Critical(err)
			return err
		}
		if blockData.SlotNumber == math.MaxInt64 {
			errSlot := fmt.Errorf("slot number %d out of range", blockData.SlotNumber)
			lp.lggr.Critical(err.Error())
			return errSlot
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

		eventData, decodeErr := base64.StdEncoding.DecodeString(programEvent.Data)
		if decodeErr != nil {
			return decodeErr
		}
		if len(eventData) < 8 {
			err = fmt.Errorf("Assumption violation: %w, log.Data=%s", ErrMissingDiscriminator, log.Data)
			lp.lggr.Criticalw(err.Error())
			return err
		}
		log.Data = eventData[8:]

		log.SubkeyValues = make([]IndexedValue, 0, len(filter.SubkeyPaths))
		for _, path := range filter.SubkeyPaths {
			subKeyVal, decodeSubKeyErr := lp.filters.DecodeSubKey(ctx, log.Data, filter.ID, path)
			if decodeSubKeyErr != nil {
				return decodeSubKeyErr
			}
			indexedVal, newIndexedValErr := NewIndexedValue(subKeyVal)
			if newIndexedValErr != nil {
				return newIndexedValErr
			}
			log.SubkeyValues = append(log.SubkeyValues, indexedVal)
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

	err = lp.orm.InsertLogs(ctx, logs)
	if err != nil {
		return err
	}
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

func (lp *LogPoller) loadFilters(ctx context.Context) error {
	retryTicker := services.TickerConfig{Initial: 0, JitterPct: services.DefaultJitter}.NewTicker(time.Second)
	defer retryTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-retryTicker.C:
		}
		err := lp.filters.LoadFilters(ctx)
		if err == nil {
			return nil
		}
		lp.lggr.Errorw("Failed loading filters in init logpoller loop, retrying later", "err", err)
	}
	// unreachable
}

func (lp *LogPoller) run(ctx context.Context) {
	err := lp.loadFilters(ctx)
	if err != nil {
		lp.lggr.Warnw("Failed loading filters", "err", err)
		return
	}

	// safe to start fetching logs, now that filters are loaded
	lp.collector.Start(ctx)

	var blocks chan struct {
		BlockNumber int64
		Logs        any // to be defined
	}

	for {
		select {
		case <-ctx.Done():
			return
		case block := <-blocks:
			filtersToBackfill := lp.filters.GetFiltersToBackfill()

			// TODO: NONEVM-916 parse, filters and persist logs
			// NOTE: removal of filters occurs in the separate goroutine, so there is a chance that upon insert
			// of log corresponding filter won't be present in the db. Ensure to refilter and retry on insert error
			for i := range filtersToBackfill {
				filter := filtersToBackfill[i]
				lp.eng.Go(func(ctx context.Context) {
					lp.startFilterBackfill(ctx, filter, block.BlockNumber)
				})
			}
		}
	}
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

func (lp *LogPoller) startFilterBackfill(ctx context.Context, filter Filter, toBlock int64) {
	// TODO: NONEVM-916 start backfill
	lp.lggr.Debugw("Starting filter backfill", "filter", filter)
	err := lp.filters.MarkFilterBackfilled(ctx, filter.ID)
	if err != nil {
		lp.lggr.Errorw("Failed to mark filter backfill", "filter", filter, "err", err)
	}
}

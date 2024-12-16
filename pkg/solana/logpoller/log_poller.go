package logpoller

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/internal"
)

var (
	ErrFilterNameConflict = errors.New("filter with such name already exists")
)

type ORM interface {
	ChainID() string
	InsertFilter(ctx context.Context, filter Filter) (id int64, err error)
	SelectFilters(ctx context.Context) ([]Filter, error)
	DeleteFilters(ctx context.Context, filters map[int64]Filter) error
	MarkFilterDeleted(ctx context.Context, id int64) (err error)
	MarkFilterBackfilled(ctx context.Context, id int64) (err error)
	InsertLogs(context.Context, []Log) (err error)
}

type ILogPoller interface {
	Start(context.Context) error
	Close() error
	RegisterFilter(ctx context.Context, filter Filter) error
	UnregisterFilter(ctx context.Context, name string) error
}

type LogPoller struct {
	services.Service
	eng *services.Engine
	services.StateMachine
	lggr      logger.SugaredLogger
	orm       ORM
	client    internal.Loader[client.Reader]
	collector *EncodedLogCollector

	filters             *filters
	discriminatorLookup map[string]string
	events              []ProgramEvent
	codec               commontypes.RemoteCodec

	chStop services.StopChan
}

func New(lggr logger.SugaredLogger, orm ORM, cl internal.Loader[client.Reader]) ILogPoller {
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
	lp.eng.Go(lp.run)
	lp.eng.Go(lp.backgroundWorkerRun)
	cl, err := lp.client.Get()
	if err != nil {
		return err
	}
	lp.collector = NewEncodedLogCollector(cl, lp, lp.lggr)
	return nil
}

func makeLogIndex(txIndex int, txLogIndex uint) int64 {
	if txIndex < 0 || txIndex > math.MaxUint32 || txLogIndex > math.MaxUint32 {
		panic(fmt.Sprintf("txIndex or txLogIndex out of range: txIndex=%d, txLogIndex=%d", txIndex, txLogIndex))
	}
	return int64(math.MaxUint32*uint32(txIndex) + uint32(txLogIndex))
}

// Process - process stream of events coming from log ingester
func (lp *LogPoller) Process(programEvent ProgramEvent) (err error) {
	ctx, cancel := utils.ContextFromChan(lp.chStop)
	defer cancel()

	blockData := programEvent.BlockData

	var logs []Log
	for filter := range lp.filters.MatchingFiltersForEncodedEvent(programEvent) {
		log := Log{
			FilterID:       filter.ID,
			ChainID:        lp.orm.ChainID(),
			LogIndex:       makeLogIndex(blockData.TransactionIndex, blockData.TransactionLogIndex),
			BlockHash:      Hash(blockData.BlockHash),
			BlockNumber:    int64(blockData.BlockHeight),
			BlockTimestamp: blockData.BlockTime.Time(), // TODO: is this a timezone safe conversion?
			Address:        filter.Address,
			EventSig:       filter.EventSig,
			TxHash:         Signature(blockData.TransactionHash),
		}

		log.Data, err = base64.StdEncoding.DecodeString(programEvent.Data)
		if err != nil {
			return err
		}

		var event any
		err = lp.filters.EventCodec(filter.ID).Decode(ctx, log.Data, &event, filter.EventName)
		if err != nil {
			return err
		}

		err = lp.ExtractSubkeys(reflect.TypeOf(event), filter.SubkeyPaths)
		if err != nil {
			return err
		}

		// TODO: fill in, and keep track of SequenceNumber for each filter. (Initialize from db on LoadFilters, then increment each time?)

		logs = append(logs, log)
	}

	lp.orm.InsertLogs(ctx, logs)
	return nil
}

func (lp *LogPoller) ExtractSubkeys(t reflect.Type, paths SubkeyPaths) error {
	s := reflect.TypeOf(event)
	if s.Kind() != reflect.Struct {
		return fmt.Errorf("event type must be struct, got %v. event=%v", t, event)
	}

	for _, path := range paths[0] {
		field, err := s.FieldByName(path)
		for depth := 0; depth < len(paths); depth++ {
			for _, path := range paths[depth] {
				field, err = field.Type.FieldByName(path)
			}
		}
	}

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
		if err != nil {
			lp.lggr.Errorw("Failed loading filters in init logpoller loop, retrying later", "err", err)
			continue
		}
	}
	// unreachable
}

func (lp *LogPoller) run(ctx context.Context) {
	err := lp.loadFilters(ctx)
	if err != nil {
		lp.lggr.Warnw("Failed loading filters", "err", err)
		return
	}

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

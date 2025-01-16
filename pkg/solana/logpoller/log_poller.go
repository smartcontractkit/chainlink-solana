package logpoller

import (
	"context"
	"errors"
	"time"

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
}

type LogPoller struct {
	services.Service
	eng *services.Engine

	lggr logger.SugaredLogger
	orm  ORM

	filters *filters
}

func New(lggr logger.SugaredLogger, orm ORM) *LogPoller {
	lggr = logger.Sugared(logger.Named(lggr, "LogPoller"))
	lp := &LogPoller{
		orm:     orm,
		lggr:    lggr,
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
		if err != nil {
			lp.lggr.Errorw("Failed loading filters in init logpoller loop, retrying later", "err", err)
			continue
		}

		return nil
	}
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

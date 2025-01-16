package logpoller

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"maps"
	"sync"
	"sync/atomic"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
)

type filters struct {
	orm  ORM
	lggr logger.SugaredLogger

	filtersByID       map[int64]*Filter
	filtersByName     map[string]int64
	filtersByAddress  map[PublicKey]map[EventSignature]map[int64]struct{}
	filtersToBackfill map[int64]struct{}
	filtersToDelete   map[int64]Filter
	filtersMutex      sync.RWMutex
	loadedFilters     atomic.Bool
}

func newFilters(lggr logger.SugaredLogger, orm ORM) *filters {
	return &filters{
		orm:  orm,
		lggr: lggr,
	}
}

// PruneFilters - prunes all filters marked to be deleted from the database and all corresponding logs.
func (fl *filters) PruneFilters(ctx context.Context) error {
	err := fl.LoadFilters(ctx)
	if err != nil {
		return fmt.Errorf("failed to load filters: %w", err)
	}

	fl.filtersMutex.Lock()
	filtersToDelete := fl.filtersToDelete
	fl.filtersToDelete = make(map[int64]Filter)
	fl.filtersMutex.Unlock()

	if len(filtersToDelete) == 0 {
		return nil
	}

	err = fl.orm.DeleteFilters(ctx, filtersToDelete)
	if err != nil {
		fl.filtersMutex.Lock()
		defer fl.filtersMutex.Unlock()
		maps.Copy(fl.filtersToDelete, filtersToDelete)
		return fmt.Errorf("failed to delete filters: %w", err)
	}

	return nil
}

// RegisterFilter persists provided filter and ensures that any log emitted by a contract with filter.Address
// that matches filter.EventSig signature will be captured starting from filter.StartingBlock.
// The filter may be unregistered later by filter.Name.
// In case of Filter.Name collision (within the chain scope) returns ErrFilterNameConflict if
// one of the fields defining resulting logs (Address, EventSig, EventIDL, SubkeyPaths) does not match original filter.
// Otherwise, updates remaining fields and schedules backfill.
// Warnings/debug information is keyed by filter name.
func (fl *filters) RegisterFilter(ctx context.Context, filter Filter) error {
	if len(filter.Name) == 0 {
		return errors.New("name is required")
	}

	err := fl.LoadFilters(ctx)
	if err != nil {
		return fmt.Errorf("failed to load filters: %w", err)
	}

	fl.filtersMutex.Lock()
	defer fl.filtersMutex.Unlock()

	filter.IsBackfilled = false
	if existingFilterID, ok := fl.filtersByName[filter.Name]; ok {
		existingFilter := fl.filtersByID[existingFilterID]
		if !existingFilter.MatchSameLogs(filter) {
			return ErrFilterNameConflict
		}
		if existingFilter.IsBackfilled {
			// if existing filter was already backfilled, but starting block was higher we need to backfill filter again
			filter.IsBackfilled = existingFilter.StartingBlock <= filter.StartingBlock
		}

		fl.removeFilterFromIndexes(*existingFilter)
	}

	filterID, err := fl.orm.InsertFilter(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to insert filter: %w", err)
	}

	filter.ID = filterID

	fl.filtersByName[filter.Name] = filter.ID
	fl.filtersByID[filter.ID] = &filter
	filtersForAddress, ok := fl.filtersByAddress[filter.Address]
	if !ok {
		filtersForAddress = make(map[EventSignature]map[int64]struct{})
		fl.filtersByAddress[filter.Address] = filtersForAddress
	}

	filtersForEventSig, ok := filtersForAddress[filter.EventSig]
	if !ok {
		filtersForEventSig = make(map[int64]struct{})
		filtersForAddress[filter.EventSig] = filtersForEventSig
	}

	filtersForEventSig[filter.ID] = struct{}{}
	if !filter.IsBackfilled {
		fl.filtersToBackfill[filter.ID] = struct{}{}
	}
	return nil
}

// UnregisterFilter will mark the filter with the given name for pruning and async prune all corresponding logs.
// If the name does not exist, it will log an error but not return an error.
// Warnings/debug information is keyed by filter name.
func (fl *filters) UnregisterFilter(ctx context.Context, name string) error {
	err := fl.LoadFilters(ctx)
	if err != nil {
		return fmt.Errorf("failed to load filters: %w", err)
	}

	fl.filtersMutex.Lock()
	defer fl.filtersMutex.Unlock()

	filterID, ok := fl.filtersByName[name]
	if !ok {
		fl.lggr.Warnw("Filter not found in filtersByName", "name", name)
		return nil
	}

	filter, ok := fl.filtersByID[filterID]
	if !ok {
		fl.lggr.Errorw("Filter not found in filtersByID", "id", filterID, "name", name)
		return nil
	}

	if err := fl.orm.MarkFilterDeleted(ctx, filter.ID); err != nil {
		return fmt.Errorf("failed to mark filter deleted: %w", err)
	}

	fl.removeFilterFromIndexes(*filter)

	fl.filtersToDelete[filter.ID] = *filter
	return nil
}

func (fl *filters) removeFilterFromIndexes(filter Filter) {
	delete(fl.filtersByName, filter.Name)
	delete(fl.filtersToBackfill, filter.ID)
	delete(fl.filtersByID, filter.ID)

	filtersForAddress, ok := fl.filtersByAddress[filter.Address]
	if !ok {
		fl.lggr.Warnw("Filter not found in filtersByAddress", "name", filter.Name, "address", filter.Address)
		return
	}

	filtersForEventSig, ok := filtersForAddress[filter.EventSig]
	if !ok {
		fl.lggr.Warnw("Filter not found in filtersByEventSig", "name", filter.Name, "address", filter.Address)
		return
	}

	delete(filtersForEventSig, filter.ID)
	if len(filtersForEventSig) == 0 {
		delete(filtersForAddress, filter.EventSig)
	}

	if len(filtersForAddress) == 0 {
		delete(fl.filtersByAddress, filter.Address)
	}
}

// MatchingFilters - returns iterator to go through all matching filters.
// Requires LoadFilters to be called at least once.
func (fl *filters) MatchingFilters(addr PublicKey, eventSignature EventSignature) iter.Seq[Filter] {
	if !fl.loadedFilters.Load() {
		fl.lggr.Critical("Invariant violation: expected filters to be loaded before call to MatchingFilters")
		return nil
	}
	return func(yield func(Filter) bool) {
		fl.filtersMutex.RLock()
		defer fl.filtersMutex.RUnlock()
		filters, ok := fl.filtersByAddress[addr]
		if !ok {
			return
		}

		for filterID := range filters[eventSignature] {
			filter, ok := fl.filtersByID[filterID]
			if !ok {
				fl.lggr.Errorw("expected filter to exist in filtersByID", "filterID", filterID)
				continue
			}
			if !yield(*filter) {
				return
			}
		}
	}
}

// GetFiltersToBackfill - returns copy of backfill queue
// Requires LoadFilters to be called at least once.
func (fl *filters) GetFiltersToBackfill() []Filter {
	if !fl.loadedFilters.Load() {
		fl.lggr.Critical("Invariant violation: expected filters to be loaded before call to MatchingFilters")
		return nil
	}
	fl.filtersMutex.Lock()
	defer fl.filtersMutex.Unlock()
	result := make([]Filter, 0, len(fl.filtersToBackfill))
	for filterID := range fl.filtersToBackfill {
		filter, ok := fl.filtersByID[filterID]
		if !ok {
			fl.lggr.Errorw("expected filter to exist in filtersByID", "filterID", filterID)
			continue
		}
		result = append(result, *filter)
	}

	return result
}

func (fl *filters) MarkFilterBackfilled(ctx context.Context, filterID int64) error {
	fl.filtersMutex.Lock()
	defer fl.filtersMutex.Unlock()
	filter, ok := fl.filtersByID[filterID]
	if !ok {
		return fmt.Errorf("filter %d not found", filterID)
	}
	err := fl.orm.MarkFilterBackfilled(ctx, filterID)
	if err != nil {
		return fmt.Errorf("failed to mark filter backfilled: %w", err)
	}

	filter.IsBackfilled = true
	delete(fl.filtersToBackfill, filter.ID)
	return nil
}

// LoadFilters - loads filters from database. Can be called multiple times without side effects.
func (fl *filters) LoadFilters(ctx context.Context) error {
	if fl.loadedFilters.Load() {
		return nil
	}

	fl.lggr.Debugw("Loading filters from db")
	fl.filtersMutex.Lock()
	defer fl.filtersMutex.Unlock()
	// reset filters' indexes to ensure we do not have partial data from the previous run
	fl.filtersByID = make(map[int64]*Filter)
	fl.filtersByName = make(map[string]int64)
	fl.filtersByAddress = make(map[PublicKey]map[EventSignature]map[int64]struct{})
	fl.filtersToBackfill = make(map[int64]struct{})
	fl.filtersToDelete = make(map[int64]Filter)

	filters, err := fl.orm.SelectFilters(ctx)
	if err != nil {
		return fmt.Errorf("failed to select filters from db: %w", err)
	}

	for i := range filters {
		filter := filters[i]
		if filter.IsDeleted {
			fl.filtersToDelete[filter.ID] = filter
			continue
		}

		fl.filtersByID[filter.ID] = &filter

		if _, ok := fl.filtersByName[filter.Name]; ok {
			errMsg := fmt.Sprintf("invariant violation while loading from db: expected filters to have unique name: %s ", filter.Name)
			fl.lggr.Critical(errMsg)
			return errors.New(errMsg)
		}

		fl.filtersByName[filter.Name] = filter.ID
		filtersForAddress, ok := fl.filtersByAddress[filter.Address]
		if !ok {
			filtersForAddress = make(map[EventSignature]map[int64]struct{})
			fl.filtersByAddress[filter.Address] = filtersForAddress
		}

		filtersForEventSig, ok := filtersForAddress[filter.EventSig]
		if !ok {
			filtersForEventSig = make(map[int64]struct{})
			filtersForAddress[filter.EventSig] = filtersForEventSig
		}

		if _, ok := filtersForEventSig[filter.ID]; ok {
			errMsg := fmt.Sprintf("invariant violation while loading from db: expected filters to have unique ID: %d ", filter.ID)
			fl.lggr.Critical(errMsg)
			return errors.New(errMsg)
		}

		filtersForEventSig[filter.ID] = struct{}{}
		if !filter.IsBackfilled {
			fl.filtersToBackfill[filter.ID] = struct{}{}
		}
	}

	fl.loadedFilters.Store(true)

	return nil
}

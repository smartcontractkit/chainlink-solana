package logpoller

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"maps"
	"math/rand"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

type filters struct {
	orm  ORM
	lggr logger.SugaredLogger

	filtersByID            map[int64]*types.Filter
	filtersByName          map[string]int64
	filtersByAddress       map[types.PublicKey]map[types.EventSignature]map[int64]struct{}
	filtersToBackfill      map[int64]struct{}
	filtersToDelete        map[int64]types.Filter
	filtersMutex           sync.RWMutex
	loadedFilters          atomic.Bool
	knownPrograms          map[string]uint               // fast lookup to see if a base58-encoded ProgramID matches any registered filters
	knownDiscriminators    map[types.EventSignature]uint // fast lookup based on raw discriminator bytes as string
	seqNums                map[int64]int64
	seqNumsMutex           sync.Mutex
	decoders               map[int64]types.Decoder
	discriminatorExtractor solcommoncodec.DiscriminatorExtractor
	cpiEventExtractor      *CPIEventExtractor
}

func newFilters(lggr logger.Logger, orm ORM, cpiEventExtractor *CPIEventExtractor) *filters {
	return &filters{
		orm:                    orm,
		lggr:                   logger.Sugared(lggr),
		decoders:               make(map[int64]types.Decoder),
		discriminatorExtractor: solcommoncodec.NewDiscriminatorExtractor(),
		cpiEventExtractor:      cpiEventExtractor,
	}
}

// IncrementSeqNum increments the sequence number for a filterID and returns the new
// number. This means the sequence number assigned to the first log matched after registration will be 1.
func (fl *filters) IncrementSeqNum(filterID int64) int64 {
	fl.seqNumsMutex.Lock()
	defer fl.seqNumsMutex.Unlock()
	fl.seqNums[filterID]++
	return fl.seqNums[filterID]
}

// PruneFilters - prunes all filters marked to be deleted from the database and all corresponding logs.
func (fl *filters) PruneFilters(ctx context.Context) error {
	err := fl.LoadFilters(ctx)
	if err != nil {
		return fmt.Errorf("failed to load filters: %w", err)
	}

	fl.filtersMutex.Lock()
	filtersToDelete := fl.filtersToDelete
	fl.filtersToDelete = make(map[int64]types.Filter)
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

// Returns a randomly shuffled snapshot of all currently registered filters
func (fl *filters) shuffledFilters() iter.Seq[types.Filter] {
	fl.filtersMutex.Lock()
	filterSlice := make([]*types.Filter, 0, len(fl.filtersByID))
	for _, filter := range fl.filtersByID {
		filterSlice = append(filterSlice, filter)
	}
	fl.filtersMutex.Unlock()

	r := rand.New(rand.NewSource(time.Now().UnixNano())) // nolint:gosec // disable G404
	r.Shuffle(len(filterSlice), func(i, j int) {
		filterSlice[i], filterSlice[j] = filterSlice[j], filterSlice[i]
	})

	return func(yield func(types.Filter) bool) {
		for _, filter := range filterSlice {
			if !yield(*filter) {
				return
			}
		}
	}
}

func (fl *filters) PruneLogs(ctx context.Context) error {
	err := fl.LoadFilters(ctx)
	if err != nil {
		return fmt.Errorf("failed to load filters: %w", err)
	}

	for filter := range fl.shuffledFilters() {
		// Intentionally not holding the filtersMutex lock here, since getting through all of these DELETE
		// operations could take a while. New filters may be added while we are pruning the old ones, but
		// those won't be eligible yet for pruning anyway. Some may be removed (along with their logs) while
		// we're iterating through the snapshot we have. For those, the DELETE will succeed immediately
		// since there are no rows to delete.

		_, err = fl.orm.PruneLogsForFilter(ctx, filter)
		if err != nil {
			return fmt.Errorf("failed to prune logs for filter %s: %w", filter.Name, err)
		}
		time.Sleep(1 * time.Second) // pause for a moment to avoid overwhelming the database
	}

	return nil
}

func (fl *filters) HasFilter(ctx context.Context, name string) bool {
	exists, err := fl.orm.HasFilter(ctx, name)
	if err != nil {
		return false
	}

	return exists
}

// RegisterFilter persists provided filter and ensures that any log emitted by a contract with filter.Address
// that matches filter.EventSig signature will be captured starting from filter.StartingBlock.
// The filter may be unregistered later by filter.Name.
// In case of Filter.Name collision (within the chain scope) returns ErrFilterNameConflict if
// one of the fields defining resulting logs (Address, EventSig, EventIDL, SubKeyPaths) does not match original filter.
// Otherwise, updates remaining fields and schedules backfill.
// Warnings/debug information is keyed by filter name.
func (fl *filters) RegisterFilter(ctx context.Context, filter types.Filter) error {
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

			// also trigger a backfill if IncludeReverted is being updated from false to true
			if !existingFilter.IncludeReverted && filter.IncludeReverted {
				filter.IsBackfilled = false
			}
		}

		fl.removeFilterFromIndexes(*existingFilter)
	}

	// ensure that the value of IncludeReverted isn't different from any other filters with the same address and event type
	if contractFilters, okAddr := fl.filtersByAddress[filter.Address]; okAddr {
		if similarFilters, okEv := contractFilters[filter.EventSig]; okEv {
			for id := range similarFilters {
				if conflicting := fl.filtersByID[id]; conflicting.IncludeReverted != filter.IncludeReverted {
					return fmt.Errorf("IncludeReverted=%v for filter %v conflicts with IncludeReverted=%v for filter %v",
						conflicting.IncludeReverted, conflicting, filter.IncludeReverted, filter)
				}
			}
		}
	}

	decoder, err := newDecoder(filter)
	if err != nil {
		return err
	}

	filterID, err := fl.orm.InsertFilter(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to insert filter: %w", err)
	}

	filter.ID = filterID
	fl.addToIndices(filter, decoder)

	return nil
}

func newDecoder(filter types.Filter) (types.Decoder, error) {
	cEntry, err := codecv1.NewEventArgsEntry(filter.EventName, codecv1.EventIDLTypes(filter.EventIdl), true, nil, binary.LittleEndian())
	if err != nil {
		return nil, err
	}

	return solcommoncodec.EntryAsModifierRemoteCodec(cEntry, filter.EventName)
}

func (fl *filters) addToIndices(filter types.Filter, decoder types.Decoder) {
	fl.filtersByID[filter.ID] = &filter

	if _, ok := fl.filtersByName[filter.Name]; ok {
		errMsg := fmt.Sprintf("invariant violation while loading from db: expected filters to have unique name: %s ", filter.Name)
		fl.lggr.Critical(errMsg)
	}

	fl.decoders[filter.ID] = decoder
	fl.filtersByName[filter.Name] = filter.ID
	filtersForAddress, ok := fl.filtersByAddress[filter.Address]
	if !ok {
		filtersForAddress = make(map[types.EventSignature]map[int64]struct{})
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
	}

	filtersForEventSig[filter.ID] = struct{}{}
	if !filter.IsBackfilled {
		fl.filtersToBackfill[filter.ID] = struct{}{}
	}

	if filter.IsCPIFilter() {
		if fl.cpiEventExtractor != nil {
			fl.cpiEventExtractor.AddFilter(filter)
		} else {
			fl.lggr.Critical("cpiEventExtractor is nil but filter is a CPI filter")
		}
	} else {
		programID := filter.Address.ToSolana().String()
		fl.knownPrograms[programID]++
		fl.knownDiscriminators[filter.EventSig]++
	}
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

// removeFilterFromIndexes removes the filter from all indexes
// WARNING: not thread safe, should only be called while fl.filtersMutex is locked
func (fl *filters) removeFilterFromIndexes(filter types.Filter) {
	delete(fl.filtersByName, filter.Name)
	delete(fl.filtersToBackfill, filter.ID)
	delete(fl.filtersByID, filter.ID)
	fl.seqNumsMutex.Lock()
	delete(fl.seqNums, filter.ID)
	fl.seqNumsMutex.Unlock()
	delete(fl.decoders, filter.ID)

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

	if filter.IsCPIFilter() {
		if fl.cpiEventExtractor != nil {
			fl.cpiEventExtractor.RemoveFilter(filter)
		} else {
			fl.lggr.Critical("cpiEventExtractor is nil but filter is a CPI filter")
		}
	} else {
		programID := filter.Address.ToSolana().String()
		if refcount, ok := fl.knownPrograms[programID]; ok {
			refcount--
			if refcount > 0 {
				fl.knownPrograms[programID] = refcount
			} else {
				delete(fl.knownPrograms, programID)
			}
		}

		if refcount, ok := fl.knownDiscriminators[filter.EventSig]; ok {
			refcount--
			if refcount > 0 {
				fl.knownDiscriminators[filter.EventSig] = refcount
			} else {
				delete(fl.knownDiscriminators, filter.EventSig)
			}
		}
	}
}

func (fl *filters) GetDistinctAddresses(ctx context.Context) ([]types.PublicKey, error) {
	err := fl.LoadFilters(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load filters: %w", err)
	}

	fl.filtersMutex.RLock()
	defer fl.filtersMutex.RUnlock()

	var result []types.PublicKey
	set := map[types.PublicKey]struct{}{}
	for _, filter := range fl.filtersByID {
		if _, ok := set[filter.Address]; ok {
			continue
		}

		set[filter.Address] = struct{}{}
		result = append(result, filter.Address)
	}

	return result, nil
}

// MatchingFilters - returns iterator to go through all matching filters.
// Requires LoadFilters to be called at least once.
func (fl *filters) matchingFilters(addr types.PublicKey, eventSignature types.EventSignature, isCPI bool) iter.Seq[types.Filter] {
	if !fl.loadedFilters.Load() {
		fl.lggr.Critical("Invariant violation: expected filters to be loaded before call to matchingFilters")
		return nil
	}
	return func(yield func(types.Filter) bool) {
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
			if filter.IsCPIFilter() != isCPI {
				continue
			}
			if !yield(*filter) {
				return
			}
		}
	}
}

// MatchingFiltersForEncodedEvent - similar to MatchingFilters but accepts a raw encoded event. Under normal operation,
// this will be called on every new event that happens on the blockchain, so it's important it returns immediately if it
// doesn't match any registered filters.
func (fl *filters) MatchingFiltersForEncodedEvent(event types.ProgramEvent) iter.Seq[types.Filter] {
	// If this log message corresponds to an anchor event, then it must begin with an 8 byte discriminator,
	// which will appear as the first 11 bytes of base64-encoded data. Standard base64 encoding RFC requires
	// that any base64-encoded string must be padding with the = char to make its length a multiple of 4, so
	// 12 is the minimum length for a valid anchor event.
	if len(event.Data) < 12 {
		return nil
	}

	discriminator := fl.discriminatorExtractor.Extract(event.Data)

	// already filtered to only include matching events for CPI events
	if !event.IsCPI {
		isKnown := func() (ok bool) {
			fl.filtersMutex.RLock()
			defer fl.filtersMutex.RUnlock()

			if _, ok = fl.knownPrograms[event.Program]; !ok {
				return ok
			}

			_, ok = fl.knownDiscriminators[discriminator]
			return ok
		}

		if !isKnown() {
			return nil
		}
	}

	addr, err := solana.PublicKeyFromBase58(event.Program)
	if err != nil {
		fl.lggr.Errorw("failed to parse Program ID for event", "EventProgram", event)
		return nil
	}

	return fl.matchingFilters(types.PublicKey(addr), discriminator, event.IsCPI)
}

// GetFiltersToBackfill - returns copy of backfill queue
// Requires LoadFilters to be called at least once.
func (fl *filters) GetFiltersToBackfill() []types.Filter {
	if !fl.loadedFilters.Load() {
		fl.lggr.Critical("Invariant violation: expected filters to be loaded before call to MatchingFilters")
		return nil
	}
	fl.filtersMutex.Lock()
	defer fl.filtersMutex.Unlock()
	result := make([]types.Filter, 0, len(fl.filtersToBackfill))
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

// GetFilters returns a copy of all currently registered filters, keyed by filter name.
func (fl *filters) GetFilters(ctx context.Context) (map[string]types.Filter, error) {
	err := fl.LoadFilters(ctx)
	if err != nil {
		return nil, err
	}
	fl.filtersMutex.RLock()
	defer fl.filtersMutex.RUnlock()

	result := make(map[string]types.Filter, len(fl.filtersByID))
	for _, filter := range fl.filtersByID {
		result[filter.Name] = *filter
	}
	return result, nil
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

// UpdateStartingBlocks will update the starting blocks of all backfilled filters to startingBlock
// and set IsBackfilled=false for them. It will also update the starting blocks of any filters which
// are already scheduled for backfilling, but only if the new startingBlock is less than the one they
// currently have set. All filters will then be added to the filtersToBackfill index, so their logs will
// be (re-)fetched from the specified starting point on the next iteration of the main LogPoller run loop.
func (fl *filters) UpdateStartingBlocks(startingBlock int64) {
	fl.filtersMutex.Lock()
	defer fl.filtersMutex.Unlock()

	startingBlocks := make(map[int64]int64, len(fl.filtersByID))
	for id, filter := range fl.filtersByID {
		newStartingBlock := filter.StartingBlock
		if filter.IsBackfilled || startingBlock < newStartingBlock {
			newStartingBlock = startingBlock
		}
		startingBlocks[id] = newStartingBlock
	}

	for id, blk := range startingBlocks {
		fl.filtersByID[id].IsBackfilled = false
		fl.filtersByID[id].StartingBlock = blk
		fl.filtersToBackfill[id] = struct{}{}
	}
}

// LoadFilters - loads filters from database. Can be called multiple times without side effects.
func (fl *filters) LoadFilters(ctx context.Context) error {
	if fl.loadedFilters.Load() {
		return nil
	}

	fl.lggr.Debugw("Loading filters from db")
	fl.filtersMutex.Lock()
	defer fl.filtersMutex.Unlock()
	if fl.loadedFilters.Load() {
		return nil
	}
	// reset filters' indexes to ensure we do not have partial data from the previous run
	fl.filtersByID = make(map[int64]*types.Filter)
	fl.filtersByName = make(map[string]int64)
	fl.filtersByAddress = make(map[types.PublicKey]map[types.EventSignature]map[int64]struct{})
	fl.filtersToBackfill = make(map[int64]struct{})
	fl.filtersToDelete = make(map[int64]types.Filter)
	fl.knownPrograms = make(map[string]uint)
	fl.knownDiscriminators = make(map[types.EventSignature]uint)

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

		var decoder types.Decoder
		decoder, err = newDecoder(filter)
		if err != nil {
			return fmt.Errorf("failed to create decoder for filter %d: %w", filter.ID, err)
		}

		fl.addToIndices(filter, decoder)
	}
	fl.seqNumsMutex.Lock()
	fl.seqNums, err = fl.orm.SelectSeqNums(ctx)
	fl.seqNumsMutex.Unlock()
	if err != nil {
		return fmt.Errorf("failed to select sequence numbers from db: %w", err)
	}

	fl.loadedFilters.Store(true)

	return nil
}

// DecodeSubKey accepts raw Borsh-encoded event data, a filter ID and a subKeyPath. It uses the decoder
// associated with that filter to decode the event and extract the subKey value from the specified subKeyPath.
// WARNING: not thread safe, should only be called while fl.filtersMutex is held and after filters have been loaded.
func (fl *filters) DecodeSubKey(ctx context.Context, lggr logger.SugaredLogger, raw []byte, ID int64, subKeyPath []string) (any, error) {
	filter, ok := fl.filtersByID[ID]
	if !ok {
		return nil, fmt.Errorf("filter %d not found", ID)
	}
	decoder, ok := fl.decoders[ID]
	if !ok {
		return nil, fmt.Errorf("decoder %d not found", ID)
	}
	decodedEvent, err := decoder.CreateType(filter.EventName, false)
	if err != nil || decodedEvent == nil {
		return nil, err
	}
	if err = decoder.Decode(ctx, raw, decodedEvent, filter.EventName); err != nil {
		err = fmt.Errorf("failed to decode sub key raw data: %v, for filter: %s, for subKeyPath: %v, err: %w", raw, subKeyPath, filter.Name, err)
		lggr.Criticalw(err.Error())
		return nil, err
	}
	return ExtractField(decodedEvent, subKeyPath)
}

// ExtractField extracts the value of a field or nested subfield from a composite datatype composed
// of a series of nested structs and maps. Pointers at any level are automatically dereferenced, as long
// as they aren't nil. path is an ordered list of nested subfield names to traverse. For now, slices and
// arrays are not supported. (If the need arises, we could support them by converting the field to an
// integer to extract a specific element from a slice or array.)
func ExtractField(data any, path []string) (any, error) {
	v := reflect.ValueOf(data)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			if len(path) > 0 {
				return nil, fmt.Errorf("cannot extract field '%s' from a nil pointer", path[0])
			}
			return nil, nil // as long as this is the last field in the path, nil pointer is not a problem
		}
		v = v.Elem()
	}

	if len(path) == 0 {
		return v.Interface(), nil
	}
	field, path := path[0], path[1:]

	switch v.Kind() {
	case reflect.Struct:
		v = v.FieldByName(field)
		if !v.IsValid() {
			return nil, fmt.Errorf("field '%s' of struct %v does not exist", field, data)
		}
		return ExtractField(v.Interface(), path)
	case reflect.Map:
		var keyVal reflect.Value
		if keyType := v.Type().Key(); keyType.Kind() != reflect.String {
			// This map does not have string keys, so let's try int (or anything convertible to int)
			intKey, err := strconv.Atoi(field)
			if err != nil {
				return nil, fmt.Errorf("map key '%s' for non-string type '%T' is not convertable to an integer", field, v.Type())
			}
			if !keyType.ConvertibleTo(reflect.TypeOf(intKey)) {
				return nil, fmt.Errorf("map has type '%T', must be a string or convertable to an integer", v.Type())
			}
			keyVal = reflect.ValueOf(intKey)
		} else {
			keyVal = reflect.ValueOf(field)
		}
		v = v.MapIndex(keyVal)
		if !v.IsValid() {
			return nil, fmt.Errorf("key '%s' of map %v does not exist", field, data)
		}
		return ExtractField(v.Interface(), path)
	default:
		return nil, fmt.Errorf("extracting a field from a %s type is not supported", v.Kind().String())
	}
}

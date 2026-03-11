package logpoller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

func TestFilters_LoadFilters(t *testing.T) {
	orm := mocks.NewMockORM(t)
	fs := newFilters(logger.Sugared(logger.Test(t)), orm, nil)
	ctx := t.Context()
	orm.On("SelectFilters", mock.Anything).Return(nil, errors.New("db failed")).Once()
	deleted := types.Filter{
		ID:        3,
		Name:      "Deleted",
		IsDeleted: true,
	}
	happyPath := types.Filter{
		ID:           1,
		Name:         "Happy path",
		EventName:    "happyPath1",
		EventSig:     types.NewEventSignatureFromName("happyPath1"),
		IsBackfilled: true,
	}
	happyPath2 := types.Filter{
		ID:        2,
		Name:      "Happy path 2",
		EventName: "happyPath2",
		EventSig:  types.NewEventSignatureFromName("happyPath2"),
	}
	orm.On("SelectFilters", mock.Anything).Return([]types.Filter{
		deleted,
		happyPath,
		happyPath2,
	}, nil).Once()

	orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{
		1: 18,
		2: 25,
		3: 0,
	}, nil).Once()

	err := fs.LoadFilters(ctx)
	require.EqualError(t, err, "failed to select filters from db: db failed")
	err = fs.LoadFilters(ctx)
	require.NoError(t, err)
	// only one filter to delete
	require.Len(t, fs.filtersToDelete, 1)
	require.Equal(t, deleted, fs.filtersToDelete[deleted.ID])
	// filtersByAddress only contains not deleted filters
	require.Len(t, fs.filtersByAddress, 1)
	require.Len(t, fs.filtersByAddress[happyPath.Address], 2)
	require.Len(t, fs.filtersByAddress[happyPath.Address][happyPath.EventSig], 1)
	// both filters are properly indexed
	requireIndexed(t, fs, happyPath)
	requireIndexed(t, fs, happyPath2)
	// only happyPath2 requires backfill
	require.Len(t, fs.filtersToBackfill, 1)
	require.Contains(t, fs.filtersToBackfill, happyPath2.ID)
	// any call following successful should be noop
	err = fs.LoadFilters(ctx)
	require.NoError(t, err)
}

func requireIndexed(t *testing.T, fs *filters, f types.Filter) {
	require.NotNil(t, fs.filtersByID[f.ID])
	require.Equal(t, f, *fs.filtersByID[f.ID])
	require.Equal(t, f.ID, fs.filtersByName[f.Name])
	byEventSig := fs.filtersByAddress[f.Address]
	require.NotNil(t, byEventSig)
	eventSigIDs := byEventSig[f.EventSig]
	require.Contains(t, eventSigIDs, f.ID)
	require.Contains(t, fs.decoders, f.ID)
	require.Contains(t, fs.knownDiscriminators, f.EventSig)
	require.Contains(t, fs.knownPrograms, f.Address.String())
}

func requireNoInIndices(t *testing.T, fs *filters, f types.Filter) {
	require.Nil(t, fs.filtersByID[f.ID])
	require.NotContains(t, fs.filtersByName, f.Name)
	require.NotContains(t, fs.filtersByAddress, f.Address)
	byEventSig := fs.filtersByAddress[f.Address]
	if byEventSig != nil && byEventSig[f.EventSig] != nil {
		require.NotContains(t, byEventSig[f.EventSig], f.ID)
	}
	require.NotContains(t, fs.decoders, f.ID)
	require.NotContains(t, fs.knownDiscriminators, f.EventSig)
	require.NotContains(t, fs.knownPrograms, f.Address.String())
	require.NotContains(t, fs.seqNums, f.ID)
	require.NotContains(t, fs.filtersToBackfill, f.ID)
}

func TestFilters_RegisterFilter(t *testing.T) {
	lggr := logger.Sugared(logger.Test(t))
	t.Run("Returns an error if name is empty", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		err := fs.RegisterFilter(t.Context(), types.Filter{})
		require.EqualError(t, err, "name is required")
	})
	t.Run("Returns an error if fails to load filters from db", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		orm.On("SelectFilters", mock.Anything).Return(nil, errors.New("db failed")).Once()
		err := fs.RegisterFilter(t.Context(), types.Filter{Name: "Filter"})
		require.EqualError(t, err, "failed to load filters: failed to select filters from db: db failed")
	})
	t.Run("Returns an error if trying to update primary fields", func(t *testing.T) {
		testCases := []struct {
			Name        string
			ModifyField func(*types.Filter)
		}{
			{
				Name: "Address",
				ModifyField: func(f *types.Filter) {
					privateKey, err := solana.NewRandomPrivateKey()
					require.NoError(t, err)
					f.Address = types.PublicKey(privateKey.PublicKey())
				},
			},
			{
				Name: "EventSig",
				ModifyField: func(f *types.Filter) {
					f.EventSig = types.EventSignature{3, 2, 1}
				},
			},
			{
				Name: "SubKeyPaths",
				ModifyField: func(f *types.Filter) {
					f.SubkeyPaths = [][]string{{uuid.NewString()}}
				},
			},
		}
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("Updating %s", tc.Name), func(t *testing.T) {
				orm := mocks.NewMockORM(t)
				fs := newFilters(lggr, orm, nil)
				const filterName = "Filter"
				dbFilter := types.Filter{Name: filterName}
				orm.On("SelectFilters", mock.Anything).Return([]types.Filter{dbFilter}, nil).Once()
				orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil)
				newFilter := dbFilter
				tc.ModifyField(&newFilter)
				err := fs.RegisterFilter(t.Context(), newFilter)
				require.EqualError(t, err, ErrFilterNameConflict.Error())
			})
		}
	})
	t.Run("properly handles IncludeReverted field", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		addr := newRandomPublicKey(t)
		eventSig := newRandomEventSignature(t)

		filter1 := types.Filter{
			ID:              1,
			Name:            "existingFilter",
			Address:         addr,
			EventSig:        eventSig,
			IncludeReverted: false,
			IsBackfilled:    true,
		}
		orm.EXPECT().SelectFilters(mock.Anything).Return(
			[]types.Filter{filter1}, nil).Once()
		filter2 := types.Filter{
			Name:            "new filter",
			Address:         addr,
			EventSig:        eventSig,
			IncludeReverted: true,
			IsBackfilled:    true,
		}
		orm.EXPECT().SelectSeqNums(mock.Anything).Return(nil, nil).Once()
		err := fs.RegisterFilter(t.Context(), filter2)
		require.ErrorContains(t, err, "conflicts with IncludeReverted=true", "shouldn't allow more than one value for IncludeReverted for an event")

		orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, filter types.Filter) (int64, error) {
			assert.True(t, filter.IncludeReverted, "IncludeReverted should be true now")
			assert.False(t, filter.IsBackfilled, "new backfill should be triggered when IsReverted updated from false to true")
			return 2, nil
		}).Once()
		filter1.IncludeReverted = true // update IncludeReverted field of filter1 to true
		err = fs.RegisterFilter(t.Context(), filter1)
		require.NoError(t, err)

		orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, filter types.Filter) (int64, error) {
			assert.True(t, filter.IncludeReverted)
			assert.False(t, filter.IsBackfilled, "backfill should happen when new filter is added") // should trigger new backfill since reverted has been updated to true
			return 3, nil
		}).Once()

		// should succeed this time
		err = fs.RegisterFilter(t.Context(), filter2)
		assert.NoError(t, err)
	})
	t.Run("Happy path", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		const filterName = "Filter"
		orm.On("SelectFilters", mock.Anything).Return(nil, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(int64(0), errors.New("failed to insert")).Once()
		filter := types.Filter{Name: filterName}
		err := fs.RegisterFilter(t.Context(), filter)
		require.Error(t, err)

		// can read after db issue is resolved
		const filterID = int64(1)
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(filterID, nil).Once()
		err = fs.RegisterFilter(t.Context(), filter)
		require.NoError(t, err)
		// can update non-primary fields
		filter.StartingBlock++
		filter.Retention++
		filter.MaxLogsKept++
		filter.IncludeReverted = true
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(filterID, nil).Once()
		err = fs.RegisterFilter(t.Context(), filter)
		require.NoError(t, err)
		storedFilters := slices.Collect(fs.matchingFilters(filter.Address, filter.EventSig, false))
		require.Len(t, storedFilters, 1)
		filter.ID = 1
		require.Equal(t, filter, storedFilters[0])
		// all indices contain filter
		requireIndexed(t, fs, filter)
	})
	t.Run("Can reregister after unregister", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		const filterName = "Filter"
		orm.On("SelectFilters", mock.Anything).Return(nil, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()
		const filterID = int64(10)
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(filterID, nil).Once()
		err := fs.RegisterFilter(t.Context(), types.Filter{Name: filterName})
		require.NoError(t, err)
		requireIndexed(t, fs, types.Filter{Name: filterName, ID: filterID})
		orm.On("MarkFilterDeleted", mock.Anything, filterID).Return(nil).Once()
		err = fs.UnregisterFilter(t.Context(), filterName)
		require.NoError(t, err)
		requireNoInIndices(t, fs, types.Filter{Name: filterName, ID: filterID})
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(filterID+1, nil).Once()
		err = fs.RegisterFilter(t.Context(), types.Filter{Name: filterName})
		require.NoError(t, err)
		require.Len(t, fs.filtersToDelete, 1)
		require.Equal(t, types.Filter{Name: filterName, ID: filterID}, fs.filtersToDelete[filterID])
		require.Len(t, fs.filtersToBackfill, 1)
		require.Contains(t, fs.filtersToBackfill, filterID+1)
		requireIndexed(t, fs, types.Filter{Name: filterName, ID: filterID + 1})
	})
}

func TestFilters_UnregisterFilter(t *testing.T) {
	lggr := logger.Sugared(logger.Test(t))
	t.Run("Returns an error if fails to load filters from db", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		orm.On("SelectFilters", mock.Anything).Return(nil, errors.New("db failed")).Once()
		err := fs.UnregisterFilter(t.Context(), "Filter")
		require.EqualError(t, err, "failed to load filters: failed to select filters from db: db failed")
	})
	t.Run("Noop if filter is not present", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		const filterName = "Filter"
		orm.On("SelectFilters", mock.Anything).Return(nil, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()
		err := fs.UnregisterFilter(t.Context(), filterName)
		require.NoError(t, err)
	})
	t.Run("Returns error if fails to mark filter as deleted", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		const filterName = "Filter"
		const id int64 = 10
		orm.On("SelectFilters", mock.Anything).Return([]types.Filter{{ID: id, Name: filterName}}, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()
		orm.On("MarkFilterDeleted", mock.Anything, id).Return(errors.New("db query failed")).Once()
		err := fs.UnregisterFilter(t.Context(), filterName)
		require.EqualError(t, err, "failed to mark filter deleted: db query failed")
	})
	t.Run("Happy path", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		const filterName = "Filter"
		const id int64 = 10
		f := types.Filter{ID: id, Name: filterName}
		orm.On("SelectFilters", mock.Anything).Return([]types.Filter{f}, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()
		orm.On("MarkFilterDeleted", mock.Anything, id).Return(nil).Once()
		err := fs.UnregisterFilter(t.Context(), filterName)
		require.NoError(t, err)
		require.Contains(t, fs.filtersToDelete, f.ID)
		requireNoInIndices(t, fs, f)
	})
}

func TestFilters_PruneFilters(t *testing.T) {
	lggr := logger.Sugared(logger.Test(t))
	t.Run("Happy path", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		toDelete := types.Filter{
			ID:        1,
			Name:      "To delete",
			IsDeleted: true,
		}
		orm.On("SelectFilters", mock.Anything).Return([]types.Filter{
			toDelete,
			{
				ID:   2,
				Name: "To keep",
			},
		}, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{
			2: 25,
		}, nil).Once()
		orm.On("DeleteFilters", mock.Anything, map[int64]types.Filter{toDelete.ID: toDelete}).Return(nil).Once()
		err := fs.PruneFilters(t.Context())
		require.NoError(t, err)
		require.Len(t, fs.filtersToDelete, 0)
	})
	t.Run("If DB removal fails will add filters back into removal slice ", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		toDelete := types.Filter{
			ID:        1,
			Name:      "To delete",
			IsDeleted: true,
		}
		orm.On("SelectFilters", mock.Anything).Return([]types.Filter{
			toDelete,
			{
				ID:   2,
				Name: "To keep",
			},
		}, nil).Once()
		orm.EXPECT().SelectSeqNums(mock.Anything).Return(map[int64]int64{
			1: 18,
			2: 25,
		}, nil).Once()
		newToDelete := types.Filter{
			ID:   3,
			Name: "To delete 2",
		}
		orm.On("DeleteFilters", mock.Anything, map[int64]types.Filter{toDelete.ID: toDelete}).Return(errors.New("db failed")).Run(func(_ mock.Arguments) {
			orm.On("MarkFilterDeleted", mock.Anything, newToDelete.ID).Return(nil).Once()
			orm.On("InsertFilter", mock.Anything, mock.Anything).Return(newToDelete.ID, nil).Once()
			require.NoError(t, fs.RegisterFilter(t.Context(), newToDelete))
			require.NoError(t, fs.UnregisterFilter(t.Context(), newToDelete.Name))
		}).Once()
		err := fs.PruneFilters(t.Context())
		require.EqualError(t, err, "failed to delete filters: db failed")
		require.Equal(t, fs.filtersToDelete, map[int64]types.Filter{newToDelete.ID: newToDelete, toDelete.ID: toDelete})
	})
}

func TestFilters_MatchingFilters(t *testing.T) {
	orm := mocks.NewMockORM(t)
	lggr := logger.Sugared(logger.Test(t))
	expectedFilter1 := types.Filter{
		ID:       1,
		Name:     "expectedFilter1",
		Address:  newRandomPublicKey(t),
		EventSig: newRandomEventSignature(t),
	}
	expectedFilter2 := types.Filter{
		ID:       2,
		Name:     "expectedFilter2",
		Address:  expectedFilter1.Address,
		EventSig: expectedFilter1.EventSig,
	}
	sameAddress := types.Filter{
		ID:       3,
		Name:     "sameAddressWrongEventSig",
		Address:  expectedFilter1.Address,
		EventSig: newRandomEventSignature(t),
	}

	sameEventSig := types.Filter{
		ID:       4,
		Name:     "wrongAddressSameEventSig",
		Address:  newRandomPublicKey(t),
		EventSig: expectedFilter1.EventSig,
	}
	orm.On("SelectFilters", mock.Anything).Return([]types.Filter{expectedFilter1, expectedFilter2, sameAddress, sameEventSig}, nil).Once()
	orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{
		1: 18,
		2: 25,
		3: 14,
		4: 0,
	}, nil)
	filters := newFilters(lggr, orm, nil)
	err := filters.LoadFilters(t.Context())
	require.NoError(t, err)
	matchingFilters := slices.Collect(filters.matchingFilters(expectedFilter1.Address, expectedFilter1.EventSig, false))
	require.Len(t, matchingFilters, 2)
	require.Contains(t, matchingFilters, expectedFilter1)
	require.Contains(t, matchingFilters, expectedFilter2)
	// if at least one key does not match - returns empty iterator
	require.Empty(t, slices.Collect(filters.matchingFilters(newRandomPublicKey(t), expectedFilter1.EventSig, false)))
	require.Empty(t, slices.Collect(filters.matchingFilters(expectedFilter1.Address, newRandomEventSignature(t), false)))
	require.Empty(t, slices.Collect(filters.matchingFilters(newRandomPublicKey(t), newRandomEventSignature(t), false)))
}

func TestFilters_GetFiltersToBackfill(t *testing.T) {
	orm := mocks.NewMockORM(t)
	lggr := logger.Sugared(logger.Test(t))
	backfilledFilter := types.Filter{
		ID:            1,
		Name:          "backfilled",
		StartingBlock: 100,
		IsBackfilled:  true,
	}
	notBackfilled := types.Filter{
		ID:            2,
		StartingBlock: 101,
		Name:          "notBackfilled",
	}
	orm.EXPECT().SelectFilters(mock.Anything).Return([]types.Filter{backfilledFilter, notBackfilled}, nil).Once()
	orm.EXPECT().SelectSeqNums(mock.Anything).Return(map[int64]int64{
		1: 18,
		2: 25,
	}, nil)
	filters := newFilters(lggr, orm, nil)
	err := filters.LoadFilters(t.Context())
	require.NoError(t, err)
	// filters that were not backfilled are properly identified on load
	ensureInQueue := func(expectedFilters ...types.Filter) {
		filtersToBackfill := filters.GetFiltersToBackfill()
		require.Len(t, filtersToBackfill, len(expectedFilters))
		for _, expectedFilter := range expectedFilters {
			require.Contains(t, filtersToBackfill, expectedFilter)
		}
	}
	ensureInQueue(notBackfilled)
	// filter remains in queue if failed to mark as backfilled
	orm.EXPECT().MarkFilterBackfilled(mock.Anything, notBackfilled.ID).Return(errors.New("db call failed")).Once()
	err = filters.MarkFilterBackfilled(t.Context(), notBackfilled.ID)
	require.Error(t, err)
	ensureInQueue(notBackfilled)
	// filter is removed from queue, if marked as backfilled
	orm.EXPECT().MarkFilterBackfilled(mock.Anything, notBackfilled.ID).Return(nil).Once()
	err = filters.MarkFilterBackfilled(t.Context(), notBackfilled.ID)
	require.NoError(t, err)
	require.Empty(t, filters.GetFiltersToBackfill())
	// re adding identical filter won't trigger backfill
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).Return(backfilledFilter.ID, nil).Once()
	require.NoError(t, filters.RegisterFilter(t.Context(), backfilledFilter))
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).Return(notBackfilled.ID, nil).Once()
	require.NoError(t, filters.RegisterFilter(t.Context(), notBackfilled))
	require.Empty(t, filters.GetFiltersToBackfill())
	// older StartingBlock trigger backfill
	notBackfilled.StartingBlock = notBackfilled.StartingBlock - 1
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).Return(notBackfilled.ID, nil).Once()
	require.NoError(t, filters.RegisterFilter(t.Context(), notBackfilled))
	ensureInQueue(notBackfilled)
	// new filter is always added to the queue
	newFilter := types.Filter{Name: "new filter"}
	orm.EXPECT().InsertFilter(mock.Anything, newFilter).Return(3, nil).Once()
	require.NoError(t, filters.RegisterFilter(t.Context(), newFilter))
	ensureInQueue(notBackfilled, types.Filter{ID: 3, Name: "new filter"})
}

func TestFilters_ExtractField(t *testing.T) {
	type innerInner struct {
		P string
		Q int
	}
	type innerStruct struct {
		PtrString    *string
		ByteSlice    []byte
		DoubleNested innerInner
		MapStringInt map[string]int
		MapIntString map[int]string
	}
	myString := "string"
	myInt32 := int32(16)

	testStruct := struct {
		A int
		B string
		C *int32
		D innerStruct
	}{
		5,
		"hello",
		&myInt32,
		innerStruct{
			&myString,
			[]byte("bytes"),
			innerInner{"goodbye", 8},
			map[string]int{"key1": 1, "key2": 2},
			map[int]string{1: "val1", 2: "val2"},
		},
	}

	cases := []struct {
		Name   string
		Path   string
		Result any
	}{
		{"int from struct", "A", int(5)},
		{"string from struct", "B", "hello"},
		{"*int32 from struct", "C", myInt32},
		{"*string from nested struct", "D.PtrString", myString},
		{"[]byte from nested struct", "D.ByteSlice", []byte("bytes")},
		{"string from double-nested struct", "D.DoubleNested.P", "goodbye"},
		{"map[string]int from nested struct", "D.MapStringInt.key2", 2},
		{"key in map not found", "D.MapIntString.3", nil},
		{"non-integer key for map[int]string", "D.MapIntString.NotAnInt", nil},
		{"invalid field name in nested struct", "D.NoSuchField", nil},
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			result, err := ExtractField(&testStruct, strings.Split(c.Path, "."))
			if c.Result == nil {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.Result, result)
		})
	}
}

func TestFilters_IncrementSeqNum_Concurrent(t *testing.T) {
	orm := mocks.NewMockORM(t)
	lggr := logger.Sugared(logger.Test(t))
	fs := newFilters(lggr, orm, nil)

	filter1 := types.Filter{ID: 1, Name: "filter1", EventName: "event1", EventSig: types.NewEventSignatureFromName("event1")}
	filter2 := types.Filter{ID: 2, Name: "filter2", EventName: "event2", EventSig: types.NewEventSignatureFromName("event2")}
	orm.On("SelectFilters", mock.Anything).Return([]types.Filter{filter1, filter2}, nil).Once()
	orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{1: 0, 2: 0}, nil).Once()

	err := fs.LoadFilters(t.Context())
	require.NoError(t, err)

	const numGoroutines = 50
	const incrementsPerGoroutine = 100

	seqNumsFilter1 := make(chan int64, numGoroutines*incrementsPerGoroutine)
	seqNumsFilter2 := make(chan int64, numGoroutines*incrementsPerGoroutine)

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range incrementsPerGoroutine {
				seqNum := fs.IncrementSeqNum(filter1.ID)
				seqNumsFilter1 <- seqNum
			}
		}()
	}

	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range incrementsPerGoroutine {
				seqNum := fs.IncrementSeqNum(filter2.ID)
				seqNumsFilter2 <- seqNum
			}
		}()
	}

	wg.Wait()
	close(seqNumsFilter1)
	close(seqNumsFilter2)

	seenFilter1 := make(map[int64]struct{})
	for seqNum := range seqNumsFilter1 {
		_, exists := seenFilter1[seqNum]
		require.False(t, exists, "duplicate sequence number %d found for filter1", seqNum)
		seenFilter1[seqNum] = struct{}{}
	}
	require.Len(t, seenFilter1, numGoroutines*incrementsPerGoroutine, "expected %d unique sequence numbers for filter1", numGoroutines*incrementsPerGoroutine)

	seenFilter2 := make(map[int64]struct{})
	for seqNum := range seqNumsFilter2 {
		_, exists := seenFilter2[seqNum]
		require.False(t, exists, "duplicate sequence number %d found for filter2", seqNum)
		seenFilter2[seqNum] = struct{}{}
	}
	require.Len(t, seenFilter2, numGoroutines*incrementsPerGoroutine, "expected %d unique sequence numbers for filter2", numGoroutines*incrementsPerGoroutine)

	require.Equal(t, int64(numGoroutines*incrementsPerGoroutine), fs.seqNums[filter1.ID])
	require.Equal(t, int64(numGoroutines*incrementsPerGoroutine), fs.seqNums[filter2.ID])
}

func TestFilters_UpdateStartingBlocks(t *testing.T) {
	orm := mocks.NewMockORM(t)
	lggr := logger.Sugared(logger.Test(t))
	filters := newFilters(lggr, orm, nil)

	origFilters := []types.Filter{{
		ID:            1,
		Name:          "backfilled",
		StartingBlock: 29500,
		IsBackfilled:  true,
	}, {
		ID:            2,
		StartingBlock: 52000,
		Name:          "notBackfilled",
	}}
	ids := make([]int64, 2)
	for i, filter := range origFilters {
		ids[i] = filter.ID
	}

	var err error

	cases := []struct {
		name           string
		replayBlock    int64
		expectedBlocks []int64
	}{
		{
			name:           "updates StartingBlock of both filters",
			replayBlock:    51500,
			expectedBlocks: []int64{51500, 51500},
		},
		{
			name:           "updates StartingBlock of backfilled filter",
			replayBlock:    53000,
			expectedBlocks: []int64{53000, origFilters[1].StartingBlock},
		},
	}

	orm.EXPECT().SelectFilters(mock.Anything).Return(origFilters, nil).Once()
	orm.EXPECT().SelectSeqNums(mock.Anything).Return(map[int64]int64{
		1: 18,
		2: 25,
	}, nil)

	err = filters.LoadFilters(t.Context())
	require.NoError(t, err)
	// ensure both filters were loaded
	require.Equal(t, origFilters[0], *filters.filtersByID[ids[0]])
	require.Equal(t, origFilters[1], *filters.filtersByID[ids[1]])
	// ensure non-backfilled filters were added to filtersToBackfill
	require.Len(t, filters.filtersToBackfill, 1)
	require.Contains(t, filters.filtersToBackfill, origFilters[1].ID)

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			newFilters := make([]types.Filter, len(origFilters))
			copy(newFilters, origFilters)
			filters.filtersByID[ids[0]] = &newFilters[0]
			filters.filtersByID[ids[1]] = &newFilters[1]
			filters.filtersToBackfill = map[int64]struct{}{ids[0]: {}}
			filters.UpdateStartingBlocks(tt.replayBlock)
			assert.Len(t, filters.filtersToBackfill, 2) // all filters should end up in the backfill queue

			for i, id := range ids {
				assert.Equal(t, tt.expectedBlocks[i], filters.filtersByID[id].StartingBlock,
					"unexpected starting block for \"%s\" filter", filters.filtersByID[id].Name)
				assert.False(t, filters.filtersByID[id].IsBackfilled)
				assert.Contains(t, filters.filtersToBackfill, id)
			}
		})
	}
}

func TestFilters_GetFilters(t *testing.T) {
	lggr := logger.Sugared(logger.Test(t))

	t.Run("returns error when load fails", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		orm.On("SelectFilters", mock.Anything).Return(nil, errors.New("db error")).Once()

		result, err := fs.GetFilters(t.Context())
		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("returns empty map when no filters exist", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		orm.On("SelectFilters", mock.Anything).Return([]types.Filter{}, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()

		result, err := fs.GetFilters(t.Context())
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Empty(t, result)
	})

	t.Run("returns all filters keyed by name", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)

		filter1 := types.Filter{
			ID:        1,
			Name:      "filter1",
			EventName: "event1",
			EventSig:  types.NewEventSignatureFromName("event1"),
		}
		filter2 := types.Filter{
			ID:        2,
			Name:      "filter2",
			EventName: "event2",
			EventSig:  types.NewEventSignatureFromName("event2"),
		}

		orm.On("SelectFilters", mock.Anything).Return([]types.Filter{filter1, filter2}, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{1: 0, 2: 0}, nil).Once()

		result, err := fs.GetFilters(t.Context())
		require.NoError(t, err)
		require.Len(t, result, 2)
		require.Equal(t, filter1, result["filter1"])
		require.Equal(t, filter2, result["filter2"])
	})

	t.Run("excludes deleted filters", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)

		activeFilter := types.Filter{
			ID:        1,
			Name:      "activeFilter",
			EventName: "event1",
			EventSig:  types.NewEventSignatureFromName("event1"),
		}
		deletedFilter := types.Filter{
			ID:        2,
			Name:      "deletedFilter",
			IsDeleted: true,
		}

		orm.On("SelectFilters", mock.Anything).Return([]types.Filter{activeFilter, deletedFilter}, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{1: 0}, nil).Once()

		result, err := fs.GetFilters(t.Context())
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, activeFilter, result["activeFilter"])
		require.NotContains(t, result, "deletedFilter")
	})

	t.Run("returns a copy that does not affect internal state", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)

		filter1 := types.Filter{
			ID:            1,
			Name:          "filter1",
			EventName:     "event1",
			EventSig:      types.NewEventSignatureFromName("event1"),
			StartingBlock: 100,
		}

		orm.On("SelectFilters", mock.Anything).Return([]types.Filter{filter1}, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{1: 0}, nil).Once()

		result, err := fs.GetFilters(t.Context())
		require.NoError(t, err)
		require.Len(t, result, 1)

		// Modify the returned filter
		modifiedFilter := result["filter1"]
		modifiedFilter.StartingBlock = 9999
		result["filter1"] = modifiedFilter

		// Add a new entry to the returned map
		result["newFilter"] = types.Filter{Name: "newFilter"}

		// Get filters again and verify internal state was not affected
		result2, err := fs.GetFilters(t.Context())
		require.NoError(t, err)
		require.Len(t, result2, 1)
		require.Equal(t, int64(100), result2["filter1"].StartingBlock)
		require.NotContains(t, result2, "newFilter")
	})

	t.Run("concurrent access is safe", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)

		filter1 := types.Filter{
			ID:        1,
			Name:      "filter1",
			EventName: "event1",
			EventSig:  types.NewEventSignatureFromName("event1"),
		}

		orm.On("SelectFilters", mock.Anything).Return([]types.Filter{filter1}, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{1: 0}, nil).Once()

		err := fs.LoadFilters(t.Context())
		require.NoError(t, err)

		const numGoroutines = 50
		const readsPerGoroutine = 100

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for range numGoroutines {
			go func() {
				defer wg.Done()
				for range readsPerGoroutine {
					result, err := fs.GetFilters(t.Context())
					assert.NoError(t, err)
					assert.Len(t, result, 1)
					assert.Equal(t, filter1, result["filter1"])
				}
			}()
		}

		wg.Wait()
	})
}

package logpoller

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

func TestFilters_LoadFilters(t *testing.T) {
	orm := newMockORM(t)
	fs := newFilters(logger.Sugared(logger.Test(t)), orm)
	ctx := tests.Context(t)
	orm.On("SelectFilters", mock.Anything).Return(nil, errors.New("db failed")).Once()
	deleted := Filter{
		ID:        3,
		Name:      "Deleted",
		IsDeleted: true,
	}
	happyPath := Filter{
		ID:   1,
		Name: "Happy path",
	}
	happyPath2 := Filter{
		ID:   2,
		Name: "Happy path 2",
	}
	orm.On("SelectFilters", mock.Anything).Return([]Filter{
		deleted,
		happyPath,
		happyPath2,
	}, nil).Once()

	err := fs.LoadFilters(ctx)
	require.EqualError(t, err, "failed to select filters from db: db failed")
	err = fs.LoadFilters(ctx)
	require.NoError(t, err)
	// only one filter to delete
	require.Len(t, fs.filtersToDelete, 1)
	require.Equal(t, deleted, fs.filtersToDelete[deleted.ID])
	// both happy path are indexed
	require.Len(t, fs.filtersByAddress, 1)
	require.Len(t, fs.filtersByAddress[happyPath.Address], 1)
	require.Len(t, fs.filtersByAddress[happyPath.Address][happyPath.EventSig], 2)
	require.Contains(t, fs.filtersByAddress[happyPath.Address][happyPath.EventSig], happyPath.ID)
	require.Equal(t, happyPath, *fs.filtersByID[happyPath.ID])
	require.Contains(t, fs.filtersByAddress[happyPath.Address][happyPath.EventSig], happyPath2.ID)
	require.Equal(t, happyPath2, *fs.filtersByID[happyPath2.ID])
	require.Len(t, fs.filtersByName, 2)
	require.Equal(t, fs.filtersByName[happyPath.Name], happyPath.ID)
	require.Equal(t, fs.filtersByName[happyPath2.Name], happyPath2.ID)
	// any call following successful should be noop
	err = fs.LoadFilters(ctx)
	require.NoError(t, err)
}

func TestFilters_RegisterFilter(t *testing.T) {
	lggr := logger.Sugared(logger.Test(t))
	t.Run("Returns an error if name is empty", func(t *testing.T) {
		orm := newMockORM(t)
		fs := newFilters(lggr, orm)
		err := fs.RegisterFilter(tests.Context(t), Filter{})
		require.EqualError(t, err, "name is required")
	})
	t.Run("Returns an error if fails to load filters from db", func(t *testing.T) {
		orm := newMockORM(t)
		fs := newFilters(lggr, orm)
		orm.On("SelectFilters", mock.Anything).Return(nil, errors.New("db failed")).Once()
		err := fs.RegisterFilter(tests.Context(t), Filter{Name: "Filter"})
		require.EqualError(t, err, "failed to load filters: failed to select filters from db: db failed")
	})
	t.Run("Returns an error if trying to update primary fields", func(t *testing.T) {
		testCases := []struct {
			Name        string
			ModifyField func(*Filter)
		}{
			{
				Name: "Address",
				ModifyField: func(f *Filter) {
					privateKey, err := solana.NewRandomPrivateKey()
					require.NoError(t, err)
					f.Address = PublicKey(privateKey.PublicKey())
				},
			},
			{
				Name: "EventSig",
				ModifyField: func(f *Filter) {
					f.EventSig = EventSignature{3, 2, 1}
				},
			},
			{
				Name: "EventIDL",
				ModifyField: func(f *Filter) {
					f.EventIDL = uuid.NewString()
				},
			},
			{
				Name: "SubkeyPaths",
				ModifyField: func(f *Filter) {
					f.SubkeyPaths = [][]string{{uuid.NewString()}}
				},
			},
		}
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("Updating %s", tc.Name), func(t *testing.T) {
				orm := newMockORM(t)
				fs := newFilters(lggr, orm)
				const filterName = "Filter"
				dbFilter := Filter{Name: filterName}
				orm.On("SelectFilters", mock.Anything).Return([]Filter{dbFilter}, nil).Once()
				newFilter := dbFilter
				tc.ModifyField(&newFilter)
				err := fs.RegisterFilter(tests.Context(t), newFilter)
				require.EqualError(t, err, ErrFilterNameConflict.Error())
			})
		}
	})
	t.Run("Happy path", func(t *testing.T) {
		orm := newMockORM(t)
		fs := newFilters(lggr, orm)
		const filterName = "Filter"
		orm.On("SelectFilters", mock.Anything).Return(nil, nil).Once()
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(int64(0), errors.New("failed to insert")).Once()
		filter := Filter{Name: filterName}
		err := fs.RegisterFilter(tests.Context(t), filter)
		require.Error(t, err)
		// can readd after db issue is resolved
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(int64(1), nil).Once()
		err = fs.RegisterFilter(tests.Context(t), filter)
		require.NoError(t, err)
		// can update non-primary fields
		filter.EventName = uuid.NewString()
		filter.StartingBlock++
		filter.Retention++
		filter.MaxLogsKept++
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(int64(1), nil).Once()
		err = fs.RegisterFilter(tests.Context(t), filter)
		require.NoError(t, err)
		storedFilters := slices.Collect(fs.MatchingFilters(filter.Address, filter.EventSig))
		require.Len(t, storedFilters, 1)
		filter.ID = 1
		require.Equal(t, filter, storedFilters[0])
	})
	t.Run("Can reregister after unregister", func(t *testing.T) {
		orm := newMockORM(t)
		fs := newFilters(lggr, orm)
		const filterName = "Filter"
		orm.On("SelectFilters", mock.Anything).Return(nil, nil).Once()
		const filterID = int64(10)
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(filterID, nil).Once()
		err := fs.RegisterFilter(tests.Context(t), Filter{Name: filterName})
		require.NoError(t, err)
		orm.On("MarkFilterDeleted", mock.Anything, filterID).Return(nil).Once()
		err = fs.UnregisterFilter(tests.Context(t), filterName)
		require.NoError(t, err)
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(filterID+1, nil).Once()
		err = fs.RegisterFilter(tests.Context(t), Filter{Name: filterName})
		require.NoError(t, err)
		require.Len(t, fs.filtersToDelete, 1)
		require.Equal(t, Filter{Name: filterName, ID: filterID}, fs.filtersToDelete[filterID])
		require.Len(t, fs.filtersToBackfill, 1)
		require.Contains(t, fs.filtersToBackfill, filterID+1)
	})
}

func TestFilters_UnregisterFilter(t *testing.T) {
	lggr := logger.Sugared(logger.Test(t))
	t.Run("Returns an error if fails to load filters from db", func(t *testing.T) {
		orm := newMockORM(t)
		fs := newFilters(lggr, orm)
		orm.On("SelectFilters", mock.Anything).Return(nil, errors.New("db failed")).Once()
		err := fs.UnregisterFilter(tests.Context(t), "Filter")
		require.EqualError(t, err, "failed to load filters: failed to select filters from db: db failed")
	})
	t.Run("Noop if filter is not present", func(t *testing.T) {
		orm := newMockORM(t)
		fs := newFilters(lggr, orm)
		const filterName = "Filter"
		orm.On("SelectFilters", mock.Anything).Return(nil, nil).Once()
		err := fs.UnregisterFilter(tests.Context(t), filterName)
		require.NoError(t, err)
	})
	t.Run("Returns error if fails to mark filter as deleted", func(t *testing.T) {
		orm := newMockORM(t)
		fs := newFilters(lggr, orm)
		const filterName = "Filter"
		const id int64 = 10
		orm.On("SelectFilters", mock.Anything).Return([]Filter{{ID: id, Name: filterName}}, nil).Once()
		orm.On("MarkFilterDeleted", mock.Anything, id).Return(errors.New("db query failed")).Once()
		err := fs.UnregisterFilter(tests.Context(t), filterName)
		require.EqualError(t, err, "failed to mark filter deleted: db query failed")
	})
	t.Run("Happy path", func(t *testing.T) {
		orm := newMockORM(t)
		fs := newFilters(lggr, orm)
		const filterName = "Filter"
		const id int64 = 10
		orm.On("SelectFilters", mock.Anything).Return([]Filter{{ID: id, Name: filterName}}, nil).Once()
		orm.On("MarkFilterDeleted", mock.Anything, id).Return(nil).Once()
		err := fs.UnregisterFilter(tests.Context(t), filterName)
		require.NoError(t, err)
		require.Len(t, fs.filtersToDelete, 1)
		require.Len(t, fs.filtersToBackfill, 0)
		require.Len(t, fs.filtersByName, 0)
		require.Len(t, fs.filtersByAddress, 0)
	})
}

func TestFilters_PruneFilters(t *testing.T) {
	lggr := logger.Sugared(logger.Test(t))
	t.Run("Happy path", func(t *testing.T) {
		orm := newMockORM(t)
		fs := newFilters(lggr, orm)
		toDelete := Filter{
			ID:        1,
			Name:      "To delete",
			IsDeleted: true,
		}
		orm.On("SelectFilters", mock.Anything).Return([]Filter{
			toDelete,
			{
				ID:   2,
				Name: "To keep",
			},
		}, nil).Once()
		orm.On("DeleteFilters", mock.Anything, map[int64]Filter{toDelete.ID: toDelete}).Return(nil).Once()
		err := fs.PruneFilters(tests.Context(t))
		require.NoError(t, err)
		require.Len(t, fs.filtersToDelete, 0)
	})
	t.Run("If DB removal fails will add filters back into removal slice ", func(t *testing.T) {
		orm := newMockORM(t)
		fs := newFilters(lggr, orm)
		toDelete := Filter{
			ID:        1,
			Name:      "To delete",
			IsDeleted: true,
		}
		orm.On("SelectFilters", mock.Anything).Return([]Filter{
			toDelete,
			{
				ID:   2,
				Name: "To keep",
			},
		}, nil).Once()
		newToDelete := Filter{
			ID:   3,
			Name: "To delete 2",
		}
		orm.On("DeleteFilters", mock.Anything, map[int64]Filter{toDelete.ID: toDelete}).Return(errors.New("db failed")).Run(func(_ mock.Arguments) {
			orm.On("MarkFilterDeleted", mock.Anything, newToDelete.ID).Return(nil).Once()
			orm.On("InsertFilter", mock.Anything, mock.Anything).Return(newToDelete.ID, nil).Once()
			require.NoError(t, fs.RegisterFilter(tests.Context(t), newToDelete))
			require.NoError(t, fs.UnregisterFilter(tests.Context(t), newToDelete.Name))
		}).Once()
		err := fs.PruneFilters(tests.Context(t))
		require.EqualError(t, err, "failed to delete filters: db failed")
		require.Equal(t, fs.filtersToDelete, map[int64]Filter{newToDelete.ID: newToDelete, toDelete.ID: toDelete})
	})
}

func TestFilters_MatchingFilters(t *testing.T) {
	orm := newMockORM(t)
	lggr := logger.Sugared(logger.Test(t))
	expectedFilter1 := Filter{
		ID:       1,
		Name:     "expectedFilter1",
		Address:  newRandomPublicKey(t),
		EventSig: newRandomEventSignature(t),
	}
	expectedFilter2 := Filter{
		ID:       2,
		Name:     "expectedFilter2",
		Address:  expectedFilter1.Address,
		EventSig: expectedFilter1.EventSig,
	}
	sameAddress := Filter{
		ID:       3,
		Name:     "sameAddressWrongEventSig",
		Address:  expectedFilter1.Address,
		EventSig: newRandomEventSignature(t),
	}

	sameEventSig := Filter{
		ID:       4,
		Name:     "wrongAddressSameEventSig",
		Address:  newRandomPublicKey(t),
		EventSig: expectedFilter1.EventSig,
	}
	orm.On("SelectFilters", mock.Anything).Return([]Filter{expectedFilter1, expectedFilter2, sameAddress, sameEventSig}, nil).Once()
	filters := newFilters(lggr, orm)
	err := filters.LoadFilters(tests.Context(t))
	require.NoError(t, err)
	matchingFilters := slices.Collect(filters.MatchingFilters(expectedFilter1.Address, expectedFilter1.EventSig))
	require.Len(t, matchingFilters, 2)
	require.Contains(t, matchingFilters, expectedFilter1)
	require.Contains(t, matchingFilters, expectedFilter2)
	// if at least one key does not match - returns empty iterator
	require.Empty(t, slices.Collect(filters.MatchingFilters(newRandomPublicKey(t), expectedFilter1.EventSig)))
	require.Empty(t, slices.Collect(filters.MatchingFilters(expectedFilter1.Address, newRandomEventSignature(t))))
	require.Empty(t, slices.Collect(filters.MatchingFilters(newRandomPublicKey(t), newRandomEventSignature(t))))
}

func TestFilters_GetFiltersToBackfill(t *testing.T) {
	orm := newMockORM(t)
	lggr := logger.Sugared(logger.Test(t))
	backfilledFilter := Filter{
		ID:            1,
		Name:          "backfilled",
		StartingBlock: 100,
		IsBackfilled:  true,
	}
	notBackfilled := Filter{
		ID:            2,
		StartingBlock: 101,
		Name:          "notBackfilled",
	}
	orm.EXPECT().SelectFilters(mock.Anything).Return([]Filter{backfilledFilter, notBackfilled}, nil).Once()
	filters := newFilters(lggr, orm)
	err := filters.LoadFilters(tests.Context(t))
	require.NoError(t, err)
	// filters that were not backfilled are properly identified on load
	ensureInQueue := func(expectedFilters ...Filter) {
		filtersToBackfill := filters.GetFiltersToBackfill()
		require.Len(t, filtersToBackfill, len(expectedFilters))
		for _, expectedFilter := range expectedFilters {
			require.Contains(t, filtersToBackfill, expectedFilter)
		}
	}
	ensureInQueue(notBackfilled)
	// filter remains in queue if failed to mark as backfilled
	orm.EXPECT().MarkFilterBackfilled(mock.Anything, notBackfilled.ID).Return(errors.New("db call failed")).Once()
	err = filters.MarkFilterBackfilled(tests.Context(t), notBackfilled.ID)
	require.Error(t, err)
	ensureInQueue(notBackfilled)
	// filter is removed from queue, if marked as backfilled
	orm.EXPECT().MarkFilterBackfilled(mock.Anything, notBackfilled.ID).Return(nil).Once()
	err = filters.MarkFilterBackfilled(tests.Context(t), notBackfilled.ID)
	require.NoError(t, err)
	require.Empty(t, filters.GetFiltersToBackfill())
	// re adding identical filter won't trigger backfill
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).Return(backfilledFilter.ID, nil).Once()
	require.NoError(t, filters.RegisterFilter(tests.Context(t), backfilledFilter))
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).Return(notBackfilled.ID, nil).Once()
	require.NoError(t, filters.RegisterFilter(tests.Context(t), notBackfilled))
	require.Empty(t, filters.GetFiltersToBackfill())
	// older StartingBlock trigger backfill
	notBackfilled.StartingBlock = notBackfilled.StartingBlock - 1
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).Return(notBackfilled.ID, nil).Once()
	require.NoError(t, filters.RegisterFilter(tests.Context(t), notBackfilled))
	ensureInQueue(notBackfilled)
	// new filter is always added to the queue
	newFilter := Filter{Name: "new filter", ID: 3}
	orm.EXPECT().InsertFilter(mock.Anything, newFilter).Return(newFilter.ID, nil).Once()
	require.NoError(t, filters.RegisterFilter(tests.Context(t), newFilter))
	ensureInQueue(notBackfilled, newFilter)
}

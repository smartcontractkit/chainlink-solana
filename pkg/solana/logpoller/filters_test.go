package logpoller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	binary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	codecv2 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v2"
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
	addr := newRandomPublicKey(t)
	happyPath := decoderReadyTestFilter(t, 1, "Happy path", "TestEvent", addr)
	happyPath.IsBackfilled = true
	happyPath2 := decoderReadyTestFilter(t, 2, "Happy path 2", "TestItem", addr)
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

// decoderReadyTestFilter returns a filter LoadFilters can load (newDecoder succeeds).
// eventName must exist in codecv1.FetchLogpollerTypeTestIDL() (e.g. "TestEvent", "TestItem").
func decoderReadyTestFilter(t *testing.T, id int64, name, eventName string, address types.PublicKey) types.Filter {
	t.Helper()
	return types.Filter{
		ID:          id,
		Name:        name,
		Address:     address,
		EventName:   eventName,
		EventSig:    types.NewEventSignatureFromName(eventName),
		ContractIdl: codecv1.FetchLogpollerTypeTestIDL(),
		SubkeyPaths: [][]string{{"Field1"}},
	}
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
				addr := newRandomPublicKey(t)
				dbFilter := decoderReadyTestFilter(t, 1, filterName, "TestEvent", addr)
				orm.On("SelectFilters", mock.Anything).Return([]types.Filter{dbFilter}, nil).Once()
				orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()
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
		const eventName = "TestEvent"

		filter1 := decoderReadyTestFilter(t, 1, "existingFilter", eventName, addr)
		filter1.IncludeReverted = false
		filter1.IsBackfilled = true
		orm.EXPECT().SelectFilters(mock.Anything).Return(
			[]types.Filter{filter1}, nil).Once()
		filter2 := decoderReadyTestFilter(t, 0, "new filter", eventName, addr)
		filter2.IncludeReverted = true
		filter2.IsBackfilled = true
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
	t.Run("Leaves in-memory state unchanged when re-register insert fails", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		const filterName = "Filter"
		addr := newRandomPublicKey(t)
		registered := decoderReadyTestFilter(t, 100, filterName, "TestEvent", addr)
		orm.On("SelectFilters", mock.Anything).Return(nil, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()
		const filterID = int64(1)
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(filterID, nil).Once()
		err := fs.RegisterFilter(t.Context(), registered)
		require.NoError(t, err)
		want := *fs.filtersByID[filterID]
		requireIndexed(t, fs, want)

		updated := want
		updated.StartingBlock = 200
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(int64(0), errors.New("failed to insert")).Once()
		err = fs.RegisterFilter(t.Context(), updated)
		require.EqualError(t, err, "failed to insert filter: failed to insert")
		requireIndexed(t, fs, want)

		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(filterID, nil).Once()
		err = fs.RegisterFilter(t.Context(), updated)
		require.NoError(t, err)
		want.StartingBlock = 200
		requireIndexed(t, fs, want)
	})
	t.Run("Happy path", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		const filterName = "Filter"
		addr := newRandomPublicKey(t)
		filter := decoderReadyTestFilter(t, 0, filterName, "TestEvent", addr)
		orm.On("SelectFilters", mock.Anything).Return(nil, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(int64(0), errors.New("failed to insert")).Once()
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
	t.Run("Happy path versioned", func(t *testing.T) {
		// 1. Create decoder codecs for the same event using two different IDL formats
		// 2. Create encoded event data
		// 3. Decode subkey from event data using both codecs and verify the results are the same
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		orm.On("SelectFilters", mock.Anything).Return(nil, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()
		eventName := "TestEvent"

		// Test with codec v1 (CodecEventIdl)
		codecV1Filter := types.Filter{
			Name:        "codecV1Filter",
			ContractIdl: codecv1.FetchLogpollerTypeTestIDL(),
			EventName:   eventName,
		}
		const codecV1FilterID = int64(1)
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(codecV1FilterID, nil).Once()
		err := fs.RegisterFilter(t.Context(), codecV1Filter)
		require.NoError(t, err)

		// Test with codec v2 (Codecv2EventIdl)
		codecV2Filter := types.Filter{
			Name:        "codecV2Filter",
			ContractIdl: codecv2.FetchLogpollerTypeTestIDL(),
			EventName:   eventName,
		}
		const codecV2FilterID = int64(2)
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(codecV2FilterID, nil).Once()
		err = fs.RegisterFilter(t.Context(), codecV2Filter)
		require.NoError(t, err)

		// Verify both filters are registered
		require.Len(t, fs.filtersToBackfill, 2)
		require.Contains(t, fs.filtersToBackfill, codecV1FilterID)
		require.Contains(t, fs.filtersToBackfill, codecV2FilterID)

		// Test DecodeSubKey for both codec versions
		// Define test event structs matching the IDL field names
		type TestEvent struct {
			Field1 int64
		}

		// Create test data
		testValue := int64(12345)

		// Borsh encode for event data
		// Events use sha256("event:<EventName>")[:8] as discriminator
		discriminator := solcommoncodec.NewDiscriminatorHashPrefix(eventName, false)
		eventData := TestEvent{Field1: testValue}
		buf := new(bytes.Buffer)
		buf.Write(discriminator)
		require.NoError(t, binary.NewBorshEncoder(buf).Encode(eventData))
		encodedBytes := buf.Bytes()

		// Test DecodeSubKey for v1
		v1Result, err := fs.DecodeSubKey(t.Context(), lggr, encodedBytes, codecV1FilterID, []string{"Field1"})
		require.NoError(t, err)
		require.Equal(t, testValue, v1Result)

		// Test DecodeSubKey for v2
		v2Result, err := fs.DecodeSubKey(t.Context(), lggr, encodedBytes, codecV2FilterID, []string{"Field1"})
		require.NoError(t, err)
		require.Equal(t, testValue, v2Result)
	})
	t.Run("Can reregister after unregister", func(t *testing.T) {
		orm := mocks.NewMockORM(t)
		fs := newFilters(lggr, orm, nil)
		const filterName = "Filter"
		addr := newRandomPublicKey(t)
		registered := decoderReadyTestFilter(t, 0, filterName, "TestEvent", addr)
		orm.On("SelectFilters", mock.Anything).Return(nil, nil).Once()
		orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{}, nil).Once()
		const filterID = int64(10)
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(filterID, nil).Once()
		err := fs.RegisterFilter(t.Context(), registered)
		require.NoError(t, err)
		want := registered
		want.ID = filterID
		requireIndexed(t, fs, want)
		orm.On("MarkFilterDeleted", mock.Anything, filterID).Return(nil).Once()
		err = fs.UnregisterFilter(t.Context(), filterName)
		require.NoError(t, err)
		requireNoInIndices(t, fs, want)
		orm.On("InsertFilter", mock.Anything, mock.Anything).Return(filterID+1, nil).Once()
		err = fs.RegisterFilter(t.Context(), registered)
		require.NoError(t, err)
		require.Len(t, fs.filtersToDelete, 1)
		require.Equal(t, want, fs.filtersToDelete[filterID])
		require.Len(t, fs.filtersToBackfill, 1)
		require.Contains(t, fs.filtersToBackfill, filterID+1)
		want2 := registered
		want2.ID = filterID + 1
		requireIndexed(t, fs, want2)
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
		f := decoderReadyTestFilter(t, id, filterName, "TestEvent", newRandomPublicKey(t))
		orm.On("SelectFilters", mock.Anything).Return([]types.Filter{f}, nil).Once()
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
		f := decoderReadyTestFilter(t, id, filterName, "TestEvent", newRandomPublicKey(t))
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
		toKeep := decoderReadyTestFilter(t, 2, "To keep", "TestEvent", newRandomPublicKey(t))
		orm.On("SelectFilters", mock.Anything).Return([]types.Filter{
			toDelete,
			toKeep,
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
		toKeep2 := decoderReadyTestFilter(t, 2, "To keep", "TestItem", newRandomPublicKey(t))
		orm.On("SelectFilters", mock.Anything).Return([]types.Filter{
			toDelete,
			toKeep2,
		}, nil).Once()
		orm.EXPECT().SelectSeqNums(mock.Anything).Return(map[int64]int64{
			1: 18,
			2: 25,
		}, nil).Once()
		newToDelete := decoderReadyTestFilter(t, 3, "To delete 2", "TestEvent", newRandomPublicKey(t))
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
	addr := newRandomPublicKey(t)
	const sharedEvent = "TestEvent"
	expectedFilter1 := decoderReadyTestFilter(t, 1, "expectedFilter1", sharedEvent, addr)
	expectedFilter2 := decoderReadyTestFilter(t, 2, "expectedFilter2", sharedEvent, addr)
	sameAddress := decoderReadyTestFilter(t, 3, "sameAddressWrongEventSig", "TestItem", addr)

	sameEventSig := decoderReadyTestFilter(t, 4, "wrongAddressSameEventSig", sharedEvent, newRandomPublicKey(t))
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
	addr := newRandomPublicKey(t)
	backfilledFilter := decoderReadyTestFilter(t, 1, "backfilled", "TestEvent", addr)
	backfilledFilter.StartingBlock = 100
	backfilledFilter.IsBackfilled = true
	notBackfilled := decoderReadyTestFilter(t, 2, "notBackfilled", "TestItem", addr)
	notBackfilled.StartingBlock = 101
	orm.EXPECT().SelectFilters(mock.Anything).Return([]types.Filter{backfilledFilter, notBackfilled}, nil).Once()
	orm.EXPECT().SelectSeqNums(mock.Anything).Return(map[int64]int64{
		1: 18,
		2: 25,
	}, nil)
	filters := newFilters(lggr, orm, nil)
	err := filters.LoadFilters(t.Context())
	require.NoError(t, err)
	// filters that were not backfilled are properly identified on load
	ensureInQueue := func(targetBlock int64, expectedStartingBlock int64, expectedFilters ...types.Filter) {
		filtersToBackfill, actualStartingBlock := filters.GetFiltersToBackfill(targetBlock)
		require.Len(t, filtersToBackfill, len(expectedFilters))
		require.Equal(t, expectedStartingBlock, actualStartingBlock)
		for _, expectedFilter := range expectedFilters {
			require.Contains(t, filtersToBackfill, expectedFilter)
		}
	}
	ensureInQueue(110, 101, notBackfilled)
	// filter remains in queue if failed to mark as backfilled
	orm.EXPECT().MarkFilterBackfilled(mock.Anything, notBackfilled.ID).Return(errors.New("db call failed")).Once()
	err = filters.UpdateBackfillProgress(t.Context(), notBackfilled, 110, true)
	require.Error(t, err)
	ensureInQueue(110, 101, notBackfilled)
	// filter is removed from queue, if marked as backfilled
	orm.EXPECT().MarkFilterBackfilled(mock.Anything, notBackfilled.ID).Return(nil).Once()
	err = filters.UpdateBackfillProgress(t.Context(), notBackfilled, 110, true)
	require.NoError(t, err)
	requireEmptyBackfillQueue := func() {
		filterToBackfill, startingBlock := filters.GetFiltersToBackfill(110)
		require.Empty(t, filterToBackfill)
		require.Equal(t, int64(110), startingBlock)
	}
	requireEmptyBackfillQueue()
	// re adding identical filter won't trigger backfill
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).Return(backfilledFilter.ID, nil).Once()
	require.NoError(t, filters.RegisterFilter(t.Context(), backfilledFilter))
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).Return(notBackfilled.ID, nil).Once()
	require.NoError(t, filters.RegisterFilter(t.Context(), notBackfilled))
	requireEmptyBackfillQueue()
	// older StartingBlock trigger backfill
	notBackfilled.StartingBlock = notBackfilled.StartingBlock - 1
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).Return(notBackfilled.ID, nil).Once()
	require.NoError(t, filters.RegisterFilter(t.Context(), notBackfilled))
	ensureInQueue(110, 100, notBackfilled)
	// new filter is always added to the queue
	newAddr := newRandomPublicKey(t)
	newFilter := decoderReadyTestFilter(t, 0, "new filter", "TestEvent", newAddr)
	const newFilterID = int64(3)
	orm.EXPECT().InsertFilter(mock.Anything, newFilter).Return(newFilterID, nil).Once()
	require.NoError(t, filters.RegisterFilter(t.Context(), newFilter))
	newFilter.ID = newFilterID
	ensureInQueue(110, 100, notBackfilled, *filters.filtersByID[newFilterID])
	// update of the starting block via RegisterFilter between GetFiltersToBackfill and UpdateBackfillProgress prevents filter from being marked as backfilled and keeps it in the queue
	filtersToBackfill, startingBlock := filters.GetFiltersToBackfill(110)
	require.Len(t, filtersToBackfill, 2)
	require.Equal(t, int64(100), startingBlock)
	notBackfilled.StartingBlock = notBackfilled.StartingBlock - 1
	orm.EXPECT().InsertFilter(mock.Anything, notBackfilled).Return(notBackfilled.ID, nil).Once()
	require.NoError(t, filters.RegisterFilter(t.Context(), notBackfilled))
	orm.EXPECT().MarkFilterBackfilled(mock.Anything, newFilterID).Return(nil).Once()
	for _, filter := range filtersToBackfill {
		err = filters.UpdateBackfillProgress(t.Context(), filter, 110, true)
		require.NoError(t, err)
	}
	ensureInQueue(110, 99, notBackfilled)
	// replay between GetFiltersToBackfill and UpdateBackfillProgress prevents filter from being marked as backfilled and keeps it in the queue
	filtersToBackfill, startingBlock = filters.GetFiltersToBackfill(110)
	require.Len(t, filtersToBackfill, 1)
	require.Equal(t, int64(99), startingBlock)
	filters.UpdateStartingBlocks(98)
	for _, filter := range filtersToBackfill {
		err = filters.UpdateBackfillProgress(t.Context(), filter, 110, true)
		require.NoError(t, err)
	}
	// reflect new starting block and backfill status caused by UpdateStartingBlocks
	for _, filter := range []*types.Filter{&notBackfilled, &backfilledFilter, &newFilter} {
		filter.StartingBlock = 98
		filter.IsBackfilled = false
	}
	// all filters are now in the queue due to global starting block update
	ensureInQueue(110, 98, notBackfilled, backfilledFilter, newFilter)
	// partial backfill update doesn't remove filters from the queue
	for _, filter := range []types.Filter{notBackfilled, backfilledFilter, newFilter} {
		err = filters.UpdateBackfillProgress(t.Context(), filter, 109, false)
		require.NoError(t, err)
	}
	require.NoError(t, err)
	ensureInQueue(110, 109, notBackfilled, backfilledFilter, newFilter)
	// full backfill update removes filters from the queue
	for _, filter := range []types.Filter{notBackfilled, backfilledFilter, newFilter} {
		orm.EXPECT().MarkFilterBackfilled(mock.Anything, filter.ID).Return(nil).Once()
		err = filters.UpdateBackfillProgress(t.Context(), filter, 110, true)
		require.NoError(t, err)
	}
	filtersToBackfill, _ = filters.GetFiltersToBackfill(110)
	require.Empty(t, filtersToBackfill)
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
			result, err := solcommoncodec.ExtractField(&testStruct, strings.Split(c.Path, "."))
			if c.Result == nil {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.Result, result)
		})
	}
}

func TestFilters_RegisterFilter_PreservesSeqNumOnUpdate(t *testing.T) {
	lggr := logger.Sugared(logger.Test(t))
	orm := mocks.NewMockORM(t)
	fs := newFilters(lggr, orm, nil)

	const filterID = int64(1)
	const filterName = "Filter"
	addr := newRandomPublicKey(t)
	filter := decoderReadyTestFilter(t, filterID, filterName, "TestEvent", addr)

	orm.On("SelectFilters", mock.Anything).Return([]types.Filter{filter}, nil).Once()
	orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{filterID: 42}, nil).Once()
	require.NoError(t, fs.LoadFilters(t.Context()))

	filter.Retention = time.Hour
	orm.On("InsertFilter", mock.Anything, mock.Anything).Return(filterID, nil).Once()
	require.NoError(t, fs.RegisterFilter(t.Context(), filter))

	logs := []types.Log{{FilterID: filterID}}
	fs.StageSeqNums(logs)
	require.Equal(t, int64(43), logs[0].SequenceNum)
	fs.CommitSeqNums(logs)
	require.Equal(t, int64(43), fs.seqNums[filterID])
}

func TestFilters_StageSeqNums_Commit(t *testing.T) {
	fs := newFilters(logger.Sugared(logger.Test(t)), nil, nil)
	fs.seqNums = map[int64]int64{1: 10, 2: 20}

	logs := []types.Log{
		{FilterID: 1},
		{FilterID: 1},
		{FilterID: 2},
	}
	fs.StageSeqNums(logs)

	require.Equal(t, int64(11), logs[0].SequenceNum)
	require.Equal(t, int64(12), logs[1].SequenceNum)
	require.Equal(t, int64(21), logs[2].SequenceNum)
	require.Equal(t, int64(10), fs.seqNums[1], "in-memory state unchanged before commit")
	require.Equal(t, int64(20), fs.seqNums[2])

	fs.CommitSeqNums(logs)
	require.Equal(t, int64(12), fs.seqNums[1])
	require.Equal(t, int64(21), fs.seqNums[2])
}

func TestFilters_StageSeqNums_Commit_Concurrent(t *testing.T) {
	orm := mocks.NewMockORM(t)
	lggr := logger.Sugared(logger.Test(t))
	fs := newFilters(lggr, orm, nil)

	addr1 := newRandomPublicKey(t)
	addr2 := newRandomPublicKey(t)
	filter1 := decoderReadyTestFilter(t, 1, "filter1", "TestEvent", addr1)
	filter2 := decoderReadyTestFilter(t, 2, "filter2", "TestItem", addr2)
	orm.On("SelectFilters", mock.Anything).Return([]types.Filter{filter1, filter2}, nil).Once()
	orm.On("SelectSeqNums", mock.Anything).Return(map[int64]int64{1: 0, 2: 0}, nil).Once()

	err := fs.LoadFilters(t.Context())
	require.NoError(t, err)

	const numGoroutines = 50
	const incrementsPerGoroutine = 100

	seqNumsFilter1 := make(chan int64, numGoroutines*incrementsPerGoroutine)
	seqNumsFilter2 := make(chan int64, numGoroutines*incrementsPerGoroutine)

	var wg sync.WaitGroup
	// prod code calls Process() sequentially, so for this concurrent test we lock around the stage/commit calls to simulate prod behavior.
	var processMu sync.Mutex
	wg.Add(numGoroutines * 2)

	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range incrementsPerGoroutine {
				logs := []types.Log{{FilterID: filter1.ID}}
				processMu.Lock()
				fs.StageSeqNums(logs)
				fs.CommitSeqNums(logs)
				processMu.Unlock()
				seqNumsFilter1 <- logs[0].SequenceNum
			}
		}()
	}

	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range incrementsPerGoroutine {
				logs := []types.Log{{FilterID: filter2.ID}}
				processMu.Lock()
				fs.StageSeqNums(logs)
				fs.CommitSeqNums(logs)
				processMu.Unlock()
				seqNumsFilter2 <- logs[0].SequenceNum
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

	addr := newRandomPublicKey(t)
	orig0 := decoderReadyTestFilter(t, 1, "backfilled", "TestEvent", addr)
	orig0.StartingBlock = 29500
	orig0.IsBackfilled = true
	orig1 := decoderReadyTestFilter(t, 2, "notBackfilled", "TestItem", addr)
	orig1.StartingBlock = 52000
	origFilters := []types.Filter{orig0, orig1}
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
			filters.filtersToBackfill = map[int64]int64{ids[0]: 0}
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

		a1 := newRandomPublicKey(t)
		a2 := newRandomPublicKey(t)
		filter1 := decoderReadyTestFilter(t, 1, "filter1", "TestEvent", a1)
		filter2 := decoderReadyTestFilter(t, 2, "filter2", "TestItem", a2)

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

		activeFilter := decoderReadyTestFilter(t, 1, "activeFilter", "TestEvent", newRandomPublicKey(t))
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

		filter1 := decoderReadyTestFilter(t, 1, "filter1", "TestEvent", newRandomPublicKey(t))
		filter1.StartingBlock = 100

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

		filter1 := decoderReadyTestFilter(t, 1, "filter1", "TestEvent", newRandomPublicKey(t))

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

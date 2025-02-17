package logpoller

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil/sqltest"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

func TestLogPollerFilters(t *testing.T) {
	sqltest.SkipInMemory(t)
	t.Parallel()

	lggr := logger.Test(t)

	privateKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privateKey.PublicKey()
	t.Run("Ensure all fields are readable/writable", func(t *testing.T) {
		filters := []Filter{
			{
				Name:          "happy path",
				Address:       PublicKey(pubKey),
				EventName:     "event",
				EventSig:      EventSignature{1, 2, 3},
				StartingBlock: 1,
				SubkeyPaths:   SubKeyPaths([][]string{{"a", "b"}, {"c"}}),
				EventIdl: EventIdl{
					Event: codec.IdlEvent{
						Name:   "MyEvent",
						Fields: []codec.IdlEventField{{Name: "MyField", Type: codec.NewIdlStringType(codec.IdlTypeDuration), Index: true}},
					},
					Types: codec.IdlTypeDefSlice{
						{Name: "NilType", Type: codec.IdlTypeDefTy{Kind: codec.IdlTypeDefTyKindStruct, Fields: &codec.IdlTypeDefStruct{}}},
					},
				},
				Retention:   1000,
				MaxLogsKept: 3,
			},
			{
				Name:          "empty sub key paths",
				Address:       PublicKey(pubKey),
				EventName:     "event",
				EventSig:      EventSignature{1, 2, 3},
				StartingBlock: 1,
				SubkeyPaths:   SubKeyPaths([][]string{}),
				Retention:     1000,
				MaxLogsKept:   3,
			},
			{
				Name:          "nil sub key paths",
				Address:       PublicKey(pubKey),
				EventName:     "event",
				EventSig:      EventSignature{1, 2, 3},
				StartingBlock: 1,
				SubkeyPaths:   nil,
				Retention:     1000,
				MaxLogsKept:   3,
			},
		}

		for _, filter := range filters {
			t.Run("Read/write filter: "+filter.Name, func(t *testing.T) {
				ctx := tests.Context(t)
				chainID := uuid.NewString()
				dbx := sqltest.NewDB(t, sqltest.TestURL(t))
				orm := NewORM(chainID, dbx, lggr)
				id, err := orm.InsertFilter(ctx, filter)
				require.NoError(t, err)
				filter.ID = id
				dbFilter, err := orm.GetFilterByID(ctx, id)
				require.NoError(t, err)
				require.Equal(t, filter, dbFilter)

				exists, err := orm.HasFilter(ctx, dbFilter.Name)

				require.NoError(t, err)
				require.True(t, exists)

				dbFilters, err := orm.SelectFilters(ctx)
				require.NoError(t, err)
				i := slices.IndexFunc(dbFilters, func(f Filter) bool {
					return f.ID == id
				})
				require.NotEqual(t, -1, i, "Expected filter to be present in slice")
				require.Equal(t, filter, dbFilters[i])
			})
		}
	})
	t.Run("Updates non primary fields if name and chainID is not unique", func(t *testing.T) {
		chainID := uuid.NewString()
		dbx := sqltest.NewDB(t, sqltest.TestURL(t))
		orm := NewORM(chainID, dbx, lggr)
		filter := newRandomFilter(t)
		ctx := tests.Context(t)
		id, err := orm.InsertFilter(ctx, filter)
		require.NoError(t, err)
		filter.EventName = uuid.NewString()
		filter.StartingBlock++
		filter.Retention++
		filter.MaxLogsKept++
		id2, err := orm.InsertFilter(ctx, filter)
		require.NoError(t, err)
		require.Equal(t, id, id2)
		dbFilter, err := orm.GetFilterByID(ctx, id)
		require.NoError(t, err)
		filter.ID = id
		require.Equal(t, filter, dbFilter)
	})
	t.Run("Allows reuse name of a filter marked as deleted", func(t *testing.T) {
		chainID := uuid.NewString()
		dbx := sqltest.NewDB(t, sqltest.TestURL(t))
		orm := NewORM(chainID, dbx, lggr)
		filter := newRandomFilter(t)
		ctx := tests.Context(t)
		filterID, err := orm.InsertFilter(ctx, filter)
		require.NoError(t, err)
		// mark deleted
		err = orm.MarkFilterDeleted(ctx, filterID)
		require.NoError(t, err)
		// ensure marked as deleted
		dbFilter, err := orm.GetFilterByID(ctx, filterID)
		require.NoError(t, err)
		require.True(t, dbFilter.IsDeleted, "expected to be deleted")
		newFilterID, err := orm.InsertFilter(ctx, filter)
		require.NoError(t, err)
		require.NotEqual(t, newFilterID, filterID, "expected db to generate new filter as we can not be sure that new one matches the same logs")
	})
	t.Run("Allows reuse name for a filter with different chainID", func(t *testing.T) {
		dbx := sqltest.NewDB(t, sqltest.TestURL(t))
		orm1 := NewORM(uuid.NewString(), dbx, lggr)
		orm2 := NewORM(uuid.NewString(), dbx, lggr)
		filter := newRandomFilter(t)
		ctx := tests.Context(t)
		filterID1, err := orm1.InsertFilter(ctx, filter)
		require.NoError(t, err)
		filterID2, err := orm2.InsertFilter(ctx, filter)
		require.NoError(t, err)
		require.NotEqual(t, filterID1, filterID2)
	})
	t.Run("Deletes log on parent filter deletion", func(t *testing.T) {
		dbx := sqltest.NewDB(t, sqltest.TestURL(t))
		chainID := uuid.NewString()
		orm := NewORM(chainID, dbx, lggr)
		filter := newRandomFilter(t)
		ctx := tests.Context(t)
		filterID, err := orm.InsertFilter(ctx, filter)
		require.NoError(t, err)
		log := newRandomLog(t, filterID, chainID, "My Event")

		err = orm.InsertLogs(ctx, []Log{log})
		require.NoError(t, err)
		logs, err := orm.SelectLogs(ctx, 0, log.BlockNumber, log.Address, log.EventSig)
		require.NoError(t, err)
		require.Len(t, logs, 1)
		err = orm.MarkFilterDeleted(ctx, filterID)
		require.NoError(t, err)
		// logs are expected to be present in db even if filter was marked as deleted
		logs, err = orm.SelectLogs(ctx, 0, log.BlockNumber, log.Address, log.EventSig)
		require.NoError(t, err)
		require.Len(t, logs, 1)
		err = orm.DeleteFilter(ctx, filterID)
		require.NoError(t, err)
		logs, err = orm.SelectLogs(ctx, 0, log.BlockNumber, log.Address, log.EventSig)
		require.NoError(t, err)
		require.Len(t, logs, 0)
	})

	genEnsureIsBackfilled := func(ctx context.Context, orm *DSORM) func([]int64, bool) {
		return func(filterIDs []int64, expectedIsBackfilled bool) {
			for _, filterID := range filterIDs {
				filter, err := orm.GetFilterByID(ctx, filterID)
				require.NoError(t, err)
				require.Equal(t, expectedIsBackfilled, filter.IsBackfilled)
			}
		}
	}

	t.Run("MarkBackfilled updated corresponding field", func(t *testing.T) {
		dbx := sqltest.NewDB(t, sqltest.TestURL(t))
		chainID := uuid.NewString()
		orm := NewORM(chainID, dbx, lggr)

		filter := newRandomFilter(t)
		ctx := tests.Context(t)
		filter.IsBackfilled = true
		filterID, err := orm.InsertFilter(ctx, filter)
		filterIDs := []int64{filterID}
		require.NoError(t, err)

		ensureIsBackfilled := genEnsureIsBackfilled(ctx, orm)

		ensureIsBackfilled(filterIDs, true)
		// insert overrides
		filter.IsBackfilled = false
		_, err = orm.InsertFilter(ctx, filter)
		require.NoError(t, err)
		ensureIsBackfilled(filterIDs, false)
		// mark changes value to true
		err = orm.MarkFilterBackfilled(ctx, filterID)
		require.NoError(t, err)
		ensureIsBackfilled(filterIDs, true)
	})
}

func TestLogPollerLogs(t *testing.T) {
	sqltest.SkipInMemory(t)
	t.Parallel()

	lggr := logger.Test(t)
	chainID := uuid.NewString()
	dbx := sqltest.NewDB(t, sqltest.TestURL(t))
	orm := NewORM(chainID, dbx, lggr)

	ctx := tests.Context(t)
	// create filter as it's required for a log
	filterID, err := orm.InsertFilter(ctx, newRandomFilter(t))
	require.NoError(t, err)
	filterID2, err := orm.InsertFilter(ctx, newRandomFilter(t))
	require.NoError(t, err)
	log := newRandomLog(t, filterID, chainID, "My Event")
	log2 := newRandomLog(t, filterID2, chainID, "My Event")
	err = orm.InsertLogs(ctx, []Log{log, log2})
	require.NoError(t, err)
	// insert of the same Log should not produce two instances
	err = orm.InsertLogs(ctx, []Log{log})
	require.NoError(t, err)

	dbLogs, err := orm.SelectLogs(ctx, 0, 1000000, log.Address, log.EventSig)
	require.NoError(t, err)
	require.Len(t, dbLogs, 1)
	sanitize(&log, &dbLogs[0])
	require.Equal(t, log, dbLogs[0])

	dbLogs, err = orm.SelectLogs(ctx, 0, 1000000, log2.Address, log2.EventSig)
	require.NoError(t, err)
	require.Len(t, dbLogs, 1)
	sanitize(&log2, &dbLogs[0])
	require.Equal(t, log2, dbLogs[0])

	t.Run("SelectSequenceNums", func(t *testing.T) {
		seqNums, err := orm.SelectSeqNums(tests.Context(t))
		require.NoError(t, err)
		require.Len(t, seqNums, 2)
	})
}

func TestLogPoller_GetLatestBlock(t *testing.T) {
	t.Parallel()
	sqltest.SkipInMemory(t)
	lggr := logger.Test(t)
	dbx := sqltest.NewDB(t, sqltest.TestURL(t))

	createLogsForBlocks := func(ctx context.Context, orm *DSORM, blocks ...int64) {
		filterID, err := orm.InsertFilter(ctx, newRandomFilter(t))
		require.NoError(t, err)
		for _, block := range blocks {
			log := newRandomLog(t, filterID, orm.chainID, "My Event")
			log.BlockNumber = block
			err = orm.InsertLogs(ctx, []Log{log})
			require.NoError(t, err)
		}
	}
	ctx := tests.Context(t)
	orm1 := NewORM(uuid.NewString(), dbx, lggr)
	createLogsForBlocks(tests.Context(t), orm1, 10, 11, 12)
	orm2 := NewORM(uuid.NewString(), dbx, lggr)
	createLogsForBlocks(context.Background(), orm2, 100, 110, 120)
	latestBlockChain1, err := orm1.GetLatestBlock(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(12), latestBlockChain1)
	latestBlockChain2, err := orm2.GetLatestBlock(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(120), latestBlockChain2)
}

func newRandomFilter(t *testing.T) Filter {
	return Filter{
		Name:          uuid.NewString(),
		Address:       newRandomPublicKey(t),
		EventName:     "event",
		EventSig:      newRandomEventSignature(t),
		StartingBlock: 1,
		SubkeyPaths:   [][]string{{"a", "b"}, {"c"}},
		Retention:     1000,
		MaxLogsKept:   3,
	}
}

func TestFilteredLogs(t *testing.T) {
	sqltest.SkipInMemory(t)
	t.Parallel()
	lggr := logger.Test(t)
	dbx := sqltest.NewDB(t, sqltest.TestURL(t))
	orm := NewORM(chainID, dbx, lggr)
	ctx := tests.Context(t)

	tests := []struct {
		name     string
		input    []Log
		expected []Log
	}{
		{
			name: "simple, no duplicates",
			input: []Log{
				{BlockNumber: 1, LogIndex: 0},
				{BlockNumber: 2, LogIndex: 1},
				{BlockNumber: 2, LogIndex: 2},
				{BlockNumber: 3, LogIndex: 0},
			},
			expected: []Log{
				{BlockNumber: 1, LogIndex: 0},
				{BlockNumber: 2, LogIndex: 1},
				{BlockNumber: 2, LogIndex: 2},
				{BlockNumber: 3, LogIndex: 0},
			},
		},
		{
			name: "with duplicates",
			input: []Log{
				{BlockNumber: 1, LogIndex: 0}, // dup
				{BlockNumber: 2, LogIndex: 2}, // dup
				{BlockNumber: 3, LogIndex: 2},
			},
			expected: []Log{
				{BlockNumber: 1, LogIndex: 0},
				{BlockNumber: 2, LogIndex: 1},
				{BlockNumber: 2, LogIndex: 2},
				{BlockNumber: 3, LogIndex: 0},
				{BlockNumber: 3, LogIndex: 2},
			},
		},
	}

	filterID, err := orm.InsertFilter(ctx, newRandomFilter(t))
	require.NoError(t, err)

	blockTimestamp := time.Now().UTC()

	for _, tt := range tests {
		data := []byte("non-null data")
		for i := range tt.input {
			l := &tt.input[i]
			l.ChainID = chainID
			l.FilterID = filterID
			l.BlockTimestamp = blockTimestamp
			l.Data = data
		}
		for j := range tt.expected {
			l := &tt.expected[j]
			l.ChainID = chainID
			l.FilterID = filterID
			l.BlockTimestamp = blockTimestamp
			l.SubkeyValues = IndexedValues{}
			l.Data = data
		}
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, orm.InsertLogs(ctx, tt.input))
			logs, err := orm.FilteredLogs(ctx, nil, query.LimitAndSort{}, "")
			require.NoError(t, err)
			require.Len(t, logs, len(tt.expected))
			for i, log := range logs {
				sanitize(&tt.expected[i], &log)
				assert.Equal(t, tt.expected[i], log)
			}
		})
	}
}

func sanitize(expected, actual *Log) {
	// TODO: override ScanLocation with custom TimestamptzCodec?
	// See: https://github.com/jackc/pgx/issues/2117
	actual.CreatedAt = actual.CreatedAt.UTC().Round(0)
	actual.BlockTimestamp = actual.BlockTimestamp.UTC().Round(0)

	// fill in fields populated by db write itself
	expected.ID = actual.ID
	expected.CreatedAt = actual.CreatedAt
}

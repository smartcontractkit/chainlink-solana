//go:build db_tests

package logpoller

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"

	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil/pg"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

// NOTE: at the moment it's not possible to run all db tests at once. This issue will be addressed separately

func TestLogPollerFilters(t *testing.T) {
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
				EventIdl:      EventIdl{},
				SubkeyPaths:   SubkeyPaths([][]string{{"a", "b"}, {"c"}}),
				Retention:     1000,
				MaxLogsKept:   3,
			},
			{
				Name:          "empty sub key paths",
				Address:       PublicKey(pubKey),
				EventName:     "event",
				EventSig:      EventSignature{1, 2, 3},
				StartingBlock: 1,
				SubkeyPaths:   SubkeyPaths([][]string{}),
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
				dbx := pg.NewTestDB(t, pg.TestURL(t))
				orm := NewORM(chainID, dbx, lggr)
				id, err := orm.InsertFilter(ctx, filter)
				require.NoError(t, err)
				filter.ID = id
				dbFilter, err := orm.GetFilterByID(ctx, id)
				require.NoError(t, err)
				require.Equal(t, filter, dbFilter)
			})
		}
	})
	t.Run("Updates non primary fields if name and chainID is not unique", func(t *testing.T) {
		chainID := uuid.NewString()
		dbx := pg.NewTestDB(t, pg.TestURL(t))
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
		dbx := pg.NewTestDB(t, pg.TestURL(t))
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
		dbx := pg.NewTestDB(t, pg.TestURL(t))
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
		dbx := pg.NewTestDB(t, pg.TestURL(t))
		chainID := uuid.NewString()
		orm := NewORM(chainID, dbx, lggr)
		filter := newRandomFilter(t)
		ctx := tests.Context(t)
		filterID, err := orm.InsertFilter(ctx, filter)
		require.NoError(t, err)
		log := newRandomLog(t, filterID, chainID, "myEvent")
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
	t.Run("MarkBackfilled updated corresponding filed", func(t *testing.T) {
		dbx := pg.NewTestDB(t, pg.TestURL(t))
		chainID := uuid.NewString()
		orm := NewORM(chainID, dbx, lggr)

		filter := newRandomFilter(t)
		ctx := tests.Context(t)
		filter.IsBackfilled = true
		filterID, err := orm.InsertFilter(ctx, filter)
		require.NoError(t, err)
		ensureIsBackfilled := func(expectedIsBackfilled bool) {
			filter, err = orm.GetFilterByID(ctx, filterID)
			require.NoError(t, err)
			require.Equal(t, expectedIsBackfilled, filter.IsBackfilled)
		}
		ensureIsBackfilled(true)
		// insert overrides
		filter.IsBackfilled = false
		_, err = orm.InsertFilter(ctx, filter)
		require.NoError(t, err)
		ensureIsBackfilled(false)
		// mark changes value to true
		err = orm.MarkFilterBackfilled(ctx, filterID)
		require.NoError(t, err)
		ensureIsBackfilled(true)
	})
}

func TestLogPollerLogs(t *testing.T) {
	lggr := logger.Test(t)
	chainID := uuid.NewString()
	dbx := pg.NewTestDB(t, pg.TestURL(t))
	orm := NewORM(chainID, dbx, lggr)

	ctx := tests.Context(t)
	// create filter as it's required for a log
	filterID, err := orm.InsertFilter(ctx, newRandomFilter(t))
	filterID2, err := orm.InsertFilter(ctx, newRandomFilter(t))
	require.NoError(t, err)
	log := newRandomLog(t, filterID, chainID, "myEvent")
	log2 := newRandomLog(t, filterID2, chainID, "myEvent")
	err = orm.InsertLogs(ctx, []Log{log, log2})
	require.NoError(t, err)
	// insert of the same Log should not produce two instances
	err = orm.InsertLogs(ctx, []Log{log})
	require.NoError(t, err)

	dbLogs, err := orm.SelectLogs(ctx, 0, 1000000, log.Address, log.EventSig)
	require.NoError(t, err)
	require.Len(t, dbLogs, 1)
	log.ID = dbLogs[0].ID
	log.CreatedAt = dbLogs[0].CreatedAt
	require.Equal(t, log, dbLogs[0])

	dbLogs, err = orm.SelectLogs(ctx, 0, 1000000, log2.Address, log2.EventSig)
	require.NoError(t, err)
	require.Len(t, dbLogs, 1)
	log2.ID = dbLogs[0].ID
	log2.CreatedAt = dbLogs[0].CreatedAt
	require.Equal(t, log2, dbLogs[0])

	t.Run("SelectSequenceNums", func(t *testing.T) {
		seqNums, err := orm.SelectSeqNums(tests.Context(t))
		require.NoError(t, err)
		require.Len(t, seqNums, 2)
	})
}

func TestLogPoller_GetLatestBlock(t *testing.T) {
	lggr := logger.Test(t)
	dbx := pg.NewTestDB(t, pg.TestURL(t))

	createLogsForBlocks := func(ctx context.Context, orm *DSORM, blocks ...int64) {
		filterID, err := orm.InsertFilter(ctx, newRandomFilter(t))
		require.NoError(t, err)
		for _, block := range blocks {
			log := newRandomLog(t, filterID, orm.chainID, "myEvent")
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

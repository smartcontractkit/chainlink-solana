//go:build db_tests

package logpoller

import (
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil/pg"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

// NOTE: at the moment it's not possible to run all db tests at once. This issue will be addressed separately

func TestLogPollerFilters(t *testing.T) {
	lggr := logger.Test(t)
	chainID := uuid.NewString()
	dbx := pg.NewTestDB(t, pg.TestURL(t))
	orm := NewORM(chainID, dbx, lggr)

	privateKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privateKey.PublicKey()
	t.Run("Ensure all fields are readable/writable", func(t *testing.T) {
		filters := []Filter{
			{
				Name:          "happy path",
				Address:       PublicKey(pubKey),
				EventName:     "event",
				EventSig:      []byte{1, 2, 3},
				StartingBlock: 1,
				EventIDL:      "{}",
				SubkeyPaths:   SubkeyPaths([][]string{{"a", "b"}, {"c"}}),
				Retention:     1000,
				MaxLogsKept:   3,
			},
			{
				Name:          "empty sub key paths",
				Address:       PublicKey(pubKey),
				EventName:     "event",
				EventSig:      []byte{1, 2, 3},
				StartingBlock: 1,
				EventIDL:      "{}",
				SubkeyPaths:   SubkeyPaths([][]string{}),
				Retention:     1000,
				MaxLogsKept:   3,
			},
			{
				Name:          "nil sub key paths",
				Address:       PublicKey(pubKey),
				EventName:     "event",
				EventSig:      []byte{1, 2, 3},
				StartingBlock: 1,
				EventIDL:      "{}",
				SubkeyPaths:   nil,
				Retention:     1000,
				MaxLogsKept:   3,
			},
		}

		for _, filter := range filters {
			t.Run("Read/write filter: "+filter.Name, func(t *testing.T) {
				ctx := tests.Context(t)
				id, err := orm.InsertFilter(ctx, filter)
				require.NoError(t, err)
				filter.ID = id
				dbFilter, err := orm.GetFilterByID(ctx, id)
				require.NoError(t, err)
				require.Equal(t, filter, dbFilter)
			})
		}
	})
	t.Run("Returns and error if name is not unique", func(t *testing.T) {
		filter := newRandomFilter(t)
		ctx := tests.Context(t)
		_, err = orm.InsertFilter(ctx, filter)
		require.NoError(t, err)
		filter.EventSig = []byte(uuid.NewString())
		_, err = orm.InsertFilter(ctx, filter)
		require.EqualError(t, err, `ERROR: duplicate key value violates unique constraint "solana_log_poller_filter_name" (SQLSTATE 23505)`)
	})
}

func newRandomFilter(t *testing.T) Filter {
	privateKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privateKey.PublicKey()
	return Filter{
		Name:          uuid.NewString(),
		Address:       PublicKey(pubKey),
		EventName:     "event",
		EventSig:      []byte{1, 2, 3},
		StartingBlock: 1,
		EventIDL:      "{}",
		SubkeyPaths:   [][]string{{"a", "b"}, {"c"}},
		Retention:     1000,
		MaxLogsKept:   3,
	}
}

func TestLogPollerLogs(t *testing.T) {
	lggr := logger.Test(t)
	chainID := uuid.NewString()
	dbx := pg.NewTestDB(t, pg.TestURL(t))
	orm := NewORM(chainID, dbx, lggr)

	privateKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privateKey.PublicKey()

	ctx := tests.Context(t)
	// create filter as it's required for a log
	filterID, err := orm.InsertFilter(ctx, Filter{
		Name:          "awesome filter",
		Address:       PublicKey(pubKey),
		EventName:     "event",
		EventSig:      []byte{1, 2, 3},
		StartingBlock: 1,
		EventIDL:      "{}",
		SubkeyPaths:   [][]string{{"a", "b"}, {"c"}},
		Retention:     1000,
		MaxLogsKept:   3,
	})
	require.NoError(t, err)
	data := []byte("solana is fun")
	signature, err := privateKey.Sign(data)
	require.NoError(t, err)
	log := Log{
		FilterID:       filterID,
		ChainID:        chainID,
		LogIndex:       1,
		BlockHash:      Hash(pubKey),
		BlockNumber:    10,
		BlockTimestamp: time.Unix(1731590113, 0),
		Address:        PublicKey(pubKey),
		EventSig:       []byte{3, 2, 1},
		SubkeyValues:   pq.ByteaArray([][]byte{{3, 2, 1}, {1}, {1, 2}, pubKey.Bytes()}),
		TxHash:         Signature(signature),
		Data:           data,
	}
	err = orm.InsertLogs(ctx, []Log{log})
	require.NoError(t, err)
	// insert of the same Log should not produce two instances
	err = orm.InsertLogs(ctx, []Log{log})
	require.NoError(t, err)
	dbLogs, err := orm.SelectLogs(ctx, 0, 100, log.Address, log.EventSig)
	require.NoError(t, err)
	require.Len(t, dbLogs, 1)
	log.ID = dbLogs[0].ID
	log.CreatedAt = dbLogs[0].CreatedAt
	require.Equal(t, log, dbLogs[0])
}

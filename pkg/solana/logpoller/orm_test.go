//go:build db_tests

package logpoller

import (
	"os"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/lib/pq"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/pg"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/stretchr/testify/require"
)

// NOTE: at the moment it's not possible to run all db tests at once. This issue will be addressed separately

func TestLogPollerFilters(t *testing.T) {
	lggr := logger.Test(t)
	dbURL, ok := os.LookupEnv("CL_DATABASE_URL")
	require.True(t, ok, "CL_DATABASE_URL must be set")
	chainID := uuid.NewString()
	dbx := pg.NewSqlxDB(t, dbURL)
	orm := NewORM(chainID, dbx, lggr)

	privateKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privateKey.PublicKey()
	filters := []Filter{
		{
			Name:          "happy path",
			Address:       PublicKey(pubKey),
			EventName:     "event",
			EventSig:      []byte{1, 2, 3},
			StartingBlock: 1,
			EventIDL:      "{}",
			SubKeyPaths:   SubKeyPaths([][]string{{"a", "b"}, {"c"}}),
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
			SubKeyPaths:   SubKeyPaths([][]string{}),
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
			SubKeyPaths:   nil,
			Retention:     1000,
			MaxLogsKept:   3,
		},
	}

	for _, filter := range filters {
		t.Run("Save filter: "+filter.Name, func(t *testing.T) {
			ctx := tests.Context(t)
			id, err := orm.InsertFilter(ctx, filter)
			require.NoError(t, err)
			filter.ID = id
			dbFilter, err := orm.GetFilterByID(ctx, id)
			require.NoError(t, err)
			require.Equal(t, filter, dbFilter)

			// subsequent insert of the same filter won't produce new db row
			secondID, err := orm.InsertFilter(ctx, filter)
			require.NoError(t, err)
			require.Equal(t, secondID, id)
		})
	}
}

func TestLogPollerLogs(t *testing.T) {
	lggr := logger.Test(t)
	dbURL, ok := os.LookupEnv("CL_DATABASE_URL")
	require.True(t, ok, "CL_DATABASE_URL must be set")
	chainID := uuid.NewString()
	dbx := pg.NewSqlxDB(t, dbURL)
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
		SubKeyPaths:   [][]string{{"a", "b"}, {"c"}},
		Retention:     1000,
		MaxLogsKept:   3,
	})
	require.NoError(t, err)
	data := []byte("solana is fun")
	signature, err := privateKey.Sign(data)
	require.NoError(t, err)
	log := Log{
		FilterId:       filterID,
		ChainId:        chainID,
		LogIndex:       1,
		BlockHash:      Hash(pubKey),
		BlockNumber:    10,
		BLockTimestamp: time.Now(),
		Address:        PublicKey(pubKey),
		EventSig:       []byte{3, 2, 1},
		SubKeyValues:   pq.ByteaArray([][]byte{{3, 2, 1}, {1}, {1, 2}, pubKey.Bytes()}),
		TxHash:         Signature(signature),
		Data:           data,
	}
	err = orm.InsertLogs(ctx, []Log{log})
	require.NoError(t, err)
	dbLogs, err := orm.SelectLogs(ctx, 0, 100, log.Address, log.EventSig)
	require.NoError(t, err)
	require.Len(t, dbLogs, 1)
	require.Equal(t, log, dbLogs[0])
}

package logpoller

import (
	"context"
	"encoding/base64"
	"math/rand"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commonutils "github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/utils"
)

func TestProcess(t *testing.T) {
	ctx := tests.Context(t)

	addr := newRandomPublicKey(t)
	eventName := "myEvent"
	eventSig := utils.Discriminator("event", eventName)

	filterID := rand.Int63()
	chainID := uuid.NewString()

	txIndex := int(rand.Int31())
	txLogIndex := uint(rand.Uint32())

	expectedLog := newRandomLog(t, filterID, chainID, eventName)

	expectedLog.LogIndex = makeLogIndex(txIndex, txLogIndex)

	orm := newMockORM(t)
	cl := clientmocks.NewReaderWriter(t)
	loader := commonutils.NewLazyLoad(func() (client.Reader, error) { return cl, nil })
	lggr := logger.Sugared(logger.Test(t))
	lp := New(lggr, orm, loader)

	filter := Filter{
		Name:     "test filter",
		Address:  addr,
		EventSig: eventSig,
	}
	orm.EXPECT().SelectFilters(mock.Anything).Return([]Filter{filter}, nil)
	orm.EXPECT().SelectSeqNums(mock.Anything).Return(map[int64]int64{}, nil)
	orm.EXPECT().InsertFilter(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, f Filter) (int64, error) {
		require.Equal(t, f, filter)
		return filterID, nil
	}).Once()

	orm.EXPECT().InsertLogs(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, logs []Log) error {
		require.Len(t, logs, 1)
		log := logs[0]
		assert.Equal(t, log, expectedLog)
		return nil
	})
	lp.RegisterFilter(ctx, filter)

	event := struct {
		a int
		b string
	}{55, "hello"}

	data, err := bin.MarshalBorsh(&event)
	require.NoError(t, err)
	data = append(eventSig[:], data...)

	require.NoError(t, err)

	ev := ProgramEvent{
		Program: addr.ToSolana().String(),
		Prefix:  ">",
		BlockData: BlockData{
			SlotNumber:          3,
			BlockHeight:         5,
			BlockHash:           solana.HashFromBytes([]byte{1, 2, 3}),
			BlockTime:           solana.UnixTimeSeconds(expectedLog.BlockTimestamp.Unix()),
			TransactionHash:     expectedLog.TxHash.ToSolana(),
			TransactionIndex:    txIndex,
			TransactionLogIndex: txLogIndex,
		},
		Data: base64.StdEncoding.EncodeToString(data),
	}
	err = lp.Process(ev)
	require.NoError(t, err)

	orm.EXPECT().MarkFilterDeleted(mock.Anything, mock.Anything).Return(nil).Once()
	lp.UnregisterFilter(ctx, filter.Name)
}

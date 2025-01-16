package logpoller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

func TestProcess(t *testing.T) {
	ctx := tests.Context(t)

	addr := newRandomPublicKey(t)
	eventName := "myEvent"
	eventSig := Discriminator("event", eventName)
	event := struct {
		A int64
		B string
	}{55, "hello"}
	subkeyValA, err := NewIndexedValue(event.A)
	require.NoError(t, err)
	subkeyValB, err := NewIndexedValue(event.B)
	require.NoError(t, err)

	filterID := rand.Int63()
	chainID := uuid.NewString()

	txIndex := int(rand.Int31())
	txLogIndex := uint(rand.Uint32())

	expectedLog := newRandomLog(t, filterID, chainID, eventName)
	expectedLog.Address = addr
	expectedLog.LogIndex, err = makeLogIndex(txIndex, txLogIndex)
	require.NoError(t, err)
	expectedLog.SequenceNum = 1
	expectedLog.SubkeyValues = []IndexedValue{subkeyValA, subkeyValB}

	expectedLog.Data, err = bin.MarshalBorsh(&event)
	require.NoError(t, err)

	ev := ProgramEvent{
		Program: addr.ToSolana().String(),
		BlockData: BlockData{
			SlotNumber:          uint64(expectedLog.BlockNumber),
			BlockHeight:         3,
			BlockHash:           expectedLog.BlockHash.ToSolana(),
			BlockTime:           solana.UnixTimeSeconds(expectedLog.BlockTimestamp.Unix()),
			TransactionHash:     expectedLog.TxHash.ToSolana(),
			TransactionIndex:    txIndex,
			TransactionLogIndex: txLogIndex,
		},
		Data: base64.StdEncoding.EncodeToString(append(eventSig[:], expectedLog.Data...)),
	}

	orm := newMockORM(t)
	cl := clientmocks.NewReaderWriter(t)
	lggr := logger.Sugared(logger.Test(t))
	lp := New(lggr, orm, cl)

	var idlTypeInt64 codec.IdlType
	var idlTypeString codec.IdlType

	err = json.Unmarshal([]byte("\"i64\""), &idlTypeInt64)
	require.NoError(t, err)
	err = json.Unmarshal([]byte("\"string\""), &idlTypeString)
	require.NoError(t, err)

	idl := EventIdl{
		codec.IdlEvent{
			Name: "myEvent",
			Fields: []codec.IdlEventField{{
				Name: "A",
				Type: idlTypeInt64,
			}, {
				Name: "B",
				Type: idlTypeString,
			}},
		},
		[]codec.IdlTypeDef{},
	}

	filter := Filter{
		Name:        "test filter",
		EventName:   eventName,
		Address:     addr,
		EventSig:    eventSig,
		EventIdl:    idl,
		SubkeyPaths: [][]string{{"A"}, {"B"}},
	}
	orm.EXPECT().SelectFilters(mock.Anything).Return([]Filter{filter}, nil).Once()
	orm.EXPECT().SelectSeqNums(mock.Anything).Return(map[int64]int64{}, nil).Once()
	orm.EXPECT().ChainID().Return(chainID).Once()
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
	err = lp.RegisterFilter(ctx, filter)
	require.NoError(t, err)

	err = lp.Process(ev)
	require.NoError(t, err)

	orm.EXPECT().MarkFilterDeleted(mock.Anything, mock.Anything).Return(nil).Once()
	err = lp.UnregisterFilter(ctx, filter.Name)
	require.NoError(t, err)
}

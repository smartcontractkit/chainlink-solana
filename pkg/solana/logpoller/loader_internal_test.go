package logpoller

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
)

func TestScheduleBlocksFetching_EmptySlots(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	client := mocks.NewRPCClient(t)
	metrics, err := NewSolLpMetrics(t.Name())
	require.NoError(t, err)

	collector := NewEncodedLogCollector(client, logger.Test(t), t.Name(), metrics, nil, 10)
	require.NoError(t, collector.Start(ctx))
	t.Cleanup(func() { require.NoError(t, collector.Close()) })

	ch, err := collector.scheduleBlocksFetching(ctx, nil)
	require.NoError(t, err)

	_, ok := <-ch
	require.False(t, ok, "blocks channel should be closed when no slots are provided")
}

func TestScheduleBlocksFetching_HappyPath(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	client := mocks.NewRPCClient(t)
	metrics, err := NewSolLpMetrics(t.Name())
	require.NoError(t, err)

	const batchSize = 1
	slots := []uint64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109}

	collector := NewEncodedLogCollector(client, logger.Test(t), t.Name(), metrics, nil, batchSize)
	require.NoError(t, collector.Start(ctx))
	t.Cleanup(func() { require.NoError(t, collector.Close()) })

	blockTime := solana.UnixTimeSeconds(128)
	client.EXPECT().
		GetBlockWithOpts(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, slot uint64, _ *rpc.GetBlockOpts) (*rpc.GetBlockResult, error) {
			tx := solana.Transaction{Signatures: []solana.Signature{{1}}}
			txB, txErr := tx.MarshalBinary()
			require.NoError(t, txErr)
			return &rpc.GetBlockResult{
				BlockHeight: &slot,
				BlockTime:   &blockTime,
				Blockhash:   solana.Hash{1, 2, 3},
				Transactions: []rpc.TransactionWithMeta{{
					Transaction: rpc.DataBytesOrJSONFromBytes(txB),
					Meta:        &rpc.TransactionMeta{LogMessages: []string{}},
				}},
			}, nil
		}).
		Times(len(slots))

	ch, err := collector.scheduleBlocksFetching(ctx, slots)
	require.NoError(t, err)
	expectedSlots := make(map[uint64]struct{})
	for _, slot := range slots {
		expectedSlots[slot] = struct{}{}
	}

	prev := uint64(0)
	for block := range ch {
		if prev != 0 {
			require.Equal(t, uint64(1), block.SlotNumber-prev, "expected blocks to be received in consecutive order since batch size is 1")
		}
		require.Contains(t, expectedSlots, block.SlotNumber, "received block for unexpected slot")
		delete(expectedSlots, block.SlotNumber)
	}
	require.Empty(t, expectedSlots)
}

package chainaccessor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"

	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

func Test_deduplicateEvents(t *testing.T) {
	accessor := &SolanaAccessor{lggr: logger.Test(t)}

	t.Run("empty events", func(t *testing.T) {
		result, err := accessor.deduplicateEvents(nil, nil)
		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("no duplicates", func(t *testing.T) {
		events := []*ccipocr3.SendRequestedEvent{
			{SequenceNumber: 1},
			{SequenceNumber: 2},
			{SequenceNumber: 3},
		}
		logs := []logpollertypes.Log{
			{LogIndex: 1},
			{LogIndex: 2},
			{LogIndex: 3},
		}
		result, err := accessor.deduplicateEvents(events, logs)
		require.NoError(t, err)
		require.Len(t, result, 3)
	})

	t.Run("duplicates prefer higher LogIndex", func(t *testing.T) {
		events := []*ccipocr3.SendRequestedEvent{
			{SequenceNumber: 1},
			{SequenceNumber: 1}, // duplicate
			{SequenceNumber: 2},
			{SequenceNumber: 2}, // duplicate
		}
		logs := []logpollertypes.Log{
			{LogIndex: 1},
			{LogIndex: 2}, // higher, should be kept
			{LogIndex: 5}, // higher, should be kept
			{LogIndex: 3},
		}
		result, err := accessor.deduplicateEvents(events, logs)
		require.NoError(t, err)
		require.Len(t, result, 2)
		require.Equal(t, ccipocr3.SeqNum(1), result[0].SequenceNumber)
		require.Equal(t, ccipocr3.SeqNum(2), result[1].SequenceNumber)
	})

	t.Run("result sorted by SequenceNumber ascending", func(t *testing.T) {
		events := []*ccipocr3.SendRequestedEvent{
			{SequenceNumber: 3},
			{SequenceNumber: 1},
			{SequenceNumber: 2},
		}
		logs := []logpollertypes.Log{
			{LogIndex: 1},
			{LogIndex: 2},
			{LogIndex: 3},
		}
		result, err := accessor.deduplicateEvents(events, logs)
		require.NoError(t, err)
		require.Len(t, result, 3)
		require.Equal(t, ccipocr3.SeqNum(1), result[0].SequenceNumber)
		require.Equal(t, ccipocr3.SeqNum(2), result[1].SequenceNumber)
		require.Equal(t, ccipocr3.SeqNum(3), result[2].SequenceNumber)
	})
}

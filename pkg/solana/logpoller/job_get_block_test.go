package logpoller

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

type outcomeDependantTestMetric struct {
	succeeded float64
	reverted  float64
}
type solLpPromTest struct {
	id                 string
	txsTruncated       outcomeDependantTestMetric
	txsLogParsingError outcomeDependantTestMetric
}

func (p solLpPromTest) assertEqual(t *testing.T) {
	assert.InDelta(t, p.txsTruncated.succeeded, testutil.ToFloat64(promSolLp.txsTruncated.succeeded.WithLabelValues(p.id)), 0.0001, "mismatch: truncated succeeded")
	assert.InDelta(t, p.txsTruncated.reverted, testutil.ToFloat64(promSolLp.txsTruncated.reverted.WithLabelValues(p.id)), 0.0001, "mismatch: truncated reverted")
	assert.InDelta(t, p.txsLogParsingError.succeeded, testutil.ToFloat64(promSolLp.txsLogParsingError.succeeded.WithLabelValues(p.id)), 0.0001, "mismatch: log parsing error succeeded")
	assert.InDelta(t, p.txsLogParsingError.reverted, testutil.ToFloat64(promSolLp.txsLogParsingError.reverted.WithLabelValues(p.id)), 0.0001, "mismatch: log parsing error reverted")
}

// resetPromMetricsForLabel clears the prometheus counters for the given label
// to avoid counter accumulation across test runs when using -race or -count flags
func resetPromMetricsForLabel(label string) {
	promSolLp.txsTruncated.succeeded.DeleteLabelValues(label)
	promSolLp.txsTruncated.reverted.DeleteLabelValues(label)
	promSolLp.txsLogParsingError.succeeded.DeleteLabelValues(label)
	promSolLp.txsLogParsingError.reverted.DeleteLabelValues(label)
}

func TestGetBlockJob(t *testing.T) {
	const slotNumber = uint64(42)

	t.Run("String contains slot number", func(t *testing.T) {
		lggr := logger.Sugared(logger.Test(t))
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		job := newGetBlockJob(nil, nil, nil, lggr, slotNumber, metrics, nil)
		require.Equal(t, "getBlock for slotNumber: 42", job.String())
	})
	t.Run("Error if fails to get block", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		expectedError := errors.New("rpc failed")
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(nil, expectedError).Once()
		client.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(0, nil).Once()
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		job := newGetBlockJob(nil, client, make(chan types.Block), lggr, slotNumber, metrics, nil)
		err = job.Run(t.Context())
		require.ErrorIs(t, err, expectedError)
	})
	t.Run("Success if fails to get block because of pruning", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		expectedError := errors.New("rpc failed")
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(nil, expectedError).Once()
		client.EXPECT().GetFirstAvailableBlock(mock.Anything).Return(slotNumber+1, nil).Once()
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		job := newGetBlockJob(nil, client, make(chan types.Block, 1), lggr, slotNumber, metrics, nil)
		err = job.Run(t.Context())
		require.NoError(t, err)
		result := <-job.blocks
		require.Equal(t, types.Block{
			SlotNumber: slotNumber,
			BlockHash:  nil,
			Events:     []types.ProgramEvent{},
		}, result)
		select {
		case <-job.Done():
		default:
			t.Fatal("expected job to be done")
		}
	})
	t.Run("Error if block height is not present", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		block := rpc.GetBlockResult{}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		job := newGetBlockJob(nil, client, make(chan types.Block), lggr, slotNumber, metrics, nil)
		err = job.Run(t.Context())
		require.ErrorContains(t, err, "block at slot 42 returned from rpc is missing block number")
	})
	t.Run("Error if block time is not present", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))

		block := rpc.GetBlockResult{BlockHeight: ptr(uint64(10))}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		job := newGetBlockJob(nil, client, make(chan types.Block), lggr, slotNumber, metrics, nil)
		err = job.Run(t.Context())
		require.ErrorContains(t, err, "block at slot 42 returned from rpc is missing block time")
	})
	t.Run("Error if transaction field is not present", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		block := rpc.GetBlockResult{BlockHeight: ptr(uint64(10)), BlockTime: ptr(solana.UnixTimeSeconds(10)), Transactions: []rpc.TransactionWithMeta{{Transaction: nil}}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		job := newGetBlockJob(nil, client, make(chan types.Block), lggr, slotNumber, metrics, nil)
		err = job.Run(t.Context())
		require.ErrorContains(t, err, "failed to parse transaction 0 in slot 42: missing transaction field")
	})
	t.Run("Error if fails to get transaction", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		block := rpc.GetBlockResult{BlockHeight: ptr(uint64(10)), BlockTime: ptr(solana.UnixTimeSeconds(10)), Transactions: []rpc.TransactionWithMeta{{Transaction: rpc.DataBytesOrJSONFromBytes([]byte("{"))}}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		job := newGetBlockJob(nil, client, make(chan types.Block), lggr, slotNumber, metrics, nil)
		err = job.Run(t.Context())
		require.ErrorContains(t, err, "failed to parse transaction 0 in slot 42")
	})
	t.Run("Error if Tx has no signatures", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		tx := solana.Transaction{}
		txB, err := tx.MarshalBinary()
		require.NoError(t, err)
		block := rpc.GetBlockResult{BlockHeight: ptr(uint64(10)), BlockTime: ptr(solana.UnixTimeSeconds(10)), Transactions: []rpc.TransactionWithMeta{{Transaction: rpc.DataBytesOrJSONFromBytes(txB)}}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		job := newGetBlockJob(nil, client, make(chan types.Block), lggr, slotNumber, metrics, nil)
		err = job.Run(t.Context())
		require.ErrorContains(t, err, "expected all transactions to have at least one signature 0 in slot 42")
	})
	t.Run("Error if Tx has no Meta", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		tx := solana.Transaction{Signatures: []solana.Signature{{1, 2, 3}}}
		txB, err := tx.MarshalBinary()
		require.NoError(t, err)
		block := rpc.GetBlockResult{BlockHeight: ptr(uint64(10)), BlockTime: ptr(solana.UnixTimeSeconds(10)), Transactions: []rpc.TransactionWithMeta{{Transaction: rpc.DataBytesOrJSONFromBytes(txB)}}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		job := newGetBlockJob(nil, client, make(chan types.Block), lggr, slotNumber, metrics, nil)
		err = job.Run(t.Context())
		require.ErrorContains(t, err, "expected transaction to have meta. signature: 2AnZxg8HN2sGa7GC7iWGDgpXbEasqXQNEumCjvHUFDcBnfRKAdaN3SvKLhbQwheN15xDkL5D5mdX21A5gH1MdYB; slot: 42; idx: 0")
	})
	t.Run("Can abort even if no one waits for result", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		tx := solana.Transaction{Signatures: make([]solana.Signature, 1)}
		txB, err := tx.MarshalBinary()
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		block := rpc.GetBlockResult{BlockHeight: ptr(uint64(10)), BlockTime: ptr(solana.UnixTimeSeconds(10)), Transactions: []rpc.TransactionWithMeta{{Transaction: rpc.DataBytesOrJSONFromBytes(txB), Meta: &rpc.TransactionMeta{}}}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).RunAndReturn(func(ctx context.Context, u uint64, opts *rpc.GetBlockOpts) (*rpc.GetBlockResult, error) {
			cancel()
			return &block, nil
		}).Once()
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		job := newGetBlockJob(ctx.Done(), client, make(chan types.Block), lggr, slotNumber, metrics, nil)
		err = job.Run(ctx)
		require.ErrorIs(t, err, context.Canceled)
		select {
		case <-job.Done():
			require.Fail(t, "expected done channel to be open as job was aborted")
		default:
		}
	})
	t.Run("Happy path", func(t *testing.T) {
		resetPromMetricsForLabel(t.Name()) // Reset counters to avoid accumulation across test runs
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		tx1Signature := solana.Signature{4, 5, 6}
		tx2Signature := solana.Signature{7, 8, 9}
		txSigToDataBytes := func(sig solana.Signature) *rpc.DataBytesOrJSON {
			tx := solana.Transaction{Signatures: []solana.Signature{sig}}
			binary, err := tx.MarshalBinary()
			require.NoError(t, err)
			return rpc.DataBytesOrJSONFromBytes(binary)
		}
		txWithMeta1 := rpc.TransactionWithMeta{Transaction: txSigToDataBytes(tx1Signature), Meta: &rpc.TransactionMeta{LogMessages: []string{"log1", "log2"}}}
		txWithMeta2 := rpc.TransactionWithMeta{Transaction: txSigToDataBytes(tx2Signature), Meta: &rpc.TransactionMeta{LogMessages: []string{"log3"}}}
		// tx3 must be ignored due to error
		txWithMeta3 := rpc.TransactionWithMeta{Transaction: txSigToDataBytes(solana.Signature{10, 11}), Meta: &rpc.TransactionMeta{LogMessages: []string{"log4", "Log truncated"}, Err: errors.New("some error")}}
		height := uint64(41)
		blockTime := solana.UnixTimeSeconds(128)
		block := rpc.GetBlockResult{BlockHeight: &height, BlockTime: ptr(blockTime), Blockhash: solana.Hash{1, 2, 3}, Transactions: []rpc.TransactionWithMeta{txWithMeta1, txWithMeta2, txWithMeta3}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		job := newGetBlockJob(nil, client, make(chan types.Block, 1), lggr, slotNumber, metrics, nil)
		job.parseProgramLogs = func(logs []string) ([]types.ProgramOutput, error) {
			result := types.ProgramOutput{
				Program: "myProgram",
			}
			for _, l := range logs {
				if l == "Log truncated" {
					result.Truncated = true
					continue
				}
				result.Events = append(result.Events, types.ProgramEvent{Data: l, Program: "myProgram"})
			}
			return []types.ProgramOutput{result}, nil
		}
		err = job.Run(t.Context())
		require.NoError(t, err)
		result := <-job.blocks
		require.Equal(t, types.Block{
			SlotNumber: slotNumber,
			BlockHash:  &block.Blockhash,
			Events: []types.ProgramEvent{
				{
					BlockData: types.BlockData{
						SlotNumber:          slotNumber,
						BlockHeight:         height,
						BlockHash:           block.Blockhash,
						TransactionHash:     tx1Signature,
						TransactionLogIndex: 0,
						TransactionIndex:    0,
						BlockTime:           blockTime,
					},
					Program: "myProgram",
					Data:    "log1",
				},
				{
					BlockData: types.BlockData{
						SlotNumber:          slotNumber,
						BlockHeight:         height,
						BlockHash:           block.Blockhash,
						TransactionHash:     tx1Signature,
						TransactionLogIndex: 1,
						TransactionIndex:    0,
						BlockTime:           blockTime,
					},
					Program: "myProgram",
					Data:    "log2",
				},
				{
					BlockData: types.BlockData{
						SlotNumber:          slotNumber,
						BlockHeight:         height,
						BlockHash:           block.Blockhash,
						TransactionHash:     tx2Signature,
						TransactionLogIndex: 0,
						TransactionIndex:    1,
						BlockTime:           blockTime,
					},
					Program: "myProgram",
					Data:    "log3",
				},
				{
					BlockData: types.BlockData{
						SlotNumber:          slotNumber,
						BlockHeight:         height,
						BlockHash:           block.Blockhash,
						BlockTime:           blockTime,
						TransactionHash:     solana.Signature{10, 11},
						TransactionLogIndex: 0,
						TransactionIndex:    2,
						Error:               fmt.Errorf("some error"),
					},
					Program: "myProgram",
					Data:    "log4",
				},
			},
		}, result)

		// Verify metrics - use t.Name() as the unique ID to avoid cross-test pollution
		expectedMetrics := solLpPromTest{
			id:                 t.Name(),
			txsTruncated:       outcomeDependantTestMetric{reverted: 1}, // the tx that was truncated also had an error
			txsLogParsingError: outcomeDependantTestMetric{},
		}
		expectedMetrics.assertEqual(t)

		select {
		case <-job.Done():
		default:
			t.Fatal("expected job to be done")
		}
	})

	t.Run("Unexpected parsing error", func(t *testing.T) {
		resetPromMetricsForLabel(t.Name()) // Reset counters to avoid accumulation across test runs
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		tx1Signature := solana.Signature{4, 5, 6}
		txSigToDataBytes := func(sig solana.Signature) *rpc.DataBytesOrJSON {
			tx := solana.Transaction{Signatures: []solana.Signature{sig}}
			binary, err := tx.MarshalBinary()
			require.NoError(t, err)
			return rpc.DataBytesOrJSONFromBytes(binary)
		}
		txWithMeta1 := rpc.TransactionWithMeta{Transaction: txSigToDataBytes(tx1Signature), Meta: &rpc.TransactionMeta{LogMessages: []string{"log1", "log2"}}}
		height := uint64(41)
		blockTime := solana.UnixTimeSeconds(128)
		block := rpc.GetBlockResult{BlockHeight: &height, BlockTime: ptr(blockTime), Blockhash: solana.Hash{1, 2, 3}, Transactions: []rpc.TransactionWithMeta{txWithMeta1}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		job := newGetBlockJob(nil, client, make(chan types.Block, 1), lggr, slotNumber, metrics, nil)
		job.parseProgramLogs = func(logs []string) ([]types.ProgramOutput, error) {
			return nil, errors.New("unexpected test parsing error")
		}
		err = job.Run(t.Context())
		require.NoError(t, err)
		result := <-job.blocks

		require.Equal(t, types.Block{
			SlotNumber: slotNumber,
			BlockHash:  &block.Blockhash,
			Events:     []types.ProgramEvent{}, // could not process tx due to parsing error
		}, result)

		// Verify metrics - use t.Name() as the unique ID to avoid cross-test pollution
		expectedMetrics := solLpPromTest{
			id:                 t.Name(),
			txsTruncated:       outcomeDependantTestMetric{},
			txsLogParsingError: outcomeDependantTestMetric{succeeded: 1}, // the tx whose logs failed to parse had succeeded onchain
		}
		expectedMetrics.assertEqual(t)

		select {
		case <-job.Done():
		default:
			t.Fatal("expected job to be done")
		}
	})
}

func ptr[T any](v T) *T {
	return &v
}

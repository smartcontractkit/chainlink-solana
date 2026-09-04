package logpoller

import (
	"context"
	"encoding/base64"
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
	blocksSkipped      float64
}

func (p solLpPromTest) assertEqual(t *testing.T) {
	assert.InDelta(t, p.txsTruncated.succeeded, testutil.ToFloat64(promSolLp.txsTruncated.succeeded.WithLabelValues(p.id)), 0.0001, "mismatch: truncated succeeded")
	assert.InDelta(t, p.txsTruncated.reverted, testutil.ToFloat64(promSolLp.txsTruncated.reverted.WithLabelValues(p.id)), 0.0001, "mismatch: truncated reverted")
	assert.InDelta(t, p.txsLogParsingError.succeeded, testutil.ToFloat64(promSolLp.txsLogParsingError.succeeded.WithLabelValues(p.id)), 0.0001, "mismatch: log parsing error succeeded")
	assert.InDelta(t, p.txsLogParsingError.reverted, testutil.ToFloat64(promSolLp.txsLogParsingError.reverted.WithLabelValues(p.id)), 0.0001, "mismatch: log parsing error reverted")
	assert.InDelta(t, p.blocksSkipped, testutil.ToFloat64(promLpBlocksSkipped.WithLabelValues(p.id)), 0.0001, "mismatch: block skipped")
}

// resetPromMetricsForLabel clears the prometheus counters for the given label
// to avoid counter accumulation across test runs when using -race or -count flags
func resetPromMetricsForLabel(label string) {
	promSolLp.txsTruncated.succeeded.DeleteLabelValues(label)
	promSolLp.txsTruncated.reverted.DeleteLabelValues(label)
	promSolLp.txsLogParsingError.succeeded.DeleteLabelValues(label)
	promSolLp.txsLogParsingError.reverted.DeleteLabelValues(label)
	promLpBlocksSkipped.DeleteLabelValues(label)
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
	t.Run("Abort emits block skipped metric", func(t *testing.T) {
		resetPromMetricsForLabel(t.Name())
		lggr := logger.Sugared(logger.Test(t))
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		blocks := make(chan types.Block, 1)
		job := newGetBlockJob(nil, nil, blocks, lggr, slotNumber, metrics, nil)
		err = job.Abort(t.Context())
		require.NoError(t, err)

		result := <-blocks
		assert.True(t, result.Aborted)
		assert.Equal(t, slotNumber, result.SlotNumber)

		expectedMetrics := solLpPromTest{
			id:            t.Name(),
			blocksSkipped: 1,
		}
		expectedMetrics.assertEqual(t)

		select {
		case <-job.Done():
		default:
			t.Fatal("expected job to be done")
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

	t.Run("Can parse a real v1 transaction fetched from devnet", func(t *testing.T) {
		// signature 2TYJx5SUdDk16F79XtzYnPN9xJdZGE3uV2Sdyx4B6qDNcJr5sCmAfvanR1iJYRPN9NvgmPJFjWC48L74UjAdtf4i,
		// fetched from devnet via getTransaction with maxSupportedTransactionVersion=1 and encoding=base64.
		const rawTxBase64 = "gQIACgwAAACq2gUre9xa2z/ldC51RithZ9L9aAc+p+sJ6V2MJwe1PAEQAawPKl/5X+e2HmZvgahqRpI32z+068bN1nZXs7QyQnWBrTHEzH6EYXLYouw55rgM6q1dotGE6vsp7CW4gQCyQM+aWYngH1KqcdSSnbSmnC+KD5TvPjXeIxZ1txpehF2rB26u7rNdg35WuF+8qDNV3qMUG5z9W5Cz6MbufAo0lC9eg6nVMcg4+XXkBHoyjS05fVyKshMxQKvtHJFOqaNM1TtELLORIVfxOpM9ATQoLQMrX/7NAaLb8bd5BgjfAC6npl/IHQ/vqIYMs7g/CJsCJL6KZoe3rkn1lMC5tNfpOJMtx71r2jruWGPiFgQi7ixv89aruYChuVkAhwQ4h3atSSe80KmkZRziYm6y2Xn9E0qlwIRiCYWZJIDDOm1AKuwhjmtqMVIduprQivBrbYzpqqz6gh6ncSy7S5JpSm8DNvs1TXS2FccMV1tw/ADXFrs+B7UxAFPod5lElck702jpzsFO0/6/8xuJScnUESApvF0+/1Dasy9BZIvWE5NzM+G/pl/IHOGe3NLSw0CwL6Yb4dW63eFZKDPd+SAJ2M9oVFUG3fbh12Whk9nL4UbO63msHLSF7V9bN5E6jPWFfv8AqQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAUQAz/MeAA/Df1b5nUHSbhLn0gjucyDtstU8a2wzuIG7EwwAA+TcWAAYSYAAAAAcCCAMJCgsEBQEMBg0ODwbXPD0ucjeAsGQAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAIcA6cqOUPozdZqMCTD6UklcRr+oAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAANAHAABX7S9XKzeT0qrCRPX4NK5jObwzeDzAtgjPb7GC1JECggynP++sGNHD7BRaJE6K7VUIPzBYfJN1sPLTI6K73KEBJmKK9PTj/28qHhL1vi2moHKDklrkvL18HpVhIkLzOqzmrVFZo7E3Bg9Ta3/T9hskJA5h9BTQLhfUNJWmnXxfAQ=="

		rawTx, err := base64.StdEncoding.DecodeString(rawTxBase64)
		require.NoError(t, err)

		// sanity check the fixture actually decodes to a v1 message, so a future regression in
		// solana-go's decoder can't hide behind an unrelated job.Run error
		txWithMeta := rpc.TransactionWithMeta{Transaction: rpc.DataBytesOrJSONFromBytes(rawTx)}
		decodedTx, decodeErr := txWithMeta.GetTransaction()
		require.NoError(t, decodeErr)
		require.Equal(t, solana.MessageVersionV1, decodedTx.Message.GetVersion())
		require.Equal(t, "2kxgiLQzv6cPVENTw7uxViGGz6DWWPPrLv6aRzyj8AEkMUBv7tBqTWuS4DbANqXZq7pYvSC1zYfxyTbGAs5KW7cL", decodedTx.Signatures[0].String())
		require.Len(t, decodedTx.Message.Instructions, 1)
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		txWithMeta.Meta = &rpc.TransactionMeta{
			LogMessages: []string{
				"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA invoke [1]",
				"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA consumed 76 of 200000 compute units",
				"Program TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA success",
				"Program ComputeBudget111111111111111111111111111111 invoke [1]",
				"Program ComputeBudget111111111111111111111111111111 success",
			},
		}
		height := uint64(492551830)
		blockTime := solana.UnixTimeSeconds(1788447393)
		block := rpc.GetBlockResult{BlockHeight: &height, BlockTime: ptr(blockTime), Blockhash: solana.Hash{1, 2, 3}, Transactions: []rpc.TransactionWithMeta{txWithMeta}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		metrics, err := NewSolLpMetrics(t.Name())
		require.NoError(t, err)
		job := newGetBlockJob(nil, client, make(chan types.Block, 1), lggr, slotNumber, metrics, nil)

		err = job.Run(t.Context())
		require.NoError(t, err)

		result := <-job.blocks
		require.Equal(t, slotNumber, result.SlotNumber)
		require.Equal(t, &block.Blockhash, result.BlockHash)
		require.Empty(t, result.Events) // no "Program log:"/"Program data:" lines in this tx, so no events expected

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

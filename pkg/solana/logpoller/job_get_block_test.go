package logpoller

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
)

func TestGetBlockJob(t *testing.T) {
	const slotNumber = uint64(42)
	t.Run("String contains slot number", func(t *testing.T) {
		lggr := logger.Sugared(logger.Test(t))
		job := newGetBlockJob(nil, nil, lggr, slotNumber)
		require.Equal(t, "getBlock for slotNumber: 42", job.String())
	})
	t.Run("Error if fails to get block", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		expectedError := errors.New("rpc failed")
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(nil, expectedError).Once()
		job := newGetBlockJob(client, make(chan Block), lggr, slotNumber)
		err := job.Run(tests.Context(t))
		require.ErrorIs(t, err, expectedError)
	})
	t.Run("Error if block height is not present", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		block := rpc.GetBlockResult{}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		job := newGetBlockJob(client, make(chan Block), lggr, slotNumber)
		err := job.Run(tests.Context(t))
		require.ErrorContains(t, err, "block at slot 42 returned from rpc is missing block number")
	})
	t.Run("Error if block time is not present", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))

		block := rpc.GetBlockResult{BlockHeight: ptr(uint64(10))}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		job := newGetBlockJob(client, make(chan Block), lggr, slotNumber)
		err := job.Run(tests.Context(t))
		require.ErrorContains(t, err, "block at slot 42 returned from rpc is missing block time")
	})
	t.Run("Error if transaction field is not present", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		block := rpc.GetBlockResult{BlockHeight: ptr(uint64(10)), BlockTime: ptr(solana.UnixTimeSeconds(10)), Transactions: []rpc.TransactionWithMeta{{Transaction: nil}}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		job := newGetBlockJob(client, make(chan Block), lggr, slotNumber)
		err := job.Run(tests.Context(t))
		require.ErrorContains(t, err, "failed to parse transaction 0 in slot 42: missing transaction field")
	})
	t.Run("Error if fails to get transaction", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		block := rpc.GetBlockResult{BlockHeight: ptr(uint64(10)), BlockTime: ptr(solana.UnixTimeSeconds(10)), Transactions: []rpc.TransactionWithMeta{{Transaction: rpc.DataBytesOrJSONFromBytes([]byte("{"))}}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		job := newGetBlockJob(client, make(chan Block), lggr, slotNumber)
		err := job.Run(tests.Context(t))
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
		job := newGetBlockJob(client, make(chan Block), lggr, slotNumber)
		err = job.Run(tests.Context(t))
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
		job := newGetBlockJob(client, make(chan Block), lggr, slotNumber)
		err = job.Run(tests.Context(t))
		require.ErrorContains(t, err, "expected transaction to have meta. signature: 2AnZxg8HN2sGa7GC7iWGDgpXbEasqXQNEumCjvHUFDcBnfRKAdaN3SvKLhbQwheN15xDkL5D5mdX21A5gH1MdYB; slot: 42; idx: 0")
	})
	t.Run("Can abort even if no one waits for result", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		lggr := logger.Sugared(logger.Test(t))
		tx := solana.Transaction{Signatures: make([]solana.Signature, 1)}
		txB, err := tx.MarshalBinary()
		require.NoError(t, err)
		block := rpc.GetBlockResult{BlockHeight: ptr(uint64(10)), BlockTime: ptr(solana.UnixTimeSeconds(10)), Transactions: []rpc.TransactionWithMeta{{Transaction: rpc.DataBytesOrJSONFromBytes(txB), Meta: &rpc.TransactionMeta{}}}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		job := newGetBlockJob(client, make(chan Block), lggr, slotNumber)
		ctx, cancel := context.WithCancel(tests.Context(t))
		cancel()
		err = job.Run(ctx)
		require.ErrorIs(t, err, context.Canceled)
		select {
		case <-job.Done():
			require.Fail(t, "expected done channel to be open as job was aborted")
		default:
		}
	})
	t.Run("Happy path", func(t *testing.T) {
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
		txWithMeta3 := rpc.TransactionWithMeta{Transaction: txSigToDataBytes(solana.Signature{10, 11}), Meta: &rpc.TransactionMeta{LogMessages: []string{"log4"}, Err: fmt.Errorf("some error")}}
		height := uint64(41)
		blockTime := solana.UnixTimeSeconds(128)
		block := rpc.GetBlockResult{BlockHeight: &height, BlockTime: ptr(blockTime), Blockhash: solana.Hash{1, 2, 3}, Transactions: []rpc.TransactionWithMeta{txWithMeta1, txWithMeta2, txWithMeta3}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		job := newGetBlockJob(client, make(chan Block, 1), lggr, slotNumber)
		job.parseProgramLogs = func(logs []string) []ProgramOutput {
			result := ProgramOutput{
				Program: "myProgram",
			}
			for _, l := range logs {
				result.Events = append(result.Events, ProgramEvent{Data: l, Program: "myProgram"})
			}
			return []ProgramOutput{result}
		}
		err := job.Run(tests.Context(t))
		require.NoError(t, err)
		result := <-job.blocks
		require.Equal(t, Block{
			SlotNumber: slotNumber,
			BlockHash:  block.Blockhash,
			Events: []ProgramEvent{
				{
					BlockData: BlockData{
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
					BlockData: BlockData{
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
					BlockData: BlockData{
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
			},
		}, result)
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

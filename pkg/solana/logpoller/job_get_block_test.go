package logpoller

import (
	"context"
	"errors"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
)

func TestGetBlockJob(t *testing.T) {
	const slotNumber = uint64(42)
	t.Run("String contains slot number", func(t *testing.T) {
		job := newGetBlockJob(nil, nil, slotNumber)
		require.Equal(t, "getBlock for slotNumber: 42", job.String())
	})
	t.Run("Error if fails to get block", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		expectedError := errors.New("rpc failed")
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(nil, expectedError).Once()
		job := newGetBlockJob(client, make(chan Block), slotNumber)
		err := job.Run(tests.Context(t))
		require.ErrorIs(t, err, expectedError)
	})
	t.Run("Error if fails to get transaction", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		block := rpc.GetBlockResult{Transactions: []rpc.TransactionWithMeta{{Transaction: rpc.DataBytesOrJSONFromBytes([]byte("{"))}}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		job := newGetBlockJob(client, make(chan Block), slotNumber)
		err := job.Run(tests.Context(t))
		require.ErrorContains(t, err, "failed to parse transaction 0 in slot 42")
	})
	t.Run("Error if Tx has no signatures", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		tx := solana.Transaction{}
		txB, err := tx.MarshalBinary()
		require.NoError(t, err)
		block := rpc.GetBlockResult{Transactions: []rpc.TransactionWithMeta{{Transaction: rpc.DataBytesOrJSONFromBytes(txB)}}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		job := newGetBlockJob(client, make(chan Block), slotNumber)
		err = job.Run(tests.Context(t))
		require.ErrorContains(t, err, "expected all transactions to have at least one signature 0 in slot 42")
	})
	t.Run("Can abort even if no one waits for result", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		tx := solana.Transaction{Signatures: make([]solana.Signature, 1)}
		txB, err := tx.MarshalBinary()
		require.NoError(t, err)
		block := rpc.GetBlockResult{Transactions: []rpc.TransactionWithMeta{{Transaction: rpc.DataBytesOrJSONFromBytes(txB), Meta: &rpc.TransactionMeta{}}}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		job := newGetBlockJob(client, make(chan Block), slotNumber)
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
		height := uint64(41)
		block := rpc.GetBlockResult{BlockHeight: &height, Blockhash: solana.Hash{1, 2, 3}, Transactions: []rpc.TransactionWithMeta{txWithMeta1, txWithMeta2}}
		client.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()
		job := newGetBlockJob(client, make(chan Block, 1), slotNumber)
		job.parseProgramLogs = func(logs []string) []ProgramOutput {
			result := ProgramOutput{}
			for _, l := range logs {
				result.Events = append(result.Events, ProgramEvent{Data: l})
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
					},
					Prefix: "",
					Data:   "log1",
				},
				{
					BlockData: BlockData{
						SlotNumber:          slotNumber,
						BlockHeight:         height,
						BlockHash:           block.Blockhash,
						TransactionHash:     tx1Signature,
						TransactionLogIndex: 1,
						TransactionIndex:    0,
					},
					Prefix: "",
					Data:   "log2",
				},
				{
					BlockData: BlockData{
						SlotNumber:          slotNumber,
						BlockHeight:         height,
						BlockHash:           block.Blockhash,
						TransactionHash:     tx2Signature,
						TransactionLogIndex: 0,
						TransactionIndex:    1,
					},
					Prefix: "",
					Data:   "log3",
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

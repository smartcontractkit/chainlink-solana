package logpoller

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

func TestIsDecodableTransactionVersion(t *testing.T) {
	t.Parallel()

	require.True(t, isDecodableTransactionVersion(rpc.LegacyTransactionVersion), "legacy must be decodable")
	require.True(t, isDecodableTransactionVersion(rpc.TransactionVersion(0)), "v0 must be decodable")
	require.False(t, isDecodableTransactionVersion(rpc.TransactionVersion(1)), "v1 is not decodable by solana-go")
	require.False(t, isDecodableTransactionVersion(rpc.TransactionVersion(2)))
}

// getBlock must be asked for a version above what we can decode, otherwise the RPC fails the
// whole call with -32015 when a block contains a newer transaction.
func TestGetBlockJob_RequestsVersionAboveDecodable(t *testing.T) {
	require.Greater(t, client.MaxRequestedTransactionVersion, client.MaxSupportTransactionVersion,
		"requested version must exceed the decodable one or a single newer tx stalls the whole slot")

	rpcClient := mocks.NewRPCClient(t)
	lggr := logger.Sugared(logger.Test(t))

	var gotVersion *uint64
	rpcClient.EXPECT().GetBlockWithOpts(mock.Anything, uint64(42), mock.Anything).
		RunAndReturn(func(_ context.Context, _ uint64, opts *rpc.GetBlockOpts) (*rpc.GetBlockResult, error) {
			gotVersion = opts.MaxSupportedTransactionVersion
			return &rpc.GetBlockResult{
				BlockHeight: ptr(uint64(10)),
				BlockTime:   ptr(solana.UnixTimeSeconds(10)),
			}, nil
		}).Once()

	metrics, err := NewSolLpMetrics(t.Name())
	require.NoError(t, err)
	job := newGetBlockJob(nil, rpcClient, make(chan types.Block, 1), lggr, 42, metrics, nil)
	require.NoError(t, job.Run(t.Context()))

	require.NotNil(t, gotVersion)
	require.Equal(t, client.MaxRequestedTransactionVersion, *gotVersion)
}

// A transaction whose version we cannot decode must be skipped rather than parsed with the v0
// layout, and must not stall the rest of the block.
func TestGetBlockJob_SkipsUndecodableTransactionVersion(t *testing.T) {
	const slotNumber = uint64(42)
	promLpTxsUnsupportedVersion.DeleteLabelValues(t.Name())

	rpcClient := mocks.NewRPCClient(t)
	lggr := logger.Sugared(logger.Test(t))

	txBytes := func(sig solana.Signature) *rpc.DataBytesOrJSON {
		tx := solana.Transaction{Signatures: []solana.Signature{sig}}
		bts, err := tx.MarshalBinary()
		require.NoError(t, err)
		return rpc.DataBytesOrJSONFromBytes(bts)
	}

	legacyTx := rpc.TransactionWithMeta{
		Version:     rpc.LegacyTransactionVersion,
		Transaction: txBytes(solana.Signature{1}),
		Meta:        &rpc.TransactionMeta{LogMessages: []string{"legacy-log"}},
	}
	// undecodable: sits between the two decodable transactions to prove it is skipped
	// individually rather than aborting the block
	v1Tx := rpc.TransactionWithMeta{
		Version:     rpc.TransactionVersion(1),
		Transaction: txBytes(solana.Signature{2}),
		Meta:        &rpc.TransactionMeta{LogMessages: []string{"v1-log"}},
	}
	v0Tx := rpc.TransactionWithMeta{
		Version:     rpc.TransactionVersion(0),
		Transaction: txBytes(solana.Signature{3}),
		Meta:        &rpc.TransactionMeta{LogMessages: []string{"v0-log"}},
	}

	block := rpc.GetBlockResult{
		BlockHeight:  ptr(uint64(41)),
		BlockTime:    ptr(solana.UnixTimeSeconds(128)),
		Blockhash:    solana.Hash{1, 2, 3},
		Transactions: []rpc.TransactionWithMeta{legacyTx, v1Tx, v0Tx},
	}
	rpcClient.EXPECT().GetBlockWithOpts(mock.Anything, slotNumber, mock.Anything).Return(&block, nil).Once()

	metrics, err := NewSolLpMetrics(t.Name())
	require.NoError(t, err)
	job := newGetBlockJob(nil, rpcClient, make(chan types.Block, 1), lggr, slotNumber, metrics, nil)
	job.parseProgramLogs = func(logs []string) ([]types.ProgramOutput, error) {
		out := types.ProgramOutput{Program: "myProgram"}
		for _, l := range logs {
			out.Events = append(out.Events, types.ProgramEvent{Data: l, Program: "myProgram"})
		}
		return []types.ProgramOutput{out}, nil
	}

	require.NoError(t, job.Run(t.Context()), "an undecodable transaction must not fail the block")

	result := <-job.blocks
	data := make([]string, 0, len(result.Events))
	txIndexes := make([]int, 0, len(result.Events))
	for _, e := range result.Events {
		data = append(data, e.Data)
		txIndexes = append(txIndexes, e.BlockData.TransactionIndex)
	}

	require.Equal(t, []string{"legacy-log", "v0-log"}, data, "only decodable transactions should yield events")
	// index 1 is absent and index 2 keeps its position: skipping must not renumber
	// transactions, which would duplicate or misattribute events
	require.Equal(t, []int{0, 2}, txIndexes)

	require.InDelta(t, 1.0, testutil.ToFloat64(promLpTxsUnsupportedVersion.WithLabelValues(t.Name())), 0.0001,
		"skipped transaction should be counted")
}

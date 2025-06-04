package chainwriter_test

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	ccipsolana "github.com/smartcontractkit/chainlink-ccip/chains/solana"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/stretchr/testify/require"
)

func Test_CCIPExecutionReportBuffer(t *testing.T) {
	args := ccipsolana.SVMExecCallArgs{
		Report: make([]byte, 1500),
	}
	var accounts solana.AccountMetaSlice
	prorgamID := GetRandomPubKey(t)
	feePayer := GetRandomPubKey(t)

	t.Run("CCIP Execution report happy path", func(t *testing.T) {
		bufferIxs, closeBufferIx, accounts, newArgs, err := chainwriter.CCIPExecutionReportBuffer(t.Context(), args, accounts, prorgamID, feePayer)
		require.NoError(t, err)

		require.Len(t, bufferIxs, 2)
		bufferIx := bufferIxs[0]
		require.Len(t, bufferIx.Accounts(), 4)
		bufferIxPDA := bufferIx.Accounts()[0] // First account is the buffer PDA

		require.NotNil(t, closeBufferIx)
		require.Len(t, closeBufferIx.Accounts(), 3)
		closeBufferIxPDA := closeBufferIx.Accounts()[0] // First account is the buffer PDA
		require.Equal(t, bufferIxPDA, closeBufferIxPDA)

		require.Len(t, accounts, 1) // Only account should be the buffer PDA
		mainIxBufferPDA := accounts[0]
		require.Equal(t, closeBufferIxPDA, mainIxBufferPDA)
		require.IsType(t, ccipsolana.SVMExecCallArgs{}, newArgs)
		castedArgs := newArgs.(ccipsolana.SVMExecCallArgs)
		require.Len(t, castedArgs.Report, 0)
	})
}

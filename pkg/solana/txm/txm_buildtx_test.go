package txm

import (
	"testing"

	solanaGo "github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	ksmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

// buildTx must leave the caller's transaction unchanged.
func TestTxm_BuildTx_DoesNotMutateOriginalTx(t *testing.T) {
	t.Parallel()

	ks := ksmocks.NewSimpleKeystore(t)
	ks.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil)
	loader := utils.NewStaticLoader[client.ReaderWriter](clientmocks.NewReaderWriter(t))
	txm, err := NewTxm("buildtx_mutation_test", loader, nil, config.NewDefault(), ks, logger.Test(t))
	require.NoError(t, err)

	// include an address-table account index (2, past the end of AccountKeys)
	var tx solanaGo.Transaction
	tx.Message.AccountKeys = []solanaGo.PublicKey{{1}, {2}}
	tx.Message.Instructions = []solanaGo.CompiledInstruction{{ProgramIDIndex: 1, Accounts: []uint16{0, 2}}}

	msg := pendingTx{tx: tx, cfg: txmutils.TxConfig{ComputeUnitLimit: 200_000}}
	original := utils.DeepCopyTx(tx)

	first, err := txm.buildTx(t.Context(), msg, 0)
	require.NoError(t, err)
	second, err := txm.buildTx(t.Context(), msg, 0)
	require.NoError(t, err)

	require.Equal(t, original, msg.tx, "buildTx mutated the caller's transaction")
	require.Equal(t, first, second, "buildTx is not deterministic for identical input")
}

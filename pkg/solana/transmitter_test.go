package solana

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
)

// custom mock txm instead of mockery generated because SetTxConfig causes circular imports
// and only one function is needed to be mocked
type verifyTxSize struct {
	t *testing.T
	s *solana.PrivateKey
}

func (txm verifyTxSize) Enqueue(_ context.Context, _ string, tx *solana.Transaction, _ ...txm.SetTxConfig) error {
	// additional components that transaction manager adds to the transaction
	require.NoError(txm.t, fees.SetComputeUnitPrice(tx, 0))
	require.NoError(txm.t, fees.SetComputeUnitLimit(tx, 0))

	_, err := tx.Sign(func(_ solana.PublicKey) *solana.PrivateKey { return txm.s })
	require.NoError(txm.t, err)

	data, err := tx.MarshalBinary()
	require.NoError(txm.t, err)
	require.LessOrEqual(txm.t, len(data), 1232, "exceeds maximum solana transaction size")
	assert.Equal(txm.t, 936, len(data), "does not match expected ocr2 transmit transaction size")

	return nil
}

func TestTransmitter_TxSize(t *testing.T) {
	mustNewRandomPublicKey := func() solana.PublicKey {
		k, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)
		return k.PublicKey()
	}

	signer, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	mockTxm := verifyTxSize{
		t: t,
		s: &signer,
	}

	rw := clientmocks.NewReaderWriter(t)
	rw.On("LatestBlockhash", mock.Anything).Return(&rpc.GetLatestBlockhashResult{
		Value: &rpc.LatestBlockhashResult{},
	}, nil)

	transmitter := Transmitter{
		stateID:            mustNewRandomPublicKey(),
		programID:          mustNewRandomPublicKey(),
		storeProgramID:     mustNewRandomPublicKey(),
		transmissionsID:    mustNewRandomPublicKey(),
		transmissionSigner: signer.PublicKey(),
		reader:             rw,
		stateCache:         &StateCache{},
		lggr:               logger.Test(t),
		txManager:          mockTxm,
	}

	sigs := []types.AttributedOnchainSignature{}
	F := 5 // typical configuration value
	for i := 0; i < F+1; i++ {
		sigs = append(sigs, types.AttributedOnchainSignature{
			Signature: make([]byte, 65), // expected length of signature
		})
	}
	require.NoError(t, transmitter.Transmit(tests.Context(t), types.ReportContext{}, make([]byte, ReportLen), sigs))
}

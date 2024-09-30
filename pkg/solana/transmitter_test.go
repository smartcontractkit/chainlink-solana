package solana

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	txmmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
)

func TestTransmitter_TxSize(t *testing.T) {
	mustNewRandomPublicKey := func() solana.PublicKey {
		k, err := solana.NewRandomPrivateKey()
		require.NoError(t, err)
		return k.PublicKey()
	}

	signer, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	txm := txmmocks.NewTxManager(t)
	txm.On("Enqueue", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		tx, ok := args[1].(*solana.Transaction)
		require.True(t, ok)

		// additional components that transaction manager adds to the transaction
		require.NoError(t, fees.SetComputeUnitPrice(tx, 0))
		require.NoError(t, fees.SetComputeUnitLimit(tx, 0))

		_, err := tx.Sign(func(_ solana.PublicKey) *solana.PrivateKey { return &signer })
		require.NoError(t, err)

		data, err := tx.MarshalBinary()
		require.NoError(t, err)
		require.LessOrEqual(t, len(data), 1232, "exceeds maximum solana transaction size")
		assert.Equal(t, 936, len(data), "does not match expected ocr2 transmit transaction size")

	}).Return(nil)

	rw := clientmocks.NewReaderWriter(t)
	rw.On("LatestBlockhash").Return(&rpc.GetLatestBlockhashResult{
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
		txManager:          txm,
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

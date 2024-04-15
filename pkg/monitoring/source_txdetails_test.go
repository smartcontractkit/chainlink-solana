package monitoring

import (
	"encoding/json"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func TestTxDetailsSource(t *testing.T) {
	cr := mocks.NewChainReader(t)

	lgr, logs := logger.TestObserved(t, zapcore.DebugLevel)

	f := NewTxDetailsSourceFactory(cr, lgr)

	assert.Equal(t, types.TxDetailsType, f.GetType())

	cfg := config.SolanaFeedConfig{
		ContractAddress: types.SampleTxResultProgram,
	}
	s, err := f.NewSource(nil, cfg)
	require.NoError(t, err)

	// empty response
	cr.On("GetSignaturesForAddressWithOpts", mock.Anything, mock.Anything, mock.Anything).Return([]*rpc.TransactionSignature{}, nil).Once()
	res, err := s.Fetch(tests.Context(t))
	require.NoError(t, err)
	data := testutils.ParseTxDetails(t, res)
	assert.Equal(t, 0, len(data))

	// nil GetTransaction response
	cr.On("GetSignaturesForAddressWithOpts", mock.Anything, mock.Anything, mock.Anything).Return([]*rpc.TransactionSignature{
		{},
	}, nil).Once()
	cr.On("GetTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
	res, err = s.Fetch(tests.Context(t))
	require.NoError(t, err)
	data = testutils.ParseTxDetails(t, res)
	assert.Equal(t, 1, len(data))
	assert.Equal(t, 1, logs.FilterLevelExact(zapcore.DebugLevel).FilterMessage("GetTransaction returned nil").Len())

	// invalid tx
	cr.On("GetSignaturesForAddressWithOpts", mock.Anything, mock.Anything, mock.Anything).Return([]*rpc.TransactionSignature{
		{},
	}, nil).Once()
	blockTime := solana.UnixTimeSeconds(0)
	cr.On("GetTransaction", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.GetTransactionResult{
		BlockTime:   &blockTime,
		Transaction: &rpc.TransactionResultEnvelope{},
		Meta:        &rpc.TransactionMeta{},
	}, nil).Once()
	res, err = s.Fetch(tests.Context(t))
	require.NoError(t, err)
	data = testutils.ParseTxDetails(t, res)
	assert.Equal(t, 1, len(data))
	assert.Equal(t, 1, logs.FilterLevelExact(zapcore.DebugLevel).FilterMessage("tx not valid for tracking").Len())

	// happy path
	var rpcResponse rpc.GetTransactionResult
	require.NoError(t, json.Unmarshal([]byte(types.SampleTxResultJSON), &rpcResponse))
	cr.On("GetSignaturesForAddressWithOpts", mock.Anything, mock.Anything, mock.Anything).Return([]*rpc.TransactionSignature{
		{},
	}, nil).Once()
	cr.On("GetTransaction", mock.Anything, mock.Anything, mock.Anything).Return(&rpcResponse, nil).Once()
	res, err = s.Fetch(tests.Context(t))
	require.NoError(t, err)
	data = testutils.ParseTxDetails(t, res)
	assert.Equal(t, 1, len(data))
	assert.Nil(t, data[0].Err)
	assert.NotEqual(t, solana.PublicKey{}, data[0].Sender)
	assert.NotZero(t, data[0].ObservationCount)
	assert.NotZero(t, data[0].Fee)
	assert.NotZero(t, data[0].Slot)
}

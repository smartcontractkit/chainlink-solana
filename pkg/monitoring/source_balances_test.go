package monitoring

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

func TestFeedBalancesSource(t *testing.T) {
	cr := mocks.NewChainReader(t)
	lgr := logger.Test(t)
	ctx := tests.Context(t)

	factory := NewFeedBalancesSourceFactory(cr, lgr)
	assert.Equal(t, types.BalanceType, factory.GetType())

	// generate source
	_, err := factory.NewSource(nil, nil)
	assert.Error(t, err)
	source, err := factory.NewSource(nil, config.SolanaFeedConfig{
		ContractAddress: testutils.GeneratePublicKey(),
		StateAccount:    testutils.GeneratePublicKey(),
	})
	require.NoError(t, err)

	cr.On("GetState", mock.Anything, mock.Anything, mock.Anything).Return(pkgSolana.State{}, uint64(0), fmt.Errorf("fail")).Once()
	cr.On("GetState", mock.Anything, mock.Anything, mock.Anything).Return(pkgSolana.State{
		Transmissions: testutils.GeneratePublicKey(),
		Config: pkgSolana.Config{
			TokenVault:                testutils.GeneratePublicKey(),
			RequesterAccessController: testutils.GeneratePublicKey(),
			BillingAccessController:   testutils.GeneratePublicKey(),
		},
	}, uint64(0), nil)

	// GetState error
	_, err = source.Fetch(ctx)
	require.ErrorContains(t, err, "failed to get contract state")

	cr.On("GetBalance", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.GetBalanceResult{
		Value: rand.Uint64() + 1, // will never be 0
	}, nil)

	// happy path
	out, err := source.Fetch(ctx)
	require.NoError(t, err)
	balances, ok := out.(types.Balances)
	require.True(t, ok)

	// validate balances
	assert.Equal(t, len(balances.Values), len(balances.Addresses))
	for k, v := range balances.Addresses {
		assert.NotEqual(t, solana.PublicKey{}, v)
		bal, ok := balances.Values[k]
		assert.True(t, ok)
		assert.NotEqual(t, uint64(0), bal)
	}
}

// TestBalancesSource checks the underlying error handling for BalancesSource
func TestBalancesSource(t *testing.T) {
	cr := mocks.NewChainReader(t)
	lgr, logs := logger.TestObserved(t, zapcore.ErrorLevel)
	ctx := tests.Context(t)

	b := balancesSource{
		client: cr,
		log:    lgr,
	}

	// nil check
	_, err := b.Fetch(ctx)
	assert.ErrorContains(t, err, "balancesSource.addresses is nil")

	// zero addresses
	b.addresses = map[string]solana.PublicKey{}
	out, err := b.Fetch(ctx)
	require.NoError(t, err)
	res, ok := out.(types.Balances)
	require.True(t, ok)
	assert.Equal(t, 0, len(res.Values))
	assert.Equal(t, 0, len(res.Addresses))

	// error handling
	cr.On("GetBalance", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("fail")).Once()
	cr.On("GetBalance", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
	cr.On("GetBalance", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.GetBalanceResult{
		Value: rand.Uint64() + 1, // will never be 0
	}, nil)

	// fail on get balance
	b.addresses["0"] = solana.PublicKey{1}
	b.addresses["1"] = solana.PublicKey{11}
	_, err = b.Fetch(ctx)
	assert.ErrorContains(t, err, ErrBalancesSource)
	assert.ErrorContains(t, err, ErrGetBalance)
	assert.ErrorContains(t, err, ErrGetBalanceNil)
	assert.Equal(t, 1, logs.FilterMessageSnippet(ErrGetBalance).Len())
	assert.Equal(t, 1, logs.FilterMessageSnippet(ErrGetBalanceNil).Len())

	// happy path
	out, err = b.Fetch(ctx)
	require.NoError(t, err)
	balances, ok := out.(types.Balances)
	require.True(t, ok)

	// validate balances
	assert.Equal(t, len(balances.Values), len(balances.Addresses))
	for k, v := range balances.Addresses {
		assert.NotEqual(t, solana.PublicKey{}, v)
		bal, ok := balances.Values[k]
		assert.True(t, ok)
		assert.NotEqual(t, uint64(0), bal)
	}
}

func TestNodeBalancesSource(t *testing.T) {
	cr := mocks.NewChainReader(t)
	lgr := logger.Test(t)
	ctx := tests.Context(t)
	key := solana.PublicKey{1}

	factory := NewNodeBalancesSourceFactory(cr, lgr)
	assert.Equal(t, types.BalanceType, factory.GetType())

	s, err := factory.NewSource(nil, []commonMonitoring.NodeConfig{
		config.SolanaNodeConfig{
			ID:          t.Name(),
			NodeAddress: []string{key.String()},
		},
	})

	cr.On("GetBalance", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.GetBalanceResult{
		Value: rand.Uint64() + 1, // will never be 0
	}, nil)

	require.NoError(t, err)
	out, err := s.Fetch(ctx)
	require.NoError(t, err)
	balances, ok := out.(types.Balances)
	require.True(t, ok)
	assert.Equal(t, 1, len(balances.Values))
	assert.Equal(t, 1, len(balances.Addresses))
}

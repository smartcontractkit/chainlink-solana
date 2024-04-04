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
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

func TestFeedBalancesSource(t *testing.T) {
	cr := mocks.NewChainReader(t)
	lgr, logs := logger.TestObserved(t, zapcore.ErrorLevel)
	ctx := utils.Context(t)

	factory := NewFeedBalancesSourceFactory(cr, lgr)
	assert.Equal(t, balancesType, factory.GetType())

	// generate source
	source, err := factory.NewSource(nil, nil)
	assert.Error(t, err)
	source, err = factory.NewSource(nil, config.SolanaFeedConfig{
		ContractAddress: testutils.GeneratePublicKey(),
		StateAccount:    testutils.GeneratePublicKey(),
	})

	cr.On("GetState", mock.Anything, mock.Anything, mock.Anything).Return(pkgSolana.State{}, uint64(0), fmt.Errorf("fail")).Once()
	cr.On("GetState", mock.Anything, mock.Anything, mock.Anything).Return(pkgSolana.State{
		Transmissions: testutils.GeneratePublicKey(),
		Config: pkgSolana.Config{
			TokenVault:                testutils.GeneratePublicKey(),
			RequesterAccessController: testutils.GeneratePublicKey(),
			BillingAccessController:   testutils.GeneratePublicKey(),
		},
	}, uint64(0), nil)

	cr.On("GetBalance", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("fail")).Once()
	cr.On("GetBalance", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
	cr.On("GetBalance", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.GetBalanceResult{
		Value: rand.Uint64() + 1, // will never be 0
	}, nil)

	// fail on get state
	_, err = source.Fetch(ctx)
	assert.ErrorContains(t, err, "failed to get contract state")

	// fail on get balance
	_, err = source.Fetch(ctx)
	assert.ErrorContains(t, err, "error while fetching balances")
	assert.Equal(t, 1, logs.FilterMessageSnippet("GetBalance failed").Len())
	assert.Equal(t, 1, logs.FilterMessageSnippet("GetBalance returned nil").Len())

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

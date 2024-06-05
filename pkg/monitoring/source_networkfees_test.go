package monitoring

import (
	"testing"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
)

func TestNetworkFeesSource(t *testing.T) {
	cr := mocks.NewChainReader(t)
	lgr := logger.Test(t)
	ctx := tests.Context(t)

	factory := NewNetworkFeesSourceFactory(cr, lgr)
	assert.Equal(t, types.NetworkFeesType, factory.GetType())

	// generate source
	source, err := factory.NewSource(nil, nil)
	require.NoError(t, err)
	cr.On("GetLatestBlock", mock.Anything, mock.Anything).Return(&rpc.GetBlockResult{}, nil).Once()

	// happy path
	out, err := source.Fetch(ctx)
	require.NoError(t, err)
	slot, ok := out.(fees.BlockData)
	require.True(t, ok)
	assert.Equal(t, 0, len(slot.Fees))
	assert.Equal(t, 0, len(slot.Prices))
}

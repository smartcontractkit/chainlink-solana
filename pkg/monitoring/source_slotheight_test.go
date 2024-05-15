package monitoring

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func TestSlotHeightSource(t *testing.T) {
	cr := mocks.NewChainReader(t)
	lgr := logger.Test(t)
	ctx := tests.Context(t)

	factory := NewSlotHeightSourceFactory(cr, lgr)
	assert.Equal(t, types.SlotHeightType, factory.GetType())

	// generate source
	source, err := factory.NewSource(nil, nil)
	require.NoError(t, err)
	cr.On("GetSlot", mock.Anything, mock.Anything, mock.Anything).Return(uint64(1), nil).Once()

	// happy path
	out, err := source.Fetch(ctx)
	require.NoError(t, err)
	slot, ok := out.(types.SlotHeight)
	require.True(t, ok)
	assert.Equal(t, types.SlotHeight(1), slot)
}

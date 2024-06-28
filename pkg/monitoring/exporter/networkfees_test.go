package exporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
)

func TestNetworkFees(t *testing.T) {
	ctx := tests.Context(t)
	m := mocks.NewNetworkFees(t)
	m.On("Set", mock.Anything, mock.Anything).Once()
	m.On("Cleanup").Once()

	factory := NewNetworkFeesFactory(logger.Test(t), m)

	chainConfig := testutils.GenerateChainConfig()
	exporter, err := factory.NewExporter(commonMonitoring.ExporterParams{ChainConfig: chainConfig})
	require.NoError(t, err)

	// happy path
	exporter.Export(ctx, fees.BlockData{})
	exporter.Cleanup(ctx)

	// test passing uint64 instead of NetworkFees - should not call mock
	// NetworkFees alias of uint64
	exporter.Export(ctx, uint64(10))
}

func TestAggregateFees(t *testing.T) {
	input := metrics.NetworkFeesInput{}
	v0 := []int{10, 12, 3, 4, 1, 2}
	v1 := []int{5, 1, 10, 2, 3, 12, 4}

	require.NoError(t, aggregateFees(input, "0", v0))
	require.NoError(t, aggregateFees(input, "1", v1))

	assert.Equal(t, uint64(3), input["0"]["median"])
	assert.Equal(t, uint64(5), input["0"]["avg"])
	assert.Equal(t, uint64(1), input["0"]["min"])
	assert.Equal(t, uint64(12), input["0"]["max"])
	assert.Equal(t, uint64(2), input["0"]["lowerQuartile"])
	assert.Equal(t, uint64(10), input["0"]["upperQuartile"])

	assert.Equal(t, uint64(4), input["1"]["median"])
	assert.Equal(t, uint64(5), input["1"]["avg"])
	assert.Equal(t, uint64(1), input["1"]["min"])
	assert.Equal(t, uint64(12), input["1"]["max"])
	assert.Equal(t, uint64(2), input["1"]["lowerQuartile"])
	assert.Equal(t, uint64(10), input["1"]["upperQuartile"])
}

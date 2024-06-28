package exporter

import (
	"context"
	"errors"
	"slices"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/mathutil"
	"golang.org/x/exp/constraints"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
)

func NewNetworkFeesFactory(
	lgr commonMonitoring.Logger,
	metrics metrics.NetworkFees,
) commonMonitoring.ExporterFactory {
	return &networkFeesFactory{
		metrics,
		lgr,
	}
}

type networkFeesFactory struct {
	metrics metrics.NetworkFees
	lgr     commonMonitoring.Logger
}

func (p *networkFeesFactory) NewExporter(
	params commonMonitoring.ExporterParams,
) (commonMonitoring.Exporter, error) {
	return &networkFees{
		params.ChainConfig.GetNetworkName(),
		p.metrics,
		p.lgr,
	}, nil
}

type networkFees struct {
	chain   string
	metrics metrics.NetworkFees
	lgr     commonMonitoring.Logger
}

func (p *networkFees) Export(ctx context.Context, data interface{}) {
	blockData, ok := data.(fees.BlockData)
	if !ok {
		return // skip if input could not be parsed
	}

	input := metrics.NetworkFeesInput{}
	if err := aggregateFees(input, "computeUnitPrice", blockData.Prices); err != nil {
		p.lgr.Errorw("failed to calculate computeUnitPrice", "error", err)
		return
	}
	if err := aggregateFees(input, "totalFee", blockData.Fees); err != nil {
		p.lgr.Errorw("failed to calculate totalFee", "error", err)
		return
	}

	p.metrics.Set(input, p.chain)
}

func (p *networkFees) Cleanup(_ context.Context) {
	p.metrics.Cleanup()
}

func aggregateFees[V constraints.Integer](input metrics.NetworkFeesInput, name string, data []V) error {
	// skip if empty list
	if len(data) == 0 {
		return nil
	}

	slices.Sort(data) // ensure sorted

	// calculate median / avg
	medianPrice, medianPriceErr := mathutil.Median(data...)
	input.Set(name, "median", uint64(medianPrice))
	avgPrice, avgPriceErr := mathutil.Avg(data...)
	input.Set(name, "avg", uint64(avgPrice))

	// calculate lower / upper quartile
	var lowerData, upperData []V
	l := len(data)
	if l%2 == 0 {
		lowerData = data[:l/2]
		upperData = data[l/2:]
	} else {
		lowerData = data[:l/2]
		upperData = data[l/2+1:]
	}
	lowerQuartilePrice, lowerQuartilePriceErr := mathutil.Median(lowerData...)
	input.Set(name, "lowerQuartile", uint64(lowerQuartilePrice))
	upperQuartilePrice, upperQuartilePriceErr := mathutil.Median(upperData...)
	input.Set(name, "upperQuartile", uint64(upperQuartilePrice))

	// calculate min/max
	input.Set(name, "max", uint64(slices.Max(data)))
	input.Set(name, "min", uint64(slices.Min(data)))

	return errors.Join(medianPriceErr, avgPriceErr, lowerQuartilePriceErr, upperQuartilePriceErr)
}

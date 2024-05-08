package metrics

import (
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
)

//go:generate mockery --name Fees --output ./mocks/

type Fees interface {
	Set(txFee uint64, computeUnitPrice fees.ComputeUnitPrice, feedInput FeedInput)
	Cleanup(feedInput FeedInput)
}

var _ Fees = (*feeMetrics)(nil)

type feeMetrics struct {
	txFee       simpleGauge
	computeUnit simpleGauge
}

func NewFees(log commonMonitoring.Logger) *feeMetrics {
	return &feeMetrics{
		txFee:       newSimpleGauge(log, types.TxFeeMetric),
		computeUnit: newSimpleGauge(log, types.ComputeUnitPriceMetric),
	}
}

func (sh *feeMetrics) Set(txFee uint64, computeUnitPrice fees.ComputeUnitPrice, feedInput FeedInput) {
	sh.txFee.set(float64(txFee), feedInput.ToPromLabels())
	sh.computeUnit.set(float64(computeUnitPrice), feedInput.ToPromLabels())
}

func (sh *feeMetrics) Cleanup(feedInput FeedInput) {
	sh.txFee.delete(feedInput.ToPromLabels())
	sh.computeUnit.delete(feedInput.ToPromLabels())
}

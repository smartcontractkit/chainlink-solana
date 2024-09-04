package exporter

import (
	"context"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/mathutil"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
)

func NewFeesFactory(
	log commonMonitoring.Logger,
	metrics metrics.Fees,
) commonMonitoring.ExporterFactory {
	return &feesFactory{
		log,
		metrics,
	}
}

type feesFactory struct {
	log     commonMonitoring.Logger
	metrics metrics.Fees
}

func (p *feesFactory) NewExporter(
	params commonMonitoring.ExporterParams,
) (commonMonitoring.Exporter, error) {
	return &feesExporter{
		metrics.FeedInput{
			AccountAddress: params.FeedConfig.GetContractAddress(),
			FeedID:         params.FeedConfig.GetContractAddress(),
			ChainID:        params.ChainConfig.GetChainID(),
			ContractStatus: params.FeedConfig.GetContractStatus(),
			ContractType:   params.FeedConfig.GetContractType(),
			FeedName:       params.FeedConfig.GetName(),
			FeedPath:       params.FeedConfig.GetPath(),
			NetworkID:      params.ChainConfig.GetNetworkID(),
			NetworkName:    params.ChainConfig.GetNetworkName(),
		},
		p.log,
		p.metrics,
	}, nil
}

type feesExporter struct {
	label   metrics.FeedInput // static for each feed
	log     commonMonitoring.Logger
	metrics metrics.Fees
}

func (f *feesExporter) Export(ctx context.Context, data interface{}) {
	details, err := types.MakeTxDetails(data)
	if err != nil {
		return // skip if input could not be parsed
	}

	// skip on no updates
	if len(details) == 0 {
		return
	}

	// calculate average of non empty TxDetails
	var feeArr []uint64
	var computeUnitsArr []fees.ComputeUnitPrice
	for _, d := range details {
		if d.Empty() {
			continue
		}
		feeArr = append(feeArr, d.Fee)
		computeUnitsArr = append(computeUnitsArr, d.ComputeUnitPrice)
	}
	if len(feeArr) == 0 || len(computeUnitsArr) == 0 {
		f.log.Errorf("exporter could not find non-empty TxDetails")
		return
	}

	fee, err := mathutil.Avg(feeArr...)
	if err != nil {
		f.log.Errorf("fee average: %v", err)
		return
	}
	computeUnits, err := mathutil.Avg(computeUnitsArr...)
	if err != nil {
		f.log.Errorf("computeUnits average: %v", err)
		return
	}

	f.metrics.Set(fee, computeUnits, f.label)
}

func (f *feesExporter) Cleanup(_ context.Context) {
	f.metrics.Cleanup(f.label)
}

package exporter

import (
	"context"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
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

func (p *feesExporter) Export(ctx context.Context, data interface{}) {
	details, err := types.MakeTxDetails(data)
	if err != nil {
		return // skip if input could not be parsed
	}

	// skip on no updates
	if len(details) == 0 {
		return
	}

	// calculate average of non empty TxDetails
	var count int
	var fee uint64
	var computeUnits fees.ComputeUnitPrice
	for _, d := range details {
		if d.Empty() {
			continue
		}
		count += 1
		fee += d.Fee                       // TODO: overflow
		computeUnits += d.ComputeUnitPrice // TODO: overflow
	}
	if count == 0 {
		return
	}

	// TODO: handle avg with generics?
	p.metrics.Set(fee/uint64(count), computeUnits/fees.ComputeUnitPrice(count), p.label)
}

func (p *feesExporter) Cleanup(_ context.Context) {
	p.metrics.Cleanup(p.label)
}

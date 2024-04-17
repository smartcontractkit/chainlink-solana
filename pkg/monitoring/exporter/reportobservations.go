package exporter

import (
	"context"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func NewReportObservationsFactory(
	log commonMonitoring.Logger,
	metrics metrics.ReportObservations,
) commonMonitoring.ExporterFactory {
	return &reportObservationsFactory{
		log,
		metrics,
	}
}

type reportObservationsFactory struct {
	log     commonMonitoring.Logger
	metrics metrics.ReportObservations
}

func (p *reportObservationsFactory) NewExporter(
	params commonMonitoring.ExporterParams,
) (commonMonitoring.Exporter, error) {
	return &reportObservations{
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

type reportObservations struct {
	label   metrics.FeedInput // static for each feed
	log     commonMonitoring.Logger
	metrics metrics.ReportObservations
}

func (p *reportObservations) Export(ctx context.Context, data interface{}) {
	details, err := types.MakeTxDetails(data)
	if err != nil {
		return // skip if input could not be parsed
	}

	// skip on no updates
	if len(details) == 0 {
		return
	}

	// sanity check: find non-empty detail
	// assumption: details ordered from latest -> earliest
	var latest types.TxDetails
	for _, d := range details {
		if !d.Empty() {
			latest = d
			break
		}
	}
	if latest.Empty() {
		p.log.Errorw("exporter could not find non-empty TxDetails", "feed", p.label.ToPromLabels())
		return
	}

	p.metrics.SetCount(latest.ObservationCount, p.label)
}

func (p *reportObservations) Cleanup(_ context.Context) {
	p.metrics.Cleanup(p.label)
}

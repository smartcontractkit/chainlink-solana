package exporter

import (
	"context"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/metrics"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func NewSlotHeightFactory(
	_ commonMonitoring.Logger,
	metrics metrics.SlotHeight,
) commonMonitoring.ExporterFactory {
	return &slotHeightFactory{
		metrics,
	}
}

type slotHeightFactory struct {
	metrics metrics.SlotHeight
}

func (p *slotHeightFactory) NewExporter(
	params commonMonitoring.ExporterParams,
) (commonMonitoring.Exporter, error) {
	return &slotHeight{
		params.ChainConfig.GetNetworkName(),
		params.ChainConfig.GetRPCEndpoint(),
		p.metrics,
	}, nil
}

type slotHeight struct {
	chain, url string
	metrics    metrics.SlotHeight
}

func (p *slotHeight) Export(ctx context.Context, data interface{}) {
	slot, ok := data.(types.SlotHeight)
	if !ok {
		return // skip if input could not be parsed
	}

	p.metrics.Set(slot, p.chain, p.url)
}

func (p *slotHeight) Cleanup(_ context.Context) {
	p.metrics.Cleanup()
}

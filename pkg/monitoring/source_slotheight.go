package monitoring

import (
	"context"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

func NewSlotHeightSourceFactory(
	client ChainReader,
	log commonMonitoring.Logger,
) commonMonitoring.NetworkSourceFactory {
	return &slotHeightSourceFactory{
		client,
		log,
	}
}

type slotHeightSourceFactory struct {
	client ChainReader
	log    commonMonitoring.Logger
}

func (s *slotHeightSourceFactory) NewSource(
	_ commonMonitoring.ChainConfig,
	_ []commonMonitoring.NodeConfig,
) (commonMonitoring.Source, error) {
	return &slotHeightSource{s.client}, nil
}

func (s *slotHeightSourceFactory) GetType() string {
	return types.SlotHeightType
}

type slotHeightSource struct {
	client ChainReader
}

func (t *slotHeightSource) Fetch(ctx context.Context) (interface{}, error) {
	slot, err := t.client.GetSlot(ctx)
	return types.SlotHeight(slot), err
}

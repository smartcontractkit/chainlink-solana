package headtracker

import (
	"github.com/smartcontractkit/chainlink-relay/pkg/headtracker"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-relay/pkg/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
)

type HeadBroadcaster = headtracker.HeadBroadcaster[*types.Head, types.Hash]

var _ commontypes.HeadBroadcaster[*types.Head, types.Hash] = &HeadBroadcaster{}

func NewBroadcaster(
	lggr logger.Logger,
) *HeadBroadcaster {
	return headtracker.NewHeadBroadcaster[*types.Head, types.Hash](lggr)
}

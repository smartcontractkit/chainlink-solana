package headtracker

import (
	"github.com/smartcontractkit/chainlink-relay/pkg/headtracker"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-relay/pkg/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
)

type headBroadcaster = headtracker.HeadBroadcaster[*types.Head, types.Hash]

var _ commontypes.HeadBroadcaster[*types.Head, types.Hash] = &headBroadcaster{}

func NewBroadcaster(
	lggr logger.Logger,
) *headBroadcaster {
	return headtracker.NewHeadBroadcaster[*types.Head, types.Hash](lggr)
}

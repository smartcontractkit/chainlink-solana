package headtracker

import (
	"github.com/smartcontractkit/chainlink-relay/pkg/headtracker"
	htrktypes "github.com/smartcontractkit/chainlink-relay/pkg/headtracker/types"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-relay/pkg/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
)

type HeadListener = headtracker.HeadListener[*types.Head, commontypes.Subscription, types.ChainID, types.Hash]

var _ commontypes.HeadListener[*types.Head, types.Hash] = &HeadListener{}

func NewListener(
	lggr logger.Logger,
	solanaClient htrktypes.Client[*types.Head, commontypes.Subscription, types.ChainID, types.Hash],
	config htrktypes.Config,
	chStop chan struct{},
) *HeadListener {
	return headtracker.NewHeadListener[*types.Head, commontypes.Subscription,
		types.ChainID, types.Hash](lggr, solanaClient, config, chStop)
}

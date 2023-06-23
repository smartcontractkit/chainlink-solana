package headtracker

import (
	"github.com/smartcontractkit/chainlink-relay/pkg/headtracker"
	htrktypes "github.com/smartcontractkit/chainlink-relay/pkg/headtracker/types"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-relay/pkg/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
)

type headListener = headtracker.HeadListener[*types.Head, commontypes.Subscription, types.ChainID, types.Hash]

var _ commontypes.HeadListener[*types.Head, types.Hash] = &headListener{}

func NewListener(
	lggr logger.Logger,
	solanaClient htrktypes.Client[*types.Head, commontypes.Subscription, types.ChainID, types.Hash],
	config htrktypes.Config,
	chStop chan struct{},
) *headListener {
	return headtracker.NewHeadListener[*types.Head, commontypes.Subscription,
		types.ChainID, types.Hash](lggr, solanaClient, config, chStop)
}

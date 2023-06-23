package headtracker

import (
	"github.com/smartcontractkit/chainlink-relay/pkg/headtracker"
	htrktypes "github.com/smartcontractkit/chainlink-relay/pkg/headtracker/types"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-relay/pkg/types"
	"github.com/smartcontractkit/chainlink-relay/pkg/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
)

type headTracker = headtracker.HeadTracker[*types.Head, commontypes.Subscription, types.ChainID, types.Hash]

var _ commontypes.HeadTracker[*types.Head, types.Hash] = (*headTracker)(nil)

func NewHeadTracker(
	lggr logger.Logger,
	solanaClient htrktypes.Client[*types.Head, commontypes.Subscription, types.ChainID, types.Hash],
	config htrktypes.Config,
	headBroadcaster commontypes.HeadBroadcaster[*types.Head, types.Hash],
	headSaver commontypes.HeadSaver[*types.Head, types.Hash],
	mailMon *utils.MailboxMonitor,
) commontypes.HeadTracker[*types.Head, types.Hash] {
	return headtracker.NewHeadTracker(
		lggr,
		solanaClient,
		config,
		headBroadcaster,
		headSaver,
		mailMon,
		func() *types.Head { return nil },
	)
}

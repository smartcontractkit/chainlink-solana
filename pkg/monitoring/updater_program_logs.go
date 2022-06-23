package monitoring

import (
	"context"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
)

func NewProgramLogsUpdater(
	program solana.PublicKey,
	client *ws.Client,
	commitment rpc.CommitmentType,
	log relayMonitoring.Logger,
) relayMonitoring.Updater {
	return &programLogsUpdater{
		program,
		client,
		commitment,
		log,
		make(chan interface{}),
	}
}

type programLogsUpdater struct {
	program    solana.PublicKey
	client     *ws.Client
	commitment rpc.CommitmentType
	log        relayMonitoring.Logger
	updates    chan interface{}
}

func (a *programLogsUpdater) Run(ctx context.Context) {
SUBSCRIBE_LOOP:
	for {
		a.log.Infow("subscribing to program logs")
		subscription, err := a.client.LogsSubscribeMentions(a.program, a.commitment)
		if err != nil {
			a.log.Errorw("error creating subscription to program logs")
			// TODO (dru) a better reconnect logic: exp backoff, error-specific handling.
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
				continue SUBSCRIBE_LOOP
			}
		}
		for {
			result, err := subscription.Recv()
			if err != nil {
				a.log.Errorw("error reading message from logs subscription. Disconnecting!", "error", err)
				subscription.Unsubscribe()
				select {
				case <-ctx.Done():
					return
				case <-time.After(1 * time.Second):
					continue SUBSCRIBE_LOOP
				}
			}
			select {
			case a.updates <- result:
			case <-ctx.Done():
				subscription.Unsubscribe()
				return
			}
		}
	}
}

func (a *programLogsUpdater) Updates() <-chan interface{} {
	return a.updates
}

package monitoring

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc/ws"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
)

func NewLogsUpdater(
	client *ws.Client,
	program solana.PublicKey,
	log relayMonitoring.Logger,
) Updater {
	return &logsUpdater{
		client,
		program,
		make(chan interface{}),
		log,
	}
}

type logsUpdater struct {
	client  *ws.Client
	program solana.PublicKey
	updates chan interface{}
	log     relayMonitoring.Logger
}

func (l *logsUpdater) Run(ctx context.Context) {
SUBSCRIBE_LOOP:
	for {
		subscription, err := l.client.LogsSubscribeMentions(l.program, commitment)
		if err != nil {
			l.log.Errorw("error creating logs subscription, retrying: %w", err)
			// TODO (dru) a better reconnect logic: exp backoff, error-specific handling.
			continue SUBSCRIBE_LOOP
		}
	RECEIVE_LOOP:
		for {
			result, err := subscription.Recv()
			if err != nil {
				l.log.Errorw("error reading message from subscription, reconnecting: %w", err)
				subscription.Unsubscribe()
				continue SUBSCRIBE_LOOP
			}
			log := Log{
				Slot:      result.Context.Slot,
				Signature: result.Value.Signature[:],
				Err:       result.Value.Err,
				Logs:      filterAndDeserializeLogs(result.Value.Logs),
			}
			select {
			case l.updates <- log:
			case <-ctx.Done():
				subscription.Unsubscribe()
				return
			}
		}
	}
}

func (l *logsUpdater) Updates() <-chan interface{} {
	return l.updates
}

// Helpers

func filterAndDeserializeLogs(logs []string) []string {

}

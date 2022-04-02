package monitoring

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/event"
)

func NewLogsUpdater(
	client *ws.Client,
	program solana.PublicKey,
	commitment rpc.CommitmentType,
	log relayMonitoring.Logger,
) Updater {
	return &logsUpdater{
		client,
		program,
		commitment,
		make(chan interface{}),
		log,
	}
}

type logsUpdater struct {
	client     *ws.Client
	program    solana.PublicKey
	commitment rpc.CommitmentType
	updates    chan interface{}
	log        relayMonitoring.Logger
}

func (l *logsUpdater) Run(ctx context.Context) {
SUBSCRIBE_LOOP:
	for {
		subscription, err := l.client.LogsSubscribeMentions(l.program, l.commitment)
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
			encodedEvents := event.ExtractEvents(result.Value.Logs, l.program.String())
			events, err := event.DecodeMultiple(encodedEvents)
			if err != nil {
				l.log.Errorw("error decoding events", "error", err)
				continue RECEIVE_LOOP
			}
			jsonEvents := []string{}
			for _, rawEvent := range events {
				jsonEvent, err := json.Marshal(rawEvent)
				if err != nil {
					l.log.Errorw("error encoding event as json", "error", err)
					continue RECEIVE_LOOP
				}
				jsonEvents = append(jsonEvents, string(jsonEvent))
			}
			log := Log{
				Slot:      result.Context.Slot,
				Signature: result.Value.Signature[:],
				Err:       result.Value.Err,
				Events:    jsonEvents,
			}
			fmt.Println(">>>>>>>>>", jsonEvents)
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

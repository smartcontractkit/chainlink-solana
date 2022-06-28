package ingestor

import (
	"context"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
)

func NewAccountUpdater(
	account solana.PublicKey,
	client *ws.Client,
	commitment rpc.CommitmentType,
	log relayMonitoring.Logger,
) relayMonitoring.Updater {
	return &accountUpdater{
		account,
		client,
		commitment,
		log,
		make(chan interface{}),
	}
}

type accountUpdater struct {
	account    solana.PublicKey
	client     *ws.Client
	commitment rpc.CommitmentType
	log        relayMonitoring.Logger
	updates    chan interface{}
}

func (a *accountUpdater) Run(ctx context.Context) {
SUBSCRIBE_LOOP:
	for {
		a.log.Infow("subscribing to account updates")
		subscription, err := a.client.AccountSubscribe(a.account, a.commitment)
		if err != nil {
			a.log.Errorw("error creating account subscription")
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
				a.log.Errorw("error reading message from account subscription. Disconnecting!", "error", err)
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

func (a *accountUpdater) Updates() <-chan interface{} {
	return a.updates
}

package monitoring

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc/ws"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
)

type Account struct {
	Slot       uint64
	Lamports   uint64
	Owner      solana.PublicKey
	Data       []byte
	Executable bool
	RentEpoch  uint64
}

// accountSubscriber produces a stream of Account instances.
type accountSubscriber struct {
	account solana.PublicKey
	client  *ws.Client
	updates chan interface{}
	log     relayMonitoring.Logger
}

func (a *accountSubscriber) Run(ctx context.Context) {
SUBSCRIBE_LOOP:
	for {
		subscription, err := a.client.AccountSubscribe(a.account, commitment)
		if err != nil {
			a.log.Errorw("error creating account subscription, retrying: %w", err)
			// TODO (dru) a better reconnect logic: exp backoff, error-specific handling.
			continue SUBSCRIBE_LOOP
		}
	RECEIVE_LOOP:
		for {
			result, err := subscription.Recv()
			if err != nil {
				a.log.Errorw("error reading message from subscription, reconnecting: %w", err)
				subscription.Unsubscribe()
				continue SUBSCRIBE_LOOP
			}
			var data []byte
			if result.Value.Account.Data != nil {
				data = result.Value.Account.Data.GetBinary()
			}
			account := Account{
				Slot:       result.Context.Slot,
				Lamports:   result.Value.Account.Lamports,
				Owner:      result.Value.Account.Owner,
				Data:       data,
				Executable: result.Value.Account.Executable,
				RentEpoch:  result.Value.Account.RentEpoch,
			}
			select {
			case a.updates <- account:
			case <-ctx.Done():
				subscription.Unsubscribe()
				return
			}
		}
	}
}

func (a *accountSubscriber) Updates() <-chan interface{} {
	return a.updates
}

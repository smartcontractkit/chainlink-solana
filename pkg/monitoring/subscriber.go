package monitoring

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc/ws"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
)

type Subscriber interface {
	// Run should be executed as a goroutine otherwise it will block.
	Run(context.Context)
	// You should never close the channel returned by Updates()!
	// You should always read from the channel returned by Updates() in a
	// select statement with the same context you passed to Run()
	Updates() <-chan interface{}
}

type SubscriberFactory interface {
	NewAccountSubscriber(solana.PublicKey) Subscriber
	NewLogsSubscriber(solana.PublicKey) Subscriber
}

func NewSubscriberFactory(client *ws.Client, log relayMonitoring.Logger) SubscriberFactory {
	return &subscriberFactory{client, log}
}

type subscriberFactory struct {
	client *ws.Client
	log    relayMonitoring.Logger
}

func (s *SubscriberFactory) NewAccountSubscriber(account solana.PublicKey) Subscriber {
	return &accountSubscriber{
		account,
		s.client,
		make(chan interface{}),
		s.log.With("component", "account-subscriber", "account", account.String()),
	}
}

func (s *SubscriberFactory) NewLogsSubscriber(program solana.PublicKey) Subscriber {
	return &logsSubscriber{
		program,
		client,
		make(chan interface{}),
		s.log.With("component", "logs-subscriber", "program", program.String()),
	}
}

type Mapper func(interface{}, relayMonitoring.ChainConfig, relayMonitoring.FeedConfig) map[string]interface{}

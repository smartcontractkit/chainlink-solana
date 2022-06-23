package monitoring

import (
	"context"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
)

func NewBlocksUpdater(
	program solana.PublicKey,
	client *ws.Client,
	commitment rpc.CommitmentType,
	log relayMonitoring.Logger,
) relayMonitoring.Updater {
	return &blocksUpdater{
		program,
		client,
		commitment,
		log,
		make(chan interface{}),
	}
}

type blocksUpdater struct {
	program    solana.PublicKey
	client     *ws.Client
	commitment rpc.CommitmentType
	log        relayMonitoring.Logger
	updates    chan interface{}
}

func (b *blocksUpdater) Run(ctx context.Context) {
SUBSCRIBE_LOOP:
	for {
		b.log.Infow("subscribing to blocks mentioning the aggregator program")
		filter := ws.NewBlockSubscribeFilterMentionsAccountOrProgram(b.program)
		populateRewards := true
		opts := &ws.BlockSubscribeOpts{
			Commitment:         b.commitment,
			Encoding:           solana.EncodingBase64,
			TransactionDetails: rpc.TransactionDetailsFull,
			Rewards:            &populateRewards,
		}
		subscription, err := b.client.BlockSubscribe(filter, opts)
		if err != nil {
			b.log.Errorw("error creating subscription to blocks", "error", err, "program", b.program.String())
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
				b.log.Errorw("error reading message from blocks subscription. Disconnecting!", "error", err, "program", b.program.String())
				subscription.Unsubscribe()
				select {
				case <-ctx.Done():
					return
				case <-time.After(1 * time.Second):
					continue SUBSCRIBE_LOOP
				}
			}
			select {
			case b.updates <- result:
			case <-ctx.Done():
				subscription.Unsubscribe()
				return
			}
		}
	}
}

func (b *blocksUpdater) Updates() <-chan interface{} {
	return b.updates
}

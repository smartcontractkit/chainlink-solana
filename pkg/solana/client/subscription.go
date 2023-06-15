package client

import (
	"context"

	commontypes "github.com/smartcontractkit/chainlink-solana/pkg/common/types"
)

var _ commontypes.Subscription = (*Subscription)(nil)

type Subscription struct {
	ctx     context.Context
	cancel  context.CancelFunc
	errChan chan error
	client  *Client
}

func NewSubscription(ctx context.Context, client *Client) *Subscription {
	ctx, cancel := context.WithCancel(ctx)
	return &Subscription{
		ctx:     ctx,
		cancel:  cancel,
		client:  client,
		errChan: make(chan error),
	}
}

func (s *Subscription) Unsubscribe() {
	s.cancel()
}

func (s *Subscription) Err() <-chan error {
	return s.errChan
}

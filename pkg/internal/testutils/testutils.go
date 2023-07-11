package testutils

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	clientmocks "github.com/smartcontractkit/chainlink-relay/pkg/headtracker/types/mocks"
	commontypes "github.com/smartcontractkit/chainlink-relay/pkg/types"
	commonmocks "github.com/smartcontractkit/chainlink-relay/pkg/types/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
)

// TODO: These can refactor to chainlink internal testutils

// Chain Agnostic Test Utils

// Context returns a context with the test's deadline, if available.
func Context(tb testing.TB) context.Context {
	ctx := context.Background()
	var cancel func()
	switch t := tb.(type) {
	case *testing.T:
		if d, ok := t.Deadline(); ok {
			ctx, cancel = context.WithDeadline(ctx, d)
		}
	}
	if cancel == nil {
		ctx, cancel = context.WithCancel(ctx)
	}
	tb.Cleanup(cancel)
	return ctx
}

// DefaultWaitTimeout is the default wait timeout. If you have a *testing.T, use WaitTimeout instead.
const DefaultWaitTimeout = 30 * time.Second

// WaitTimeout returns a timeout based on the test's Deadline, if available.
// Especially important to use in parallel tests, as their individual execution
// can get paused for arbitrary amounts of time.
func WaitTimeout(t *testing.T) time.Duration {
	if d, ok := t.Deadline(); ok {
		// 10% buffer for cleanup and scheduling delay
		return time.Until(d) * 9 / 10
	}
	return DefaultWaitTimeout
}

// TestInterval is just a sensible poll interval that gives fast tests without
// risk of spamming
const TestInterval = 100 * time.Millisecond

// NewHeadtrackerConfig returns a new Solana Headtracker Config with overrides.
func NewHeadtrackerConfig(config *headtracker.Config, overrideFn func(*headtracker.Config)) *headtracker.Config {
	overrideFn(config)
	return config
}

type MockChain struct {
	Client *clientmocks.Client[
		*types.Head,
		commontypes.Subscription,
		types.ChainID,
		types.Hash]

	CheckFilterLogs  func(int64, int64)
	subsMu           sync.RWMutex
	subs             []*commonmocks.Subscription
	errChs           []chan error
	subscribeCalls   atomic.Int32
	unsubscribeCalls atomic.Int32
}

func (m *MockChain) SubscribeCallCount() int32 {
	return m.subscribeCalls.Load()
}

func (m *MockChain) UnsubscribeCallCount() int32 {
	return m.unsubscribeCalls.Load()
}

func (m *MockChain) NewSub(t *testing.T) commontypes.Subscription {
	m.subscribeCalls.Add(1)
	sub := commonmocks.NewSubscription(t)
	errCh := make(chan error)
	sub.On("Err").
		Return(func() <-chan error { return errCh }).Maybe()
	sub.On("Unsubscribe").
		Run(func(mock.Arguments) {
			m.unsubscribeCalls.Add(1)
			close(errCh)
		}).Return().Maybe()
	m.subsMu.Lock()
	m.subs = append(m.subs, sub)
	m.errChs = append(m.errChs, errCh)
	m.subsMu.Unlock()
	return sub
}

func (m *MockChain) SubsErr(err error) {
	m.subsMu.Lock()
	defer m.subsMu.Unlock()
	for _, errCh := range m.errChs {
		errCh <- err
	}
}

package headtracker_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/mock"

	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	commonmocks "github.com/smartcontractkit/chainlink-relay/pkg/types/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/internal/cltest"
	"github.com/smartcontractkit/chainlink-solana/pkg/internal/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
)

func Test_HeadListener_HappyPath(t *testing.T) {
	// Logic:
	// - spawn a listener instance
	// - mock SubscribeNewHead/Err/Unsubscribe to track these calls
	// - send 3 heads
	// - ask listener to stop
	// Asserts:
	// - check Connected()/ReceivingHeads() are updated
	// - 3 heads is passed to callback
	// - ethClient methods are invoked

	lggr, _ := logger.New()
	client := cltest.NewClientMockWithDefaultChain(t)
	cfg := headtracker.NewConfig()
	chStop := make(chan struct{})
	hl := headtracker.NewListener(lggr, client, cfg, chStop)

	var headCount atomic.Int32
	handler := func(context.Context, *types.Head) error {
		headCount.Add(1)
		return nil
	}

	subscribeAwaiter := cltest.NewAwaiter()
	unsubscribeAwaiter := cltest.NewAwaiter()
	var chHeads chan<- *types.Head
	var chErr = make(chan error)
	var chSubErr <-chan error = chErr
	sub := commonmocks.NewSubscription(t)
	client.On("SubscribeNewHead", mock.Anything, mock.AnythingOfType("chan<- *types.Head")).Return(sub, nil).Once().Run(func(args mock.Arguments) {
		chHeads = args.Get(1).(chan<- *types.Head)
		subscribeAwaiter.ItHappened()
	})
	sub.On("Err").Return(chSubErr)
	sub.On("Unsubscribe").Return().Once().Run(func(mock.Arguments) {
		unsubscribeAwaiter.ItHappened()
		close(chHeads)
		close(chErr)
	})

	doneAwaiter := cltest.NewAwaiter()
	done := func() {
		doneAwaiter.ItHappened()
	}
	go hl.ListenForNewHeads(handler, done)

	subscribeAwaiter.AwaitOrFail(t, testutils.WaitTimeout(t))
	require.Eventually(t, hl.Connected, testutils.WaitTimeout(t), testutils.TestInterval)

	chHeads <- cltest.Head(0)
	chHeads <- cltest.Head(1)
	chHeads <- cltest.Head(2)

	require.True(t, hl.ReceivingHeads())

	close(chStop)
	doneAwaiter.AwaitOrFail(t)

	unsubscribeAwaiter.AwaitOrFail(t)
	require.Equal(t, int32(3), headCount.Load())
}

func Test_HeadListener_NotReceivingHeads(t *testing.T) {
	// Logic:
	// - same as Test_HeadListener_HappyPath, but
	// - send one head, make sure ReceivingHeads() is true
	// - do not send any heads within BlockEmissionIdleWarningThreshold and check ReceivingHeads() is false

	lggr, _ := logger.New()
	client := cltest.NewClientMockWithDefaultChain(t)
	cfg := headtracker.NewConfig()
	chStop := make(chan struct{})
	hl := headtracker.NewListener(lggr, client, cfg, chStop)

	firstHeadAwaiter := cltest.NewAwaiter()
	// handler := func(context.Context, *types.Head) error {
	// 	firstHeadAwaiter.ItHappened()
	// 	return nil
	// }

	var headCount atomic.Int32
	handler := func(context.Context, *types.Head) error {
		headCount.Add(1)
		return nil
	}

	subscribeAwaiter := cltest.NewAwaiter()
	unsubscribeAwaiter := cltest.NewAwaiter()
	var chHeads chan<- *types.Head
	var chErr = make(chan error)
	var chSubErr <-chan error = chErr
	sub := commonmocks.NewSubscription(t)
	client.On("SubscribeNewHead", mock.Anything, mock.AnythingOfType("chan<- *types.Head")).Return(sub, nil).Once().Run(func(args mock.Arguments) {
		chHeads = args.Get(1).(chan<- *types.Head)
		subscribeAwaiter.ItHappened()
	})
	sub.On("Err").Return(chSubErr)
	sub.On("Unsubscribe").Return().Once().Run(func(_ mock.Arguments) {
		unsubscribeAwaiter.ItHappened()
		close(chHeads)
		close(chErr)
	}) // TODO: Fix error here

	doneAwaiter := cltest.NewAwaiter()
	done := func() {
		doneAwaiter.ItHappened()
	}
	go hl.ListenForNewHeads(handler, done)

	subscribeAwaiter.AwaitOrFail(t, testutils.WaitTimeout(t))

	chHeads <- cltest.Head(0)
	firstHeadAwaiter.AwaitOrFail(t)

	require.True(t, hl.ReceivingHeads())

	time.Sleep(time.Second * 2)

	require.False(t, hl.ReceivingHeads())

	close(chStop)
	doneAwaiter.AwaitOrFail(t)
}

func Test_HeadListener_SubscriptionErr(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		closeErr bool
	}{
		{"nil error", nil, false},
		{"socket error", errors.New("close 1006 (abnormal closure): unexpected EOF"), false},
		{"close Err channel", nil, true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			lggr, _ := logger.New()
			client := cltest.NewClientMockWithDefaultChain(t)
			cfg := headtracker.NewConfig()
			chStop := make(chan struct{})
			hl := headtracker.NewListener(lggr, client, cfg, chStop)

			hnhCalled := make(chan *types.Head)
			hnh := func(_ context.Context, header *types.Head) error {
				hnhCalled <- header
				return nil
			}
			doneAwaiter := cltest.NewAwaiter()
			done := doneAwaiter.ItHappened

			chSubErrTest := make(chan error)
			var chSubErr <-chan error = chSubErrTest
			sub := commonmocks.NewSubscription(t)
			// sub.Err is called twice because we enter the select loop two times: once
			// initially and once again after exactly one head has been received
			sub.On("Err").Return(chSubErr).Twice()

			subscribeAwaiter := cltest.NewAwaiter()
			var headsCh chan<- *types.Head
			// Initial subscribe
			client.On("SubscribeNewHead", mock.Anything, mock.AnythingOfType("chan<- *types.Head")).Return(sub, nil).Once().Run(func(args mock.Arguments) {
				headsCh = args.Get(1).(chan<- *types.Head)
				subscribeAwaiter.ItHappened()
			})
			go func() {
				hl.ListenForNewHeads(hnh, done)
			}()

			// Put a head on the channel to ensure we test all code paths
			subscribeAwaiter.AwaitOrFail(t, testutils.WaitTimeout(t))
			head := cltest.Head(0)
			headsCh <- head

			h := <-hnhCalled
			assert.Equal(t, head, h)

			// Expect a call to unsubscribe on error
			sub.On("Unsubscribe").Once().Run(func(_ mock.Arguments) {
				close(headsCh)
				if !test.closeErr {
					close(chSubErrTest)
				}
			})
			// Expect a resubscribe
			chSubErrTest2 := make(chan error)
			var chSubErr2 <-chan error = chSubErrTest2
			sub2 := commonmocks.NewSubscription(t)
			sub2.On("Err").Return(chSubErr2)
			subscribeAwaiter2 := cltest.NewAwaiter()

			var headsCh2 chan<- *types.Head
			client.On("SubscribeNewHead", mock.Anything, mock.AnythingOfType("chan<- *types.Head")).Return(sub2, nil).Once().Run(func(args mock.Arguments) {
				headsCh2 = args.Get(1).(chan<- *types.Head)
				subscribeAwaiter2.ItHappened()
			})

			// Sending test error
			if test.closeErr {
				close(chSubErrTest)
			} else {
				chSubErrTest <- test.err
			}

			// Wait for it to resubscribe
			subscribeAwaiter2.AwaitOrFail(t, testutils.WaitTimeout(t))

			head2 := cltest.Head(1)
			headsCh2 <- head2

			h2 := <-hnhCalled
			assert.Equal(t, head2, h2)

			// Second call to unsubscribe on close
			sub2.On("Unsubscribe").Once().Run(func(_ mock.Arguments) {
				close(headsCh2)
				// geth guarantees that Unsubscribe closes the errors channel
				close(chSubErrTest2)
			})
			close(chStop)
			doneAwaiter.AwaitOrFail(t)
		})
	}
}

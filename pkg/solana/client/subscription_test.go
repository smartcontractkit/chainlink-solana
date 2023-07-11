package client

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/db"
	"github.com/stretchr/testify/assert"
	"github.com/test-go/testify/require"
)

func initClient(t *testing.T) (*Client, context.Context) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	url := DummyUrl(t)
	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	cfg := config.NewConfig(db.ChainCfg{}, lggr)

	c, err := NewClient(url, cfg, requestTimeout, lggr)
	require.NoError(t, err)

	return c, ctx
}

func TestSubscription_New(t *testing.T) {
	c, ctx := initClient(t)

	t.Run("happy path", func(t *testing.T) {
		subscription := NewSubscription(ctx, c)
		assert.NotNil(t, subscription)
		assert.NotNil(t, subscription.ctx)
		assert.NotNil(t, subscription.cancel)
		assert.NotNil(t, subscription.client)
		assert.NotNil(t, subscription.errChan)
	})

	// Edge case: pass a nil client
	t.Run("nil client", func(t *testing.T) {
		subscription := NewSubscription(ctx, nil)
		assert.NotNil(t, subscription)
		assert.Nil(t, subscription.client)
	})
}

func TestSubscription_Unsubscribe(t *testing.T) {
	c, ctx := initClient(t)

	t.Run("happy path", func(t *testing.T) {
		subscription := NewSubscription(ctx, c)

		// The Done channel should not be closed yet
		select {
		case <-subscription.ctx.Done():
			t.Fatal("Expected context to not be done yet")
		default:
		}

		subscription.Unsubscribe()

		select {
		// Success
		case <-subscription.ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Expected context to be done")
		}
	})

	// Edge case: unsubscribe twice
	t.Run("double unsubscribe", func(t *testing.T) {
		subscription := NewSubscription(ctx, c)
		subscription.Unsubscribe()
		subscription.Unsubscribe() // Shouldn't panic or error
	})
}

func TestSubscription_Err(t *testing.T) {
	c, ctx := initClient(t)
	t.Run("happy path", func(t *testing.T) {
		subscription := NewSubscription(ctx, c)

		errCh := subscription.Err()
		assert.NotNil(t, errCh)

		// Send an error to the error channel
		expectedError := errors.New("mock error")
		go func() {
			subscription.errChan <- expectedError
		}()

		select {
		case err := <-errCh:
			assert.ErrorIs(t, err, expectedError)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Expected error was not received")
		}
	})

	// Edge case: no error sent
	t.Run("no error", func(t *testing.T) {
		subscription := NewSubscription(ctx, c)
		errCh := subscription.Err()
		assert.NotNil(t, errCh)

		select {
		case err := <-errCh:
			t.Fatalf("Did not expect error, got %v", err)
		case <-time.After(100 * time.Millisecond):
			// Success: no error received as expected
		}
	})
}

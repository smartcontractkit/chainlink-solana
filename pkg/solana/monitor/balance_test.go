package monitor

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/internal"
)

func TestBalanceMonitor(t *testing.T) {
	const chainID = "Chainlinktest-42"
	ks := keystore{}
	for i := 0; i < 3; i++ {
		var k solana.PublicKey
		n, err := rand.Read(k[:])
		require.NoError(t, err)
		require.Equal(t, len(k), n)
		ks = append(ks, k)
	}

	bals := []uint64{0, 1, 1_000_000_000}
	expBals := []string{
		"0.000000000",
		"0.000000001",
		"1.000000000",
	}

	client := new(mocks.ReaderWriter)
	client.Test(t)
	type update struct{ acc, bal string }
	var exp []update
	for i := range bals {
		acc := ks[i]
		client.On("Balance", acc).Return(bals[i], nil)
		exp = append(exp, update{acc.String(), expBals[i]})
	}
	cfg := &config{balancePollPeriod: time.Second}
	b := newBalanceMonitor(chainID, cfg, logger.Test(t), ks, nil)
	var got []update
	done := make(chan struct{})
	b.updateFn = func(acc solana.PublicKey, lamports uint64) {
		select {
		case <-done:
			return
		default:
		}
		v := internal.LamportsToSol(lamports) // convert from lamports to SOL
		got = append(got, update{acc.String(), fmt.Sprintf("%.9f", v)})
		if len(got) == len(exp) {
			close(done)
		}
	}
	b.reader = client

	require.NoError(t, b.Start(tests.Context(t)))
	t.Cleanup(func() {
		assert.NoError(t, b.Close())
		client.AssertExpectations(t)
	})
	select {
	case <-time.After(tests.WaitTimeout(t)):
		t.Fatal("timed out waiting for balance monitor")
	case <-done:
	}

	assert.EqualValues(t, exp, got)
}

type config struct {
	balancePollPeriod time.Duration
}

func (c *config) BalancePollPeriod() time.Duration {
	return c.balancePollPeriod
}

type keystore []solana.PublicKey

func (k keystore) Accounts(ctx context.Context) (ks []string, err error) {
	for _, acc := range k {
		ks = append(ks, acc.String())
	}
	return
}

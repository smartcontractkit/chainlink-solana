//go:build !race

package testing

import (
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

func TestSetupLocalSolNode_SimultaneousNetworks(t *testing.T) {
	// run two networks
	network0 := SetupLocalSolNode(t)
	network1 := SetupLocalSolNode(t)

	account := solana.NewWallet()
	pubkey := account.PublicKey()

	// client configs
	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	cfg := config.NewDefault()

	// check & fund address
	checkFunded := func(t *testing.T, url string) {
		ctx := t.Context()
		// create client
		c, err := client.NewClient(url, cfg, requestTimeout, lggr)
		require.NoError(t, err)

		// check init balance
		bal, err := c.Balance(ctx, pubkey)
		assert.NoError(t, err)
		assert.Equal(t, uint64(0), bal)

		FundTestAccounts(t, []solana.PublicKey{pubkey}, url)

		// check end balance
		bal, err = c.Balance(ctx, pubkey)
		assert.NoError(t, err)
		assert.Equal(t, 100*solana.LAMPORTS_PER_SOL, bal) // once funds get sent to the system program it should be unrecoverable (so this number should remain > 0)
	}

	checkFunded(t, network0)
	checkFunded(t, network1)
}

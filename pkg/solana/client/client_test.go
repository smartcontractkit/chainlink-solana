package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/db"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Reader_Integration(t *testing.T) {
	url := SetupLocalSolNode(t)
	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privKey.PublicKey()
	FundTestAccounts(t, []solana.PublicKey{pubKey}, url)

	requestTimeout := 5 * time.Second
	lggr := logger.TestLogger(t)
	cfg := config.NewConfig(db.ChainCfg{}, lggr)

	c, err := NewClient(url, cfg, requestTimeout, lggr)
	require.NoError(t, err)

	// check balance
	bal, err := c.Balance(pubKey)
	assert.NoError(t, err)
	assert.Equal(t, uint64(100_000_000_000), bal) // once funds get sent to the system program it should be unrecoverable (so this number should remain > 0)

	// check SlotHeight
	slot0, err := c.SlotHeight()
	assert.NoError(t, err)
	assert.Greater(t, slot0, uint64(0))
	time.Sleep(time.Second)
	slot1, err := c.SlotHeight()
	assert.NoError(t, err)
	assert.Greater(t, slot1, slot0)

	// fetch recent blockhash
	hash, err := c.LatestBlockhash()
	assert.NoError(t, err)
	assert.NotEqual(t, hash.Value.Blockhash, solana.Hash{}) // not an empty hash

	// GetFeeForMessage (transfer to self, successful)
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(
				1,
				pubKey,
				pubKey,
			).Build(),
		},
		hash.Value.Blockhash,
		solana.TransactionPayer(pubKey),
	)
	assert.NoError(t, err)

	fee, err := c.GetFeeForMessage(tx.Message.ToBase64())
	assert.NoError(t, err)
	assert.Equal(t, uint64(5000), fee)

	// get chain ID based on gensis hash
	network, err := c.ChainID()
	assert.NoError(t, err)
	assert.Equal(t, "localnet", network)

	// get account info (also tested inside contract_test)
	res, err := c.GetAccountInfoWithOpts(context.TODO(), solana.PublicKey{}, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentFinalized})
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), res.Value.Lamports)
	assert.Equal(t, "NativeLoader1111111111111111111111111111111", res.Value.Owner.String())
}

func TestClient_Reader_ChainID(t *testing.T) {
	genesisHashes := []string{
		DevnetGenesisHash,  // devnet
		TestnetGenesisHash, // testnet
		MainnetGenesisHash, // mainnet
		"GH7ome3EiwEr7tu9JuTh2dpYWBJK3z69Xm1ZE3MEE6JC", // localnet (random)
	}
	networks := []string{"devnet", "testnet", "mainnet", "localnet"}
	hashCounter := 0

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out := fmt.Sprintf(`{"jsonrpc":"2.0","result":"%s","id":1}`, genesisHashes[hashCounter])
		hashCounter++
		_, err := w.Write([]byte(out))
		require.NoError(t, err)
	}))
	defer mockServer.Close()

	requestTimeout := 5 * time.Second
	lggr := logger.TestLogger(t)
	cfg := config.NewConfig(db.ChainCfg{}, lggr)
	c, err := NewClient(mockServer.URL, cfg, requestTimeout, lggr)
	require.NoError(t, err)

	// get chain ID based on gensis hash
	for _, n := range networks {
		network, err := c.ChainID()
		assert.NoError(t, err)
		assert.Equal(t, n, network)
	}
}

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/db"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Reader_HappyPath(t *testing.T) {
	genesisHashes := []string{
		"EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG", // devnet
		"4uhcVJyU9pJkvQyS88uRDiswHXSCkY3zQawwpjk2NsNY", // testnet
		"5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d", // mainnet
		"GH7ome3EiwEr7tu9JuTh2dpYWBJK3z69Xm1ZE3MEE6JC", // localnet (random)
	}
	networks := []string{"devnet", "testnet", "mainnet", "localnet"}
	hashCounter := 0

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// read message
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var msg jsonrpc.RPCRequest
		require.NoError(t, json.Unmarshal(body, &msg))

		var out string
		switch msg.Method {
		case "getBalance":
			out = `{"jsonrpc":"2.0","result":{"context": {"slot":1},"value": 1},"id":1}`
		case "getSlot":
			out = `{"jsonrpc":"2.0","result":1,"id":1}`
		case "getLatestBlockhash":
			out = `{"jsonrpc":"2.0","result":{"context":{"slot":1},"value":{"blockhash":"11111111111111111111111111111111","feeCalculator":{"lamportsPerSignature":1}}},"id":0}`
		case "getAccountInfo":
			out = `{"jsonrpc":"2.0","result":{"context":{"slot":1},"value":{"data":["c29sYW5hX3N5c3RlbV9wcm9ncmFt","base64"],"executable":true,"lamports":1,"owner":"11111111111111111111111111111111","rentEpoch":0}},"id":0}`
		case "getGenesisHash":
			out = fmt.Sprintf(`{"jsonrpc":"2.0","result":"%s","id":1}`, genesisHashes[hashCounter])
			hashCounter++
		default:
			out = `{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":0}`
		}

		_, err = w.Write([]byte(out))
		require.NoError(t, err)
	}))
	defer mockServer.Close()

	requestTimeout := 5 * time.Second
	lggr := logger.TestLogger(t)
	cfg := config.NewConfig(db.ChainCfg{}, lggr)
	c, err := NewClient(mockServer.URL, cfg, requestTimeout, lggr)
	require.NoError(t, err)

	// check balance
	bal, err := c.Balance(solana.PublicKey{})
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), bal) // once funds get sent to the system program it should be unrecoverable (so this number should remain > 0)

	// check SlotHeight
	slot, err := c.SlotHeight()
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), slot)

	// fetch recent blockhash
	hash, err := c.LatestBlockhash()
	assert.NoError(t, err)
	assert.Equal(t, "11111111111111111111111111111111", hash.Value.Blockhash.String())

	// get chain ID based on gensis hash
for _, n := range networks {
	network, err := c.ChainID()
	assert.NoError(t, err)
	assert.Equal(t, n, network)
}

	// get account info (also tested inside contract_test)
	res, err := c.GetAccountInfoWithOpts(context.TODO(), solana.PublicKey{}, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentFinalized})
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), res.Value.Lamports)
	assert.Equal(t, solana.PublicKey{}, res.Value.Owner)
}

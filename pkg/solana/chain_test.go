package solana

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"
	"github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	mn "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode"
	solcfg "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
)

const TestSolanaGenesisHashTemplate = `{"jsonrpc":"2.0","result":"%s","id":1}`

func TestSolanaChain_GetClient(t *testing.T) {
	checkOnce := map[string]struct{}{}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out := fmt.Sprintf(TestSolanaGenesisHashTemplate, client.MainnetGenesisHash) // mainnet genesis hash

		if !strings.Contains(r.URL.Path, "/mismatch") {
			// devnet gensis hash
			out = fmt.Sprintf(TestSolanaGenesisHashTemplate, client.DevnetGenesisHash)

			// clients with correct chainID should request chainID only once
			if _, exists := checkOnce[r.URL.Path]; exists {
				assert.NoError(t, fmt.Errorf("rpc has been called once already for successful client '%s'", r.URL.Path))
			}
			checkOnce[r.URL.Path] = struct{}{}
		}

		_, err := w.Write([]byte(out))
		require.NoError(t, err)
	}))
	defer mockServer.Close()

	ch := solcfg.Chain{}
	ch.SetDefaults()
	cfg := &solcfg.TOMLConfig{
		ChainID: ptr("devnet"),
		Chain:   ch,
	}
	cfg.SetDefaults()
	testChain := chain{
		id:          "devnet",
		cfg:         cfg,
		lggr:        logger.Test(t),
		clientCache: map[string]*verifiedCachedClient{},
	}

	cfg.Nodes = []*solcfg.Node{
		{
			Name: ptr("devnet"),
			URL:  config.MustParseURL(mockServer.URL + "/1"),
		},
		{
			Name: ptr("devnet"),
			URL:  config.MustParseURL(mockServer.URL + "/2"),
		},
	}
	_, err := testChain.getClient()
	assert.NoError(t, err)

	// random nodes (happy path, 1 valid + multiple invalid)
	cfg.Nodes = []*solcfg.Node{
		{
			Name: ptr("devnet"),
			URL:  config.MustParseURL(mockServer.URL + "/1"),
		},
		{
			Name: ptr("devnet"),
			URL:  config.MustParseURL(mockServer.URL + "/mismatch/1"),
		},
		{
			Name: ptr("devnet"),
			URL:  config.MustParseURL(mockServer.URL + "/mismatch/2"),
		},
		{
			Name: ptr("devnet"),
			URL:  config.MustParseURL(mockServer.URL + "/mismatch/3"),
		},
		{
			Name: ptr("devnet"),
			URL:  config.MustParseURL(mockServer.URL + "/mismatch/4"),
		},
	}
	_, err = testChain.getClient()
	assert.NoError(t, err)

	// empty nodes response
	cfg.Nodes = nil
	_, err = testChain.getClient()
	assert.Error(t, err)

	// no valid nodes to select from
	cfg.Nodes = []*solcfg.Node{
		{
			Name: ptr("devnet"),
			URL:  config.MustParseURL(mockServer.URL + "/mismatch/1"),
		},
		{
			Name: ptr("devnet"),
			URL:  config.MustParseURL(mockServer.URL + "/mismatch/2"),
		},
	}
	_, err = testChain.getClient()
	assert.NoError(t, err)
}

func TestSolanaChain_VerifiedClient(t *testing.T) {
	ctx := tests.Context(t)
	called := false
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out := `{ "jsonrpc": "2.0", "result": 1234, "id": 1 }` // getSlot response

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		// handle getGenesisHash request
		if strings.Contains(string(body), "getGenesisHash") {
			// should only be called once, chainID will be cached in chain
			// allowing `mismatch` to be ignored, since invalid nodes will try to verify the chain ID
			// if it is not verified
			if !strings.Contains(r.URL.Path, "/mismatch") && called {
				assert.NoError(t, errors.New("rpc has been called once already"))
			}
			// devnet genesis hash
			out = fmt.Sprintf(TestSolanaGenesisHashTemplate, client.DevnetGenesisHash)
		}
		_, err = w.Write([]byte(out))
		require.NoError(t, err)
		called = true
	}))
	defer mockServer.Close()

	ch := solcfg.Chain{}
	ch.SetDefaults()
	cfg := &solcfg.TOMLConfig{
		ChainID: ptr("devnet"),
		Chain:   ch,
	}
	cfg.SetDefaults()

	testChain := chain{
		cfg:         cfg,
		lggr:        logger.Test(t),
		clientCache: map[string]*verifiedCachedClient{},
	}
	nName := t.Name() + "-" + uuid.NewString()
	node := &solcfg.Node{
		Name: &nName,
		URL:  config.MustParseURL(mockServer.URL),
	}

	// happy path
	testChain.id = "devnet"
	_, err := testChain.verifiedClient(node)
	require.NoError(t, err)

	// retrieve cached client and retrieve slot height
	c, err := testChain.verifiedClient(node)
	require.NoError(t, err)
	slot, err := c.SlotHeight(ctx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1234), slot)

	node.URL = config.MustParseURL(mockServer.URL + "/mismatch")
	testChain.id = "incorrect"
	c, err = testChain.verifiedClient(node)
	assert.NoError(t, err)
	_, err = c.ChainID(tests.Context(t))
	// expect error from id mismatch (even if using a cached client) when performing RPC calls
	assert.Error(t, err)
	assert.Equal(t, fmt.Sprintf("client returned mismatched chain id (expected: %s, got: %s): %s", "incorrect", "devnet", node.URL), err.Error())
}

func TestSolanaChain_VerifiedClient_ParallelClients(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out := fmt.Sprintf(TestSolanaGenesisHashTemplate, client.DevnetGenesisHash)
		_, err := w.Write([]byte(out))
		require.NoError(t, err)
	}))
	defer mockServer.Close()

	ch := solcfg.Chain{}
	ch.SetDefaults()
	cfg := &solcfg.TOMLConfig{
		ChainID: ptr("devnet"),
		Enabled: ptr(true),
		Chain:   ch,
	}
	cfg.SetDefaults()
	testChain := chain{
		id:          "devnet",
		cfg:         cfg,
		lggr:        logger.Test(t),
		clientCache: map[string]*verifiedCachedClient{},
	}
	nName := t.Name() + "-" + uuid.NewString()
	node := &solcfg.Node{
		Name: &nName,
		URL:  config.MustParseURL(mockServer.URL),
	}

	var wg sync.WaitGroup
	wg.Add(2)

	var client0 client.ReaderWriter
	var client1 client.ReaderWriter
	var err0 error
	var err1 error

	// call verifiedClient in parallel
	go func() {
		client0, err0 = testChain.verifiedClient(node)
		assert.NoError(t, err0)
		wg.Done()
	}()
	go func() {
		client1, err1 = testChain.verifiedClient(node)
		assert.NoError(t, err1)
		wg.Done()
	}()

	wg.Wait()

	// check if pointers are all the same
	assert.Equal(t, testChain.clientCache[mockServer.URL], client0)
	assert.Equal(t, testChain.clientCache[mockServer.URL], client1)
}

func ptr[T any](t T) *T {
	return &t
}

func TestChain_Transact(t *testing.T) {
	ctx := tests.Context(t)
	url := client.SetupLocalSolNode(t)
	lgr, logs := logger.TestObserved(t, zapcore.DebugLevel)

	// transaction parameters
	sender, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	receiver, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	amount := big.NewInt(100_000_000_000 - 5_000) // total balance - tx fee
	client.FundTestAccounts(t, solana.PublicKeySlice{sender.PublicKey()}, url)

	// configuration
	cfg := solcfg.NewDefault()
	cfg.Nodes = append(cfg.Nodes, &solcfg.Node{
		Name:     ptr("localnet-" + t.Name()),
		URL:      config.MustParseURL(url),
		SendOnly: false,
	})

	// mocked keystore
	mkey := mocks.NewSimpleKeystore(t)
	mkey.On("Sign", mock.Anything, sender.PublicKey().String(), mock.Anything).Return(func(_ context.Context, _ string, data []byte) []byte {
		sig, _ := sender.Sign(data)
		return sig[:]
	}, nil)

	c, err := newChain("localnet", cfg, mkey, lgr)
	require.NoError(t, err)
	require.NoError(t, c.txm.Start(ctx))

	require.NoError(t, c.Transact(ctx, sender.PublicKey().String(), receiver.PublicKey().String(), amount, true))
	tests.AssertLogEventually(t, logs, "tx state: confirmed")
	tests.AssertLogEventually(t, logs, "stopped tx retry")
	require.NoError(t, c.txm.Close())

	filteredLogs := logs.FilterMessage("tx state: confirmed").All()
	require.Len(t, filteredLogs, 1)
	sig, ok := filteredLogs[0].ContextMap()["signature"]
	require.True(t, ok)

	// inspect transaction
	solClient := rpc.New(url)
	res, err := solClient.GetTransaction(ctx, solana.MustSignatureFromBase58(sig.(string)), &rpc.GetTransactionOpts{Commitment: "confirmed"})
	require.NoError(t, err)
	require.Nil(t, res.Meta.Err) // no error

	// validate balances change as expected
	require.Equal(t, amount.Uint64()+5_000, res.Meta.PreBalances[0])
	require.Zero(t, res.Meta.PostBalances[0])
	require.Zero(t, res.Meta.PreBalances[1])
	require.Equal(t, amount.Uint64(), res.Meta.PostBalances[1])

	tx, err := res.Transaction.GetTransaction()
	require.NoError(t, err)
	require.Len(t, tx.Message.Instructions, 3)
	price, err := fees.ParseComputeUnitPrice(tx.Message.Instructions[0].Data)
	require.NoError(t, err)
	assert.Equal(t, fees.ComputeUnitPrice(0), price)
	limit, err := fees.ParseComputeUnitLimit(tx.Message.Instructions[2].Data)
	require.NoError(t, err)
	assert.Equal(t, fees.ComputeUnitLimit(500), limit)
}

func TestSolanaChain_MultiNode_GetClient(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out := fmt.Sprintf(TestSolanaGenesisHashTemplate, client.MainnetGenesisHash) // mainnet genesis hash
		if !strings.Contains(r.URL.Path, "/mismatch") {
			// devnet gensis hash
			out = fmt.Sprintf(TestSolanaGenesisHashTemplate, client.DevnetGenesisHash)
		}
		_, err := w.Write([]byte(out))
		require.NoError(t, err)
	}))
	defer mockServer.Close()

	ch := solcfg.Chain{}
	ch.SetDefaults()
	mnCfg := solcfg.MultiNodeConfig{
		MultiNode: solcfg.MultiNode{
			Enabled: ptr(true),
		},
	}
	mnCfg.SetDefaults()

	cfg := &solcfg.TOMLConfig{
		ChainID:   ptr("devnet"),
		Chain:     ch,
		MultiNode: mnCfg,
	}
	cfg.Nodes = []*solcfg.Node{
		{
			Name: ptr("devnet"),
			URL:  config.MustParseURL(mockServer.URL + "/1"),
		},
		{
			Name: ptr("devnet"),
			URL:  config.MustParseURL(mockServer.URL + "/2"),
		},
	}

	testChain, err := newChain("devnet", cfg, nil, logger.Test(t))
	require.NoError(t, err)

	err = testChain.Start(tests.Context(t))
	require.NoError(t, err)
	defer func() {
		closeErr := testChain.Close()
		require.NoError(t, closeErr)
	}()

	selectedClient, err := testChain.getClient()
	assert.NoError(t, err)

	id, err := selectedClient.ChainID(tests.Context(t))
	assert.NoError(t, err)
	assert.Equal(t, "devnet", id.String())
}

func TestChain_MultiNode_TransactionSender(t *testing.T) {
	ctx := tests.Context(t)
	url := client.SetupLocalSolNode(t)
	lgr, _ := logger.TestObserved(t, zapcore.DebugLevel)

	// transaction parameters
	sender, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	receiver, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	client.FundTestAccounts(t, solana.PublicKeySlice{sender.PublicKey()}, url)

	// configuration
	cfg := solcfg.NewDefault()
	cfg.MultiNode.MultiNode.Enabled = ptr(true)
	cfg.Nodes = append(cfg.Nodes,
		&solcfg.Node{
			Name:     ptr("localnet-" + t.Name() + "-primary"),
			URL:      config.MustParseURL(client.SetupLocalSolNode(t)),
			SendOnly: false,
		})

	// mocked keystore
	mkey := mocks.NewSimpleKeystore(t)
	c, err := newChain("localnet", cfg, mkey, lgr)
	require.NoError(t, err)
	require.NoError(t, c.Start(ctx))
	defer func() {
		require.NoError(t, c.Close())
	}()

	createTx := func(from solana.PrivateKey, to solana.PrivateKey) *solana.Transaction {
		cl, err := c.getClient()
		require.NoError(t, err)

		hash, hashErr := cl.LatestBlockhash(tests.Context(t))
		assert.NoError(t, hashErr)

		tx, txErr := solana.NewTransaction(
			[]solana.Instruction{
				system.NewTransferInstruction(
					1,
					from.PublicKey(),
					to.PublicKey(),
				).Build(),
			},
			hash.Value.Blockhash,
			solana.TransactionPayer(from.PublicKey()),
		)
		assert.NoError(t, txErr)
		_, signErr := tx.Sign(
			func(key solana.PublicKey) *solana.PrivateKey {
				if from.PublicKey().Equals(key) {
					return &from
				}
				return nil
			},
		)
		assert.NoError(t, signErr)
		return tx
	}

	t.Run("successful transaction", func(t *testing.T) {
		// Send tx using transaction sender
		result := c.txSender.SendTransaction(ctx, createTx(sender, receiver))
		require.NotNil(t, result)
		require.NoError(t, result.Error())
		require.Equal(t, mn.Successful, result.Code())
		require.NotEmpty(t, result.Signature())
	})

	t.Run("unsigned transaction error", func(t *testing.T) {
		// create + sign transaction
		unsignedTx := func(to solana.PublicKey) *solana.Transaction {
			cl, err := c.getClient()
			require.NoError(t, err)

			hash, hashErr := cl.LatestBlockhash(tests.Context(t))
			assert.NoError(t, hashErr)

			tx, txErr := solana.NewTransaction(
				[]solana.Instruction{
					system.NewTransferInstruction(
						1,
						sender.PublicKey(),
						to,
					).Build(),
				},
				hash.Value.Blockhash,
				solana.TransactionPayer(sender.PublicKey()),
			)
			assert.NoError(t, txErr)
			return tx
		}

		// Send tx using transaction sender
		result := c.txSender.SendTransaction(ctx, unsignedTx(receiver.PublicKey()))
		require.NotNil(t, result)
		require.NoError(t, result.Error())
		require.Error(t, result.TxError())
		require.Equal(t, mn.Fatal, result.Code())
		require.Empty(t, result.Signature())
	})

	t.Run("empty transaction", func(t *testing.T) {
		result := c.txSender.SendTransaction(ctx, &solana.Transaction{})
		require.NotNil(t, result)
		require.NoError(t, result.Error())
		require.Error(t, result.TxError())
		require.Equal(t, mn.Fatal, result.Code())
		require.Empty(t, result.Signature())
	})
}

func TestSolanaChain_MultiNode_Txm(t *testing.T) {
	cfg := solcfg.NewDefault()
	cfg.MultiNode.MultiNode.Enabled = ptr(true)
	cfg.Nodes = []*solcfg.Node{
		{
			Name: ptr("primary"),
			URL:  config.MustParseURL(client.SetupLocalSolNode(t)),
		},
	}

	// setup keys
	key, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := key.PublicKey()

	// setup receiver key
	privKeyReceiver, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKeyReceiver := privKeyReceiver.PublicKey()

	// mocked keystore
	mkey := mocks.NewSimpleKeystore(t)
	mkey.On("Sign", mock.Anything, pubKey.String(), mock.Anything).Return(func(_ context.Context, _ string, data []byte) []byte {
		sig, _ := key.Sign(data)
		return sig[:]
	}, nil)
	mkey.On("Sign", mock.Anything, pubKeyReceiver.String(), mock.Anything).Return([]byte{}, config.KeyNotFoundError{ID: pubKeyReceiver.String(), KeyType: "Solana"})

	testChain, err := newChain("localnet", cfg, mkey, logger.Test(t))
	require.NoError(t, err)

	err = testChain.Start(tests.Context(t))
	require.NoError(t, err)
	defer func() {
		require.NoError(t, testChain.Close())
	}()

	// fund keys
	client.FundTestAccounts(t, []solana.PublicKey{pubKey}, cfg.Nodes[0].URL.String())

	// track initial balance
	selectedClient, err := testChain.getClient()
	require.NoError(t, err)
	receiverBal, err := selectedClient.Balance(tests.Context(t), pubKeyReceiver)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), receiverBal)

	createTx := func(signer solana.PublicKey, sender solana.PublicKey, receiver solana.PublicKey, amt uint64) *solana.Transaction {
		selectedClient, err = testChain.getClient()
		assert.NoError(t, err)
		hash, hashErr := selectedClient.LatestBlockhash(tests.Context(t))
		assert.NoError(t, hashErr)
		tx, txErr := solana.NewTransaction(
			[]solana.Instruction{
				system.NewTransferInstruction(
					amt,
					sender,
					receiver,
				).Build(),
			},
			hash.Value.Blockhash,
			solana.TransactionPayer(signer),
		)
		require.NoError(t, txErr)
		return tx
	}

	// Send funds twice, along with an invalid transaction
	require.NoError(t, testChain.txm.Enqueue(tests.Context(t), "test_success", createTx(pubKey, pubKey, pubKeyReceiver, solana.LAMPORTS_PER_SOL)))

	// Wait for new block hash
	currentBh, err := selectedClient.LatestBlockhash(tests.Context(t))
	require.NoError(t, err)
	timeout := time.After(time.Minute)

NewBlockHash:
	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for new block hash")
		default:
			newBh, bhErr := selectedClient.LatestBlockhash(tests.Context(t))
			require.NoError(t, bhErr)
			if newBh.Value.LastValidBlockHeight > currentBh.Value.LastValidBlockHeight {
				break NewBlockHash
			}
		}
	}

	require.NoError(t, testChain.txm.Enqueue(tests.Context(t), "test_success_2", createTx(pubKey, pubKey, pubKeyReceiver, solana.LAMPORTS_PER_SOL)))
	require.Error(t, testChain.txm.Enqueue(tests.Context(t), "test_invalidSigner", createTx(pubKeyReceiver, pubKey, pubKeyReceiver, solana.LAMPORTS_PER_SOL))) // cannot sign tx before enqueuing

	// wait for all txes to finish
	ctx, cancel := context.WithCancel(tests.Context(t))
	t.Cleanup(cancel)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
loop:
	for {
		select {
		case <-ctx.Done():
			assert.Equal(t, 0, testChain.txm.InflightTxs())
			break loop
		case <-ticker.C:
			if testChain.txm.InflightTxs() == 0 {
				cancel() // exit for loop
			}
		}
	}

	// verify funds were transferred through transaction sender
	selectedClient, err = testChain.getClient()
	assert.NoError(t, err)
	receiverBal, err = selectedClient.Balance(tests.Context(t), pubKeyReceiver)
	assert.NoError(t, err)
	require.Equal(t, 2*solana.LAMPORTS_PER_SOL, receiverBal)
}

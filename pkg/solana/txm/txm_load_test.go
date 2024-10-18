//go:build integration

package txm_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/services/servicetest"
	solanaClient "github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/internal"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	keyMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"

	relayconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

func TestTxm_Integration(t *testing.T) {
	for _, eName := range []string{"fixed", "blockhistory"} {
		estimator := eName
		t.Run("estimator-"+estimator, func(t *testing.T) {
			t.Parallel() // run estimator tests in parallel

			ctx := tests.Context(t)
			url := solanaClient.SetupLocalSolNode(t)

			// setup key
			key, err := solana.NewRandomPrivateKey()
			require.NoError(t, err)
			pubKey := key.PublicKey()

			// setup load test key
			loadTestKey, err := solana.NewRandomPrivateKey()
			require.NoError(t, err)

			// setup receiver key
			privKeyReceiver, err := solana.NewRandomPrivateKey()
			pubKeyReceiver := privKeyReceiver.PublicKey()

			// fund keys
			solanaClient.FundTestAccounts(t, []solana.PublicKey{pubKey, loadTestKey.PublicKey()}, url)

			// setup mock keystore
			mkey := keyMocks.NewSimpleKeystore(t)
			mkey.On("Sign", mock.Anything, key.PublicKey().String(), mock.Anything).Return(func(_ context.Context, _ string, data []byte) []byte {
				sig, _ := key.Sign(data)
				return sig[:]
			}, nil)
			mkey.On("Sign", mock.Anything, loadTestKey.PublicKey().String(), mock.Anything).Return(func(_ context.Context, _ string, data []byte) []byte {
				sig, _ := loadTestKey.Sign(data)
				return sig[:]
			}, nil)
			mkey.On("Sign", mock.Anything, pubKeyReceiver.String(), mock.Anything).Return([]byte{}, relayconfig.KeyNotFoundError{ID: pubKeyReceiver.String(), KeyType: "Solana"})

			// set up txm
			lggr := logger.Test(t)
			require.NoError(t, err)
			cfg := config.NewDefault()
			cfg.Chain.ConfirmPollPeriod = relayconfig.MustNewDuration(500 * time.Millisecond)
			cfg.Chain.FeeEstimatorMode = &estimator
			client, err := solanaClient.NewClient(url, cfg, 2*time.Second, lggr)
			require.NoError(t, err)
			getClient := func() (solanaClient.ReaderWriter, error) {
				return client, nil
			}
			loader := internal.NewLoader(true, getClient)
			txm := txm.NewTxm("localnet", loader, nil, cfg, mkey, lggr)

			// track initial balance
			initBal, err := client.Balance(ctx, pubKey)
			assert.NoError(t, err)
			assert.NotEqual(t, uint64(0), initBal) // should be funded

			servicetest.Run(t, txm)

			// already started
			assert.Error(t, txm.Start(ctx))

			createTx := func(signer solana.PublicKey, sender solana.PublicKey, receiver solana.PublicKey, amt uint64) *solana.Transaction {
				// create transfer tx
				hash, err := client.LatestBlockhash(ctx)
				assert.NoError(t, err)
				tx, err := solana.NewTransaction(
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
				require.NoError(t, err)
				return tx
			}

			// enqueue txs (must pass to move on to load test)
			require.NoError(t, txm.Enqueue(ctx, "test_success_0", createTx(pubKey, pubKey, pubKeyReceiver, solana.LAMPORTS_PER_SOL)))
			require.Error(t, txm.Enqueue(ctx, "test_invalidSigner", createTx(pubKeyReceiver, pubKey, pubKeyReceiver, solana.LAMPORTS_PER_SOL))) // cannot sign tx before enqueuing
			require.NoError(t, txm.Enqueue(ctx, "test_invalidReceiver", createTx(pubKey, pubKey, solana.PublicKey{}, solana.LAMPORTS_PER_SOL)))
			time.Sleep(500 * time.Millisecond) // pause 0.5s for new blockhash
			require.NoError(t, txm.Enqueue(ctx, "test_success_1", createTx(pubKey, pubKey, pubKeyReceiver, solana.LAMPORTS_PER_SOL)))
			require.NoError(t, txm.Enqueue(ctx, "test_txFail", createTx(pubKey, pubKey, pubKeyReceiver, 1000*solana.LAMPORTS_PER_SOL)))

			// load test: try to overload txs, confirm, or simulation
			for i := 0; i < 1000; i++ {
				assert.NoError(t, txm.Enqueue(ctx, fmt.Sprintf("load_%d", i), createTx(loadTestKey.PublicKey(), loadTestKey.PublicKey(), loadTestKey.PublicKey(), uint64(i))))
				time.Sleep(10 * time.Millisecond) // ~100 txs per second (note: have run 5ms delays for ~200tx/s succesfully)
			}

			// check to make sure all txs are closed out from inflight list (longest should last MaxConfirmTimeout)
			require.Eventually(t, func() bool {
				txs := txm.InflightTxs()
				t.Logf("Inflight txs: %d", txs)
				return txs == 0
			}, tests.WaitTimeout(t), time.Second)

			// check balance changes
			senderBal, err := client.Balance(ctx, pubKey)
			if assert.NoError(t, err) {
				assert.Greater(t, initBal, senderBal)
				assert.Greater(t, initBal-senderBal, 2*solana.LAMPORTS_PER_SOL) // balance change = sent + fees
			}

			receiverBal, err := client.Balance(ctx, pubKeyReceiver)
			if assert.NoError(t, err) {
				assert.Equal(t, 2*solana.LAMPORTS_PER_SOL, receiverBal)
			}
		})
	}
}

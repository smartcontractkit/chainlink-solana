//go:build integration

package txm

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	keyMocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"

	relayconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

type soltxmProm struct {
	id                                                        string
	success, error, revert, reject, drop, simRevert, simOther float64
}

func (p soltxmProm) assertEqual(t *testing.T) {
	assert.Equal(t, p.success, testutil.ToFloat64(promSolTxmSuccessTxs.WithLabelValues(p.id)), "mismatch: success")
	assert.Equal(t, p.error, testutil.ToFloat64(promSolTxmErrorTxs.WithLabelValues(p.id)), "mismatch: error")
	assert.Equal(t, p.revert, testutil.ToFloat64(promSolTxmRevertTxs.WithLabelValues(p.id)), "mismatch: revert")
	assert.Equal(t, p.reject, testutil.ToFloat64(promSolTxmRejectTxs.WithLabelValues(p.id)), "mismatch: reject")
	assert.Equal(t, p.drop, testutil.ToFloat64(promSolTxmDropTxs.WithLabelValues(p.id)), "mismatch: drop")
	assert.Equal(t, p.simRevert, testutil.ToFloat64(promSolTxmSimRevertTxs.WithLabelValues(p.id)), "mismatch: simRevert")
	assert.Equal(t, p.simOther, testutil.ToFloat64(promSolTxmSimOtherTxs.WithLabelValues(p.id)), "mismatch: simOther")
}

func (p soltxmProm) getInflight() float64 {
	return testutil.ToFloat64(promSolTxmPendingTxs.WithLabelValues(p.id))
}

// create placeholder transaction and returns func for signed tx with fee
func getTx(t *testing.T, val uint64, keystore SimpleKeystore, price fees.ComputeUnitPrice) (*solana.Transaction, func(fees.ComputeUnitPrice, bool) *solana.Transaction) {
	pubkey := solana.PublicKey{}

	// create transfer tx
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(
				val,
				pubkey,
				pubkey,
			).Build(),
		},
		solana.Hash{},
		solana.TransactionPayer(pubkey),
	)
	require.NoError(t, err)

	base := *tx // tx to send to txm, txm will add fee & sign

	return &base, func(price fees.ComputeUnitPrice, addLimit bool) *solana.Transaction {
		tx := base
		// add fee parameters
		require.NoError(t, fees.SetComputeUnitPrice(&tx, price))
		if addLimit {
			require.NoError(t, fees.SetComputeUnitLimit(&tx, 200_000)) // default
		}

		// sign tx
		txMsg, err := tx.Message.MarshalBinary()
		require.NoError(t, err)
		sigBytes, err := keystore.Sign(context.Background(), pubkey.String(), txMsg)
		require.NoError(t, err)
		var finalSig [64]byte
		copy(finalSig[:], sigBytes)
		tx.Signatures = append(tx.Signatures, finalSig)
		return &tx
	}
}

func TestTxm(t *testing.T) {
	for _, eName := range []string{"fixed", "latestblockhistory", "multipleblockshistory"} {
		estimator := eName
		t.Run("estimator-"+estimator, func(t *testing.T) {
			t.Parallel() // run estimator tests in parallel

			// set up configs needed in txm
			id := "mocknet-" + estimator + "-" + uuid.NewString()
			t.Logf("Starting new iteration: %s", id)

			ctx := tests.Context(t)
			lggr := logger.Test(t)
			cfg := config.NewDefault()
			cfg.Chain.FeeEstimatorMode = &estimator
			mc := mocks.NewReaderWriter(t)
			mc.On("GetLatestBlock", mock.Anything).Return(&rpc.GetBlockResult{}, nil).Maybe()
			mc.On("SlotHeight", mock.Anything).Return(uint64(0)).Maybe()

			// mock solana keystore
			mkey := keyMocks.NewSimpleKeystore(t)
			mkey.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil)

			txm := NewTxm(id, func() (client.ReaderWriter, error) {
				return mc, nil
			}, cfg, mkey, lggr)
			require.NoError(t, txm.Start(ctx))

			// tracking prom metrics
			prom := soltxmProm{id: id}

			// create random signature
			getSig := func() solana.Signature {
				sig := make([]byte, 64)
				rand.Read(sig)
				return solana.SignatureFromBytes(sig)
			}

			// check if cached transaction is cleared
			empty := func() bool {
				count := txm.InflightTxs()
				assert.Equal(t, float64(count), prom.getInflight()) // validate prom metric and txs length
				return count == 0
			}

			// adjust wait time based on config
			waitDuration := cfg.TxConfirmTimeout()
			waitFor := func(f func() bool) {
				for i := 0; i < int(waitDuration.Seconds()*1.5); i++ {
					if f() {
						return
					}
					time.Sleep(time.Second)
				}
				assert.NoError(t, errors.New("unable to confirm inflight txs is empty"))
			}

			// handle signature statuses calls
			statuses := map[solana.Signature]func() *rpc.SignatureStatusesResult{}
			mc.On("SignatureStatuses", mock.Anything, mock.AnythingOfType("[]solana.Signature")).Return(
				func(_ context.Context, sigs []solana.Signature) (out []*rpc.SignatureStatusesResult) {
					for i := range sigs {
						get, exists := statuses[sigs[i]]
						if !exists {
							out = append(out, nil)
							continue
						}
						out = append(out, get())
					}
					return out
				}, nil,
			)

			// happy path (send => simulate success => tx: nil => tx: processed => tx: confirmed => done)
			t.Run("happyPath", func(t *testing.T) {
				sig := getSig()
				tx, signed := getTx(t, 0, mkey, 0)
				var wg sync.WaitGroup
				wg.Add(3)

				sendCount := 0
				var countRW sync.RWMutex
				mc.On("SendTx", mock.Anything, signed(0, true)).Run(func(mock.Arguments) {
					countRW.Lock()
					sendCount++
					countRW.Unlock()
				}).After(500*time.Millisecond).Return(sig, nil)
				mc.On("SimulateTx", mock.Anything, signed(0, true), mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Once()

				// handle signature status calls
				count := 0
				statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
					defer func() { count++ }()
					defer wg.Done()

					out = &rpc.SignatureStatusesResult{}
					if count == 1 {
						out.ConfirmationStatus = rpc.ConfirmationStatusProcessed
						return
					}

					if count == 2 {
						out.ConfirmationStatus = rpc.ConfirmationStatusConfirmed
						return
					}
					return nil
				}

				// send tx
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx))
				wg.Wait()

				// no transactions stored inflight txs list
				waitFor(empty)
				// transaction should be sent more than twice
				countRW.RLock()
				t.Logf("sendTx received %d calls", sendCount)
				assert.Greater(t, sendCount, 2)
				countRW.RUnlock()

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()

				// check prom metric
				prom.success++
				prom.assertEqual(t)
			})

			// fail on initial transmit (RPC immediate rejects)
			t.Run("fail_initialTx", func(t *testing.T) {
				tx, signed := getTx(t, 1, mkey, 0)
				var wg sync.WaitGroup
				wg.Add(1)

				// should only be called once (tx does not start retry, confirming, or simulation)
				mc.On("SendTx", mock.Anything, signed(0, true)).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(solana.Signature{}, errors.New("FAIL")).Once()

				// tx should be able to queue
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx))
				wg.Wait() // wait to be picked up and processed

				// no transactions stored inflight txs list
				waitFor(empty)

				// check prom metric
				prom.error++
				prom.reject++
				prom.assertEqual(t)
			})

			// tx fails simulation (simulation error)
			t.Run("fail_simulation", func(t *testing.T) {
				tx, signed := getTx(t, 2, mkey, 0)
				sig := getSig()
				var wg sync.WaitGroup
				wg.Add(1)

				mc.On("SendTx", mock.Anything, signed(0, true)).Return(sig, nil)
				mc.On("SimulateTx", mock.Anything, signed(0, true), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{
					Err: "FAIL",
				}, nil).Once()
				// signature status is nil (handled automatically)

				// tx should be able to queue
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx))
				wg.Wait()      // wait to be picked up and processed
				waitFor(empty) // txs cleared quickly

				// check prom metric
				prom.error++
				prom.simOther++
				prom.assertEqual(t)
			})

			// tx fails simulation (rpc error, timeout should clean up b/c sig status will be nil)
			t.Run("fail_simulation_confirmNil", func(t *testing.T) {
				tx, signed := getTx(t, 3, mkey, 0)
				sig := getSig()
				retry0 := getSig()
				retry1 := getSig()
				retry2 := getSig()
				retry3 := getSig()
				var wg sync.WaitGroup
				wg.Add(1)

				mc.On("SendTx", mock.Anything, signed(0, true)).Return(sig, nil)
				mc.On("SendTx", mock.Anything, signed(1, true)).Return(retry0, nil)
				mc.On("SendTx", mock.Anything, signed(2, true)).Return(retry1, nil)
				mc.On("SendTx", mock.Anything, signed(3, true)).Return(retry2, nil).Maybe()
				mc.On("SendTx", mock.Anything, signed(4, true)).Return(retry3, nil).Maybe()
				mc.On("SimulateTx", mock.Anything, signed(0, true), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{}, errors.New("FAIL")).Once()
				// all signature statuses are nil, handled automatically

				// tx should be able to queue
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx))
				wg.Wait()      // wait to be picked up and processed
				waitFor(empty) // txs cleared after timeout

				// check prom metric
				prom.error++
				prom.drop++
				prom.assertEqual(t)

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx fails simulation with an InstructionError (indicates reverted execution)
			// manager should cancel sending retry immediately + increment reverted prom metric
			t.Run("fail_simulation_instructionError", func(t *testing.T) {
				tx, signed := getTx(t, 4, mkey, 0)
				sig := getSig()
				var wg sync.WaitGroup
				wg.Add(1)

				// {"InstructionError":[0,{"Custom":6003}]}
				tempErr := map[string][]interface{}{
					"InstructionError": {
						0, map[string]int{"Custom": 6003},
					},
				}
				mc.On("SendTx", mock.Anything, signed(0, true)).Return(sig, nil)
				mc.On("SimulateTx", mock.Anything, signed(0, true), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{
					Err: tempErr,
				}, nil).Once()
				// all signature statuses are nil, handled automatically

				// tx should be able to queue
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx))
				wg.Wait()      // wait to be picked up and processed
				waitFor(empty) // txs cleared after timeout

				// check prom metric
				prom.error++
				prom.simRevert++
				prom.assertEqual(t)

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx fails simulation with BlockHashNotFound error
			// txm should continue to confirm tx (in this case it will succeed)
			t.Run("fail_simulation_blockhashNotFound", func(t *testing.T) {
				tx, signed := getTx(t, 5, mkey, 0)
				sig := getSig()
				var wg sync.WaitGroup
				wg.Add(3)

				mc.On("SendTx", mock.Anything, signed(0, true)).Return(sig, nil)
				mc.On("SimulateTx", mock.Anything, signed(0, true), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{
					Err: "BlockhashNotFound",
				}, nil).Once()

				// handle signature status calls
				count := 0
				statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
					defer func() { count++ }()
					defer wg.Done()

					out = &rpc.SignatureStatusesResult{}
					if count == 1 {
						out.ConfirmationStatus = rpc.ConfirmationStatusConfirmed
						return
					}
					return nil
				}

				// tx should be able to queue
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx))
				wg.Wait()      // wait to be picked up and processed
				waitFor(empty) // txs cleared after timeout

				// check prom metric
				prom.success++
				prom.assertEqual(t)

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx fails simulation with AlreadyProcessed error
			// txm should continue to confirm tx (in this case it will revert)
			t.Run("fail_simulation_alreadyProcessed", func(t *testing.T) {
				tx, signed := getTx(t, 6, mkey, 0)
				sig := getSig()
				var wg sync.WaitGroup
				wg.Add(2)

				mc.On("SendTx", mock.Anything, signed(0, true)).Return(sig, nil)
				mc.On("SimulateTx", mock.Anything, signed(0, true), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{
					Err: "AlreadyProcessed",
				}, nil).Once()

				// handle signature status calls
				statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
					wg.Done()
					return &rpc.SignatureStatusesResult{
						Err:                "ERROR",
						ConfirmationStatus: rpc.ConfirmationStatusConfirmed,
					}
				}

				// tx should be able to queue
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx))
				wg.Wait()      // wait to be picked up and processed
				waitFor(empty) // txs cleared after timeout

				// check prom metric
				prom.revert++
				prom.error++
				prom.assertEqual(t)

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx passes sim, never passes processed (timeout should cleanup)
			t.Run("fail_confirm_processed", func(t *testing.T) {
				tx, signed := getTx(t, 7, mkey, 0)
				sig := getSig()
				retry0 := getSig()
				retry1 := getSig()
				retry2 := getSig()
				retry3 := getSig()
				var wg sync.WaitGroup
				wg.Add(1)

				mc.On("SendTx", mock.Anything, signed(0, true)).Return(sig, nil)
				mc.On("SendTx", mock.Anything, signed(1, true)).Return(retry0, nil)
				mc.On("SendTx", mock.Anything, signed(2, true)).Return(retry1, nil)
				mc.On("SendTx", mock.Anything, signed(3, true)).Return(retry2, nil).Maybe()
				mc.On("SendTx", mock.Anything, signed(4, true)).Return(retry3, nil).Maybe()
				mc.On("SimulateTx", mock.Anything, signed(0, true), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{}, nil).Once()

				// handle signature status calls (initial stays processed, others don't exist)
				statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
					return &rpc.SignatureStatusesResult{
						ConfirmationStatus: rpc.ConfirmationStatusProcessed,
					}
				}

				// tx should be able to queue
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx))
				wg.Wait()      // wait to be picked up and processed
				waitFor(empty) // inflight txs cleared after timeout

				// check prom metric
				prom.error++
				prom.drop++
				prom.assertEqual(t)

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx passes sim, shows processed, moves to nil (timeout should cleanup)
			t.Run("fail_confirm_processedToNil", func(t *testing.T) {
				tx, signed := getTx(t, 8, mkey, 0)
				sig := getSig()
				retry0 := getSig()
				retry1 := getSig()
				retry2 := getSig()
				retry3 := getSig()
				var wg sync.WaitGroup
				wg.Add(1)

				mc.On("SendTx", mock.Anything, signed(0, true)).Return(sig, nil)
				mc.On("SendTx", mock.Anything, signed(1, true)).Return(retry0, nil)
				mc.On("SendTx", mock.Anything, signed(2, true)).Return(retry1, nil)
				mc.On("SendTx", mock.Anything, signed(3, true)).Return(retry2, nil).Maybe()
				mc.On("SendTx", mock.Anything, signed(4, true)).Return(retry3, nil).Maybe()
				mc.On("SimulateTx", mock.Anything, signed(0, true), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{}, nil).Once()

				// handle signature status calls (initial stays processed => nil, others don't exist)
				count := 0
				statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
					defer func() { count++ }()

					if count > 2 {
						return nil
					}

					return &rpc.SignatureStatusesResult{
						ConfirmationStatus: rpc.ConfirmationStatusProcessed,
					}
				}

				// tx should be able to queue
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx))
				wg.Wait()      // wait to be picked up and processed
				waitFor(empty) // inflight txs cleared after timeout

				// check prom metric
				prom.error++
				prom.drop++
				prom.assertEqual(t)

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx passes sim, errors on confirm
			t.Run("fail_confirm_revert", func(t *testing.T) {
				tx, signed := getTx(t, 9, mkey, 0)
				sig := getSig()
				var wg sync.WaitGroup
				wg.Add(1)

				mc.On("SendTx", mock.Anything, signed(0, true)).Return(sig, nil)
				mc.On("SimulateTx", mock.Anything, signed(0, true), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{}, nil).Once()

				// handle signature status calls
				statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
					return &rpc.SignatureStatusesResult{
						ConfirmationStatus: rpc.ConfirmationStatusProcessed,
						Err:                "ERROR",
					}
				}

				// tx should be able to queue
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx))
				wg.Wait()      // wait to be picked up and processed
				waitFor(empty) // inflight txs cleared after timeout

				// check prom metric
				prom.error++
				prom.revert++
				prom.assertEqual(t)

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx passes sim, first retried TXs get dropped
			t.Run("success_retryTx", func(t *testing.T) {
				tx, signed := getTx(t, 10, mkey, 0)
				sig := getSig()
				retry0 := getSig()
				retry1 := getSig()
				retry2 := getSig()
				retry3 := getSig()
				var wg sync.WaitGroup
				wg.Add(2)

				mc.On("SendTx", mock.Anything, signed(0, true)).Return(sig, nil)
				mc.On("SendTx", mock.Anything, signed(1, true)).Return(retry0, nil)
				mc.On("SendTx", mock.Anything, signed(2, true)).Return(retry1, nil)
				mc.On("SendTx", mock.Anything, signed(3, true)).Return(retry2, nil).Maybe()
				mc.On("SendTx", mock.Anything, signed(4, true)).Return(retry3, nil).Maybe()
				mc.On("SimulateTx", mock.Anything, signed(0, true), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{}, nil).Once()

				// handle signature status calls
				statuses[retry1] = func() (out *rpc.SignatureStatusesResult) {
					defer wg.Done()
					return &rpc.SignatureStatusesResult{
						ConfirmationStatus: rpc.ConfirmationStatusConfirmed,
					}
				}

				// send tx
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx))
				wg.Wait()

				// no transactions stored inflight txs list
				waitFor(empty)

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()

				// check prom metric
				prom.success++
				prom.assertEqual(t)
			})

			// fee bumping disabled
			t.Run("feeBumpingDisabled", func(t *testing.T) {
				sig := getSig()
				tx, signed := getTx(t, 11, mkey, 0)

				defaultFeeBumpPeriod := cfg.FeeBumpPeriod()

				sendCount := 0
				var countRW sync.RWMutex
				mc.On("SendTx", mock.Anything, signed(0, true)).Run(func(mock.Arguments) {
					countRW.Lock()
					sendCount++
					countRW.Unlock()
				}).Return(sig, nil) // only sends one transaction type (no bumping)
				mc.On("SimulateTx", mock.Anything, signed(0, true), mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Once()

				// handle signature status calls
				var wg sync.WaitGroup
				wg.Add(1)
				count := 0
				start := time.Now()
				statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
					defer func() { count++ }()

					out = &rpc.SignatureStatusesResult{}
					if time.Since(start) > 2*defaultFeeBumpPeriod {
						out.ConfirmationStatus = rpc.ConfirmationStatusConfirmed
						wg.Done()
						return
					}
					out.ConfirmationStatus = rpc.ConfirmationStatusProcessed
					return
				}

				// send tx - with disabled fee bumping
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, SetFeeBumpPeriod(0)))
				wg.Wait()

				// no transactions stored inflight txs list
				waitFor(empty)
				// transaction should be sent more than twice
				countRW.RLock()
				t.Logf("sendTx received %d calls", sendCount)
				assert.Greater(t, sendCount, 2)
				countRW.RUnlock()

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()

				// check prom metric
				prom.success++
				prom.assertEqual(t)
			})

			// compute unit limit disabled
			t.Run("computeUnitLimitDisabled", func(t *testing.T) {
				sig := getSig()
				tx, signed := getTx(t, 12, mkey, 0)

				// should only match transaction without compute unit limit
				assert.Len(t, signed(0, false).Message.Instructions, 2)
				mc.On("SendTx", mock.Anything, signed(0, false)).Return(sig, nil) // only sends one transaction type (no bumping)
				mc.On("SimulateTx", mock.Anything, signed(0, false), mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Once()

				// handle signature status calls
				var wg sync.WaitGroup
				wg.Add(1)
				statuses[sig] = func() *rpc.SignatureStatusesResult {
					defer wg.Done()
					return &rpc.SignatureStatusesResult{
						ConfirmationStatus: rpc.ConfirmationStatusConfirmed,
					}
				}

				// send tx - with disabled fee bumping and disabled compute unit limit
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, SetFeeBumpPeriod(0), SetComputeUnitLimit(0)))
				wg.Wait()

				// no transactions stored inflight txs list
				waitFor(empty)

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()

				// check prom metric
				prom.success++
				prom.assertEqual(t)
			})
		})
	}
}

func TestTxm_Enqueue(t *testing.T) {
	// set up configs needed in txm
	lggr := logger.Test(t)
	cfg := config.NewDefault()
	mc := mocks.NewReaderWriter(t)
	mc.On("SendTx", mock.Anything, mock.Anything).Return(solana.Signature{}, nil).Maybe()
	mc.On("SimulateTx", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Maybe()
	ctx := tests.Context(t)

	// mock solana keystore
	mkey := keyMocks.NewSimpleKeystore(t)
	validKey := solana.PublicKeyFromBytes([]byte{1})
	invalidKey := solana.PublicKeyFromBytes([]byte{2})
	mkey.On("Sign", mock.Anything, validKey.String(), mock.Anything).Return([]byte{1}, nil)
	mkey.On("Sign", mock.Anything, invalidKey.String(), mock.Anything).Return([]byte{}, relayconfig.KeyNotFoundError{ID: invalidKey.String(), KeyType: "Solana"})

	// build txs
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(
				0,
				validKey,
				validKey,
			).Build(),
		},
		solana.Hash{},
		solana.TransactionPayer(validKey),
	)
	require.NoError(t, err)

	invalidTx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(
				0,
				invalidKey,
				invalidKey,
			).Build(),
		},
		solana.Hash{},
		solana.TransactionPayer(invalidKey),
	)
	require.NoError(t, err)

	txm := NewTxm("enqueue_test", func() (client.ReaderWriter, error) {
		return mc, nil
	}, cfg, mkey, lggr)

	require.ErrorContains(t, txm.Enqueue(ctx, "txmUnstarted", &solana.Transaction{}), "not started")
	require.NoError(t, txm.Start(ctx))
	t.Cleanup(func() { require.NoError(t, txm.Close()) })

	txs := []struct {
		name string
		tx   *solana.Transaction
		fail bool
	}{
		{"success", tx, false},
		{"invalid_key", invalidTx, true},
		{"nil_pointer", nil, true},
		{"empty_tx", &solana.Transaction{}, true},
	}

	for _, run := range txs {
		t.Run(run.name, func(t *testing.T) {
			if !run.fail {
				assert.NoError(t, txm.Enqueue(ctx, run.name, run.tx))
				return
			}
			assert.Error(t, txm.Enqueue(ctx, run.name, run.tx))
		})
	}
}

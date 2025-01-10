//go:build integration

package txm

import (
	"context"
	"errors"
	"fmt"
	"math/big"
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
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"

	relayconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	bigmath "github.com/smartcontractkit/chainlink-common/pkg/utils/big_math"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

type soltxmProm struct {
	id                                                                     string
	confirmed, error, revert, reject, drop, simRevert, simOther, finalized float64
}

func (p soltxmProm) assertEqual(t *testing.T) {
	assert.Equal(t, p.confirmed, testutil.ToFloat64(promSolTxmSuccessTxs.WithLabelValues(p.id)), "mismatch: confirmed")
	assert.Equal(t, p.error, testutil.ToFloat64(promSolTxmErrorTxs.WithLabelValues(p.id)), "mismatch: error")
	assert.Equal(t, p.revert, testutil.ToFloat64(promSolTxmRevertTxs.WithLabelValues(p.id)), "mismatch: revert")
	assert.Equal(t, p.reject, testutil.ToFloat64(promSolTxmRejectTxs.WithLabelValues(p.id)), "mismatch: reject")
	assert.Equal(t, p.drop, testutil.ToFloat64(promSolTxmDropTxs.WithLabelValues(p.id)), "mismatch: drop")
	assert.Equal(t, p.simRevert, testutil.ToFloat64(promSolTxmSimRevertTxs.WithLabelValues(p.id)), "mismatch: simRevert")
	assert.Equal(t, p.simOther, testutil.ToFloat64(promSolTxmSimOtherTxs.WithLabelValues(p.id)), "mismatch: simOther")
	assert.Equal(t, p.finalized, testutil.ToFloat64(promSolTxmFinalizedTxs.WithLabelValues(p.id)), "mismatch: finalized")
}

func (p soltxmProm) getInflight() float64 {
	return testutil.ToFloat64(promSolTxmPendingTxs.WithLabelValues(p.id))
}

// create placeholder transaction and returns func for signed tx with fee
func getTx(t *testing.T, val uint64, keystore SimpleKeystore) (*solana.Transaction, func(fees.ComputeUnitPrice, bool, fees.ComputeUnitLimit) *solana.Transaction) {
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

	return &base, func(price fees.ComputeUnitPrice, addLimit bool, limit fees.ComputeUnitLimit) *solana.Transaction {
		tx := base
		// add fee parameters
		require.NoError(t, fees.SetComputeUnitPrice(&tx, price))
		if addLimit {
			require.NoError(t, fees.SetComputeUnitLimit(&tx, limit)) // default
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

// check if cached transaction is cleared
func empty(t *testing.T, txm *Txm, prom soltxmProm) bool {
	count := txm.InflightTxs()
	assert.Equal(t, float64(count), prom.getInflight()) // validate prom metric and txs length
	return count == 0
}

// waits for the provided function to evaluate to true within the provided duration amount of time
func waitFor(t *testing.T, waitDuration time.Duration, txm *Txm, prom soltxmProm, f func(*testing.T, *Txm, soltxmProm) bool) {
	for i := 0; i < int(waitDuration.Seconds()*1.5); i++ {
		if f(t, txm, prom) {
			return
		}
		time.Sleep(time.Second)
	}
	assert.NoError(t, errors.New("unable to confirm inflight txs is empty"))
}

func TestTxm(t *testing.T) {
	for _, eName := range []string{"fixed", "blockhistory"} {
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
			mc.On("SlotHeight", mock.Anything).Return(uint64(0), nil).Maybe()

			// mock solana keystore
			mkey := keyMocks.NewSimpleKeystore(t)
			mkey.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil)

			loader := utils.NewLazyLoad(func() (client.ReaderWriter, error) { return mc, nil })
			txm := NewTxm(id, loader, nil, cfg, mkey, lggr)
			require.NoError(t, txm.Start(ctx))
			t.Cleanup(func() { require.NoError(t, txm.Close()) })

			// tracking prom metrics
			prom := soltxmProm{id: id}

			// adjust wait time based on config
			waitDuration := cfg.TxConfirmTimeout()
			computeUnitLimitDefault := fees.ComputeUnitLimit(cfg.ComputeUnitLimitDefault())

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
			// happy path (send => simulate success => tx: nil => tx: processed => tx: confirmed => finalized => done)
			t.Run("happyPath", func(t *testing.T) {
				sig := randomSignature(t)
				tx, signed := getTx(t, 0, mkey)
				var wg sync.WaitGroup
				wg.Add(1)

				sendCount := 0
				var countRW sync.RWMutex
				mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Run(func(mock.Arguments) {
					countRW.Lock()
					sendCount++
					countRW.Unlock()
				}).After(500*time.Millisecond).Return(sig, nil)
				mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Once()

				// handle signature status calls
				count := 0
				statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
					defer func() { count++ }()

					out = &rpc.SignatureStatusesResult{}
					if count == 1 {
						out.ConfirmationStatus = rpc.ConfirmationStatusProcessed
						return
					}

					if count == 2 {
						out.ConfirmationStatus = rpc.ConfirmationStatusConfirmed
						return
					}

					if count == 3 {
						out.ConfirmationStatus = rpc.ConfirmationStatusFinalized
						wg.Done()
						return
					}
					return nil
				}

				// send tx
				testTxID := uuid.New().String()
				lastValidBlockHeight := uint64(100)
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
				wg.Wait()

				// no transactions stored inflight txs list
				waitFor(t, waitDuration, txm, prom, empty)
				// transaction should be sent more than twice
				countRW.RLock()
				t.Logf("sendTx received %d calls", sendCount)
				assert.Greater(t, sendCount, 2)
				countRW.RUnlock()

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()

				// check prom metric
				prom.confirmed++
				prom.finalized++
				prom.assertEqual(t)

				_, err := txm.GetTransactionStatus(ctx, testTxID)
				require.Error(t, err) // transaction cleared from storage after finalized should not return status
			})

			// fail on initial transmit (RPC immediate rejects)
			t.Run("fail_initialTx", func(t *testing.T) {
				tx, signed := getTx(t, 1, mkey)
				var wg sync.WaitGroup
				wg.Add(1)

				// should only be called once (tx does not start retry, confirming, or simulation)
				mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(solana.Signature{}, errors.New("FAIL")).Once()

				// tx should be able to queue
				testTxID := uuid.New().String()
				lastValidBlockHeight := uint64(100)
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
				wg.Wait() // wait to be picked up and processed

				// no transactions stored inflight txs list
				waitFor(t, waitDuration, txm, prom, empty)

				// check prom metric
				prom.error++
				prom.reject++
				prom.assertEqual(t)

				_, err := txm.GetTransactionStatus(ctx, testTxID)
				require.Error(t, err) // transaction cleared from storage after finalized should not return status
			})

			// tx fails simulation (simulation error)
			t.Run("fail_simulation", func(t *testing.T) {
				tx, signed := getTx(t, 2, mkey)
				sig := randomSignature(t)
				var wg sync.WaitGroup
				wg.Add(1)

				mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
				mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{
					Err: "FAIL",
				}, nil).Once()
				// signature status is nil (handled automatically)

				// tx should be able to queue
				testTxID := uuid.New().String()
				lastValidBlockHeight := uint64(100)
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
				wg.Wait()                                  // wait to be picked up and processed
				waitFor(t, waitDuration, txm, prom, empty) // txs cleared quickly

				// check prom metric
				prom.error++
				prom.simOther++
				prom.assertEqual(t)

				_, err := txm.GetTransactionStatus(ctx, testTxID)
				require.Error(t, err) // transaction cleared from storage after finalized should not return status
			})

			// tx fails simulation (rpc error, timeout should clean up b/c sig status will be nil)
			t.Run("fail_simulation_confirmNil", func(t *testing.T) {
				tx, signed := getTx(t, 3, mkey)
				sig := randomSignature(t)
				retry0 := randomSignature(t)
				retry1 := randomSignature(t)
				retry2 := randomSignature(t)
				retry3 := randomSignature(t)
				var wg sync.WaitGroup
				wg.Add(1)

				mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
				mc.On("SendTx", mock.Anything, signed(1, true, computeUnitLimitDefault)).Return(retry0, nil)
				mc.On("SendTx", mock.Anything, signed(2, true, computeUnitLimitDefault)).Return(retry1, nil)
				mc.On("SendTx", mock.Anything, signed(3, true, computeUnitLimitDefault)).Return(retry2, nil).Maybe()
				mc.On("SendTx", mock.Anything, signed(4, true, computeUnitLimitDefault)).Return(retry3, nil).Maybe()
				mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{}, errors.New("FAIL")).Once()
				// all signature statuses are nil, handled automatically

				// tx should be able to queue
				testTxID := uuid.New().String()
				lastValidBlockHeight := uint64(100)
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
				wg.Wait()                                  // wait to be picked up and processed
				waitFor(t, waitDuration, txm, prom, empty) // txs cleared after timeout

				// check prom metric
				prom.error++
				prom.drop++
				prom.assertEqual(t)

				_, err := txm.GetTransactionStatus(ctx, testTxID)
				require.Error(t, err) // transaction cleared from storage after finalized should not return status

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx fails simulation with an InstructionError (indicates reverted execution)
			// manager should cancel sending retry immediately + increment reverted prom metric
			t.Run("fail_simulation_instructionError", func(t *testing.T) {
				tx, signed := getTx(t, 4, mkey)
				sig := randomSignature(t)
				var wg sync.WaitGroup
				wg.Add(1)

				// {"InstructionError":[0,{"Custom":6003}]}
				tempErr := map[string][]interface{}{
					"InstructionError": {
						0, map[string]int{"Custom": 6003},
					},
				}
				mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
				mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{
					Err: tempErr,
				}, nil).Once()
				// all signature statuses are nil, handled automatically

				// tx should be able to queue
				testTxID := uuid.New().String()
				lastValidBlockHeight := uint64(100)
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
				wg.Wait()                                  // wait to be picked up and processed
				waitFor(t, waitDuration, txm, prom, empty) // txs cleared after timeout

				// check prom metric
				prom.error++
				prom.simRevert++
				prom.assertEqual(t)

				_, err := txm.GetTransactionStatus(ctx, testTxID)
				require.Error(t, err) // transaction cleared from storage after finalized should not return status

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx fails simulation with BlockHashNotFound error
			// txm should continue to finalize tx (in this case it will succeed)
			t.Run("fail_simulation_blockhashNotFound", func(t *testing.T) {
				tx, signed := getTx(t, 5, mkey)
				sig := randomSignature(t)
				var wg sync.WaitGroup
				wg.Add(2)

				mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
				mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{
					Err: "BlockhashNotFound",
				}, nil).Once()

				// handle signature status calls
				count := 0
				statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
					defer func() { count++ }()

					out = &rpc.SignatureStatusesResult{}
					if count == 0 {
						out.ConfirmationStatus = rpc.ConfirmationStatusConfirmed
						return
					}
					if count == 1 {
						out.ConfirmationStatus = rpc.ConfirmationStatusFinalized
						wg.Done()
						return
					}
					return nil
				}

				// tx should be able to queue
				testTxID := uuid.New().String()
				lastValidBlockHeight := uint64(100)
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
				wg.Wait()                                  // wait to be picked up and processed
				waitFor(t, waitDuration, txm, prom, empty) // txs cleared after timeout

				// check prom metric
				prom.confirmed++
				prom.finalized++
				prom.assertEqual(t)

				_, err := txm.GetTransactionStatus(ctx, testTxID)
				require.Error(t, err) // transaction cleared from storage after finalized should not return status

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx fails simulation with AlreadyProcessed error
			// txm should continue to confirm tx (in this case it will revert)
			t.Run("fail_simulation_alreadyProcessed", func(t *testing.T) {
				tx, signed := getTx(t, 6, mkey)
				sig := randomSignature(t)
				var wg sync.WaitGroup
				wg.Add(2)

				mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
				mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Run(func(mock.Arguments) {
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
				testTxID := uuid.New().String()
				lastValidBlockHeight := uint64(100)
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
				wg.Wait()                                  // wait to be picked up and processed
				waitFor(t, waitDuration, txm, prom, empty) // txs cleared after timeout

				// check prom metric
				prom.revert++
				prom.error++
				prom.assertEqual(t)

				_, err := txm.GetTransactionStatus(ctx, testTxID)
				require.Error(t, err) // transaction cleared from storage after finalized should not return status

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx passes sim, never passes processed (timeout should cleanup)
			t.Run("fail_confirm_processed", func(t *testing.T) {
				tx, signed := getTx(t, 7, mkey)
				sig := randomSignature(t)
				retry0 := randomSignature(t)
				retry1 := randomSignature(t)
				retry2 := randomSignature(t)
				retry3 := randomSignature(t)
				var wg sync.WaitGroup
				wg.Add(1)

				mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
				mc.On("SendTx", mock.Anything, signed(1, true, computeUnitLimitDefault)).Return(retry0, nil)
				mc.On("SendTx", mock.Anything, signed(2, true, computeUnitLimitDefault)).Return(retry1, nil)
				mc.On("SendTx", mock.Anything, signed(3, true, computeUnitLimitDefault)).Return(retry2, nil).Maybe()
				mc.On("SendTx", mock.Anything, signed(4, true, computeUnitLimitDefault)).Return(retry3, nil).Maybe()
				mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{}, nil).Once()

				// handle signature status calls (initial stays processed, others don't exist)
				statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
					return &rpc.SignatureStatusesResult{
						ConfirmationStatus: rpc.ConfirmationStatusProcessed,
					}
				}

				// tx should be able to queue
				testTxID := uuid.New().String()
				lastValidBlockHeight := uint64(100)
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
				wg.Wait()                                  // wait to be picked up and processed
				waitFor(t, waitDuration, txm, prom, empty) // inflight txs cleared after timeout

				// check prom metric
				prom.error++
				prom.drop++
				prom.assertEqual(t)

				_, err := txm.GetTransactionStatus(ctx, testTxID)
				require.Error(t, err) // transaction cleared from storage after finalized should not return status

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx passes sim, gets processed, regresses to not found, gets rebroadcasted by re-org logic and stuck on processed. Eventually cleaned up by timeout.
			t.Run("reorged_tx_stucked_on_processed_is_eventually_cleaned_up", func(t *testing.T) {
				tx, signed := getTx(t, 8, mkey)
				sig := randomSignature(t)
				retry0 := randomSignature(t)
				retry1 := randomSignature(t)
				retry2 := randomSignature(t)
				retry3 := randomSignature(t)
				var wg sync.WaitGroup
				wg.Add(1)

				mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
				mc.On("SendTx", mock.Anything, signed(1, true, computeUnitLimitDefault)).Return(retry0, nil)
				mc.On("SendTx", mock.Anything, signed(2, true, computeUnitLimitDefault)).Return(retry1, nil)
				mc.On("SendTx", mock.Anything, signed(3, true, computeUnitLimitDefault)).Return(retry2, nil).Maybe()
				mc.On("SendTx", mock.Anything, signed(4, true, computeUnitLimitDefault)).Return(retry3, nil).Maybe()
				mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{}, nil).Once()

				mc.On("LatestBlockhash", mock.Anything).Return(&rpc.GetLatestBlockhashResult{
					Value: &rpc.LatestBlockhashResult{
						LastValidBlockHeight: uint64(2000),
					},
				}, nil).Once()

				// handle signature status calls (initial stays processed => nil, others don't exist)
				count := 0
				statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
					defer func() { count++ }()
					if count > 4 {
						return &rpc.SignatureStatusesResult{
							ConfirmationStatus: rpc.ConfirmationStatusProcessed,
						}
					}

					if count > 2 {
						return nil
					}

					return &rpc.SignatureStatusesResult{
						ConfirmationStatus: rpc.ConfirmationStatusProcessed,
					}
				}

				// tx should be able to queue
				testTxID := uuid.New().String()
				lastValidBlockHeight := uint64(100)
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
				wg.Wait()                                  // wait to be picked up and processed
				waitFor(t, waitDuration, txm, prom, empty) // inflight txs cleared after timeout

				// check prom metric
				prom.error++
				prom.drop++
				prom.assertEqual(t)

				_, err := txm.GetTransactionStatus(ctx, testTxID)
				require.Error(t, err) // transaction cleared from storage

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx passes sim, errors on confirm
			t.Run("fail_confirm_revert", func(t *testing.T) {
				tx, signed := getTx(t, 9, mkey)
				sig := randomSignature(t)
				var wg sync.WaitGroup
				wg.Add(1)

				mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
				mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Run(func(mock.Arguments) {
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
				testTxID := uuid.New().String()
				lastValidBlockHeight := uint64(100)
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
				wg.Wait()                                  // wait to be picked up and processed
				waitFor(t, waitDuration, txm, prom, empty) // inflight txs cleared after timeout

				// check prom metric
				prom.error++
				prom.revert++
				prom.assertEqual(t)

				_, err := txm.GetTransactionStatus(ctx, testTxID)
				require.Error(t, err) // transaction cleared from storage after finalized should not return status

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()
			})

			// tx passes sim, first retried TXs get dropped
			t.Run("success_retryTx", func(t *testing.T) {
				tx, signed := getTx(t, 10, mkey)
				sig := randomSignature(t)
				retry0 := randomSignature(t)
				retry1 := randomSignature(t)
				retry2 := randomSignature(t)
				retry3 := randomSignature(t)
				var wg sync.WaitGroup
				wg.Add(2)

				mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
				mc.On("SendTx", mock.Anything, signed(1, true, computeUnitLimitDefault)).Return(retry0, nil)
				mc.On("SendTx", mock.Anything, signed(2, true, computeUnitLimitDefault)).Return(retry1, nil)
				mc.On("SendTx", mock.Anything, signed(3, true, computeUnitLimitDefault)).Return(retry2, nil).Maybe()
				mc.On("SendTx", mock.Anything, signed(4, true, computeUnitLimitDefault)).Return(retry3, nil).Maybe()
				mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Run(func(mock.Arguments) {
					wg.Done()
				}).Return(&rpc.SimulateTransactionResult{}, nil).Once()

				// handle signature status calls
				statuses[retry1] = func() (out *rpc.SignatureStatusesResult) {
					defer wg.Done()
					return &rpc.SignatureStatusesResult{
						ConfirmationStatus: rpc.ConfirmationStatusFinalized,
					}
				}

				// send tx
				testTxID := uuid.New().String()
				lastValidBlockHeight := uint64(100)
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
				wg.Wait()

				// no transactions stored inflight txs list
				waitFor(t, waitDuration, txm, prom, empty)

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()

				// check prom metric
				prom.finalized++
				prom.assertEqual(t)

				_, err := txm.GetTransactionStatus(ctx, testTxID)
				require.Error(t, err) // transaction cleared from storage after finalized should not return status
			})

			// fee bumping disabled
			t.Run("feeBumpingDisabled", func(t *testing.T) {
				sig := randomSignature(t)
				tx, signed := getTx(t, 11, mkey)

				sendCount := 0
				var countRW sync.RWMutex
				mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Run(func(mock.Arguments) {
					countRW.Lock()
					sendCount++
					countRW.Unlock()
				}).Return(sig, nil) // only sends one transaction type (no bumping)
				mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Once()

				// handle signature status calls
				var wg sync.WaitGroup
				wg.Add(1)
				count := 0
				statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
					defer func() { count++ }()

					out = &rpc.SignatureStatusesResult{}
					if count == 1 {
						out.ConfirmationStatus = rpc.ConfirmationStatusConfirmed
						return
					}
					if count == 2 {
						out.ConfirmationStatus = rpc.ConfirmationStatusFinalized
						wg.Done()
						return
					}
					out.ConfirmationStatus = rpc.ConfirmationStatusProcessed
					return
				}

				// send tx - with disabled fee bumping
				testTxID := uuid.New().String()
				lastValidBlockHeight := uint64(100)
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight, txmutils.SetFeeBumpPeriod(0)))
				wg.Wait()

				// no transactions stored inflight txs list
				waitFor(t, waitDuration, txm, prom, empty)
				// transaction should be sent more than twice
				countRW.RLock()
				t.Logf("sendTx received %d calls", sendCount)
				assert.Greater(t, sendCount, 2)
				countRW.RUnlock()

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()

				// check prom metric
				prom.confirmed++
				prom.finalized++
				prom.assertEqual(t)

				_, err := txm.GetTransactionStatus(ctx, testTxID)
				require.Error(t, err) // transaction cleared from storage after finalized should not return status
			})

			// compute unit limit disabled
			t.Run("computeUnitLimitDisabled", func(t *testing.T) {
				sig := randomSignature(t)
				tx, signed := getTx(t, 12, mkey)

				// should only match transaction without compute unit limit
				assert.Len(t, signed(0, false, computeUnitLimitDefault).Message.Instructions, 2)
				mc.On("SendTx", mock.Anything, signed(0, false, computeUnitLimitDefault)).Return(sig, nil) // only sends one transaction type (no bumping)
				mc.On("SimulateTx", mock.Anything, signed(0, false, computeUnitLimitDefault), mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Once()

				// handle signature status calls
				var wg sync.WaitGroup
				wg.Add(1)
				count := 0
				statuses[sig] = func() *rpc.SignatureStatusesResult {
					defer func() { count++ }()
					if count == 0 {
						return &rpc.SignatureStatusesResult{
							ConfirmationStatus: rpc.ConfirmationStatusConfirmed,
						}
					}
					wg.Done()
					return &rpc.SignatureStatusesResult{
						ConfirmationStatus: rpc.ConfirmationStatusFinalized,
					}
				}

				// send tx - with disabled fee bumping and disabled compute unit limit
				testTxID := uuid.New().String()
				lastValidBlockHeight := uint64(100)
				assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight, txmutils.SetFeeBumpPeriod(0), txmutils.SetComputeUnitLimit(0)))
				wg.Wait()

				// no transactions stored inflight txs list
				waitFor(t, waitDuration, txm, prom, empty)

				// panic if sendTx called after context cancelled
				mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()

				// check prom metric
				prom.confirmed++
				prom.finalized++
				prom.assertEqual(t)

				_, err := txm.GetTransactionStatus(ctx, testTxID)
				require.Error(t, err) // transaction cleared from storage after finalized should not return status
			})
		})
	}
}

func TestTxm_disabled_confirm_timeout_with_retention(t *testing.T) {
	t.Parallel() // run estimator tests in parallel

	// set up configs needed in txm
	estimator := "fixed"
	id := "mocknet-" + estimator + "-" + uuid.NewString()
	t.Logf("Starting new iteration: %s", id)

	ctx := tests.Context(t)
	lggr := logger.Test(t)
	cfg := config.NewDefault()
	cfg.Chain.FeeEstimatorMode = &estimator
	// Disable confirm timeout
	cfg.Chain.TxConfirmTimeout = relayconfig.MustNewDuration(0 * time.Second)
	// Enable retention timeout to keep transactions after finality
	cfg.Chain.TxRetentionTimeout = relayconfig.MustNewDuration(5 * time.Second)
	mc := mocks.NewReaderWriter(t)
	mc.On("GetLatestBlock", mock.Anything).Return(&rpc.GetBlockResult{}, nil).Maybe()

	computeUnitLimitDefault := fees.ComputeUnitLimit(cfg.ComputeUnitLimitDefault())

	// mock solana keystore
	mkey := keyMocks.NewSimpleKeystore(t)
	mkey.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil)

	loader := utils.NewLazyLoad(func() (client.ReaderWriter, error) { return mc, nil })
	txm := NewTxm(id, loader, nil, cfg, mkey, lggr)
	require.NoError(t, txm.Start(ctx))
	t.Cleanup(func() { require.NoError(t, txm.Close()) })

	// tracking prom metrics
	prom := soltxmProm{id: id}

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
	t.Run("happyPath", func(t *testing.T) {
		// Test tx is not discarded due to confirm timeout and tracked to finalization
		// use unique val across tests to avoid collision during mocking
		tx, signed := getTx(t, 1, mkey)
		sig := randomSignature(t)
		retry0 := randomSignature(t)
		retry1 := randomSignature(t)
		var wg sync.WaitGroup
		wg.Add(2)

		mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
		mc.On("SendTx", mock.Anything, signed(1, true, computeUnitLimitDefault)).Return(retry0, nil).Maybe()
		mc.On("SendTx", mock.Anything, signed(2, true, computeUnitLimitDefault)).Return(retry1, nil).Maybe()
		mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Run(func(mock.Arguments) {
			wg.Done()
		}).Return(&rpc.SimulateTransactionResult{}, nil).Once()

		// handle signature status calls (initial stays processed, others don't exist)
		start := time.Now()
		statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
			out = &rpc.SignatureStatusesResult{}
			// return confirmed status after default confirmation timeout
			if time.Since(start) > 1*time.Second && time.Since(start) < 2*time.Second {
				out.ConfirmationStatus = rpc.ConfirmationStatusConfirmed
				return
			}
			// return finalized status only after the confirmation timeout
			if time.Since(start) >= 2*time.Second {
				out.ConfirmationStatus = rpc.ConfirmationStatusFinalized
				wg.Done()
				return
			}
			out.ConfirmationStatus = rpc.ConfirmationStatusProcessed
			return
		}

		// tx should be able to queue
		testTxID := uuid.New().String()
		lastValidBlockHeight := uint64(100)
		assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
		wg.Wait()                                   // wait to be picked up and processed
		waitFor(t, 5*time.Second, txm, prom, empty) // inflight txs cleared after timeout

		// panic if sendTx called after context cancelled
		mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()

		// check prom metric
		prom.confirmed++
		prom.finalized++
		prom.assertEqual(t)

		// check transaction status which should still be stored
		status, err := txm.GetTransactionStatus(ctx, testTxID)
		require.NoError(t, err)
		require.Equal(t, types.Finalized, status)

		// Sleep until retention period has passed for transaction and for another reap cycle to run
		time.Sleep(10 * time.Second)

		// check if transaction has been purged from memory
		status, err = txm.GetTransactionStatus(ctx, testTxID)
		require.Error(t, err)
		require.Equal(t, types.Unknown, status)
	})

	t.Run("stores error if initial send fails", func(t *testing.T) {
		// Test tx is not discarded due to confirm timeout and tracked to finalization
		// use unique val across tests to avoid collision during mocking
		tx, signed := getTx(t, 2, mkey)
		var wg sync.WaitGroup
		wg.Add(1)

		mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Run(func(mock.Arguments) {
			wg.Done()
		}).Return(nil, errors.New("failed to send"))

		// tx should be able to queue
		testTxID := uuid.NewString()
		lastValidBlockHeight := uint64(100)
		assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
		wg.Wait()
		waitFor(t, 5*time.Second, txm, prom, empty) // inflight txs cleared after timeout

		// panic if sendTx called after context cancelled
		mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()

		// check prom metric
		prom.error++
		prom.reject++
		prom.assertEqual(t)

		// check transaction status which should still be stored
		status, err := txm.GetTransactionStatus(ctx, testTxID)
		require.NoError(t, err)
		require.Equal(t, types.Failed, status)

		// Sleep until retention period has passed for transaction and for another reap cycle to run
		time.Sleep(15 * time.Second)

		// check if transaction has been purged from memory
		status, err = txm.GetTransactionStatus(ctx, testTxID)
		require.Error(t, err)
		require.Equal(t, types.Unknown, status)
	})

	t.Run("stores error if confirmation returns error", func(t *testing.T) {
		// Test tx is not discarded due to confirm timeout and tracked to finalization
		// use unique val across tests to avoid collision during mocking
		tx, signed := getTx(t, 3, mkey)
		sig := randomSignature(t)
		var wg sync.WaitGroup
		wg.Add(2)

		mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimitDefault)).Return(sig, nil)
		mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimitDefault), mock.Anything).Run(func(mock.Arguments) {
			wg.Done()
		}).Return(&rpc.SimulateTransactionResult{}, nil).Once()
		statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
			defer wg.Done()
			return &rpc.SignatureStatusesResult{Err: errors.New("InstructionError")}
		}

		// tx should be able to queue
		testTxID := uuid.NewString()
		lastValidBlockHeight := uint64(100)
		assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
		wg.Wait()                                   // wait till send tx
		waitFor(t, 5*time.Second, txm, prom, empty) // inflight txs cleared after timeout

		// panic if sendTx called after context cancelled
		mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()

		// check prom metric
		prom.error++
		prom.revert++
		prom.assertEqual(t)

		// check transaction status which should still be stored
		status, err := txm.GetTransactionStatus(ctx, testTxID)
		require.NoError(t, err)
		require.Equal(t, types.Fatal, status)

		// Sleep until retention period has passed for transaction and for another reap cycle to run
		time.Sleep(15 * time.Second)

		// check if transaction has been purged from memory
		status, err = txm.GetTransactionStatus(ctx, testTxID)
		require.Error(t, err)
		require.Equal(t, types.Unknown, status)
	})
}

func TestTxm_compute_unit_limit_estimation(t *testing.T) {
	t.Parallel() // run estimator tests in parallel

	// set up configs needed in txm
	estimator := "fixed"
	id := "mocknet-" + estimator + "-" + uuid.NewString()
	t.Logf("Starting new iteration: %s", id)

	ctx := tests.Context(t)
	lggr := logger.Test(t)
	cfg := config.NewDefault()
	cfg.Chain.FeeEstimatorMode = &estimator
	// Enable compute unit limit estimation feature
	estimateComputeUnitLimit := true
	cfg.Chain.EstimateComputeUnitLimit = &estimateComputeUnitLimit
	// Enable retention timeout to keep transactions after finality or error
	cfg.Chain.TxRetentionTimeout = relayconfig.MustNewDuration(5 * time.Second)
	mc := mocks.NewReaderWriter(t)
	mc.On("GetLatestBlock", mock.Anything).Return(&rpc.GetBlockResult{}, nil).Maybe()

	// mock solana keystore
	mkey := keyMocks.NewSimpleKeystore(t)
	mkey.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil)

	loader := utils.NewLazyLoad(func() (client.ReaderWriter, error) { return mc, nil })
	txm := NewTxm(id, loader, nil, cfg, mkey, lggr)
	require.NoError(t, txm.Start(ctx))
	t.Cleanup(func() { require.NoError(t, txm.Close()) })

	// tracking prom metrics
	prom := soltxmProm{id: id}

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

	t.Run("simulation_succeeds", func(t *testing.T) {
		// Test tx is not discarded due to confirm timeout and tracked to finalization
		// use unique val across tests to avoid collision during mocking
		tx, signed := getTx(t, 1, mkey)
		// add signature and compute unit limit to tx for simulation (excludes compute unit price)
		simulateTx := addSigAndLimitToTx(t, mkey, solana.PublicKey{}, *tx, MaxComputeUnitLimit)
		sig := randomSignature(t)
		var wg sync.WaitGroup
		wg.Add(3)

		computeUnitConsumed := uint64(1_000_000)
		computeUnitLimit := fees.ComputeUnitLimit(uint32(bigmath.AddPercentage(new(big.Int).SetUint64(computeUnitConsumed), EstimateComputeUnitLimitBuffer).Uint64()))
		mc.On("SendTx", mock.Anything, signed(0, true, computeUnitLimit)).Return(sig, nil)
		// First simulation before broadcast with signature and max compute unit limit set
		mc.On("SimulateTx", mock.Anything, simulateTx, mock.Anything).Run(func(mock.Arguments) {
			wg.Done()
		}).Return(&rpc.SimulateTransactionResult{UnitsConsumed: &computeUnitConsumed}, nil).Once()
		// Second simulation after broadcast with signature and compute unit limit set
		mc.On("SimulateTx", mock.Anything, signed(0, true, computeUnitLimit), mock.Anything).Run(func(mock.Arguments) {
			wg.Done()
		}).Return(&rpc.SimulateTransactionResult{UnitsConsumed: &computeUnitConsumed}, nil).Once()

		// handle signature status calls
		count := 0
		statuses[sig] = func() (out *rpc.SignatureStatusesResult) {
			defer func() { count++ }()
			out = &rpc.SignatureStatusesResult{}
			if count == 1 {
				out.ConfirmationStatus = rpc.ConfirmationStatusProcessed
				return
			}
			if count == 2 {
				out.ConfirmationStatus = rpc.ConfirmationStatusConfirmed
				return
			}
			if count == 3 {
				out.ConfirmationStatus = rpc.ConfirmationStatusFinalized
				wg.Done()
				return
			}
			return nil
		}

		// send tx
		testTxID := uuid.New().String()
		lastValidBlockHeight := uint64(100)
		assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &testTxID, lastValidBlockHeight))
		wg.Wait()

		// no transactions stored inflight txs list
		waitFor(t, txm.cfg.TxConfirmTimeout(), txm, prom, empty)

		// panic if sendTx called after context cancelled
		mc.On("SendTx", mock.Anything, tx).Panic("SendTx should not be called anymore").Maybe()

		// check prom metric
		prom.confirmed++
		prom.finalized++
		prom.assertEqual(t)

		status, err := txm.GetTransactionStatus(ctx, testTxID)
		require.NoError(t, err)
		require.Equal(t, types.Finalized, status)
	})

	t.Run("simulation_fails", func(t *testing.T) {
		// Test tx is not discarded due to confirm timeout and tracked to finalization
		// use unique val across tests to avoid collision during mocking
		tx, signed := getTx(t, 2, mkey)
		sig := randomSignature(t)

		mc.On("SendTx", mock.Anything, signed(0, true, fees.ComputeUnitLimit(0))).Return(sig, nil).Panic("SendTx should never be called").Maybe()
		mc.On("SimulateTx", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("simulation failed")).Once()

		// tx should NOT be able to queue
		lastValidBlockHeight := uint64(0)
		assert.Error(t, txm.Enqueue(ctx, t.Name(), tx, nil, lastValidBlockHeight))
	})

	t.Run("simulation_returns_error", func(t *testing.T) {
		// Test tx is not discarded due to confirm timeout and tracked to finalization
		// use unique val across tests to avoid collision during mocking
		tx, _ := getTx(t, 3, mkey)
		// add signature and compute unit limit to tx for simulation (excludes compute unit price)
		simulateTx := addSigAndLimitToTx(t, mkey, solana.PublicKey{}, *tx, MaxComputeUnitLimit)
		sig := randomSignature(t)
		mc.On("SendTx", mock.Anything, mock.Anything).Return(sig, nil).Panic("SendTx should never be called").Maybe()
		// First simulation before broadcast with max compute unit limit
		mc.On("SimulateTx", mock.Anything, simulateTx, mock.Anything).Return(&rpc.SimulateTransactionResult{Err: errors.New("InstructionError")}, nil).Once()

		txID := uuid.NewString()
		lastValidBlockHeight := uint64(100)
		// tx should NOT be able to queue
		assert.Error(t, txm.Enqueue(ctx, t.Name(), tx, &txID, lastValidBlockHeight))
		// tx should be stored in-memory and moved to errored state
		status, err := txm.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, commontypes.Fatal, status)
	})
}

func TestTxm_Enqueue(t *testing.T) {
	// set up configs needed in txm
	lggr := logger.Test(t)
	cfg := config.NewDefault()
	mc := mocks.NewReaderWriter(t)
	mc.On("SendTx", mock.Anything, mock.Anything).Return(solana.Signature{}, nil).Maybe()
	mc.On("SimulateTx", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Maybe()
	mc.On("SignatureStatuses", mock.Anything, mock.AnythingOfType("[]solana.Signature")).Return(
		func(_ context.Context, sigs []solana.Signature) (out []*rpc.SignatureStatusesResult) {
			for i := 0; i < len(sigs); i++ {
				out = append(out, &rpc.SignatureStatusesResult{})
			}
			return out
		}, nil,
	).Maybe()
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

	lastValidBlockHeight := uint64(0)
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

	loader := utils.NewLazyLoad(func() (client.ReaderWriter, error) { return mc, nil })
	txm := NewTxm("enqueue_test", loader, nil, cfg, mkey, lggr)

	require.ErrorContains(t, txm.Enqueue(ctx, "txmUnstarted", &solana.Transaction{}, nil, lastValidBlockHeight), "not started")
	require.NoError(t, txm.Start(ctx))
	t.Cleanup(func() { require.NoError(t, txm.Close()) })

	txs := []struct {
		name                 string
		tx                   *solana.Transaction
		lastValidBlockHeight uint64
		fail                 bool
	}{
		{"success", tx, 100, false},
		{"invalid_key", invalidTx, 0, true},
		{"nil_pointer", nil, 0, true},
		{"empty_tx", &solana.Transaction{}, 0, true},
	}

	for _, run := range txs {
		t.Run(run.name, func(t *testing.T) {
			if !run.fail {
				assert.NoError(t, txm.Enqueue(ctx, run.name, run.tx, nil, run.lastValidBlockHeight))
				return
			}
			assert.Error(t, txm.Enqueue(ctx, run.name, run.tx, nil, run.lastValidBlockHeight))
		})
	}
}

func addSigAndLimitToTx(t *testing.T, keystore SimpleKeystore, pubkey solana.PublicKey, tx solana.Transaction, limit fees.ComputeUnitLimit) *solana.Transaction {
	txCopy := tx
	// sign tx
	txMsg, err := tx.Message.MarshalBinary()
	require.NoError(t, err)
	sigBytes, err := keystore.Sign(context.Background(), pubkey.String(), txMsg)
	require.NoError(t, err)
	var sig [64]byte
	copy(sig[:], sigBytes)
	txCopy.Signatures = append(txCopy.Signatures, sig)
	require.NoError(t, fees.SetComputeUnitLimit(&txCopy, limit))
	return &txCopy
}

func TestTxm_ExpirationRebroadcast(t *testing.T) {
	t.Parallel()
	estimator := "fixed"
	id := "mocknet-" + estimator + "-" + uuid.NewString()
	cfg := config.NewDefault()
	cfg.Chain.FeeEstimatorMode = &estimator
	cfg.Chain.TxConfirmTimeout = relayconfig.MustNewDuration(5 * time.Second)
	cfg.Chain.TxRetentionTimeout = relayconfig.MustNewDuration(10 * time.Second) // Enable retention to keep transactions after finality and be able to check their statuses.
	lggr := logger.Test(t)
	ctx := tests.Context(t)

	// Helper function to set up common test environment
	setupTxmTest := func(
		txExpirationRebroadcast bool,
		latestBlockhashFunc func() (*rpc.GetLatestBlockhashResult, error),
		getLatestBlockHeightFunc func() (uint64, error),
		sendTxFunc func() (solana.Signature, error),
		statuses map[solana.Signature]func() *rpc.SignatureStatusesResult,
	) (*Txm, *mocks.ReaderWriter, *keyMocks.SimpleKeystore) {
		cfg.Chain.TxExpirationRebroadcast = &txExpirationRebroadcast

		mc := mocks.NewReaderWriter(t)
		if latestBlockhashFunc != nil {
			mc.On("LatestBlockhash", mock.Anything).Return(
				func(_ context.Context) (*rpc.GetLatestBlockhashResult, error) {
					return latestBlockhashFunc()
				},
			).Maybe()
		}
		if getLatestBlockHeightFunc != nil {
			mc.On("GetLatestBlockHeight", mock.Anything).Return(
				func(_ context.Context) (uint64, error) {
					return getLatestBlockHeightFunc()
				},
			).Maybe()
		}
		if sendTxFunc != nil {
			mc.On("SendTx", mock.Anything, mock.Anything).Return(
				func(_ context.Context, _ *solana.Transaction) (solana.Signature, error) {
					return sendTxFunc()
				},
			).Maybe()
		}

		mc.On("SimulateTx", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Maybe()
		if statuses != nil {
			mc.On("SignatureStatuses", mock.Anything, mock.AnythingOfType("[]solana.Signature")).Return(
				func(_ context.Context, sigs []solana.Signature) ([]*rpc.SignatureStatusesResult, error) {
					var out []*rpc.SignatureStatusesResult
					for _, sig := range sigs {
						getStatus, exists := statuses[sig]
						if !exists {
							out = append(out, nil)
						} else {
							out = append(out, getStatus())
						}
					}
					return out, nil
				},
			).Maybe()
		}

		mkey := keyMocks.NewSimpleKeystore(t)
		mkey.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil)

		loader := utils.NewLazyLoad(func() (client.ReaderWriter, error) { return mc, nil })
		txm := NewTxm(id, loader, nil, cfg, mkey, lggr)
		require.NoError(t, txm.Start(ctx))
		t.Cleanup(func() { require.NoError(t, txm.Close()) })

		return txm, mc, mkey
	}

	// tracking prom metrics
	prom := soltxmProm{id: id}

	t.Run("WithRebroadcast", func(t *testing.T) {
		txExpirationRebroadcast := true
		statuses := map[solana.Signature]func() *rpc.SignatureStatusesResult{}

		// Mock getLatestBlock to return a value greater than 0 for blockHeight
		getLatestBlockHeightFunc := func() (uint64, error) {
			return 1500, nil
		}

		rebroadcastCount := 0
		latestBlockhashFunc := func() (*rpc.GetLatestBlockhashResult, error) {
			defer func() { rebroadcastCount++ }()
			// rebroadcast call will go through because lastValidBlockHeight is bigger than blockHeight
			return &rpc.GetLatestBlockhashResult{
				Value: &rpc.LatestBlockhashResult{
					LastValidBlockHeight: uint64(2000),
				},
			}, nil
		}

		sig1 := randomSignature(t)
		sendTxFunc := func() (solana.Signature, error) {
			return sig1, nil
		}

		nowTs := time.Now()
		sigStatusCallCount := 0
		var wg sync.WaitGroup
		wg.Add(1)
		statuses[sig1] = func() *rpc.SignatureStatusesResult {
			// First transaction should be rebroadcasted.
			if time.Since(nowTs) < cfg.TxConfirmTimeout()-2*time.Second {
				return nil
			}
			// Second transaction should reach finalization.
			sigStatusCallCount++
			if sigStatusCallCount == 1 {
				return &rpc.SignatureStatusesResult{
					ConfirmationStatus: rpc.ConfirmationStatusProcessed,
				}
			}
			if sigStatusCallCount == 2 {
				return &rpc.SignatureStatusesResult{
					ConfirmationStatus: rpc.ConfirmationStatusConfirmed,
				}
			}
			wg.Done()
			return &rpc.SignatureStatusesResult{
				ConfirmationStatus: rpc.ConfirmationStatusFinalized,
			}
		}

		txm, _, mkey := setupTxmTest(txExpirationRebroadcast, latestBlockhashFunc, getLatestBlockHeightFunc, sendTxFunc, statuses)

		tx, _ := getTx(t, 0, mkey)
		txID := "test-rebroadcast"
		lastValidBlockHeight := uint64(100) // lastValidBlockHeight is smaller than blockHeight
		assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &txID, lastValidBlockHeight))
		wg.Wait()
		waitFor(t, txm.cfg.TxConfirmTimeout(), txm, prom, empty)

		// check prom metric
		prom.confirmed++
		prom.finalized++
		prom.assertEqual(t)

		// Check that transaction for txID has been finalized and rebroadcasted 1 time.
		status, err := txm.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Finalized, status)
		require.Equal(t, 1, rebroadcastCount)
	})

	t.Run("WithoutRebroadcast", func(t *testing.T) {
		txExpirationRebroadcast := false
		statuses := map[solana.Signature]func() *rpc.SignatureStatusesResult{}
		rebroadcastCount := 0

		sig1 := randomSignature(t)
		sendTxFunc := func() (solana.Signature, error) {
			return sig1, nil
		}

		nowTs := time.Now()
		var wg sync.WaitGroup
		wg.Add(1)
		statuses[sig1] = func() *rpc.SignatureStatusesResult {
			// Transaction remains unconfirmed and should not be rebroadcasted.
			if time.Since(nowTs) < cfg.TxConfirmTimeout() {
				return nil
			}
			wg.Done()
			return nil
		}

		txm, _, mkey := setupTxmTest(txExpirationRebroadcast, nil, nil, sendTxFunc, statuses)

		tx, _ := getTx(t, 5, mkey)
		txID := "test-no-rebroadcast"
		lastValidBlockHeight := uint64(0) // original lastValidBlockHeight is invalid
		assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &txID, lastValidBlockHeight))
		wg.Wait()
		waitFor(t, txm.cfg.TxConfirmTimeout(), txm, prom, empty)

		// check prom metric
		prom.drop++
		prom.error++
		prom.assertEqual(t)

		// Check that transaction for txID has not been finalized and has not been rebroadcasted
		status, err := txm.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Failed, status)
		require.Equal(t, 0, rebroadcastCount)
	})

	t.Run("WithMultipleRebroadcast", func(t *testing.T) {
		txExpirationRebroadcast := true
		statuses := map[solana.Signature]func() *rpc.SignatureStatusesResult{}

		// Mock getLatestBlock to return a value greater than 0
		getLatestBlockHeightFunc := func() (uint64, error) {
			return 1500, nil
		}

		// Mock LatestBlockhash to return an invalid blockhash in the first 2 attempts to rebroadcast.
		// the last one is valid because it is greater than the blockHeight
		rebroadcastCount := 0
		latestBlockhashFunc := func() (*rpc.GetLatestBlockhashResult, error) {
			defer func() { rebroadcastCount++ }()
			if rebroadcastCount < 2 {
				return &rpc.GetLatestBlockhashResult{
					Value: &rpc.LatestBlockhashResult{
						LastValidBlockHeight: uint64(1000),
					},
				}, nil
			}
			return &rpc.GetLatestBlockhashResult{
				Value: &rpc.LatestBlockhashResult{
					LastValidBlockHeight: uint64(2000),
				},
			}, nil
		}

		sig1 := randomSignature(t)
		sendTxFunc := func() (solana.Signature, error) {
			return sig1, nil
		}
		nowTs := time.Now()
		sigStatusCallCount := 0
		var wg sync.WaitGroup
		wg.Add(1)
		statuses[sig1] = func() *rpc.SignatureStatusesResult {
			// transaction should be rebroadcasted multiple times.
			if time.Since(nowTs) < cfg.TxConfirmTimeout()-2*time.Second {
				return nil
			}
			// Second transaction should reach finalization.
			sigStatusCallCount++
			if sigStatusCallCount == 1 {
				return &rpc.SignatureStatusesResult{
					ConfirmationStatus: rpc.ConfirmationStatusProcessed,
				}
			} else if sigStatusCallCount == 2 {
				return &rpc.SignatureStatusesResult{
					ConfirmationStatus: rpc.ConfirmationStatusConfirmed,
				}
			}
			wg.Done()
			return &rpc.SignatureStatusesResult{
				ConfirmationStatus: rpc.ConfirmationStatusFinalized,
			}
		}

		txm, _, mkey := setupTxmTest(txExpirationRebroadcast, latestBlockhashFunc, getLatestBlockHeightFunc, sendTxFunc, statuses)
		tx, _ := getTx(t, 0, mkey)
		txID := "test-rebroadcast"
		lastValidBlockHeight := uint64(100) // lastValidBlockHeight is smaller than blockHeight
		assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &txID, lastValidBlockHeight))
		wg.Wait()
		waitFor(t, txm.cfg.TxConfirmTimeout(), txm, prom, empty)

		// check prom metric
		prom.confirmed++
		prom.finalized++
		prom.assertEqual(t)

		// Check that transaction for txID has been finalized and rebroadcasted multiple times.
		status, err := txm.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Finalized, status)
		require.Equal(t, 3, rebroadcastCount)
	})

	t.Run("ConfirmedBeforeRebroadcast", func(t *testing.T) {
		txExpirationRebroadcast := true
		statuses := map[solana.Signature]func() *rpc.SignatureStatusesResult{}
		sig1 := randomSignature(t)
		sendTxFunc := func() (solana.Signature, error) {
			return sig1, nil
		}

		// Mock getLatestBlock to return a value greater than 0
		getLatestBlockHeightFunc := func() (uint64, error) {
			return 1500, nil
		}

		rebroadcastCount := 0
		latestBlockhashFunc := func() (*rpc.GetLatestBlockhashResult, error) {
			defer func() { rebroadcastCount++ }()
			return &rpc.GetLatestBlockhashResult{
				Value: &rpc.LatestBlockhashResult{
					LastValidBlockHeight: uint64(1000),
				},
			}, nil
		}

		var wg sync.WaitGroup
		wg.Add(1)
		count := 0
		statuses[sig1] = func() *rpc.SignatureStatusesResult {
			defer func() { count++ }()

			out := &rpc.SignatureStatusesResult{}
			if count == 1 {
				out.ConfirmationStatus = rpc.ConfirmationStatusConfirmed
				return out
			}
			if count == 2 {
				out.ConfirmationStatus = rpc.ConfirmationStatusFinalized
				wg.Done()
				return out
			}
			out.ConfirmationStatus = rpc.ConfirmationStatusProcessed
			return out
		}

		txm, _, mkey := setupTxmTest(txExpirationRebroadcast, latestBlockhashFunc, getLatestBlockHeightFunc, sendTxFunc, statuses)
		tx, _ := getTx(t, 0, mkey)
		txID := "test-confirmed-before-rebroadcast"
		lastValidBlockHeight := uint64(1500) // original lastValidBlockHeight is valid
		assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &txID, lastValidBlockHeight))
		wg.Wait()
		waitFor(t, txm.cfg.TxConfirmTimeout(), txm, prom, empty)

		// check prom metric
		prom.confirmed++
		prom.finalized++
		prom.assertEqual(t)

		// Check that transaction has been finalized without rebroadcast
		status, err := txm.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Finalized, status)
		require.Equal(t, 0, rebroadcastCount)
	})

	t.Run("RebroadcastWithError", func(t *testing.T) {
		txExpirationRebroadcast := true
		statuses := map[solana.Signature]func() *rpc.SignatureStatusesResult{}

		// To force rebroadcast, first call needs to be smaller than blockHeight
		// following rebroadcast call will go through because lastValidBlockHeight will be bigger than blockHeight
		getLatestBlockHeightFunc := func() (uint64, error) {
			return 1500, nil
		}

		rebroadcastCount := 0
		latestBlockhashFunc := func() (*rpc.GetLatestBlockhashResult, error) {
			defer func() { rebroadcastCount++ }()
			return &rpc.GetLatestBlockhashResult{
				Value: &rpc.LatestBlockhashResult{
					LastValidBlockHeight: uint64(2000),
				},
			}, nil
		}

		sig1 := randomSignature(t)
		sendTxFunc := func() (solana.Signature, error) {
			return sig1, nil
		}

		var wg sync.WaitGroup
		wg.Add(1)
		count := 0
		statuses[sig1] = func() *rpc.SignatureStatusesResult {
			defer func() { count++ }()
			// Transaction remains unconfirmed
			if count == 1 {
				wg.Done()
			}
			return nil
		}

		txm, _, mkey := setupTxmTest(txExpirationRebroadcast, latestBlockhashFunc, getLatestBlockHeightFunc, sendTxFunc, statuses)
		tx, _ := getTx(t, 0, mkey)
		txID := "test-rebroadcast-error"
		lastValidBlockHeight := uint64(100) // lastValidBlockHeight is smaller than blockHeight
		assert.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &txID, lastValidBlockHeight))
		wg.Wait()
		waitFor(t, cfg.TxConfirmTimeout(), txm, prom, empty)

		// check prom metric
		prom.drop++
		prom.error++
		prom.assertEqual(t)

		// Transaction should be moved to failed after trying to rebroadcast 1 time.
		status, err := txm.GetTransactionStatus(ctx, txID)
		require.NoError(t, err)
		require.Equal(t, types.Failed, status)
		require.Equal(t, 1, rebroadcastCount)
	})
}

func TestTxm_OnReorg(t *testing.T) {
	t.Parallel()
	estimator := "fixed"
	id := "mocknet-" + estimator + "-" + uuid.NewString()
	cfg := config.NewDefault()
	cfg.Chain.FeeEstimatorMode = &estimator
	cfg.Chain.TxConfirmTimeout = relayconfig.MustNewDuration(5 * time.Second)
	// Enable retention to keep transactions after finality and be able to check their statuses.
	cfg.Chain.TxRetentionTimeout = relayconfig.MustNewDuration(10 * time.Second)
	lggr := logger.Test(t)
	ctx := tests.Context(t)

	// Helper that sets up a Txm and mocks.
	setupTxmTest := func(
		txExpirationRebroadcast bool,
		latestBlockhashFunc func() (*rpc.GetLatestBlockhashResult, error),
		getLatestBlockHeightFunc func() (uint64, error),
		sendTxFunc func() (solana.Signature, error),
		statuses map[solana.Signature]func() *rpc.SignatureStatusesResult,
	) (*Txm, *mocks.ReaderWriter, *keyMocks.SimpleKeystore) {
		cfg.Chain.TxExpirationRebroadcast = &txExpirationRebroadcast

		mc := mocks.NewReaderWriter(t)
		if latestBlockhashFunc != nil {
			mc.On("LatestBlockhash", mock.Anything).Return(func(_ context.Context) (*rpc.GetLatestBlockhashResult, error) {
				return latestBlockhashFunc()
			}).Maybe()
		}
		if getLatestBlockHeightFunc != nil {
			mc.On("GetLatestBlockHeight", mock.Anything).Return(func(_ context.Context) (uint64, error) {
				return getLatestBlockHeightFunc()
			}).Maybe()
		}
		if sendTxFunc != nil {
			mc.On("SendTx", mock.Anything, mock.Anything).Return(func(_ context.Context, _ *solana.Transaction) (solana.Signature, error) {
				return sendTxFunc()
			}).Maybe()
		}
		mc.On("SimulateTx", mock.Anything, mock.Anything, mock.Anything).Return(&rpc.SimulateTransactionResult{}, nil).Maybe()
		if statuses != nil {
			mc.On("SignatureStatuses", mock.Anything, mock.AnythingOfType("[]solana.Signature")).Return(
				func(_ context.Context, sigs []solana.Signature) ([]*rpc.SignatureStatusesResult, error) {
					var out []*rpc.SignatureStatusesResult
					for _, sig := range sigs {
						getStatus, exists := statuses[sig]
						if !exists {
							out = append(out, nil)
						} else {
							out = append(out, getStatus())
						}
					}
					return out, nil
				},
			).Maybe()
		}

		mkey := keyMocks.NewSimpleKeystore(t)
		mkey.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil)

		loader := utils.NewLazyLoad(func() (client.ReaderWriter, error) { return mc, nil })
		txm := NewTxm(id, loader, nil, cfg, mkey, lggr)
		require.NoError(t, txm.Start(ctx))
		t.Cleanup(func() { require.NoError(t, txm.Close()) })

		return txm, mc, mkey
	}

	prom := soltxmProm{id: id} // track Prometheus metrics across runs

	type scenario struct {
		name           string
		initialSigFrom rpc.ConfirmationStatusType // e.g. "processed", "confirmed"
	}

	tests := []scenario{
		{
			name:           "confirmed => re-org => new tx finalizes",
			initialSigFrom: rpc.ConfirmationStatusConfirmed,
		},
		{
			name:           "processed => re-org => new tx finalizes",
			initialSigFrom: rpc.ConfirmationStatusProcessed,
		},
	}

	for _, sc := range tests {
		t.Run(sc.name, func(t *testing.T) {
			// mock latest blockhash. Re-orged tx needs to be broadcasted with a new blockhash
			latestBlockhashFunc := func() (*rpc.GetLatestBlockhashResult, error) {
				return &rpc.GetLatestBlockhashResult{
					Value: &rpc.LatestBlockhashResult{
						Blockhash:            solana.HashFromBytes([]byte{2}),
						LastValidBlockHeight: 2001,
					},
				}, nil
			}

			initialSig := randomSignature(t)
			var initialTxCtxStopped bool

			retrySig := randomSignature(t)
			sendTxFunc := func() (solana.Signature, error) {
				// We will return the initialSig until the re-org happens
				// After that, we'll return the retrySig as the prior tx is replaced and context cancelled
				if !initialTxCtxStopped {
					return initialSig, nil
				}
				return retrySig, nil
			}

			// Mock the on-chain status of the initial tx
			var initialStatusCallCount int
			var wg sync.WaitGroup
			wg.Add(1)
			statuses := map[solana.Signature]func() *rpc.SignatureStatusesResult{
				initialSig: func() *rpc.SignatureStatusesResult {
					defer func() { initialStatusCallCount++ }()
					if initialStatusCallCount < 2 {
						// keep returning sc.initialSigFrom (e.g. Confirmed or Processed)
						return &rpc.SignatureStatusesResult{
							ConfirmationStatus: sc.initialSigFrom,
						}
					}

					// simulate re-org => NotFound
					initialTxCtxStopped = true
					wg.Done()
					return nil
				},
			}

			// Mock the on-chain status of the re-orged tx. It will eventually finalize
			var retryStatusCallCount int
			wg.Add(1)
			statuses[retrySig] = func() *rpc.SignatureStatusesResult {
				defer func() { retryStatusCallCount++ }()
				switch retryStatusCallCount {
				case 0:
					return &rpc.SignatureStatusesResult{ConfirmationStatus: rpc.ConfirmationStatusProcessed}
				case 1, 2:
					return &rpc.SignatureStatusesResult{ConfirmationStatus: rpc.ConfirmationStatusConfirmed}
				default:
					wg.Done()
					return &rpc.SignatureStatusesResult{ConfirmationStatus: rpc.ConfirmationStatusFinalized}
				}
			}

			txm, _, mkey := setupTxmTest(false, latestBlockhashFunc, nil, sendTxFunc, statuses)

			// Enqueue our transaction to the Txm
			tx, _ := getTx(t, 0, mkey)
			txID := fmt.Sprintf("reorg-from-%s", sc.initialSigFrom)
			lastValidBlockHeight := uint64(100)
			require.NoError(t, txm.Enqueue(ctx, t.Name(), tx, &txID, lastValidBlockHeight))

			// Wait for the states to move from initial => re-org => new => finalized
			// Wait txm get the final states or timeouts of the transactions.
			wg.Wait()
			waitFor(t, txm.cfg.TxConfirmTimeout(), txm, prom, empty)

			// check prom metric increments
			if sc.initialSigFrom == rpc.ConfirmationStatusConfirmed {
				prom.confirmed++ // in case initial tx was confirmed
			}
			// re-orged tx should always be confirmed and finalized
			prom.confirmed++
			prom.finalized++
			prom.assertEqual(t)

			// Ensure the TX is Finalized in memory
			status, err := txm.GetTransactionStatus(ctx, txID)
			require.NoError(t, err)
			require.Equal(t, types.Finalized, status)
		})
	}
}

package txm

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"testing"
	"time"

	solanaGo "github.com/gagliardetto/solana-go"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	solanaClient "github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	cfgmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/config/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	feemocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/fees/mocks"
	ksmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func NewTestMsg() (msg pendingTx) {
	tx := solanaGo.Transaction{}
	tx.Message.AccountKeys = append(tx.Message.AccountKeys, solanaGo.PublicKey{})
	msg.tx = tx
	return msg
}

// Test race condition for saving + reading signatures when bumping fees
// A slow RPC can cause the tx (before bump) to be processed after the bumped tx
// The bumped tx will cause the storage check to move on to the next tx signature even with a inflight "non-bumped" tx
func TestTxm_SendWithRetry_Race(t *testing.T) {
	// test config
	txRetryDuration := 2 * time.Second

	// mocks init
	cfg := cfgmocks.NewConfig(t)
	ks := ksmocks.NewSimpleKeystore(t)
	lggr, observer := logger.TestObserved(t, zapcore.DebugLevel)
	fee := feemocks.NewEstimator(t)

	// fee mock
	fee.On("BaseComputeUnitPrice").Return(uint64(0))

	// config mock
	cfg.On("ComputeUnitPriceMax").Return(uint64(10))
	cfg.On("ComputeUnitPriceMin").Return(uint64(0))
	cfg.On("FeeBumpPeriod").Return(txRetryDuration / 6)
	cfg.On("TxRetryTimeout").Return(txRetryDuration)
	cfg.On("ComputeUnitLimitDefault").Return(uint32(200_000)) // default value, cannot not use 0
	cfg.On("EstimateComputeUnitLimit").Return(false)
	// keystore mock
	ks.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil)

	// assemble minimal tx for testing retry
	msg := NewTestMsg()

	testRunner := func(t *testing.T, client solanaClient.ReaderWriter) {
		// build minimal txm
		loader := utils.NewLazyLoad(func() (solanaClient.ReaderWriter, error) {
			return client, nil
		})
		txm := NewTxm("retry_race", loader, nil, cfg, ks, lggr)
		txm.fee = fee

		msg.cfg = txm.defaultTxConfig()

		_, _, _, err := txm.sendWithRetry(tests.Context(t), msg)
		require.NoError(t, err)

		time.Sleep(txRetryDuration / 4 * 5)                                     // wait 1.25x longer of tx life to capture all logs
		assert.Equal(t, observer.FilterLevelExact(zapcore.ErrorLevel).Len(), 0) // assert no error logs
		lastLog := observer.All()[len(observer.All())-1]
		assert.Contains(t, lastLog.Message, "stopped tx retry") // assert that all retry goroutines exit successfully
	}

	t.Run("delay in rebroadcasting tx", func(t *testing.T) {
		client := clientmocks.NewReaderWriter(t)
		// client mock
		txs := map[string]solanaGo.Signature{}
		var lock sync.RWMutex
		client.On("SendTx", mock.Anything, mock.Anything).Return(
			// build new sig if tx is different
			func(_ context.Context, tx *solanaGo.Transaction) solanaGo.Signature {
				strTx := tx.String()

				// if exists, slow down client response to trigger race
				lock.RLock()
				val, exists := txs[strTx]
				lock.RUnlock()
				if exists {
					time.Sleep(txRetryDuration / 3)
					return val
				}

				lock.Lock()
				defer lock.Unlock()
				// recheck existence
				val, exists = txs[strTx]
				if exists {
					return val
				}
				sig := make([]byte, 16)
				_, err := rand.Read(sig)
				require.NoError(t, err)
				txs[strTx] = solanaGo.SignatureFromBytes(sig)

				return txs[strTx]
			},
			nil,
		)
		testRunner(t, client)
	})

	t.Run("delay in broadcasting new tx", func(t *testing.T) {
		client := clientmocks.NewReaderWriter(t)
		// client mock
		txs := map[string]solanaGo.Signature{}
		var lock sync.RWMutex
		client.On("SendTx", mock.Anything, mock.Anything).Return(
			// build new sig if tx is different
			func(_ context.Context, tx *solanaGo.Transaction) solanaGo.Signature {
				strTx := tx.String()

				lock.Lock()
				// check existence
				val, exists := txs[strTx]
				if exists {
					lock.Unlock()
					return val
				}
				sig := make([]byte, 16)
				_, err := rand.Read(sig)
				require.NoError(t, err)
				txs[strTx] = solanaGo.SignatureFromBytes(sig)
				lock.Unlock()

				// don't lock on delay
				// delay every new bumping tx
				time.Sleep(txRetryDuration / 3)

				lock.RLock()
				defer lock.RUnlock()
				return txs[strTx]
			},
			nil,
		)
		testRunner(t, client)
	})

	t.Run("overlapping bumping tx", func(t *testing.T) {
		client := clientmocks.NewReaderWriter(t)
		// client mock
		txs := map[string]solanaGo.Signature{}
		var lock sync.RWMutex
		client.On("SendTx", mock.Anything, mock.Anything).Return(
			// build new sig if tx is different
			func(_ context.Context, tx *solanaGo.Transaction) solanaGo.Signature {
				strTx := tx.String()

				lock.Lock()
				// recheck existence
				val, exists := txs[strTx]
				if exists {
					lock.Unlock()
					return val
				}
				sig := make([]byte, 16)
				_, err := rand.Read(sig)
				require.NoError(t, err)
				txs[strTx] = solanaGo.SignatureFromBytes(sig)

				triggerDelay := len(txs) == 2
				lock.Unlock()

				// don't lock on delay
				// only delay on the first bump tx
				// ------------------------------
				// init tx - no delay
				// rebroadcast - no delay (tx + sig already exists, does not reach this point)
				// first bump tx - DELAY
				// rebroadcast bump tx - no delay (tx + sig already exists, does not reach this point)
				// second bump tx - no delay
				// etc
				if triggerDelay {
					time.Sleep(txRetryDuration * 2 / 3)
				}

				lock.RLock()
				defer lock.RUnlock()
				return txs[strTx]
			},
			nil,
		)
		testRunner(t, client)
	})

	t.Run("bumping tx errors and ctx cleans up waitgroup blocks", func(t *testing.T) {
		client := clientmocks.NewReaderWriter(t)
		// client mock - first tx is always successful
		msg0 := NewTestMsg()
		require.NoError(t, fees.SetComputeUnitPrice(&msg0.tx, 0))
		require.NoError(t, fees.SetComputeUnitLimit(&msg0.tx, 200_000))
		msg0.tx.Signatures = make([]solanaGo.Signature, 1)
		client.On("SendTx", mock.Anything, &msg0.tx).Return(solanaGo.Signature{1}, nil)

		// init bump tx fails, rebroadcast is successful
		msg1 := NewTestMsg()
		require.NoError(t, fees.SetComputeUnitPrice(&msg1.tx, 1))
		require.NoError(t, fees.SetComputeUnitLimit(&msg1.tx, 200_000))
		msg1.tx.Signatures = make([]solanaGo.Signature, 1)
		client.On("SendTx", mock.Anything, &msg1.tx).Return(solanaGo.Signature{}, fmt.Errorf("BUMP FAILED")).Once()
		client.On("SendTx", mock.Anything, &msg1.tx).Return(solanaGo.Signature{2}, nil)

		// init bump tx success, rebroadcast fails
		msg2 := NewTestMsg()
		require.NoError(t, fees.SetComputeUnitPrice(&msg2.tx, 2))
		require.NoError(t, fees.SetComputeUnitLimit(&msg2.tx, 200_000))
		msg2.tx.Signatures = make([]solanaGo.Signature, 1)
		client.On("SendTx", mock.Anything, &msg2.tx).Return(solanaGo.Signature{3}, nil).Once()
		client.On("SendTx", mock.Anything, &msg2.tx).Return(solanaGo.Signature{}, fmt.Errorf("REBROADCAST FAILED"))

		// always successful
		msg3 := NewTestMsg()
		require.NoError(t, fees.SetComputeUnitPrice(&msg3.tx, 4))
		require.NoError(t, fees.SetComputeUnitLimit(&msg3.tx, 200_000))
		msg3.tx.Signatures = make([]solanaGo.Signature, 1)
		client.On("SendTx", mock.Anything, &msg3.tx).Return(solanaGo.Signature{4}, nil)

		testRunner(t, client)
	})
}

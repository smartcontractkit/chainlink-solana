package txm

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	solanaGo "github.com/gagliardetto/solana-go"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	"github.com/smartcontractkit/chainlink-relay/pkg/utils"

	solanaClient "github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	clientmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/mocks"
	cfgmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/config/mocks"
	feemocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/fees/mocks"
	ksmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Test race condition for saving + reading signatures when bumping fees
// A slow RPC can cause the tx (before bump) to be processed after the bumped tx
// The bumped tx will cause the storage check to move on to the next tx signature even with a inflight "non-bumped" tx
func TestTxm_SendWithRetry_Race(t *testing.T) {
	// test config
	txRetryDuration := time.Second

	// mocks init
	client := clientmocks.NewReaderWriter(t)
	getClient := func() (solanaClient.ReaderWriter, error) {
		return client, nil
	}
	cfg := cfgmocks.NewConfig(t)
	ks := ksmocks.NewSimpleKeystore(t)
	lggr, observer := logger.TestObserved(t, zapcore.DebugLevel)
	fee := feemocks.NewEstimator(t)

	// fee mock
	fee.On("BaseComputeUnitPrice").Return(uint64(0))

	// config mock
	cfg.On("ComputeUnitPriceMax").Return(uint64(10))
	cfg.On("ComputeUnitPriceMin").Return(uint64(0))
	cfg.On("FeeBumpPeriod").Return(txRetryDuration * 3 / 4) // trigger fee bump after 75% of tx life (only 1 bump)

	// keystore mock
	ks.On("Sign", mock.Anything, mock.Anything, mock.Anything).Return([]byte{}, nil)

	// client mock
	txs := map[string]solanaGo.Signature{}
	var lock sync.RWMutex
	client.On("SendTx", mock.Anything, mock.Anything).Return(
		// build new sig if tx is different
		func(_ context.Context, tx *solanaGo.Transaction) solanaGo.Signature {
			strTx := tx.String()

			// if exists previously slow down client response to trigger race
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
			rand.Read(sig)
			txs[strTx] = solanaGo.SignatureFromBytes(sig)

			return txs[strTx]
		},
		nil,
	)

	// build minimal txm
	txm := NewTxm("retry_race", getClient, cfg, ks, lggr)
	txm.fee = fee

	// assemble minimal tx for testing retry
	tx := solanaGo.Transaction{}
	tx.Message.AccountKeys = append(tx.Message.AccountKeys, solanaGo.PublicKey{})

	_, _, _, err := txm.sendWithRetry(
		utils.Context(t),
		tx,
		txRetryDuration,
	)
	require.NoError(t, err)

	time.Sleep(txRetryDuration / 4 * 5) // wait 1.25x longer of tx life to capture all logs
	assert.Equal(t, observer.FilterLevelExact(zapcore.ErrorLevel).Len(), 0)
}
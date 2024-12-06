package txm

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

func TestPendingTxContext_add_remove_multiple(t *testing.T) {
	var wg sync.WaitGroup
	ctx := tests.Context(t)

	newProcess := func() (solana.Signature, context.CancelFunc) {
		// make random signature
		sig := randomSignature(t)

		// start subprocess to wait for context
		processCtx, cancel := context.WithCancel(ctx)
		wg.Add(1)
		go func() {
			<-processCtx.Done()
			wg.Done()
		}()
		return sig, cancel
	}

	// init inflight txs map + store some signatures and cancelFunc
	txs := newPendingTxContext()
	ids := map[solana.Signature]string{}
	n := 5
	for i := 0; i < n; i++ {
		sig, cancel := newProcess()
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		assert.NoError(t, err)
		ids[sig] = msg.id
	}

	// cannot add signature for non existent ID
	require.Error(t, txs.AddSignature(uuid.New().String(), solana.Signature{}))

	// return list of signatures
	list := txs.ListAll()
	assert.Equal(t, n, len(list))

	// stop all sub processes
	for i := 0; i < len(list); i++ {
		id, err := txs.Remove(list[i])
		assert.NoError(t, err)
		assert.Equal(t, n-i-1, len(txs.ListAll()))
		assert.Equal(t, ids[list[i]], id)

		// second remove should not return valid id - already removed
		id, err = txs.Remove(list[i])
		require.Error(t, err)
		assert.Equal(t, "", id)
	}
	wg.Wait()
}

func TestPendingTxContext_new(t *testing.T) {
	t.Parallel()
	_, cancel := context.WithCancel(tests.Context(t))
	sig := randomSignature(t)
	txs := newPendingTxContext()

	// Create new transaction
	msg := pendingTx{id: uuid.NewString()}
	err := txs.New(msg, sig, cancel)
	require.NoError(t, err)

	// Check it exists in signature map
	id, exists := txs.sigToID[sig]
	require.True(t, exists)
	require.Equal(t, msg.id, id)

	// Check it exists in broadcasted map
	tx, exists := txs.broadcastedTxs[msg.id]
	require.True(t, exists)
	require.Len(t, tx.signatures, 1)
	require.Equal(t, sig, tx.signatures[0])

	// Check status is Broadcasted
	require.Equal(t, Broadcasted, tx.state)

	// Check it does not exist in confirmed map
	_, exists = txs.confirmedTxs[msg.id]
	require.False(t, exists)

	// Check it does not exist in finalized map
	_, exists = txs.finalizedErroredTxs[msg.id]
	require.False(t, exists)
}

func TestPendingTxContext_add_signature(t *testing.T) {
	t.Parallel()
	_, cancel := context.WithCancel(tests.Context(t))
	txs := newPendingTxContext()

	t.Run("successfully add signature to transaction", func(t *testing.T) {
		sig1 := randomSignature(t)
		sig2 := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig1, cancel)
		require.NoError(t, err)

		err = txs.AddSignature(msg.id, sig2)
		require.NoError(t, err)

		// Check signature map
		id, exists := txs.sigToID[sig1]
		require.True(t, exists)
		require.Equal(t, msg.id, id)
		id, exists = txs.sigToID[sig2]
		require.True(t, exists)
		require.Equal(t, msg.id, id)

		// Check broadcasted map
		tx, exists := txs.broadcastedTxs[msg.id]
		require.True(t, exists)
		require.Len(t, tx.signatures, 2)
		require.Equal(t, sig1, tx.signatures[0])
		require.Equal(t, sig2, tx.signatures[1])

		// Check confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check finalized map
		_, exists = txs.finalizedErroredTxs[msg.id]
		require.False(t, exists)
	})

	t.Run("fails to add duplicate signature", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		err = txs.AddSignature(msg.id, sig)
		require.ErrorIs(t, err, ErrSigAlreadyExists)
	})

	t.Run("fails to add signature for missing transaction", func(t *testing.T) {
		sig1 := randomSignature(t)
		sig2 := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig1, cancel)
		require.NoError(t, err)

		err = txs.AddSignature("bad id", sig2)
		require.ErrorIs(t, err, ErrTransactionNotFound)
	})

	t.Run("fails to add signature for confirmed transaction", func(t *testing.T) {
		sig1 := randomSignature(t)
		sig2 := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig1, cancel)
		require.NoError(t, err)

		// Transition to processed state
		id, err := txs.OnProcessed(sig1)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(sig1)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		err = txs.AddSignature(msg.id, sig2)
		require.ErrorIs(t, err, ErrTransactionNotFound)
	})
}

func TestPendingTxContext_on_broadcasted_processed(t *testing.T) {
	t.Parallel()
	_, cancel := context.WithCancel(tests.Context(t))
	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	t.Run("successfully transition transaction from broadcasted to processed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to processed state
		id, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it exists in signature map
		id, exists := txs.sigToID[sig]
		require.True(t, exists)
		require.Equal(t, msg.id, id)

		// Check it exists in broadcasted map
		tx, exists := txs.broadcastedTxs[msg.id]
		require.True(t, exists)
		require.Len(t, tx.signatures, 1)
		require.Equal(t, sig, tx.signatures[0])

		// Check status is Processed
		require.Equal(t, Processed, tx.state)

		// Check it does not exist in confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in finalized map
		_, exists = txs.finalizedErroredTxs[msg.id]
		require.False(t, exists)
	})

	t.Run("fails to transition transaction from confirmed to processed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to processed state
		id, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition back to processed state
		_, err = txs.OnProcessed(sig)
		require.Error(t, err)
	})

	t.Run("fails to transition transaction from finalized to processed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to processed state
		id, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to finalized state
		id, err = txs.OnFinalized(sig, retentionTimeout)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition back to processed state
		_, err = txs.OnProcessed(sig)
		require.Error(t, err)
	})

	t.Run("fails to transition transaction from errored to processed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to errored state
		id, err := txs.OnError(sig, retentionTimeout, Errored, 0)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition back to processed state
		_, err = txs.OnProcessed(sig)
		require.Error(t, err)
	})

	t.Run("predefined error if transaction already in processed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to processed state
		id, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// No error if OnProcessed called again
		_, err = txs.OnProcessed(sig)
		require.ErrorIs(t, err, ErrAlreadyInExpectedState)
	})
}

func TestPendingTxContext_on_confirmed(t *testing.T) {
	t.Parallel()
	_, cancel := context.WithCancel(tests.Context(t))
	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	t.Run("successfully transition transaction from broadcasted/processed to confirmed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to processed state
		id, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it exists in signature map
		id, exists := txs.sigToID[sig]
		require.True(t, exists)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists = txs.broadcastedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in confirmed map
		tx, exists := txs.confirmedTxs[msg.id]
		require.True(t, exists)
		require.Len(t, tx.signatures, 1)
		require.Equal(t, sig, tx.signatures[0])

		// Check status is Confirmed
		require.Equal(t, Confirmed, tx.state)

		// Check it does not exist in finalized map
		_, exists = txs.finalizedErroredTxs[msg.id]
		require.False(t, exists)
	})

	t.Run("fails to transition transaction from finalized to confirmed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to processed state
		id, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to finalized state
		id, err = txs.OnFinalized(sig, retentionTimeout)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition back to processed state
		_, err = txs.OnConfirmed(sig)
		require.Error(t, err)
	})

	t.Run("fails to transition transaction from errored to confirmed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to errored state
		id, err := txs.OnError(sig, retentionTimeout, Errored, 0)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition back to confirmed state
		_, err = txs.OnConfirmed(sig)
		require.Error(t, err)
	})

	t.Run("predefined error if transaction already in confirmed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to processed state
		id, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// No error if OnConfirmed called again
		_, err = txs.OnConfirmed(sig)
		require.ErrorIs(t, err, ErrAlreadyInExpectedState)
	})
}

func TestPendingTxContext_on_finalized(t *testing.T) {
	t.Parallel()
	_, cancel := context.WithCancel(tests.Context(t))
	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	t.Run("successfully transition transaction from broadcasted/processed to finalized state", func(t *testing.T) {
		sig1 := randomSignature(t)
		sig2 := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig1, cancel)
		require.NoError(t, err)

		// Add second signature
		err = txs.AddSignature(msg.id, sig2)
		require.NoError(t, err)

		// Transition to finalized state
		id, err := txs.OnFinalized(sig1, retentionTimeout)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in finalized map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Finalized
		require.Equal(t, Finalized, tx.state)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToID[sig1]
		require.False(t, exists)
		_, exists = txs.sigToID[sig2]
		require.False(t, exists)
	})

	t.Run("successfully transition transaction from confirmed to finalized state", func(t *testing.T) {
		sig1 := randomSignature(t)
		sig2 := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig1, cancel)
		require.NoError(t, err)

		// Add second signature
		err = txs.AddSignature(msg.id, sig2)
		require.NoError(t, err)

		// Transition to processed state
		id, err := txs.OnProcessed(sig1)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(sig1)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to finalized state
		id, err = txs.OnFinalized(sig1, retentionTimeout)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in finalized map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Finalized
		require.Equal(t, Finalized, tx.state)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToID[sig1]
		require.False(t, exists)
		_, exists = txs.sigToID[sig2]
		require.False(t, exists)
	})

	t.Run("successfully delete transaction when finalized with 0 retention timeout", func(t *testing.T) {
		sig1 := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig1, cancel)
		require.NoError(t, err)

		// Transition to processed state
		id, err := txs.OnProcessed(sig1)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(sig1)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to finalized state
		id, err = txs.OnFinalized(sig1, 0*time.Second)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in finalized map
		_, exists = txs.finalizedErroredTxs[msg.id]
		require.False(t, exists)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToID[sig1]
		require.False(t, exists)
	})

	t.Run("fails to transition transaction from errored to finalized state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to errored state
		id, err := txs.OnError(sig, retentionTimeout, Errored, 0)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition back to confirmed state
		_, err = txs.OnFinalized(sig, retentionTimeout)
		require.Error(t, err)
	})
}

func TestPendingTxContext_on_error(t *testing.T) {
	t.Parallel()
	_, cancel := context.WithCancel(tests.Context(t))
	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	t.Run("successfully transition transaction from broadcasted/processed to errored state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to errored state
		id, err := txs.OnError(sig, retentionTimeout, Errored, 0)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in errored map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Finalized
		require.Equal(t, Errored, tx.state)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToID[sig]
		require.False(t, exists)
	})

	t.Run("successfully transitions transaction from confirmed to errored state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to errored state
		id, err := txs.OnConfirmed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to errored state
		id, err = txs.OnError(sig, retentionTimeout, Errored, 0)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in errored map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Finalized
		require.Equal(t, Errored, tx.state)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToID[sig]
		require.False(t, exists)
	})

	t.Run("successfully transition transaction from broadcasted/processed to fatally errored state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to fatally errored state
		id, err := txs.OnError(sig, retentionTimeout, FatallyErrored, 0)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in errored map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Errored
		require.Equal(t, FatallyErrored, tx.state)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToID[sig]
		require.False(t, exists)
	})

	t.Run("successfully delete transaction when errored with 0 retention timeout", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to errored state
		id, err := txs.OnConfirmed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to errored state
		id, err = txs.OnError(sig, 0*time.Second, Errored, 0)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in errored map
		_, exists = txs.finalizedErroredTxs[msg.id]
		require.False(t, exists)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToID[sig]
		require.False(t, exists)
	})

	t.Run("fails to transition transaction from finalized to errored state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to confirmed state
		id, err := txs.OnFinalized(sig, retentionTimeout)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition back to confirmed state
		id, err = txs.OnError(sig, retentionTimeout, Errored, 0)
		require.Error(t, err)
		require.Equal(t, "", id)
	})
}

func TestPendingTxContext_on_prebroadcast_error(t *testing.T) {
	t.Parallel()
	_, cancel := context.WithCancel(tests.Context(t))
	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	t.Run("successfully adds transaction with errored state", func(t *testing.T) {
		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		// Transition to errored state
		err := txs.OnPrebroadcastError(msg.id, retentionTimeout, Errored, 0)
		require.NoError(t, err)

		// Check it exists in errored map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Errored
		require.Equal(t, Errored, tx.state)
	})

	t.Run("successfully adds transaction with fatally errored state", func(t *testing.T) {
		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}

		// Transition to fatally errored state
		err := txs.OnPrebroadcastError(msg.id, retentionTimeout, FatallyErrored, 0)
		require.NoError(t, err)

		// Check it exists in errored map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Errored
		require.Equal(t, FatallyErrored, tx.state)
	})

	t.Run("fails to add transaction to errored map if id exists in another map already", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		// Add transaction to broadcasted map
		err := txs.New(msg, sig, cancel)
		require.NoError(t, err)

		// Transition to errored state
		err = txs.OnPrebroadcastError(msg.id, retentionTimeout, FatallyErrored, 0)
		require.ErrorIs(t, err, ErrIDAlreadyExists)
	})

	t.Run("predefined error if transaction already in errored state", func(t *testing.T) {
		txID := uuid.NewString()

		// Transition to errored state
		err := txs.OnPrebroadcastError(txID, retentionTimeout, Errored, 0)
		require.NoError(t, err)

		// Transition back to errored state
		err = txs.OnPrebroadcastError(txID, retentionTimeout, Errored, 0)
		require.ErrorIs(t, err, ErrAlreadyInExpectedState)
	})
}

func TestPendingTxContext_remove(t *testing.T) {
	t.Parallel()
	_, cancel := context.WithCancel(tests.Context(t))

	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	broadcastedSig1 := randomSignature(t)
	broadcastedSig2 := randomSignature(t)
	processedSig := randomSignature(t)
	confirmedSig := randomSignature(t)
	finalizedSig := randomSignature(t)
	erroredSig := randomSignature(t)

	// Create new broadcasted transaction with extra sig
	broadcastedMsg := pendingTx{id: uuid.NewString()}
	err := txs.New(broadcastedMsg, broadcastedSig1, cancel)
	require.NoError(t, err)
	err = txs.AddSignature(broadcastedMsg.id, broadcastedSig2)
	require.NoError(t, err)

	// Create new processed transaction
	processedMsg := pendingTx{id: uuid.NewString()}
	err = txs.New(processedMsg, processedSig, cancel)
	require.NoError(t, err)
	id, err := txs.OnProcessed(processedSig)
	require.NoError(t, err)
	require.Equal(t, processedMsg.id, id)

	// Create new confirmed transaction
	confirmedMsg := pendingTx{id: uuid.NewString()}
	err = txs.New(confirmedMsg, confirmedSig, cancel)
	require.NoError(t, err)
	id, err = txs.OnConfirmed(confirmedSig)
	require.NoError(t, err)
	require.Equal(t, confirmedMsg.id, id)

	// Create new finalized transaction
	finalizedMsg := pendingTx{id: uuid.NewString()}
	err = txs.New(finalizedMsg, finalizedSig, cancel)
	require.NoError(t, err)
	id, err = txs.OnFinalized(finalizedSig, retentionTimeout)
	require.NoError(t, err)
	require.Equal(t, finalizedMsg.id, id)

	// Create new errored transaction
	erroredMsg := pendingTx{id: uuid.NewString()}
	err = txs.New(erroredMsg, erroredSig, cancel)
	require.NoError(t, err)
	id, err = txs.OnError(erroredSig, retentionTimeout, Errored, 0)
	require.NoError(t, err)
	require.Equal(t, erroredMsg.id, id)

	// Remove broadcasted transaction
	id, err = txs.Remove(broadcastedSig1)
	require.NoError(t, err)
	require.Equal(t, broadcastedMsg.id, id)
	// Check removed from broadcasted map
	_, exists := txs.broadcastedTxs[broadcastedMsg.id]
	require.False(t, exists)
	// Check all signatures removed from sig map
	_, exists = txs.sigToID[broadcastedSig1]
	require.False(t, exists)
	_, exists = txs.sigToID[broadcastedSig2]
	require.False(t, exists)

	// Remove processed transaction
	id, err = txs.Remove(processedSig)
	require.NoError(t, err)
	require.Equal(t, processedMsg.id, id)
	// Check removed from broadcasted map
	_, exists = txs.broadcastedTxs[processedMsg.id]
	require.False(t, exists)
	// Check all signatures removed from sig map
	_, exists = txs.sigToID[processedSig]
	require.False(t, exists)

	// Remove confirmed transaction
	id, err = txs.Remove(confirmedSig)
	require.NoError(t, err)
	require.Equal(t, confirmedMsg.id, id)
	// Check removed from confirmed map
	_, exists = txs.confirmedTxs[confirmedMsg.id]
	require.False(t, exists)
	// Check all signatures removed from sig map
	_, exists = txs.sigToID[confirmedSig]
	require.False(t, exists)

	// Check remove cannot be called on finalized transaction
	id, err = txs.Remove(finalizedSig)
	require.Error(t, err)
	require.Equal(t, "", id)

	// Check remove cannot be called on errored transaction
	id, err = txs.Remove(erroredSig)
	require.Error(t, err)
	require.Equal(t, "", id)

	// Check sig list is empty after all removals
	require.Empty(t, txs.ListAll())
}
func TestPendingTxContext_trim_finalized_errored_txs(t *testing.T) {
	t.Parallel()
	txs := newPendingTxContext()

	// Create new finalized transaction with retention ts in the past and add to map
	finalizedMsg1 := finishedTx{retentionTs: time.Now().Add(-2 * time.Second)}
	finalizedMsg1ID := uuid.NewString()
	txs.finalizedErroredTxs[finalizedMsg1ID] = finalizedMsg1

	// Create new finalized transaction with retention ts in the future and add to map
	finalizedMsg2 := finishedTx{retentionTs: time.Now().Add(1 * time.Second)}
	finalizedMsg2ID := uuid.NewString()
	txs.finalizedErroredTxs[finalizedMsg2ID] = finalizedMsg2

	// Create new finalized transaction with retention ts in the past and add to map
	erroredMsg := finishedTx{retentionTs: time.Now().Add(-2 * time.Second)}
	erroredMsgID := uuid.NewString()
	txs.finalizedErroredTxs[erroredMsgID] = erroredMsg

	// Delete finalized/errored transactions that have passed the retention period
	txs.TrimFinalizedErroredTxs()

	// Check finalized message past retention is deleted
	_, exists := txs.finalizedErroredTxs[finalizedMsg1ID]
	require.False(t, exists)

	// Check errored message past retention is deleted
	_, exists = txs.finalizedErroredTxs[erroredMsgID]
	require.False(t, exists)

	// Check finalized message within retention period still exists
	_, exists = txs.finalizedErroredTxs[finalizedMsg2ID]
	require.True(t, exists)
}

func TestPendingTxContext_expired(t *testing.T) {
	t.Parallel()
	_, cancel := context.WithCancel(tests.Context(t))
	sig := solana.Signature{}
	txs := newPendingTxContext()

	msg := pendingTx{id: uuid.NewString()}
	err := txs.New(msg, sig, cancel)
	assert.NoError(t, err)

	msg, exists := txs.broadcastedTxs[msg.id]
	require.True(t, exists)

	// Set createTs to 10 seconds ago
	msg.createTs = time.Now().Add(-10 * time.Second)
	txs.broadcastedTxs[msg.id] = msg

	assert.False(t, txs.Expired(sig, 0*time.Second))  // false if timeout 0
	assert.True(t, txs.Expired(sig, 5*time.Second))   // expired for 5s lifetime
	assert.False(t, txs.Expired(sig, 60*time.Second)) // not expired for 60s lifetime

	id, err := txs.Remove(sig)
	assert.NoError(t, err)
	assert.Equal(t, msg.id, id)
	assert.False(t, txs.Expired(sig, 60*time.Second)) // no longer exists, should return false
}

func TestPendingTxContext_race(t *testing.T) {
	t.Run("new", func(t *testing.T) {
		txCtx := newPendingTxContext()
		var wg sync.WaitGroup
		wg.Add(2)
		var err [2]error

		go func() {
			err[0] = txCtx.New(pendingTx{id: uuid.NewString()}, solana.Signature{}, func() {})
			wg.Done()
		}()
		go func() {
			err[1] = txCtx.New(pendingTx{id: uuid.NewString()}, solana.Signature{}, func() {})
			wg.Done()
		}()

		wg.Wait()
		assert.True(t, (err[0] != nil && err[1] == nil) || (err[0] == nil && err[1] != nil), "one and only one 'add' should have errored")
	})

	t.Run("add signature", func(t *testing.T) {
		txCtx := newPendingTxContext()
		msg := pendingTx{id: uuid.NewString()}
		createErr := txCtx.New(msg, solana.Signature{}, func() {})
		require.NoError(t, createErr)
		var wg sync.WaitGroup
		wg.Add(2)
		var err [2]error

		go func() {
			err[0] = txCtx.AddSignature(msg.id, solana.Signature{1})
			wg.Done()
		}()
		go func() {
			err[1] = txCtx.AddSignature(msg.id, solana.Signature{1})
			wg.Done()
		}()

		wg.Wait()
		assert.True(t, (err[0] != nil && err[1] == nil) || (err[0] == nil && err[1] != nil), "one and only one 'add' should have errored")
	})

	t.Run("remove", func(t *testing.T) {
		txCtx := newPendingTxContext()
		msg := pendingTx{id: uuid.NewString()}
		err := txCtx.New(msg, solana.Signature{}, func() {})
		require.NoError(t, err)
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			assert.NotPanics(t, func() { txCtx.Remove(solana.Signature{}) }) //nolint // no need to check error
			wg.Done()
		}()
		go func() {
			assert.NotPanics(t, func() { txCtx.Remove(solana.Signature{}) }) //nolint // no need to check error
			wg.Done()
		}()

		wg.Wait()
	})
}

func TestGetTxState(t *testing.T) {
	t.Parallel()
	_, cancel := context.WithCancel(tests.Context(t))
	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	broadcastedSig := randomSignature(t)
	processedSig := randomSignature(t)
	confirmedSig := randomSignature(t)
	finalizedSig := randomSignature(t)
	erroredSig := randomSignature(t)
	fatallyErroredSig := randomSignature(t)

	// Create new broadcasted transaction with extra sig
	broadcastedMsg := pendingTx{id: uuid.NewString()}
	err := txs.New(broadcastedMsg, broadcastedSig, cancel)
	require.NoError(t, err)

	var state TxState
	// Create new processed transaction
	processedMsg := pendingTx{id: uuid.NewString()}
	err = txs.New(processedMsg, processedSig, cancel)
	require.NoError(t, err)
	id, err := txs.OnProcessed(processedSig)
	require.NoError(t, err)
	require.Equal(t, processedMsg.id, id)
	// Check Processed state is returned
	state, err = txs.GetTxState(processedMsg.id)
	require.NoError(t, err)
	require.Equal(t, Processed, state)

	// Create new confirmed transaction
	confirmedMsg := pendingTx{id: uuid.NewString()}
	err = txs.New(confirmedMsg, confirmedSig, cancel)
	require.NoError(t, err)
	id, err = txs.OnConfirmed(confirmedSig)
	require.NoError(t, err)
	require.Equal(t, confirmedMsg.id, id)
	// Check Confirmed state is returned
	state, err = txs.GetTxState(confirmedMsg.id)
	require.NoError(t, err)
	require.Equal(t, Confirmed, state)

	// Create new finalized transaction
	finalizedMsg := pendingTx{id: uuid.NewString()}
	err = txs.New(finalizedMsg, finalizedSig, cancel)
	require.NoError(t, err)
	id, err = txs.OnFinalized(finalizedSig, retentionTimeout)
	require.NoError(t, err)
	require.Equal(t, finalizedMsg.id, id)
	// Check Finalized state is returned
	state, err = txs.GetTxState(finalizedMsg.id)
	require.NoError(t, err)
	require.Equal(t, Finalized, state)

	// Create new errored transaction
	erroredMsg := pendingTx{id: uuid.NewString()}
	err = txs.New(erroredMsg, erroredSig, cancel)
	require.NoError(t, err)
	id, err = txs.OnError(erroredSig, retentionTimeout, Errored, 0)
	require.NoError(t, err)
	require.Equal(t, erroredMsg.id, id)
	// Check Errored state is returned
	state, err = txs.GetTxState(erroredMsg.id)
	require.NoError(t, err)
	require.Equal(t, Errored, state)

	// Create new fatally errored transaction
	fatallyErroredMsg := pendingTx{id: uuid.NewString()}
	err = txs.New(fatallyErroredMsg, fatallyErroredSig, cancel)
	require.NoError(t, err)
	id, err = txs.OnError(fatallyErroredSig, retentionTimeout, FatallyErrored, 0)
	require.NoError(t, err)
	require.Equal(t, fatallyErroredMsg.id, id)
	// Check Errored state is returned
	state, err = txs.GetTxState(fatallyErroredMsg.id)
	require.NoError(t, err)
	require.Equal(t, FatallyErrored, state)

	// Check NotFound state is returned if unknown id provided
	state, err = txs.GetTxState("unknown id")
	require.Error(t, err)
	require.Equal(t, NotFound, state)
}

func randomSignature(t *testing.T) solana.Signature {
	// make random signature
	sig := make([]byte, 64)
	_, err := rand.Read(sig)
	require.NoError(t, err)

	return solana.SignatureFromBytes(sig)
}

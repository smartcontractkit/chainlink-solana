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

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

func TestPendingTxContext_add_remove_multiple(t *testing.T) {
	var wg sync.WaitGroup
	ctx := t.Context()

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
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)
		ids[sig] = msg.id
	}

	// cannot add signature for non existent ID
	require.Error(t, txs.AddSignature(func() {}, uuid.NewString(), solana.Signature{}))
	assert.Equal(t, n, len(txs.sigToTxInfo))

	// stop all sub processes
	for sig, id := range ids {
		_, err := txs.OnError(ctx, sig, 0, utils.Errored, TxFailReject)
		assert.NoError(t, err)
		// sig does not exist in map anymore
		_, exists := txs.sigToTxInfo[sig]
		require.False(t, exists)
		// tx does not exist in broadcasted map anymore
		_, exists = txs.broadcastedProcessedTxs[id]
		require.False(t, exists)
		// tx does not exist in finalized/errored map since retention is set to 0
		_, exists = txs.finalizedErroredTxs[id]
		require.False(t, exists)
	}
	wg.Wait()
}

func TestPendingTxContext_New(t *testing.T) {
	t.Parallel()

	txs := newPendingTxContext()
	ctx := t.Context()
	_, cancel := context.WithCancel(ctx)

	t.Run("successfully adds new transaction in pending state", func(t *testing.T) {
		msg := pendingTx{id: uuid.NewString()}
		require.NoError(t, txs.New(msg))
	})

	t.Run("errors if transaction already exists in pending state", func(t *testing.T) {
		msg := pendingTx{id: uuid.NewString()}
		require.NoError(t, txs.New(msg))
		require.ErrorIs(t, txs.New(msg), ErrIDAlreadyExists)
	})

	t.Run("errors if transaction already exists in broadcasted state", func(t *testing.T) {
		msg := pendingTx{id: uuid.NewString()}
		sig := randomSignature(t)
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)
		require.ErrorIs(t, txs.New(msg), ErrIDAlreadyExists)
	})

	t.Run("errors if transaction already exists in processed state", func(t *testing.T) {
		msg := pendingTx{id: uuid.NewString()}
		sig := randomSignature(t)
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)
		_, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.ErrorIs(t, txs.New(msg), ErrIDAlreadyExists)
	})

	t.Run("errors if transaction already exists in confirmed state", func(t *testing.T) {
		msg := pendingTx{id: uuid.NewString()}
		sig := randomSignature(t)
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)
		_, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		_, err = txs.OnConfirmed(ctx, sig)
		require.NoError(t, err)
		require.ErrorIs(t, txs.New(msg), ErrIDAlreadyExists)
	})

	t.Run("errors if transaction already exists in finalized state", func(t *testing.T) {
		msg := pendingTx{id: uuid.NewString()}
		sig := randomSignature(t)
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)
		_, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		_, err = txs.OnConfirmed(ctx, sig)
		require.NoError(t, err)
		_, err = txs.OnFinalized(ctx, sig, 1*time.Second)
		require.NoError(t, err)
		require.ErrorIs(t, txs.New(msg), ErrIDAlreadyExists)
	})

	t.Run("errors if transaction already exists in errored state", func(t *testing.T) {
		msg := pendingTx{id: uuid.NewString()}
		require.NoError(t, txs.OnPrebroadcastError(ctx, msg.id, 1*time.Second, utils.Errored, TxFailReject))
		require.ErrorIs(t, txs.New(msg), ErrIDAlreadyExists)
	})
}

func TestPendingTxContext_OnBroadcasted(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	sig := randomSignature(t)
	txs := newPendingTxContext()

	// Create new transaction
	msg := pendingTx{id: uuid.NewString()}
	addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

	// Check it exists in signature map and mapped to the correct txID
	info, exists := txs.sigToTxInfo[sig]
	require.True(t, exists, "signature should exist in sigToID map")
	require.Equal(t, msg.id, info.id, "signature should map to correct transaction ID")

	// Check it exists in broadcasted map and that sigs match
	tx, exists := txs.broadcastedProcessedTxs[msg.id]
	require.True(t, exists, "transaction should exist in broadcastedProcessedTxs map")
	require.Len(t, tx.signatures, 1, "transaction should have one signature")
	require.Equal(t, sig, tx.signatures[0], "signature should match")

	// Check status is Broadcasted
	require.Equal(t, utils.Broadcasted, tx.state, "transaction state should be Broadcasted")

	// Check it does not exist in confirmed nor finalized maps
	_, exists = txs.confirmedTxs[msg.id]
	require.False(t, exists, "transaction should not exist in confirmedTxs map")
	_, exists = txs.finalizedErroredTxs[msg.id]
	require.False(t, exists, "transaction should not exist in finalizedErroredTxs map")

	// Attempt to mark the same transaction as broadcasted again
	err := txs.OnBroadcasted(msg)
	require.ErrorIs(t, err, ErrAlreadyInExpectedState, "expected ErrAlreadyInExpectedState when adding duplicate transaction ID")

	// Simulate moving the transaction to confirmedTxs map
	_, err = txs.OnConfirmed(ctx, sig)
	require.NoError(t, err, "expected no error when confirming transaction")

	// Simulate moving the transaction to finalizedErroredTxs map
	_, err = txs.OnFinalized(ctx, sig, 10*time.Second)
	require.NoError(t, err, "expected no error when finalizing transaction")
}

func TestPendingTxContext_add_signature(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	txs := newPendingTxContext()

	t.Run("successfully add signature to transaction", func(t *testing.T) {
		sig1 := randomSignature(t)
		sig2 := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig1, cancel)

		err := txs.AddSignature(cancel, msg.id, sig2)
		require.NoError(t, err)

		// Check signature map
		info, exists := txs.sigToTxInfo[sig1]
		require.True(t, exists)
		require.Equal(t, msg.id, info.id)
		info, exists = txs.sigToTxInfo[sig2]
		require.True(t, exists)
		require.Equal(t, msg.id, info.id)

		// Check broadcasted map
		tx, exists := txs.broadcastedProcessedTxs[msg.id]
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
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		err := txs.AddSignature(cancel, msg.id, sig)
		require.ErrorIs(t, err, ErrSigAlreadyExists)
	})

	t.Run("fails to add signature for missing transaction", func(t *testing.T) {
		sig1 := randomSignature(t)
		sig2 := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig1, cancel)

		err := txs.AddSignature(cancel, "bad id", sig2)
		require.ErrorIs(t, err, ErrTransactionNotFound)
	})

	t.Run("fails to add signature for confirmed transaction", func(t *testing.T) {
		sig1 := randomSignature(t)
		sig2 := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig1, cancel)

		// Transition to processed state
		id, err := txs.OnProcessed(sig1)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(ctx, sig1)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		err = txs.AddSignature(cancel, msg.id, sig2)
		require.ErrorIs(t, err, ErrTransactionNotFound)
	})
}

func TestPendingTxContext_on_broadcasted_processed(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	t.Run("successfully transition transaction from broadcasted to processed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to processed state
		id, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it exists in signature map
		info, exists := txs.sigToTxInfo[sig]
		require.True(t, exists)
		require.Equal(t, msg.id, info.id)

		// Check it exists in broadcasted map
		tx, exists := txs.broadcastedProcessedTxs[msg.id]
		require.True(t, exists)
		require.Len(t, tx.signatures, 1)
		require.Equal(t, sig, tx.signatures[0])

		// Check status is Processed
		require.Equal(t, utils.Processed, tx.state)

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
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to processed state
		id, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(ctx, sig)
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
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to processed state
		id, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(ctx, sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to finalized state
		id, err = txs.OnFinalized(ctx, sig, retentionTimeout)
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
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to errored state
		id, err := txs.OnError(ctx, sig, retentionTimeout, utils.Errored, 0)
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
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

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
	ctx, cancel := context.WithCancel(t.Context())
	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	t.Run("successfully transition transaction from broadcasted/processed to confirmed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to processed state
		id, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(ctx, sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it exists in signature map
		info, exists := txs.sigToTxInfo[sig]
		require.True(t, exists)
		require.Equal(t, msg.id, info.id)

		// Check it does not exist in broadcasted map
		_, exists = txs.broadcastedProcessedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in confirmed map
		tx, exists := txs.confirmedTxs[msg.id]
		require.True(t, exists)
		require.Len(t, tx.signatures, 1)
		require.Equal(t, sig, tx.signatures[0])

		// Check status is Confirmed
		require.Equal(t, utils.Confirmed, tx.state)

		// Check it does not exist in finalized map
		_, exists = txs.finalizedErroredTxs[msg.id]
		require.False(t, exists)
	})

	t.Run("fails to transition transaction from finalized to confirmed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to processed state
		id, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(ctx, sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to finalized state
		id, err = txs.OnFinalized(ctx, sig, retentionTimeout)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition back to processed state
		_, err = txs.OnConfirmed(ctx, sig)
		require.Error(t, err)
	})

	t.Run("fails to transition transaction from errored to confirmed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to errored state
		id, err := txs.OnError(ctx, sig, retentionTimeout, utils.Errored, 0)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition back to confirmed state
		_, err = txs.OnConfirmed(ctx, sig)
		require.Error(t, err)
	})

	t.Run("predefined error if transaction already in confirmed state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to processed state
		id, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(ctx, sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// No error if OnConfirmed called again
		_, err = txs.OnConfirmed(ctx, sig)
		require.ErrorIs(t, err, ErrAlreadyInExpectedState)
	})
}

func TestPendingTxContext_on_finalized(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	t.Run("successfully transition transaction from broadcasted/processed to finalized state", func(t *testing.T) {
		sig1 := randomSignature(t)
		sig2 := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig1, cancel)

		// Add second signature
		err := txs.AddSignature(cancel, msg.id, sig2)
		require.NoError(t, err)

		// Transition to finalized state
		id, err := txs.OnFinalized(ctx, sig1, retentionTimeout)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedProcessedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in finalized map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Finalized
		require.Equal(t, utils.Finalized, tx.state)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToTxInfo[sig1]
		require.False(t, exists)
		_, exists = txs.sigToTxInfo[sig2]
		require.False(t, exists)
	})

	t.Run("successfully transition transaction from confirmed to finalized state", func(t *testing.T) {
		sig1 := randomSignature(t)
		sig2 := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig1, cancel)

		// Add second signature
		err := txs.AddSignature(cancel, msg.id, sig2)
		require.NoError(t, err)

		// Transition to processed state
		id, err := txs.OnProcessed(sig1)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(ctx, sig1)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to finalized state
		id, err = txs.OnFinalized(ctx, sig1, retentionTimeout)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedProcessedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in finalized map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Finalized
		require.Equal(t, utils.Finalized, tx.state)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToTxInfo[sig1]
		require.False(t, exists)
		_, exists = txs.sigToTxInfo[sig2]
		require.False(t, exists)
	})

	t.Run("successfully delete transaction when finalized with 0 retention timeout", func(t *testing.T) {
		sig1 := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig1, cancel)

		// Transition to processed state
		id, err := txs.OnProcessed(sig1)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to confirmed state
		id, err = txs.OnConfirmed(ctx, sig1)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to finalized state
		id, err = txs.OnFinalized(ctx, sig1, 0*time.Second)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedProcessedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in finalized map
		_, exists = txs.finalizedErroredTxs[msg.id]
		require.False(t, exists)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToTxInfo[sig1]
		require.False(t, exists)
	})

	t.Run("fails to transition transaction from errored to finalized state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to errored state
		id, err := txs.OnError(ctx, sig, retentionTimeout, utils.Errored, 0)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition back to confirmed state
		_, err = txs.OnFinalized(ctx, sig, retentionTimeout)
		require.Error(t, err)
	})
}

func TestPendingTxContext_on_error(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	t.Run("successfully transition transaction from broadcasted/processed to errored state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to errored state
		id, err := txs.OnError(ctx, sig, retentionTimeout, utils.Errored, 0)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedProcessedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in errored map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Finalized
		require.Equal(t, utils.Errored, tx.state)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToTxInfo[sig]
		require.False(t, exists)
	})

	t.Run("successfully transitions transaction from confirmed to errored state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to errored state
		id, err := txs.OnConfirmed(ctx, sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to errored state
		id, err = txs.OnError(ctx, sig, retentionTimeout, utils.Errored, 0)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedProcessedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in errored map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Finalized
		require.Equal(t, utils.Errored, tx.state)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToTxInfo[sig]
		require.False(t, exists)
	})

	t.Run("successfully transition transaction from broadcasted/processed to fatally errored state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to fatally errored state
		id, err := txs.OnError(ctx, sig, retentionTimeout, utils.FatallyErrored, 0)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedProcessedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in errored map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Errored
		require.Equal(t, utils.FatallyErrored, tx.state)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToTxInfo[sig]
		require.False(t, exists)
	})

	t.Run("successfully delete transaction when errored with 0 retention timeout", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to confirmed state
		id, err := txs.OnConfirmed(ctx, sig)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to errored state
		id, err = txs.OnError(ctx, sig, 0*time.Second, utils.Errored, 0)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Check it does not exist in broadcasted map
		_, exists := txs.broadcastedProcessedTxs[msg.id]
		require.False(t, exists)

		// Check it does not exist in confirmed map
		_, exists = txs.confirmedTxs[msg.id]
		require.False(t, exists)

		// Check it exists in errored map
		_, exists = txs.finalizedErroredTxs[msg.id]
		require.False(t, exists)

		// Check sigs do no exist in signature map
		_, exists = txs.sigToTxInfo[sig]
		require.False(t, exists)
	})

	t.Run("fails to transition transaction from finalized to errored state", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to finalized state
		id, err := txs.OnFinalized(ctx, sig, retentionTimeout)
		require.NoError(t, err)
		require.Equal(t, msg.id, id)

		// Transition to errored state
		id, err = txs.OnError(ctx, sig, retentionTimeout, utils.Errored, 0)
		require.Error(t, err)
		require.Equal(t, "", id)
	})

	t.Run("successfully clears out signature if transaction not found", func(t *testing.T) {
		sig := randomSignature(t)
		id := uuid.NewString()
		info := txInfo{
			id:    id,
			state: utils.Confirmed,
		}
		txs.sigToTxInfo[sig] = info

		txID, err := txs.OnError(ctx, sig, retentionTimeout, utils.Errored, 0)
		require.NoError(t, err)
		require.Equal(t, id, txID)
		_, exists := txs.sigToTxInfo[sig]
		require.False(t, exists)
	})

	t.Run("successfully clears out signature and does not update existing error entry", func(t *testing.T) {
		sig := randomSignature(t)
		id := uuid.NewString()
		info := txInfo{
			id:    id,
			state: utils.Errored,
		}
		txs.sigToTxInfo[sig] = info
		tx := finishedTx{retentionTs: time.Now().Add(retentionTimeout), state: utils.Errored}
		txs.finalizedErroredTxs[id] = tx

		txID, err := txs.OnError(ctx, sig, retentionTimeout, utils.FatallyErrored, 0)
		require.NoError(t, err)
		require.Equal(t, id, txID)
		_, exists := txs.sigToTxInfo[sig]
		require.False(t, exists) // signature should be cleared
		erroredTx, erroredExists := txs.finalizedErroredTxs[id]
		require.True(t, erroredExists)                   // errored tx should still exist in map
		require.Equal(t, utils.Errored, erroredTx.state) // errored tx should retain the original state
	})
}

func TestPendingTxContext_on_prebroadcast_error(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	t.Run("successfully adds transaction with errored state", func(t *testing.T) {
		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		// Transition to errored state
		err := txs.OnPrebroadcastError(ctx, msg.id, retentionTimeout, utils.Errored, 0)
		require.NoError(t, err)

		// Check it exists in errored map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Errored
		require.Equal(t, utils.Errored, tx.state)
	})

	t.Run("successfully adds transaction with fatally errored state", func(t *testing.T) {
		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}

		// Transition to fatally errored state
		err := txs.OnPrebroadcastError(ctx, msg.id, retentionTimeout, utils.FatallyErrored, 0)
		require.NoError(t, err)

		// Check it exists in errored map
		tx, exists := txs.finalizedErroredTxs[msg.id]
		require.True(t, exists)

		// Check status is Errored
		require.Equal(t, utils.FatallyErrored, tx.state)
	})

	t.Run("fails to add transaction to errored map if id exists in another map already", func(t *testing.T) {
		sig := randomSignature(t)

		// Create new transaction
		msg := pendingTx{id: uuid.NewString()}
		addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

		// Transition to errored state
		err := txs.OnPrebroadcastError(ctx, msg.id, retentionTimeout, utils.FatallyErrored, 0)
		require.ErrorIs(t, err, ErrIDAlreadyExists)
	})

	t.Run("predefined error if transaction already in errored state", func(t *testing.T) {
		txID := uuid.NewString()

		// Transition to errored state
		err := txs.OnPrebroadcastError(ctx, txID, retentionTimeout, utils.Errored, 0)
		require.NoError(t, err)

		// Transition back to errored state
		err = txs.OnPrebroadcastError(ctx, txID, retentionTimeout, utils.Errored, 0)
		require.ErrorIs(t, err, ErrAlreadyInExpectedState)
	})
}

func TestPendingTxContext_RevertToAwaitingBroadcast(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())

	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	// Create new broadcasted transaction with extra sig
	broadcastedID := uuid.NewString()
	broadcastedSig1 := randomSignature(t)
	broadcastedSig2 := randomSignature(t)
	broadcastedMsg := pendingTx{id: broadcastedID}
	addBroadcastedTxWithSigAndCancel(t, txs, broadcastedMsg, broadcastedSig1, cancel)
	err := txs.AddSignature(cancel, broadcastedMsg.id, broadcastedSig2)
	require.NoError(t, err)

	// Create new processed transaction
	processedID := uuid.NewString()
	processedSig := randomSignature(t)
	processedMsg := pendingTx{id: processedID}
	addBroadcastedTxWithSigAndCancel(t, txs, processedMsg, processedSig, cancel)
	id, err := txs.OnProcessed(processedSig)
	require.NoError(t, err)
	require.Equal(t, processedMsg.id, id)

	// Create new confirmed transaction
	confirmedID := uuid.NewString()
	confirmedSig := randomSignature(t)
	confirmedMsg := pendingTx{id: confirmedID}
	addBroadcastedTxWithSigAndCancel(t, txs, confirmedMsg, confirmedSig, cancel)
	id, err = txs.OnConfirmed(ctx, confirmedSig)
	require.NoError(t, err)
	require.Equal(t, confirmedMsg.id, id)

	// Create new finalized transaction
	finalizedID := uuid.NewString()
	finalizedSig := randomSignature(t)
	finalizedMsg := pendingTx{id: finalizedID}
	addBroadcastedTxWithSigAndCancel(t, txs, finalizedMsg, finalizedSig, cancel)
	id, err = txs.OnFinalized(ctx, finalizedSig, retentionTimeout)
	require.NoError(t, err)
	require.Equal(t, finalizedMsg.id, id)

	// Create new errored transaction
	erroredID := uuid.NewString()
	erroredSig := randomSignature(t)
	erroredMsg := pendingTx{id: erroredID}
	addBroadcastedTxWithSigAndCancel(t, txs, erroredMsg, erroredSig, cancel)
	id, err = txs.OnError(ctx, erroredSig, retentionTimeout, utils.Errored, 0)
	require.NoError(t, err)
	require.Equal(t, erroredMsg.id, id)

	// Revert broadcasted transaction back to pending
	err = txs.RevertToAwaitingBroadcast(broadcastedID)
	require.NoError(t, err)

	// Check removed from broadcasted map
	_, exists := txs.broadcastedProcessedTxs[broadcastedMsg.id]
	require.False(t, exists)

	// Check that it is moved back to the pending map
	tx, exists := txs.queuedTxs[broadcastedMsg.id]
	require.True(t, exists)
	require.Equal(t, utils.AwaitingBroadcast, tx.state)

	// Check all signatures removed from sig map
	_, exists = txs.sigToTxInfo[broadcastedSig1]
	require.False(t, exists)
	_, exists = txs.sigToTxInfo[broadcastedSig2]
	require.False(t, exists)

	// Revert processed transaction back to pending
	err = txs.RevertToAwaitingBroadcast(processedID)
	require.NoError(t, err)

	// Check removed from broadcasted map
	tx, exists = txs.broadcastedProcessedTxs[processedMsg.id]
	require.False(t, exists)

	// Check all signatures removed from sig map
	_, exists = txs.sigToTxInfo[processedSig]
	require.False(t, exists)

	// Check that it is moved back to the pending map
	tx, exists = txs.queuedTxs[processedMsg.id]
	require.True(t, exists)
	require.Equal(t, utils.AwaitingBroadcast, tx.state)

	// Revert confirmed transaction back to pending
	err = txs.RevertToAwaitingBroadcast(confirmedID)
	require.NoError(t, err)

	// Check removed from confirmed map
	_, exists = txs.confirmedTxs[confirmedMsg.id]
	require.False(t, exists)

	// Check that it is moved back to the pending map
	tx, exists = txs.queuedTxs[confirmedMsg.id]
	require.True(t, exists)
	require.Equal(t, utils.AwaitingBroadcast, tx.state)

	// Check all signatures removed from sig map
	_, exists = txs.sigToTxInfo[confirmedSig]
	require.False(t, exists)

	// Check RevertToAwaitingBroadcast cannot be called on finalized transaction
	err = txs.RevertToAwaitingBroadcast(finalizedID)
	require.Error(t, err)

	// Check remove cannot be called on errored transaction
	err = txs.RevertToAwaitingBroadcast(erroredID)
	require.Error(t, err)

	// Check sig list is empty after all removals
	require.Empty(t, txs.ListAllSigs(ctx))
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
	_, cancel := context.WithCancel(t.Context())
	sig := solana.Signature{}
	txs := newPendingTxContext()
	txID := uuid.NewString()

	msg := pendingTx{id: txID}
	addBroadcastedTxWithSigAndCancel(t, txs, msg, sig, cancel)

	msg, exists := txs.broadcastedProcessedTxs[msg.id]
	require.True(t, exists)

	// Set createTs to 10 seconds ago
	msg.createTs = time.Now().Add(-10 * time.Second)
	txs.broadcastedProcessedTxs[msg.id] = msg

	assert.False(t, txs.Expired(sig, 0*time.Second))  // false if timeout 0
	assert.True(t, txs.Expired(sig, 5*time.Second))   // expired for 5s lifetime
	assert.False(t, txs.Expired(sig, 60*time.Second)) // not expired for 60s lifetime
}

func TestPendingTxContext_race(t *testing.T) {
	t.Run("new", func(t *testing.T) {
		txCtx := newPendingTxContext()
		var wg sync.WaitGroup
		txID := uuid.NewString()
		wg.Add(2)
		var err [2]error

		go func() {
			err[0] = txCtx.New(pendingTx{id: txID})
			wg.Done()
		}()
		go func() {
			err[1] = txCtx.New(pendingTx{id: txID})
			wg.Done()
		}()

		wg.Wait()
		assert.True(t, (err[0] != nil && err[1] == nil) || (err[0] == nil && err[1] != nil), "one and only one 'add' should have errored")
	})

	t.Run("add signature", func(t *testing.T) {
		txCtx := newPendingTxContext()
		msg := pendingTx{id: uuid.NewString()}
		createErr := txCtx.New(msg)
		require.NoError(t, createErr)
		broadcastErr := txCtx.OnBroadcasted(msg)
		require.NoError(t, broadcastErr)
		var wg sync.WaitGroup
		wg.Add(2)
		var err [2]error

		go func() {
			err[0] = txCtx.AddSignature(func() {}, msg.id, solana.Signature{1})
			wg.Done()
		}()
		go func() {
			err[1] = txCtx.AddSignature(func() {}, msg.id, solana.Signature{1})
			wg.Done()
		}()

		wg.Wait()
		assert.True(t, (err[0] != nil && err[1] == nil) || (err[0] == nil && err[1] != nil), "one and only one 'add' should have errored")
	})

	t.Run("remove", func(t *testing.T) {
		txCtx := newPendingTxContext()
		txID := uuid.NewString()
		msg := pendingTx{id: txID}
		err := txCtx.New(msg)
		require.NoError(t, err)
		err = txCtx.OnPrebroadcastError(t.Context(), msg.id, 1*time.Millisecond, utils.Errored, TxFailRevert)
		require.NoError(t, err)
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			assert.NotPanics(t, func() { txCtx.TrimFinalizedErroredTxs() }) //nolint // no need to check error
			assert.NotPanics(t, func() { txCtx.TrimFinalizedErroredTxs() }) //nolint // no need to check error
			wg.Done()
		}()
		go func() {
			assert.NotPanics(t, func() { txCtx.TrimFinalizedErroredTxs() }) //nolint // no need to check error
			assert.NotPanics(t, func() { txCtx.TrimFinalizedErroredTxs() }) //nolint // no need to check error
			wg.Done()
		}()

		wg.Wait()
	})
}

func TestGetTxState(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	txs := newPendingTxContext()
	retentionTimeout := 5 * time.Second

	// Create new broadcasted transaction with extra sig
	broadcastedSig := randomSignature(t)
	broadcastedMsg := pendingTx{id: uuid.NewString()}
	addBroadcastedTxWithSigAndCancel(t, txs, broadcastedMsg, broadcastedSig, cancel)

	// Create new processed transaction
	var state utils.TxState
	processedSig := randomSignature(t)
	processedMsg := pendingTx{id: uuid.NewString()}
	addBroadcastedTxWithSigAndCancel(t, txs, processedMsg, processedSig, cancel)
	id, err := txs.OnProcessed(processedSig)
	require.NoError(t, err)
	require.Equal(t, processedMsg.id, id)

	// Check Processed state is returned
	state, exists := txs.GetTxState(processedMsg.id)
	require.True(t, exists)
	require.Equal(t, utils.Processed, state)

	// Create new confirmed transaction
	confirmedSig := randomSignature(t)
	confirmedMsg := pendingTx{id: uuid.NewString()}
	addBroadcastedTxWithSigAndCancel(t, txs, confirmedMsg, confirmedSig, cancel)
	id, err = txs.OnConfirmed(ctx, confirmedSig)
	require.NoError(t, err)
	require.Equal(t, confirmedMsg.id, id)

	// Check Confirmed state is returned
	state, exists = txs.GetTxState(confirmedMsg.id)
	require.True(t, exists)
	require.Equal(t, utils.Confirmed, state)

	// Create new finalized transaction
	finalizedSig := randomSignature(t)
	finalizedMsg := pendingTx{id: uuid.NewString()}
	addBroadcastedTxWithSigAndCancel(t, txs, finalizedMsg, finalizedSig, cancel)
	id, err = txs.OnFinalized(ctx, finalizedSig, retentionTimeout)
	require.NoError(t, err)
	require.Equal(t, finalizedMsg.id, id)

	// Check Finalized state is returned
	state, exists = txs.GetTxState(finalizedMsg.id)
	require.True(t, exists)
	require.Equal(t, utils.Finalized, state)

	// Create new errored transaction
	erroredSig := randomSignature(t)
	erroredMsg := pendingTx{id: uuid.NewString()}
	addBroadcastedTxWithSigAndCancel(t, txs, erroredMsg, erroredSig, cancel)
	id, err = txs.OnError(ctx, erroredSig, retentionTimeout, utils.Errored, 0)
	require.NoError(t, err)
	require.Equal(t, erroredMsg.id, id)

	// Check Errored state is returned
	state, exists = txs.GetTxState(erroredMsg.id)
	require.True(t, exists)
	require.Equal(t, utils.Errored, state)

	// Create new fatally errored transaction
	fatallyErroredSig := randomSignature(t)
	fatallyErroredMsg := pendingTx{id: uuid.NewString()}
	addBroadcastedTxWithSigAndCancel(t, txs, fatallyErroredMsg, fatallyErroredSig, cancel)
	id, err = txs.OnError(ctx, fatallyErroredSig, retentionTimeout, utils.FatallyErrored, 0)
	require.NoError(t, err)
	require.Equal(t, fatallyErroredMsg.id, id)

	// Check Errored state is returned
	state, exists = txs.GetTxState(fatallyErroredMsg.id)
	require.True(t, exists)
	require.Equal(t, utils.FatallyErrored, state)

	// Check NotFound state is returned if unknown id provided
	state, exists = txs.GetTxState("unknown id")
	require.False(t, exists)
	require.Equal(t, utils.NotFound, state)
}

func randomSignature(t *testing.T) solana.Signature {
	// make random signature
	sig := make([]byte, 64)
	_, err := rand.Read(sig)
	require.NoError(t, err)

	return solana.SignatureFromBytes(sig)
}

func TestPendingTxContext_ListAllExpiredBroadcastedTxs(t *testing.T) {
	tests := []struct {
		name            string
		setup           func(t *testing.T, ctx *pendingTxContext)
		currBlockHeight uint64
		expectedTxIDs   []string
	}{
		{
			name: "No broadcasted transactions",
			setup: func(t *testing.T, ctx *pendingTxContext) {
				// No setup needed; broadcastedProcessedTxs remains empty
			},
			currBlockHeight: 1000,
			expectedTxIDs:   []string{},
		},
		{
			name: "No expired broadcasted transactions",
			setup: func(t *testing.T, ctx *pendingTxContext) {
				tx1 := pendingTx{
					id:                   "tx1",
					state:                utils.Broadcasted,
					lastValidBlockHeight: 1500,
				}
				tx2 := pendingTx{
					id:                   "tx2",
					state:                utils.Broadcasted,
					lastValidBlockHeight: 1600,
				}
				ctx.broadcastedProcessedTxs["tx1"] = tx1
				ctx.broadcastedProcessedTxs["tx2"] = tx2
			},
			currBlockHeight: 1400,
			expectedTxIDs:   []string{},
		},
		{
			name: "Some expired broadcasted transactions",
			setup: func(t *testing.T, ctx *pendingTxContext) {
				tx1 := pendingTx{
					id:                   "tx1",
					state:                utils.Broadcasted,
					lastValidBlockHeight: 1000,
				}
				tx2 := pendingTx{
					id:                   "tx2",
					state:                utils.Broadcasted,
					lastValidBlockHeight: 1500,
				}
				tx3 := pendingTx{
					id:                   "tx3",
					state:                utils.Broadcasted,
					lastValidBlockHeight: 900,
				}
				ctx.broadcastedProcessedTxs["tx1"] = tx1
				ctx.broadcastedProcessedTxs["tx2"] = tx2
				ctx.broadcastedProcessedTxs["tx3"] = tx3
			},
			currBlockHeight: 1200,
			expectedTxIDs:   []string{"tx1", "tx3"},
		},
		{
			name: "All broadcasted transactions expired with maxUint64",
			setup: func(t *testing.T, ctx *pendingTxContext) {
				tx1 := pendingTx{
					id:                   "tx1",
					state:                utils.Broadcasted,
					lastValidBlockHeight: 1000,
				}
				tx2 := pendingTx{
					id:                   "tx2",
					state:                utils.Broadcasted,
					lastValidBlockHeight: 1500,
				}
				ctx.broadcastedProcessedTxs["tx1"] = tx1
				ctx.broadcastedProcessedTxs["tx2"] = tx2
			},
			currBlockHeight: ^uint64(0), // maxUint64
			expectedTxIDs:   []string{"tx1", "tx2"},
		},
		{
			name: "Only broadcasted transactions are considered",
			setup: func(t *testing.T, ctx *pendingTxContext) {
				tx1 := pendingTx{
					id:                   "tx1",
					state:                utils.Broadcasted,
					lastValidBlockHeight: 800,
				}
				tx2 := pendingTx{
					id:                   "tx2",
					state:                utils.Processed, // Not Broadcasted
					lastValidBlockHeight: 700,
				}
				tx3 := pendingTx{
					id:                   "tx3",
					state:                utils.Processed, // Not Broadcasted
					lastValidBlockHeight: 600,
				}
				ctx.broadcastedProcessedTxs["tx1"] = tx1
				ctx.broadcastedProcessedTxs["tx2"] = tx2
				ctx.broadcastedProcessedTxs["tx3"] = tx3
			},
			currBlockHeight: 900,
			expectedTxIDs:   []string{"tx1"},
		},
		{
			name: "Broadcasted transactions with edge block heights",
			setup: func(t *testing.T, ctx *pendingTxContext) {
				tx1 := pendingTx{
					id:                   "tx1",
					state:                utils.Broadcasted,
					lastValidBlockHeight: 1000,
				}
				tx2 := pendingTx{
					id:                   "tx2",
					state:                utils.Broadcasted,
					lastValidBlockHeight: 999,
				}
				tx3 := pendingTx{
					id:                   "tx3",
					state:                utils.Broadcasted,
					lastValidBlockHeight: 1,
				}
				ctx.broadcastedProcessedTxs["tx1"] = tx1
				ctx.broadcastedProcessedTxs["tx2"] = tx2
				ctx.broadcastedProcessedTxs["tx3"] = tx3
			},
			currBlockHeight: 1000,
			expectedTxIDs:   []string{"tx2", "tx3"},
		},
	}

	for idx := range tests {
		t.Run(tests[idx].name, func(t *testing.T) {
			// Initialize a new PendingTxContext
			ctx := newPendingTxContext()

			// Setup the test case
			tests[idx].setup(t, ctx)

			// Execute the function under test
			result := ctx.ListAllExpiredBroadcastedTxs(tests[idx].currBlockHeight)

			// Extract the IDs from the result
			var resultIDs []string
			for _, tx := range result {
				resultIDs = append(resultIDs, tx.id)
			}

			// Assert that the expected IDs match the result IDs (order does not matter)
			assert.ElementsMatch(t, tests[idx].expectedTxIDs, resultIDs)
		})
	}
}

func addBroadcastedTxWithSigAndCancel(t *testing.T, txs *pendingTxContext, tx pendingTx, sig solana.Signature, cancel context.CancelFunc) {
	require.NoError(t, txs.New(tx))
	require.NoError(t, txs.OnBroadcasted(tx))
	require.NoError(t, txs.AddSignature(cancel, tx.id, sig))
}

func createTxAndAddSig(t *testing.T, txs *pendingTxContext) (string, solana.Signature) {
	sig := randomSignature(t)
	txID := uuid.NewString()
	tx := pendingTx{id: txID}
	require.NoError(t, txs.New(tx))
	require.NoError(t, txs.OnBroadcasted(tx))
	require.NoError(t, txs.AddSignature(func() {}, txID, sig))
	return txID, sig
}

func TestPendingTxContext_IsTxReorged(t *testing.T) {
	t.Parallel()
	txs := newPendingTxContext()
	ctx := t.Context()

	// This helper creates a brand new transaction/signature,
	// then sets the in-memory state to the provided memoryState
	setMemoryState := func(t *testing.T, txs *pendingTxContext, memoryState utils.TxState) (txID string, sig solana.Signature) {
		txID, sig = createTxAndAddSig(t, txs)

		switch memoryState {
		case utils.Processed:
			_, err := txs.OnProcessed(sig)
			require.NoError(t, err, "OnProcessed should succeed")
		case utils.Confirmed:
			_, err := txs.OnProcessed(sig)
			require.NoError(t, err)
			_, err = txs.OnConfirmed(ctx, sig)
			require.NoError(t, err, "OnConfirmed should succeed")
		case utils.Broadcasted: // do nothing; newly created sig is in memory=Broadcasted by default
		default:
			require.FailNowf(t, "unexpected memory state", "%v", memoryState)
		}
		return
	}

	tests := []struct {
		name        string
		memoryState utils.TxState
		chainState  utils.TxState
		wantReorg   bool
	}{
		{
			name:        "non-existent signature => no reorg",
			memoryState: utils.Broadcasted, // doesn't matter, we'll handle this case specially
			chainState:  utils.Broadcasted,
			wantReorg:   false,
		},
		{
			name:        "memory=Confirmed, chain=Confirmed => no reorg",
			memoryState: utils.Confirmed,
			chainState:  utils.Confirmed,
			wantReorg:   false,
		},
		{
			name:        "memory=Confirmed, chain=Processed => reorg",
			memoryState: utils.Confirmed,
			chainState:  utils.Processed,
			wantReorg:   true,
		},
		{
			name:        "memory=Confirmed, chain=NotFound => reorg",
			memoryState: utils.Confirmed,
			chainState:  utils.NotFound,
			wantReorg:   true,
		},
		{
			name:        "memory=Processed, chain=Confirmed => no reorg",
			memoryState: utils.Processed,
			chainState:  utils.Confirmed,
			wantReorg:   false,
		},
		{
			name:        "memory=Processed, chain=Processed => no reorg",
			memoryState: utils.Processed,
			chainState:  utils.Processed,
			wantReorg:   false,
		},
		{
			name:        "memory=Processed, chain=NotFound => reorg",
			memoryState: utils.Processed,
			chainState:  utils.NotFound,
			wantReorg:   true,
		},
		{
			name:        "memory=Broadcasted, chain=Confirmed => no reorg",
			memoryState: utils.Broadcasted,
			chainState:  utils.Confirmed,
			wantReorg:   false,
		},
		{
			name:        "memory=Broadcasted, chain=Processed => no reorg",
			memoryState: utils.Broadcasted,
			chainState:  utils.Processed,
			wantReorg:   false,
		},
		{
			name:        "memory=Broadcasted, chain=NotFound => no reorg",
			memoryState: utils.Broadcasted,
			chainState:  utils.NotFound,
			wantReorg:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// handle special case
			if tt.name == "non-existent signature => no reorg" {
				// don't create any signature in memory
				txID, hasReorg := txs.IsTxReorged(randomSignature(t), tt.chainState)
				require.False(t, hasReorg, "expected no reorg for unknown sig")
				require.Empty(t, txID, "expected empty txID for unknown sig")
				return
			}

			// create + set memory state, run IsTxReorged and assert for all other test cases
			creationTxID, sig := setMemoryState(t, txs, tt.memoryState)
			returnedTxID, hasReorg := txs.IsTxReorged(sig, tt.chainState)
			require.Equal(t, creationTxID, returnedTxID, "expected same txID")
			if tt.wantReorg {
				require.True(t, hasReorg, "expected reorg for memory=%v, chain=%v", tt.memoryState, tt.chainState)
			} else {
				require.False(t, hasReorg, "expected no reorg for memory=%v, chain=%v", tt.memoryState, tt.chainState)
			}
		})
	}
}

func TestPendingTxContext_GetReorgTx(t *testing.T) {
	t.Parallel()
	txs := newPendingTxContext()
	ctx := t.Context()

	t.Run("successfully retrieve broadcasted transaction", func(t *testing.T) {
		txID, _ := createTxAndAddSig(t, txs)

		tx, err := txs.GetPendingTx(txID)
		require.NoError(t, err)
		require.Equal(t, txID, tx.id)
		require.Equal(t, utils.Broadcasted, tx.state)
	})

	t.Run("successfully retrieve processed transaction", func(t *testing.T) {
		txID, sig := createTxAndAddSig(t, txs)
		_, err := txs.OnProcessed(sig)
		require.NoError(t, err)

		tx, err := txs.GetPendingTx(txID)
		require.NoError(t, err)
		require.Equal(t, txID, tx.id)
		require.Equal(t, utils.Processed, tx.state)
	})

	t.Run("successfully retrieve confirmed transaction", func(t *testing.T) {
		txID, sig := createTxAndAddSig(t, txs)
		_, err := txs.OnProcessed(sig)
		require.NoError(t, err)
		_, err = txs.OnConfirmed(ctx, sig)
		require.NoError(t, err)

		tx, err := txs.GetPendingTx(txID)
		require.NoError(t, err)
		require.Equal(t, txID, tx.id)
		require.Equal(t, utils.Confirmed, tx.state)
	})

	t.Run("fail to retrieve non-existent transaction", func(t *testing.T) {
		_, err := txs.GetPendingTx("non-existent-id")
		require.ErrorIs(t, err, ErrTransactionNotFound)
	})
}

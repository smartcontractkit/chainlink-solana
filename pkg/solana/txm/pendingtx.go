package txm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"golang.org/x/exp/maps"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

var (
	ErrAlreadyInExpectedState = errors.New("transaction already in expected state")
	ErrSigAlreadyExists       = errors.New("signature already exists")
	ErrIDAlreadyExists        = errors.New("id already exists")
	ErrSigDoesNotExist        = errors.New("signature does not exist")
	ErrTransactionNotFound    = errors.New("transaction not found for id")
)

type PendingTxContext interface {
	// New adds a new transaction in AwaitingBroadcast state to the storage
	New(msg pendingTx) error
	// AddSignature adds a new signature to a broadcasted transaction in the pending transaction context.
	// It associates the provided context and cancel function with the signature to manage retry and bumping cycles.
	AddSignature(cancel context.CancelFunc, id string, sig solana.Signature) error
	// RevertToAwaitingBroadcast reverts the transaction's status to AwaitingBroadcast. It also removes context and related signatures from storage associated to given tx id if not in finalized or errored state
	RevertToAwaitingBroadcast(id string) error
	// ListAllSigs returns all of the signatures being tracked for all transactions not yet finalized or errored
	ListAllSigs(ctx context.Context) []solana.Signature
	// ListAllExpiredBroadcastedTxs returns all the txes that are in broadcasted state and have expired for given block number compared against lastValidBlockHeight (last valid block number)
	// Passing maxUint64 as currBlockNumber will return all broadcasted txes.
	ListAllExpiredBroadcastedTxs(currBlockNumber uint64) []pendingTx
	// Expired returns whether or not confirmation timeout amount of time has passed since creation
	Expired(sig solana.Signature, confirmationTimeout time.Duration) bool
	// OnBroadcasted marks transaction as Broadcasted and moves it from the pending map to the broadcast map
	OnBroadcasted(msg pendingTx) error
	// OnProcessed marks transactions as Processed
	OnProcessed(sig solana.Signature) (string, error)
	// OnConfirmed marks transaction as Confirmed and moves it from broadcast map to confirmed map
	OnConfirmed(ctx context.Context, sig solana.Signature) (string, error)
	// OnFinalized marks transaction as Finalized, moves it from the broadcasted or confirmed map to finalized map, removes signatures from signature map to stop confirmation checks
	OnFinalized(ctx context.Context, sig solana.Signature, retentionTimeout time.Duration) (string, error)
	// OnPrebroadcastError adds transaction that has not yet been broadcasted to the finalized/errored map as errored, matches err type using enum
	OnPrebroadcastError(ctx context.Context, id string, retentionTimeout time.Duration, txState utils.TxState, errType TxErrType) error
	// OnError marks transaction as errored, matches err type using enum, moves it from the broadcasted or confirmed map to finalized/errored map, removes signatures from signature map to stop confirmation checks
	OnError(ctx context.Context, sig solana.Signature, retentionTimeout time.Duration, txState utils.TxState, errType TxErrType) (string, error)
	// GetTxState returns the transaction state for the provided ID if it exists
	GetTxState(id string) (utils.TxState, bool)
	// TrimFinalizedErroredTxs removes transactions that have reached their retention time
	TrimFinalizedErroredTxs() int
	// IsTxReorged determines whether the given signature has experienced a re-org by comparing its in-memory state with its current on-chain state.
	// A re-org is identified when the state of a signature regresses as follows:
	// 	- Confirmed -> Processed || Broadcasted || Not Found
	// 	- Processed -> Broadcasted || Not Found
	// The function returns the transaction ID associated with the signature and a boolean indicating whether a re-org has occurred.
	IsTxReorged(sig solana.Signature, currentState utils.TxState) (string, bool)
	// GetPendingTx returns the pendingTx for the given ID if it exists
	GetPendingTx(id string) (pendingTx, error)
	// GetTransactionSig returns the signature of the transaction for the given ID if it exists
	GetTransactionSig(id string) (solana.Signature, error)
}

// finishedTx is used to store info required to track transactions to finality or error
type pendingTx struct {
	tx                   solana.Transaction
	cfg                  utils.TxConfig
	signatures           []solana.Signature
	id                   string
	createTs             time.Time
	state                utils.TxState
	lastValidBlockHeight uint64 // to track expiration, equivalent to last valid block number.
}

// finishedTx is used to store minimal info specifically for finalized or errored transactions for external status checks
type finishedTx struct {
	retentionTs time.Time
	state       utils.TxState
	sig         solana.Signature
}

type txInfo struct {
	// id of the transaction
	id string
	// state of the signature
	state utils.TxState
}

var _ PendingTxContext = &pendingTxContext{}

type pendingTxContext struct {
	cancelBy    map[string]context.CancelFunc
	sigToTxInfo map[solana.Signature]txInfo

	queuedTxs               map[string]pendingTx  // pending transactions awaiting broadcast
	broadcastedProcessedTxs map[string]pendingTx  // broadcasted and processed transactions that may require retry and bumping
	confirmedTxs            map[string]pendingTx  // transactions that require monitoring for re-org
	finalizedErroredTxs     map[string]finishedTx // finalized and errored transactions held onto for status

	lock sync.RWMutex
}

func newPendingTxContext() *pendingTxContext {
	return &pendingTxContext{
		cancelBy:    map[string]context.CancelFunc{},
		sigToTxInfo: map[solana.Signature]txInfo{},

		queuedTxs:               map[string]pendingTx{},
		broadcastedProcessedTxs: map[string]pendingTx{},
		confirmedTxs:            map[string]pendingTx{},
		finalizedErroredTxs:     map[string]finishedTx{},
	}
}

func (c *pendingTxContext) New(tx pendingTx) error {
	err := c.withReadLock(func() error {
		// Check if ID already exists in any of the maps
		if _, exists := c.queuedTxs[tx.id]; exists {
			return ErrIDAlreadyExists
		}
		if _, exists := c.broadcastedProcessedTxs[tx.id]; exists {
			return ErrIDAlreadyExists
		}
		if _, exists := c.confirmedTxs[tx.id]; exists {
			return ErrIDAlreadyExists
		}
		if _, exists := c.finalizedErroredTxs[tx.id]; exists {
			return ErrIDAlreadyExists
		}
		return nil
	})
	if err != nil {
		return err
	}

	// upgrade to write lock if id does not exist
	_, err = c.withWriteLock(func() (string, error) {
		// Check if ID already exists in any of the maps
		if _, exists := c.queuedTxs[tx.id]; exists {
			return "", ErrIDAlreadyExists
		}
		if _, exists := c.broadcastedProcessedTxs[tx.id]; exists {
			return "", ErrIDAlreadyExists
		}
		if _, exists := c.confirmedTxs[tx.id]; exists {
			return "", ErrIDAlreadyExists
		}
		if _, exists := c.finalizedErroredTxs[tx.id]; exists {
			return "", ErrIDAlreadyExists
		}
		tx.createTs = time.Now()
		tx.state = utils.AwaitingBroadcast
		// save to the pending map
		c.queuedTxs[tx.id] = tx
		return "", nil
	})
	return err
}

func (c *pendingTxContext) OnBroadcasted(tx pendingTx) error {
	err := c.withReadLock(func() error {
		// Check if transaction already in broadcasted state
		if tempTx, exists := c.broadcastedProcessedTxs[tx.id]; exists && tempTx.state == utils.Broadcasted {
			return ErrAlreadyInExpectedState
		}
		// Transactions should only move to pending from broadcasted
		_, exists := c.queuedTxs[tx.id]
		if !exists {
			return ErrTransactionNotFound
		}
		return nil
	})
	if err != nil {
		return err
	}

	// upgrade to write lock if id does not exist
	_, err = c.withWriteLock(func() (string, error) {
		// Check if pending map has tx
		pendingMsg, exists := c.queuedTxs[tx.id]
		if !exists {
			return tx.id, ErrTransactionNotFound
		}
		tx.signatures = []solana.Signature{}
		tx.createTs = pendingMsg.createTs
		tx.state = utils.Broadcasted
		// save transaction to broadcasted map
		c.broadcastedProcessedTxs[tx.id] = tx
		// remove tx from pending map
		delete(c.queuedTxs, tx.id)
		return "", nil
	})
	return err
}

func (c *pendingTxContext) AddSignature(cancel context.CancelFunc, id string, sig solana.Signature) error {
	err := c.withReadLock(func() error {
		// signature already exists
		if _, exists := c.sigToTxInfo[sig]; exists {
			return ErrSigAlreadyExists
		}
		// new signatures should only be added for broadcasted transactions
		// otherwise, the transaction has transitioned states and no longer needs new signatures to track
		if _, exists := c.broadcastedProcessedTxs[id]; !exists {
			return ErrTransactionNotFound
		}
		return nil
	})
	if err != nil {
		return err
	}

	// upgrade to write lock if sig does not exist
	_, err = c.withWriteLock(func() (string, error) {
		if _, exists := c.sigToTxInfo[sig]; exists {
			return "", ErrSigAlreadyExists
		}
		if _, exists := c.broadcastedProcessedTxs[id]; !exists {
			return "", ErrTransactionNotFound
		}
		c.sigToTxInfo[sig] = txInfo{id: id, state: utils.Broadcasted}
		tx := c.broadcastedProcessedTxs[id]
		// save new signature
		tx.signatures = append(tx.signatures, sig)
		// save updated tx to broadcasted map
		c.broadcastedProcessedTxs[id] = tx
		// set cancel context if not already set to handle reorgs when regressing from confirmed state
		// previous context was removed so we associate a new context to our transaction to restart the retry/bumping cycle
		if _, exists := c.cancelBy[id]; !exists {
			c.cancelBy[id] = cancel
		}

		return "", nil
	})
	return err
}

// returns the id if removed (otherwise returns empty string)
// removes transactions from any state except finalized and errored
func (c *pendingTxContext) RevertToAwaitingBroadcast(id string) error {
	err := c.withReadLock(func() error {
		// check if transaction already in the expected state
		if tx, pendingExists := c.queuedTxs[id]; pendingExists && tx.state == utils.AwaitingBroadcast {
			return ErrAlreadyInExpectedState
		}
		// transaction should only revert to pending from broadcasted, processed, or confirmed state
		_, broadcastedIDExists := c.broadcastedProcessedTxs[id]
		_, confirmedIDExists := c.confirmedTxs[id]
		// transaction does not exist in tx maps
		if !broadcastedIDExists && !confirmedIDExists {
			return ErrTransactionNotFound
		}
		return nil
	})
	if err != nil {
		return err
	}

	// upgrade to write lock if sig does not exist
	_, err = c.withWriteLock(func() (string, error) {
		var tx pendingTx
		if tempTx, exists := c.broadcastedProcessedTxs[id]; exists {
			tx = tempTx
			delete(c.broadcastedProcessedTxs, id)
		}
		if tempTx, exists := c.confirmedTxs[id]; exists {
			tx = tempTx
			delete(c.confirmedTxs, id)
		}

		// call cancel func + remove from map
		if cancel, exists := c.cancelBy[id]; exists {
			cancel() // cancel context
			delete(c.cancelBy, id)
		}

		// remove all signatures associated with transaction from sig map
		for _, s := range tx.signatures {
			delete(c.sigToTxInfo, s)
		}
		tx.state = utils.AwaitingBroadcast
		c.queuedTxs[id] = tx
		return id, nil
	})
	return err
}

func (c *pendingTxContext) ListAllSigs(ctx context.Context) []solana.Signature {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return maps.Keys(c.sigToTxInfo)
}

// ListAllExpiredBroadcastedTxs returns all the txes that are in broadcasted state and have expired for given block number compared against lastValidBlockHeight (last valid block number)
// Passing maxUint64 as currBlockNumber will return all broadcasted txes.
func (c *pendingTxContext) ListAllExpiredBroadcastedTxs(currBlockNumber uint64) []pendingTx {
	c.lock.RLock()
	defer c.lock.RUnlock()
	expiredBroadcastedTxs := make([]pendingTx, 0, len(c.broadcastedProcessedTxs)) // worst case, all of them
	for _, tx := range c.broadcastedProcessedTxs {
		if tx.state == utils.Broadcasted && tx.lastValidBlockHeight < currBlockNumber {
			expiredBroadcastedTxs = append(expiredBroadcastedTxs, tx)
		}
	}
	return expiredBroadcastedTxs
}

// Expired returns if the timeout for trying to confirm a signature has been reached
func (c *pendingTxContext) Expired(sig solana.Signature, confirmationTimeout time.Duration) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	// confirmationTimeout set to 0 disables the expiration check
	if confirmationTimeout == 0 {
		return false
	}
	info, exists := c.sigToTxInfo[sig]
	if !exists {
		return false // return expired = false if timestamp does not exist (likely cleaned up by something else previously)
	}
	if tx, exists := c.broadcastedProcessedTxs[info.id]; exists {
		return time.Since(tx.createTs) > confirmationTimeout
	}
	if tx, exists := c.confirmedTxs[info.id]; exists {
		return time.Since(tx.createTs) > confirmationTimeout
	}
	return true // return expired = true even if tx does not exist to allow signature to be cleaned up
}

func (c *pendingTxContext) OnProcessed(sig solana.Signature) (string, error) {
	err := c.withReadLock(func() error {
		// validate if sig exists
		info, sigExists := c.sigToTxInfo[sig]
		if !sigExists {
			return ErrSigDoesNotExist
		}
		// Transactions should only move to processed from broadcasted
		tx, exists := c.broadcastedProcessedTxs[info.id]
		if !exists {
			return ErrTransactionNotFound
		}
		// Check if tranasction already in processed state
		if tx.state == utils.Processed {
			return ErrAlreadyInExpectedState
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	// upgrade to write lock if sig and id exist
	return c.withWriteLock(func() (string, error) {
		info, sigExists := c.sigToTxInfo[sig]
		if !sigExists {
			return info.id, ErrSigDoesNotExist
		}
		tx, exists := c.broadcastedProcessedTxs[info.id]
		if !exists {
			return info.id, ErrTransactionNotFound
		}
		// update sig and tx to Processed
		info.state, tx.state = utils.Processed, utils.Processed
		// save updated sig and tx back to the maps
		c.sigToTxInfo[sig] = info
		c.broadcastedProcessedTxs[info.id] = tx
		return info.id, nil
	})
}

func (c *pendingTxContext) OnConfirmed(ctx context.Context, sig solana.Signature) (string, error) {
	err := c.withReadLock(func() error {
		// validate if sig exists
		info, sigExists := c.sigToTxInfo[sig]
		if !sigExists {
			return ErrSigDoesNotExist
		}
		// Check if transaction already in confirmed state
		if tx, exists := c.confirmedTxs[info.id]; exists && tx.state == utils.Confirmed {
			return ErrAlreadyInExpectedState
		}
		// Transactions should only move to confirmed from broadcasted/processed
		if _, exists := c.broadcastedProcessedTxs[info.id]; !exists {
			return ErrTransactionNotFound
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	// upgrade to write lock if id exists
	return c.withWriteLock(func() (string, error) {
		info, sigExists := c.sigToTxInfo[sig]
		if !sigExists {
			return info.id, ErrSigDoesNotExist
		}
		tx, exists := c.broadcastedProcessedTxs[info.id]
		if !exists {
			return info.id, ErrTransactionNotFound
		}
		// call cancel func + remove from map to stop the retry/bumping cycle for this transaction
		if cancel, exists := c.cancelBy[info.id]; exists {
			cancel() // cancel context
			delete(c.cancelBy, info.id)
		}
		// update sig and tx state to Confirmed
		info.state, tx.state = utils.Confirmed, utils.Confirmed
		c.sigToTxInfo[sig] = info
		// move tx to confirmed map
		c.confirmedTxs[info.id] = tx
		// remove tx from broadcasted map
		delete(c.broadcastedProcessedTxs, info.id)
		return info.id, nil
	})
}

func (c *pendingTxContext) OnFinalized(ctx context.Context, sig solana.Signature, retentionTimeout time.Duration) (string, error) {
	err := c.withReadLock(func() error {
		info, sigExists := c.sigToTxInfo[sig]
		if !sigExists {
			return ErrSigDoesNotExist
		}
		// Allow transactions to transition from broadcasted, processed, or confirmed state in case there are delays between status checks
		_, broadcastedExists := c.broadcastedProcessedTxs[info.id]
		_, confirmedExists := c.confirmedTxs[info.id]
		if !broadcastedExists && !confirmedExists {
			return ErrTransactionNotFound
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	// upgrade to write lock if id exists
	return c.withWriteLock(func() (string, error) {
		info, exists := c.sigToTxInfo[sig]
		if !exists {
			return info.id, ErrSigDoesNotExist
		}
		var tx, tempTx pendingTx
		var broadcastedExists, confirmedExists bool
		if tempTx, broadcastedExists = c.broadcastedProcessedTxs[info.id]; broadcastedExists {
			tx = tempTx
		}
		if tempTx, confirmedExists = c.confirmedTxs[info.id]; confirmedExists {
			tx = tempTx
		}
		if !broadcastedExists && !confirmedExists {
			return info.id, ErrTransactionNotFound
		}
		// call cancel func + remove from map to stop the retry/bumping cycle for this transaction
		// cancel is expected to be called and removed when tx is confirmed but checked here too in case state is skipped
		if cancel, exists := c.cancelBy[info.id]; exists {
			cancel() // cancel context
			delete(c.cancelBy, info.id)
		}
		// delete from broadcasted map, if exists
		delete(c.broadcastedProcessedTxs, info.id)
		// delete from confirmed map, if exists
		delete(c.confirmedTxs, info.id)
		// remove all related signatures from the sigToTxInfo map to skip picking up this tx in the confirmation logic
		for _, s := range tx.signatures {
			delete(c.sigToTxInfo, s)
		}
		// if retention duration is set to 0, delete transaction from storage
		// otherwise, move to finalized map
		if retentionTimeout == 0 {
			return info.id, nil
		}
		finalizedTx := finishedTx{
			state:       utils.Finalized,
			retentionTs: time.Now().Add(retentionTimeout),
			sig:         sig,
		}
		// move transaction from confirmed to finalized map
		c.finalizedErroredTxs[info.id] = finalizedTx
		return info.id, nil
	})
}

func (c *pendingTxContext) OnPrebroadcastError(ctx context.Context, id string, retentionTimeout time.Duration, txState utils.TxState, _ TxErrType) error {
	err := c.withReadLock(func() error {
		// check if transaction already in the expected state
		if tx, exists := c.finalizedErroredTxs[id]; exists && tx.state == txState {
			return ErrAlreadyInExpectedState
		}
		// check if transaction in higher state than pending, implying that the transaction has progressed past pre-broadcast errors
		_, broadcastedExists := c.broadcastedProcessedTxs[id]
		_, confirmedExists := c.confirmedTxs[id]
		if broadcastedExists || confirmedExists {
			return ErrIDAlreadyExists
		}
		return nil
	})
	if err != nil {
		return err
	}

	// upgrade to write lock if id does not exist in other maps and is not in expected state already
	_, err = c.withWriteLock(func() (string, error) {
		tx, exists := c.finalizedErroredTxs[id]
		if exists && tx.state == txState {
			return "", ErrAlreadyInExpectedState
		}
		_, broadcastedExists := c.broadcastedProcessedTxs[id]
		_, confirmedExists := c.confirmedTxs[id]
		if broadcastedExists || confirmedExists {
			return "", ErrIDAlreadyExists
		}
		delete(c.queuedTxs, id)
		// If retention timeout set to 0, skip adding it to the errored map
		if retentionTimeout == 0 {
			return "", nil
		}
		erroredTx := finishedTx{
			state:       txState,
			retentionTs: time.Now().Add(retentionTimeout),
		}
		// add transaction to error map
		c.finalizedErroredTxs[id] = erroredTx
		return id, nil
	})
	return err
}

func (c *pendingTxContext) OnError(ctx context.Context, sig solana.Signature, retentionTimeout time.Duration, txState utils.TxState, _ TxErrType) (string, error) {
	err := c.withReadLock(func() error {
		// validate if signature exists
		// skip checking tx existence to allow signature to get cleaned up
		if _, sigExists := c.sigToTxInfo[sig]; !sigExists {
			return ErrSigDoesNotExist
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	// upgrade to write lock if sig exists
	return c.withWriteLock(func() (string, error) {
		info, exists := c.sigToTxInfo[sig]
		if !exists {
			return "", ErrSigDoesNotExist
		}
		// delete sig to avoid memory leak if corresponding tx is not found
		delete(c.sigToTxInfo, sig)
		var tx, tempTx pendingTx
		var broadcastedExists, confirmedExists bool
		if tempTx, broadcastedExists = c.broadcastedProcessedTxs[info.id]; broadcastedExists {
			tx = tempTx
		}
		if tempTx, confirmedExists = c.confirmedTxs[info.id]; confirmedExists {
			tx = tempTx
		}
		_, finalizedErroredExists := c.finalizedErroredTxs[info.id]

		// call cancel func + remove from map
		if cancel, exists := c.cancelBy[info.id]; exists {
			cancel() // cancel context
			delete(c.cancelBy, info.id)
		}
		// delete from broadcasted map, if exists
		delete(c.broadcastedProcessedTxs, info.id)
		// delete from confirmed map, if exists
		delete(c.confirmedTxs, info.id)
		// remove all related signatures from the sigToTxInfo map to skip picking up this tx in the confirmation logic
		if broadcastedExists || confirmedExists {
			for _, s := range tx.signatures {
				delete(c.sigToTxInfo, s)
			}
		}
		// if retention duration is set to 0, skip adding transaction to the errored map
		if retentionTimeout == 0 {
			// sanity check - no transactions should be in finalized/errored map if retention is set to 0
			delete(c.finalizedErroredTxs, info.id)
			return info.id, nil
		}
		// avoid updating finalized/errored tx entry if one already exists
		if finalizedErroredExists {
			return info.id, nil
		}
		erroredTx := finishedTx{
			state:       txState,
			retentionTs: time.Now().Add(retentionTimeout),
		}
		// move transaction from broadcasted to error map
		c.finalizedErroredTxs[info.id] = erroredTx
		return info.id, nil
	})
}

func (c *pendingTxContext) GetTxState(id string) (utils.TxState, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if tx, exists := c.queuedTxs[id]; exists {
		return tx.state, true
	}
	if tx, exists := c.broadcastedProcessedTxs[id]; exists {
		return tx.state, true
	}
	if tx, exists := c.confirmedTxs[id]; exists {
		return tx.state, true
	}
	if tx, exists := c.finalizedErroredTxs[id]; exists {
		return tx.state, true
	}
	return utils.NotFound, false
}

// TrimFinalizedErroredTxs deletes transactions from the finalized/errored map and the allTxs map after the retention period has passed
func (c *pendingTxContext) TrimFinalizedErroredTxs() int {
	var expiredIDs []string
	err := c.withReadLock(func() error {
		expiredIDs = make([]string, 0, len(c.finalizedErroredTxs))
		for id, tx := range c.finalizedErroredTxs {
			if time.Now().After(tx.retentionTs) {
				expiredIDs = append(expiredIDs, id)
			}
		}
		return nil
	})
	if err != nil {
		return 0
	}

	_, err = c.withWriteLock(func() (string, error) {
		for _, id := range expiredIDs {
			delete(c.finalizedErroredTxs, id)
		}
		return "", nil
	})
	if err != nil {
		return 0
	}
	return len(expiredIDs)
}

func (c *pendingTxContext) IsTxReorged(sig solana.Signature, sigOnChainState utils.TxState) (string, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// Grab in memory state of the signature
	info, exists := c.sigToTxInfo[sig]
	if !exists {
		return "", false
	}

	// Compare our in-memory state of the sig with the current on-chain state to determine if the sig had a regression
	sigInMemoryState := info.state
	var hasReorg bool
	switch sigInMemoryState {
	case utils.Confirmed:
		if sigOnChainState == utils.Processed || sigOnChainState == utils.Broadcasted || sigOnChainState == utils.NotFound {
			hasReorg = true
		}
	case utils.Processed:
		if sigOnChainState == utils.Broadcasted || sigOnChainState == utils.NotFound {
			hasReorg = true
		}
	default: // No reorg if the signature is not in a state that can be reorged
	}

	return info.id, hasReorg
}

func (c *pendingTxContext) GetPendingTx(id string) (pendingTx, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	var tx, tempTx pendingTx
	var broadcastedExists, confirmedExists bool
	if tempTx, broadcastedExists = c.broadcastedProcessedTxs[id]; broadcastedExists {
		tx = tempTx
	}
	if tempTx, confirmedExists = c.confirmedTxs[id]; confirmedExists {
		tx = tempTx
	}

	if !broadcastedExists && !confirmedExists {
		return pendingTx{}, ErrTransactionNotFound
	}
	return tx, nil
}

func (c *pendingTxContext) GetTransactionSig(id string) (solana.Signature, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	tx, exists := c.finalizedErroredTxs[id]
	if !exists {
		return solana.Signature{}, ErrTransactionNotFound
	}
	if tx.sig.IsZero() {
		return solana.Signature{}, fmt.Errorf("signature is zero, txState: %s", tx.state)
	}
	return tx.sig, nil
}

func (c *pendingTxContext) withReadLock(fn func() error) error {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return fn()
}

func (c *pendingTxContext) withWriteLock(fn func() (string, error)) (string, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	return fn()
}

var _ PendingTxContext = &pendingTxContextWithProm{}

type pendingTxContextWithProm struct {
	pendingTx *pendingTxContext
	chainID   string
	metrics   *solTxmMetrics
}

type TxErrType int

const (
	NoFailure TxErrType = iota
	TxFailRevert
	TxFailReject
	TxFailDrop
	TxFailSimRevert
	TxFailSimOther
	TxDependencyFail
)

func newPendingTxContextWithProm(id string, metrics *solTxmMetrics) *pendingTxContextWithProm {
	return &pendingTxContextWithProm{
		chainID:   id,
		pendingTx: newPendingTxContext(),
		metrics:   metrics,
	}
}

func (c *pendingTxContextWithProm) New(msg pendingTx) error {
	return c.pendingTx.New(msg)
}

func (c *pendingTxContextWithProm) OnBroadcasted(msg pendingTx) error {
	return c.pendingTx.OnBroadcasted(msg)
}

func (c *pendingTxContextWithProm) AddSignature(cancel context.CancelFunc, id string, sig solana.Signature) error {
	return c.pendingTx.AddSignature(cancel, id, sig)
}

func (c *pendingTxContextWithProm) OnProcessed(sig solana.Signature) (string, error) {
	return c.pendingTx.OnProcessed(sig)
}

func (c *pendingTxContextWithProm) OnConfirmed(ctx context.Context, sig solana.Signature) (string, error) {
	id, err := c.pendingTx.OnConfirmed(ctx, sig) // empty ID indicates already previously removed
	if id != "" && err == nil {                  // increment if tx was not removed
		c.metrics.IncrementSuccessTxs(ctx)
	}
	return id, err
}

func (c *pendingTxContextWithProm) RevertToAwaitingBroadcast(id string) error {
	return c.pendingTx.RevertToAwaitingBroadcast(id)
}

func (c *pendingTxContextWithProm) ListAllSigs(ctx context.Context) []solana.Signature {
	sigs := c.pendingTx.ListAllSigs(ctx)
	c.metrics.SetPendingTxs(ctx, len(sigs))
	return sigs
}

func (c *pendingTxContextWithProm) ListAllExpiredBroadcastedTxs(currBlockNumber uint64) []pendingTx {
	return c.pendingTx.ListAllExpiredBroadcastedTxs(currBlockNumber)
}

func (c *pendingTxContextWithProm) Expired(sig solana.Signature, lifespan time.Duration) bool {
	return c.pendingTx.Expired(sig, lifespan)
}

// Success - tx finalized
func (c *pendingTxContextWithProm) OnFinalized(ctx context.Context, sig solana.Signature, retentionTimeout time.Duration) (string, error) {
	id, err := c.pendingTx.OnFinalized(ctx, sig, retentionTimeout) // empty ID indicates already previously removed
	if id != "" && err == nil {                                    // increment if tx was not removed
		c.metrics.IncrementFinalizedTxs(ctx)
	}
	return id, err
}

func (c *pendingTxContextWithProm) OnError(ctx context.Context, sig solana.Signature, retentionTimeout time.Duration, txState utils.TxState, errType TxErrType) (string, error) {
	id, err := c.pendingTx.OnError(ctx, sig, retentionTimeout, txState, errType) // err indicates transaction not found so may already be removed
	if err == nil {
		incrementErrorMetrics(ctx, errType, c.chainID, c.metrics)
	}
	return id, err
}

func (c *pendingTxContextWithProm) OnPrebroadcastError(ctx context.Context, id string, retentionTimeout time.Duration, txState utils.TxState, errType TxErrType) error {
	err := c.pendingTx.OnPrebroadcastError(ctx, id, retentionTimeout, txState, errType) // err indicates transaction not found so may already be removed
	if err == nil {
		incrementErrorMetrics(ctx, errType, c.chainID, c.metrics)
	}
	return err
}

func incrementErrorMetrics(ctx context.Context, errType TxErrType, chainID string, metrics *solTxmMetrics) {
	switch errType {
	case NoFailure:
		// Return early if no failure identified
		return
	case TxFailReject:
		metrics.IncrementRejectTxs(ctx)
	case TxFailRevert:
		metrics.IncrementRevertTxs(ctx)
	case TxFailDrop:
		metrics.IncrementDropTxs(ctx)
	case TxFailSimRevert:
		metrics.IncrementSimRevertTxs(ctx)
	case TxFailSimOther:
		metrics.IncrementSimOtherTxs(ctx)
	case TxDependencyFail:
		metrics.IncrementDependencyFailTxs(ctx)
	}
	metrics.IncrementErrorTxs(ctx)
}

func (c *pendingTxContextWithProm) GetTxState(id string) (utils.TxState, bool) {
	return c.pendingTx.GetTxState(id)
}

func (c *pendingTxContextWithProm) TrimFinalizedErroredTxs() int {
	return c.pendingTx.TrimFinalizedErroredTxs()
}

func (c *pendingTxContextWithProm) IsTxReorged(sig solana.Signature, currentSigState utils.TxState) (string, bool) {
	return c.pendingTx.IsTxReorged(sig, currentSigState)
}

func (c *pendingTxContextWithProm) GetPendingTx(id string) (pendingTx, error) {
	return c.pendingTx.GetPendingTx(id)
}

func (c *pendingTxContextWithProm) GetTransactionSig(id string) (solana.Signature, error) {
	return c.pendingTx.GetTransactionSig(id)
}

package txm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"golang.org/x/exp/maps"
)

type PendingTxContext interface {
	// New adds a new tranasction in Broadcasted state to the storage
	New(msg pendingTx, sig solana.Signature, cancel context.CancelFunc) error
	// AddSignature adds a new signature for an existing transaction ID
	AddSignature(id string, sig solana.Signature) error
	// Remove removes transaction and related signatures from storage if not in finalized or errored state
	Remove(sig solana.Signature) string
	// ListAll returns all of the signatures being tracked for all transactions not yet finalized or errored
	ListAll() []solana.Signature
	// Expired returns whether or not confirmation timeout amount of time has passed since creation
	Expired(sig solana.Signature, confirmationTimeout time.Duration) bool
	// OnProcessed marks transactions as Processed
	OnProcessed(sig solana.Signature) (string, error)
	// OnConfirmed marks transaction as Confirmed and moves it from broadcast map to confirmed map
	OnConfirmed(sig solana.Signature) (string, error)
	// OnFinalized marks transaction as Finalized, moves it from confirmed map to finalized map, removes signatures from signature map to stop confirmation checks
	OnFinalized(sig solana.Signature) (string, error)
	// OnError marks transaction as errored, matches err type using enum, moves it from confirmed map to finalized map, removes signatures from signature map to stop confirmation checks
	OnError(sig solana.Signature, errType int) string
	// GetTxState returns the transaction state for the provided ID if it exists
	GetTxState(id string) (TxState, error)
	// TrimFinalizedErroredTxs removes transactions that have reaches their retention time
	TrimFinalizedErroredTxs(retentionTimeout time.Duration)
}

type pendingTx struct {
	tx               solana.Transaction
	cfg              TxConfig
	signatures       []solana.Signature
	id               string
	createTs         time.Time
	finalizedErrorTs time.Time
	state            TxState
}

var _ PendingTxContext = &pendingTxContext{}

type pendingTxContext struct {
	cancelBy map[string]context.CancelFunc
	sigToID  map[solana.Signature]string

	broadcastedTxs      map[string]pendingTx // transactions that require retry and bumping i.e broadcasted, processed
	confirmedTxs        map[string]pendingTx // transactions that require monitoring for re-org
	finalizedErroredTxs map[string]pendingTx // finalized and errored transactions held onto for status

	lock sync.RWMutex
}

func newPendingTxContext() *pendingTxContext {
	return &pendingTxContext{
		cancelBy: map[string]context.CancelFunc{},
		sigToID:  map[solana.Signature]string{},

		broadcastedTxs:      map[string]pendingTx{},
		confirmedTxs:        map[string]pendingTx{},
		finalizedErroredTxs: map[string]pendingTx{},
	}
}

func (c *pendingTxContext) New(tx pendingTx, sig solana.Signature, cancel context.CancelFunc) error {
	c.lock.RLock()
	// validate signature does not exist
	if _, exists := c.sigToID[sig]; exists {
		c.lock.RUnlock()
		return errors.New("signature already exists")
	}
	// validate id does not exist
	if _, exists := c.broadcastedTxs[tx.id]; exists {
		c.lock.RUnlock()
		return errors.New("tx id already exists in broadcasted map")
	}
	c.lock.RUnlock()

	// upgrade to write lock if sig or id do not exist
	c.lock.Lock()
	defer c.lock.Unlock()
	if _, exists := c.sigToID[sig]; exists {
		return errors.New("signature already exists")
	}
	if _, exists := c.broadcastedTxs[tx.id]; exists {
		return errors.New("tx id already exists")
	}
	// save cancel func
	c.cancelBy[tx.id] = cancel
	c.sigToID[sig] = tx.id
	// add signature to tx
	tx.signatures = append(tx.signatures, sig)
	tx.createTs = time.Now()
	tx.state = Broadcasted
	// save to the broadcasted map since transaction was just broadcasted
	c.broadcastedTxs[tx.id] = tx
	return nil
}

func (c *pendingTxContext) AddSignature(id string, sig solana.Signature) error {
	c.lock.RLock()
	// signature already exists
	if _, exists := c.sigToID[sig]; exists {
		c.lock.RUnlock()
		return errors.New("signature already exists")
	}
	// new signatures should only be added for broadcasted transactions
	// otherwise, the transaction has transitioned states and no longer needs new signatures to track
	if _, exists := c.broadcastedTxs[id]; !exists {
		c.lock.RUnlock()
		return errors.New("id does not exist in broadcasted map")
	}
	c.lock.RUnlock()

	// upgrade to write lock if sig does not exist
	c.lock.Lock()
	defer c.lock.Unlock()
	if _, exists := c.sigToID[sig]; exists {
		return errors.New("signature already exists")
	}
	if _, exists := c.broadcastedTxs[id]; !exists {
		return errors.New("id does not exist - tx likely confirmed by other signature")
	}
	c.sigToID[sig] = id
	tx := c.broadcastedTxs[id]
	// save new signature
	tx.signatures = append(tx.signatures, sig)
	// save updated tx to broadcasted map
	c.broadcastedTxs[id] = tx
	return nil
}

// returns the id if removed (otherwise returns 0-id)
// removes transactions from any state except finalized and errored
func (c *pendingTxContext) Remove(sig solana.Signature) (id string) {
	c.lock.RLock()
	// check if already removed
	id, sigExists := c.sigToID[sig]
	if !sigExists {
		c.lock.RUnlock()
		return id
	}
	_, broadcastedIDExists := c.broadcastedTxs[id]
	_, confirmedIDExists := c.confirmedTxs[id]
	// transcation does not exist in tx maps
	if !broadcastedIDExists && !confirmedIDExists {
		c.lock.RUnlock()
		return id
	}
	c.lock.RUnlock()

	// upgrade to write lock if sig and id exist
	c.lock.Lock()
	defer c.lock.Unlock()
	id, sigExists = c.sigToID[sig]
	if !sigExists {
		return id
	}
	var tx pendingTx
	if tempTx, exists := c.broadcastedTxs[id]; exists {
		tx = tempTx
		delete(c.broadcastedTxs, id)
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
		delete(c.sigToID, s)
	}
	return id
}

func (c *pendingTxContext) ListAll() []solana.Signature {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return maps.Keys(c.sigToID)
}

// Expired returns if the timeout for trying to confirm a signature has been reached
func (c *pendingTxContext) Expired(sig solana.Signature, confirmationTimeout time.Duration) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	id, exists := c.sigToID[sig]
	if !exists {
		return false // return expired = false if timestamp does not exist (likely cleaned up by something else previously)
	}
	if tx, exists := c.broadcastedTxs[id]; exists {
		return time.Since(tx.createTs) > confirmationTimeout
	}
	if tx, exists := c.confirmedTxs[id]; exists {
		return time.Since(tx.createTs) > confirmationTimeout
	}
	return false // return expired = false if tx does not exist (likely cleaned up by something else previously)
}

func (c *pendingTxContext) OnProcessed(sig solana.Signature) (string, error) {
	c.lock.RLock()
	// validate if sig exists
	id, sigExists := c.sigToID[sig]
	if !sigExists {
		c.lock.RUnlock()
		return id, errors.New("signature does not exist")
	}
	// Transactions should only move to processed from broadcasted
	if _, exists := c.broadcastedTxs[id]; !exists {
		c.lock.RUnlock()
		return id, errors.New("id does not exist in broadcasted map")
	}
	c.lock.RUnlock()

	// upgrade to write lock if sig and id exist
	c.lock.Lock()
	defer c.lock.Unlock()
	id, sigExists = c.sigToID[sig]
	if !sigExists {
		return id, errors.New("signature does not exist")
	}
	if _, exists := c.broadcastedTxs[id]; !exists {
		return id, errors.New("id does not exist in brooadcasted map")
	}
	tx := c.broadcastedTxs[id]
	// update tx state to Processed
	tx.state = Processed
	// save updated tx back to the broadcasted map
	c.broadcastedTxs[id] = tx
	return id, nil
}

func (c *pendingTxContext) OnConfirmed(sig solana.Signature) (string, error) {
	c.lock.RLock()
	// validate if sig exists
	id, sigExists := c.sigToID[sig]
	if !sigExists {
		c.lock.RUnlock()
		return id, errors.New("signature does not exist")
	}
	// Transactions should only move to confirmed from broadcasted/processed
	if _, exists := c.broadcastedTxs[id]; !exists {
		c.lock.RUnlock()
		return id, errors.New("id does not exist in broadcasted map")
	}
	c.lock.RUnlock()

	// upgrade to write lock if id exists
	c.lock.Lock()
	defer c.lock.Unlock()
	id, sigExists = c.sigToID[sig]
	if !sigExists {
		return id, errors.New("signature does not exist")
	}
	if _, exists := c.broadcastedTxs[id]; !exists {
		return id, errors.New("id does not exist in broadcasted map")
	}
	// call cancel func + remove from map to stop the retry/bumping cycle for this transaction
	if cancel, exists := c.cancelBy[id]; exists {
		cancel() // cancel context
		delete(c.cancelBy, id)
	}
	tx := c.broadcastedTxs[id]
	// update tx state to Confirmed
	tx.state = Confirmed
	// move tx to confirmed map
	c.confirmedTxs[id] = tx
	// remove tx from broadcasted map
	delete(c.broadcastedTxs, id)
	return id, nil
}

func (c *pendingTxContext) OnFinalized(sig solana.Signature) (string, error) {
	// check if already cancelled
	c.lock.RLock()
	id, sigExists := c.sigToID[sig]
	if !sigExists {
		c.lock.RUnlock()
		return id, errors.New("signature does not exist")
	}
	// Allow transactions to transition from broadcasted, processed, or confirmed state in case there are delays between status checks
	_, broadcastedExists := c.broadcastedTxs[id]
	_, confirmedExists := c.confirmedTxs[id]
	if !broadcastedExists && !confirmedExists {
		c.lock.RUnlock()
		return id, errors.New("id does not exist in broadcasted or confirmed map")
	}
	c.lock.RUnlock()

	// upgrade to write lock if id exists
	c.lock.Lock()
	defer c.lock.Unlock()
	if _, exists := c.sigToID[sig]; !exists {
		return id, errors.New("signature does not exist")
	}
	var tx pendingTx
	if tempTx, broadcastedExists := c.broadcastedTxs[id]; broadcastedExists {
		tx = tempTx
	}
	if tempTx, confirmedExists := c.confirmedTxs[id]; confirmedExists {
		tx = tempTx
	}
	if !broadcastedExists && !confirmedExists {
		return id, errors.New("id does not exist in broadcasted or confirmed map")
	}
	// call cancel func + remove from map to stop the retry/bumping cycle for this transaction
	// cancel is expected to be called and removed when tx is confirmed but checked here too in case state is skipped
	if cancel, exists := c.cancelBy[id]; exists {
		cancel() // cancel context
		delete(c.cancelBy, id)
	}
	// set current timestamp when transitioning to finalized map
	// later used for transaction retention
	tx.finalizedErrorTs = time.Now()
	// update tx state to Finalized
	tx.state = Finalized
	// move transaction from confirmed to finalized map
	c.finalizedErroredTxs[id] = tx
	if broadcastedExists {
		delete(c.broadcastedTxs, id)
	} else if confirmedExists {
		delete(c.confirmedTxs, id)
	}
	// remove all related signatures from the sigToID map to skip picking up this tx in the confirmation logic
	for _, s := range tx.signatures {
		delete(c.sigToID, s)
	}
	return id, nil
}

func (c *pendingTxContext) OnError(sig solana.Signature, _ int) string {
	// check if already cancelled
	c.lock.RLock()
	id, sigExists := c.sigToID[sig]
	if !sigExists {
		c.lock.RUnlock()
		return ""
	}
	// transaction can transition from any non-finalized state
	_, broadcastedExists := c.broadcastedTxs[id]
	_, confirmedExists := c.confirmedTxs[id]
	// transcation does not exist in any tx maps
	if !broadcastedExists && !confirmedExists {
		c.lock.RUnlock()
		return ""
	}
	c.lock.RUnlock()

	// upgrade to write lock if sig exists
	c.lock.Lock()
	defer c.lock.Unlock()
	if _, exists := c.sigToID[sig]; !exists {
		return ""
	}
	var tx pendingTx
	if tempTx, broadcastedExists := c.broadcastedTxs[id]; broadcastedExists {
		tx = tempTx
	}
	if tempTx, confirmedExists := c.confirmedTxs[id]; confirmedExists {
		tx = tempTx
	}
	// transcation does not exist in any non-finalized maps
	if !broadcastedExists && !confirmedExists {
		c.lock.RUnlock()
		return ""
	}
	// call cancel func + remove from map
	if cancel, exists := c.cancelBy[id]; exists {
		cancel() // cancel context
		delete(c.cancelBy, id)
	}

	// set current timestamp when transitioning to finalized map
	// later used for transaction retention
	tx.finalizedErrorTs = time.Now()
	// update tx state to Errored
	tx.state = Errored
	// move transaction from broadcasted to error map
	c.finalizedErroredTxs[id] = tx
	// delete from broadcasted map, if exists
	delete(c.broadcastedTxs, id)
	// delete from confirmed map, if exists
	delete(c.confirmedTxs, id)
	// remove all related signatures from the sigToID map to skip picking up this tx in the confirmation logic
	for _, s := range tx.signatures {
		delete(c.sigToID, s)
	}
	return id
}

func (c *pendingTxContext) GetTxState(id string) (TxState, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if tx, exists := c.broadcastedTxs[id]; exists {
		return tx.state, nil
	}
	if tx, exists := c.confirmedTxs[id]; exists {
		return tx.state, nil
	}
	if tx, exists := c.finalizedErroredTxs[id]; exists {
		return tx.state, nil
	}
	return NotFound, fmt.Errorf("failed to find transaction for id: %s", id)
}

// TrimFinalizedErroredTxs deletes transactions from the finalized/errored map and the allTxs map after the retention period has passed
func (c *pendingTxContext) TrimFinalizedErroredTxs(retentionTimeout time.Duration) {
	c.lock.RLock()
	var expiredIDs []string
	for id, tx := range c.finalizedErroredTxs {
		if time.Since(tx.finalizedErrorTs) > retentionTimeout {
			expiredIDs = append(expiredIDs, id)
		}
	}
	c.lock.RUnlock()

	// upgrade to write lock
	c.lock.Lock()
	defer c.lock.Unlock()
	for _, id := range expiredIDs {
		delete(c.finalizedErroredTxs, id)
	}
}

var _ PendingTxContext = &pendingTxContextWithProm{}

type pendingTxContextWithProm struct {
	pendingTx *pendingTxContext
	chainID   string
}

const (
	TxFailRevert = iota
	TxFailReject
	TxFailDrop
	TxFailSimRevert
	TxFailSimOther
)

func newPendingTxContextWithProm(id string) *pendingTxContextWithProm {
	return &pendingTxContextWithProm{
		chainID:   id,
		pendingTx: newPendingTxContext(),
	}
}

func (c *pendingTxContextWithProm) New(msg pendingTx, sig solana.Signature, cancel context.CancelFunc) error {
	return c.pendingTx.New(msg, sig, cancel)
}

func (c *pendingTxContextWithProm) AddSignature(id string, sig solana.Signature) error {
	return c.pendingTx.AddSignature(id, sig)
}

func (c *pendingTxContextWithProm) OnProcessed(sig solana.Signature) (string, error) {
	return c.pendingTx.OnProcessed(sig)
}

func (c *pendingTxContextWithProm) OnConfirmed(sig solana.Signature) (string, error) {
	id, err := c.pendingTx.OnConfirmed(sig) // empty ID indicates already previously removed
	if id != "" && err == nil {             // increment if tx was not removed
		promSolTxmSuccessTxs.WithLabelValues(c.chainID).Add(1)
	}
	return id, err
}

func (c *pendingTxContextWithProm) Remove(sig solana.Signature) string {
	return c.pendingTx.Remove(sig)
}

func (c *pendingTxContextWithProm) ListAll() []solana.Signature {
	sigs := c.pendingTx.ListAll()
	promSolTxmPendingTxs.WithLabelValues(c.chainID).Set(float64(len(sigs)))
	return sigs
}

func (c *pendingTxContextWithProm) Expired(sig solana.Signature, lifespan time.Duration) bool {
	return c.pendingTx.Expired(sig, lifespan)
}

// Success - tx finalized
func (c *pendingTxContextWithProm) OnFinalized(sig solana.Signature) (string, error) {
	id, err := c.pendingTx.OnFinalized(sig) // empty ID indicates already previously removed
	if id != "" && err == nil {             // increment if tx was not removed
		promSolTxmFinalizedTxs.WithLabelValues(c.chainID).Add(1)
	}
	return id, err
}

func (c *pendingTxContextWithProm) OnError(sig solana.Signature, errType int) string {
	// special RPC rejects transaction (signature will not be valid)
	if errType == TxFailReject {
		promSolTxmRejectTxs.WithLabelValues(c.chainID).Add(1)
		promSolTxmErrorTxs.WithLabelValues(c.chainID).Add(1)
		return ""
	}

	id := c.pendingTx.OnError(sig, errType) // empty ID indicates already removed
	if id != "" {
		switch errType {
		case TxFailRevert:
			promSolTxmRevertTxs.WithLabelValues(c.chainID).Add(1)
			promSolTxmErrorTxs.WithLabelValues(c.chainID).Add(1)
		case TxFailDrop:
			promSolTxmDropTxs.WithLabelValues(c.chainID).Add(1)
			promSolTxmErrorTxs.WithLabelValues(c.chainID).Add(1)
		}
	}

	// Increment simulation error metrics even if no tx found for sig
	// Simulation could have occurred before initial broadcast so tx was never stored
	switch errType {
	case TxFailSimRevert:
		promSolTxmSimRevertTxs.WithLabelValues(c.chainID).Add(1)
		promSolTxmErrorTxs.WithLabelValues(c.chainID).Add(1)
	case TxFailSimOther:
		promSolTxmSimOtherTxs.WithLabelValues(c.chainID).Add(1)
		promSolTxmErrorTxs.WithLabelValues(c.chainID).Add(1)
	}

	return id
}

func (c *pendingTxContextWithProm) GetTxState(id string) (TxState, error) {
	return c.pendingTx.GetTxState(id)
}

func (c *pendingTxContextWithProm) TrimFinalizedErroredTxs(retentionTimeout time.Duration) {
	c.pendingTx.TrimFinalizedErroredTxs(retentionTimeout)
}

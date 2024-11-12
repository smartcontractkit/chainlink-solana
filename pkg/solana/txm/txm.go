package txm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync"
	"time"

	solanaGo "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/loop"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	bigmath "github.com/smartcontractkit/chainlink-common/pkg/utils/big_math"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/internal"
)

const (
	MaxQueueLen                    = 1000
	MaxRetryTimeMs                 = 250              // max tx retry time (exponential retry will taper to retry every 0.25s)
	MaxSigsToConfirm               = 256              // max number of signatures in GetSignatureStatus call
	EstimateComputeUnitLimitBuffer = 10               // percent buffer added on top of estimated compute unit limits to account for any variance
	TxReapInterval                 = 10 * time.Second // interval of time between reaping transactions that have met the retention threshold
	FeeEstimatorModeFixed          = "fixed"
	FeeEstimatorModeBlockHistory   = "blockhistory"
	FeeBumpCriteriaFixedIntervals  = "fixed-intervals"
	FeeBumpCriteriaExpiration      = "expiration"
	FeeBumpCriteriaNone            = "none"
)

var _ services.Service = (*Txm)(nil)

//go:generate mockery --name SimpleKeystore --output ./mocks/ --case=underscore --filename simple_keystore.go
type SimpleKeystore interface {
	Sign(ctx context.Context, account string, data []byte) (signature []byte, err error)
	Accounts(ctx context.Context) (accounts []string, err error)
}

var _ loop.Keystore = (SimpleKeystore)(nil)

// Txm manages transactions for the solana blockchain.
// simple implementation with no persistently stored txs
type Txm struct {
	services.StateMachine
	lggr   logger.Logger
	chSend chan pendingTx
	chSim  chan pendingTx
	chStop services.StopChan
	done   sync.WaitGroup
	cfg    config.Config
	txs    PendingTxContext
	ks     SimpleKeystore
	client internal.Loader[client.ReaderWriter]
	fee    fees.Estimator
	// sendTx is an override for sending transactions rather than using a single client
	// Enabling MultiNode uses this function to send transactions to all RPCs
	sendTx func(ctx context.Context, tx *solanaGo.Transaction) (solanaGo.Signature, error)
}

type TxConfig struct {
	Timeout time.Duration // transaction broadcast timeout

	// compute unit price config
	FeeBumpCriteria      string        // "fixed-intervals": bump every fixed period. "expiration": bump based on blockhash expiration
	FeeBumpPeriod        time.Duration // "fixed-intervals": how often to bump. "expiration": how often to check if should blockhash has expired and we need to bump
	BaseComputeUnitPrice uint64        // starting price
	ComputeUnitPriceMin  uint64        // min price
	ComputeUnitPriceMax  uint64        // max price

	EstimateComputeUnitLimit bool   // enable compute limit estimations using simulation
	ComputeUnitLimit         uint32 // compute unit limit
}

// NewTxm creates a txm. Uses simulation so should only be used to send txes to trusted contracts i.e. OCR.
func NewTxm(chainID string, client internal.Loader[client.ReaderWriter],
	sendTx func(ctx context.Context, tx *solanaGo.Transaction) (solanaGo.Signature, error),
	cfg config.Config, ks SimpleKeystore, lggr logger.Logger) *Txm {
	if sendTx == nil {
		// default sendTx using a single RPC
		sendTx = func(ctx context.Context, tx *solanaGo.Transaction) (solanaGo.Signature, error) {
			c, err := client.Get()
			if err != nil {
				return solanaGo.Signature{}, err
			}
			return c.SendTx(ctx, tx)
		}
	}

	return &Txm{
		lggr:   logger.Named(lggr, "Txm"),
		chSend: make(chan pendingTx, MaxQueueLen), // queue can support 1000 pending txs
		chSim:  make(chan pendingTx, MaxQueueLen), // queue can support 1000 pending txs
		chStop: make(chan struct{}),
		cfg:    cfg,
		txs:    newPendingTxContextWithProm(chainID),
		ks:     ks,
		client: client,
		sendTx: sendTx,
	}
}

// Start subscribes to queuing channel and processes them.
func (txm *Txm) Start(ctx context.Context) error {
	return txm.StartOnce("Txm", func() error {
		// validate config
		if err := txm.validateConfig(); err != nil {
			return err
		}

		// start loops
		if err := txm.fee.Start(ctx); err != nil {
			return err
		}

		txm.done.Add(3) // waitgroup: tx retry, confirmer, simulator
		go txm.run()
		go txm.confirm()
		go txm.simulate()
		// Start reaping loop only if TxRetentionTimeout > 0
		// Otherwise, transactions are dropped immediately after finalization so the loop is not required
		if txm.cfg.TxRetentionTimeout() > 0 {
			txm.done.Add(1) // waitgroup: reaper
			go txm.reap()
		}

		return nil
	})
}

// validateConfig validates consistency of configs when starting the services.
func (txm *Txm) validateConfig() (err error) {
	// Validate fee estimator configuration
	var estimator fees.Estimator
	switch strings.ToLower(txm.cfg.FeeEstimatorMode()) {
	case FeeEstimatorModeFixed:
		estimator, err = fees.NewFixedPriceEstimator(txm.cfg)
	case FeeEstimatorModeBlockHistory:
		estimator, err = fees.NewBlockHistoryEstimator(txm.client, txm.cfg, txm.lggr)
	default:
		err = fmt.Errorf("unknown solana fee estimator type: %s", txm.cfg.FeeEstimatorMode())
	}
	if err != nil {
		return err
	}
	txm.fee = estimator

	// Validate fee bumping configuration if it's enabled.
	if txm.cfg.FeeBumpPeriod() > 0 {
		if txm.cfg.FeeBumpCriteria() != FeeBumpCriteriaFixedIntervals && txm.cfg.FeeBumpCriteria() != FeeBumpCriteriaExpiration && txm.cfg.FeeBumpCriteria() != FeeBumpCriteriaNone {
			return errors.New("invalid FeeBumpCriteria; must be 'fixed-intervals', 'expiration' or 'none'")
		}
	}

	return err
}

func (txm *Txm) run() {
	defer txm.done.Done()
	ctx, cancel := txm.chStop.NewCtx()
	defer cancel()

	for {
		select {
		case msg := <-txm.chSend:
			// process tx (pass tx copy)
			tx, id, sig, err := txm.sendWithRetry(ctx, msg)
			if err != nil {
				txm.lggr.Errorw("failed to send transaction", "error", err)
				txm.client.Reset() // clear client if tx fails immediately (potentially bad RPC)
				continue           // skip remainining
			}

			// send tx + signature to simulation queue
			msg.tx = tx
			msg.signatures = append(msg.signatures, sig)
			msg.id = id
			select {
			case txm.chSim <- msg:
			default:
				txm.lggr.Warnw("failed to enqueue tx for simulation", "queueFull", len(txm.chSend) == MaxQueueLen, "tx", msg)
			}

			txm.lggr.Debugw("transaction sent", "signature", sig.String(), "id", id)
		case <-txm.chStop:
			return
		}
	}
}

// sendWithRetry sends a transaction and retries it with exponential backoff if necessary.
// It builds the initial transaction, sends it, and then starts a retry mechanism in a goroutine.
func (txm *Txm) sendWithRetry(ctx context.Context, msg pendingTx) (solanaGo.Transaction, string, solanaGo.Signature, error) {
	// get key
	// fee payer account is index 0 account
	// https://github.com/gagliardetto/solana-go/blob/main/transaction.go#L252
	key := msg.tx.Message.AccountKeys[0].String()
	baseTx := msg.tx

	// add compute unit limit instruction - static for the transaction
	// skip if compute unit limit = 0 (otherwise would always fail)
	if msg.cfg.ComputeUnitLimit != 0 {
		if computeUnitLimitErr := fees.SetComputeUnitLimit(&baseTx, fees.ComputeUnitLimit(msg.cfg.ComputeUnitLimit)); computeUnitLimitErr != nil {
			return solanaGo.Transaction{}, "", solanaGo.Signature{}, fmt.Errorf("failed to add compute unit limit instruction: %w", computeUnitLimitErr)
		}
	}

	// Build the initial transaction
	initTx, initBuildErr := txm.buildTx(ctx, baseTx, key, 0)
	if initBuildErr != nil {
		return solanaGo.Transaction{}, "", solanaGo.Signature{}, initBuildErr
	}

	// create timeout context
	ctx, cancel := context.WithTimeout(ctx, msg.cfg.Timeout)

	// send initial tx (do not retry and exit early if fails)
	sig, initSendErr := txm.sendTx(ctx, &initTx)
	if initSendErr != nil {
		cancel()                                                         // cancel context when exiting early
		txm.txs.OnError(sig, txm.cfg.TxRetentionTimeout(), TxFailReject) //nolint // no need to check error since only incrementing metric here
		return solanaGo.Transaction{}, "", solanaGo.Signature{}, fmt.Errorf("tx failed initial transmit: %w", initSendErr)
	}

	// store tx signature + cancel function
	initStoreErr := txm.txs.New(msg, sig, cancel)
	if initStoreErr != nil {
		cancel() // cancel context when exiting early
		return solanaGo.Transaction{}, "", solanaGo.Signature{}, fmt.Errorf("failed to save tx signature (%s) to inflight txs: %w", sig, initStoreErr)
	}

	// used for tracking rebroadcasting only in SendWithRetry
	var sigs signatureList
	sigs.Allocate()
	if initSetErr := sigs.Set(0, sig); initSetErr != nil {
		return solanaGo.Transaction{}, "", solanaGo.Signature{}, fmt.Errorf("failed to save initial signature in signature list: %w", initSetErr)
	}

	txm.lggr.Debugw("tx initial broadcast", "id", msg.id, "fee", txm.getFee(0), "signature", sig)

	txm.done.Add(1)
	// retry with exponential backoff
	// until context cancelled by timeout or called externally
	// pass in copy of baseTx (used to build new tx with bumped fee) and broadcasted tx == initTx (used to retry tx without bumping)
	go func(ctx context.Context, baseTx, currentTx solanaGo.Transaction) {
		defer txm.done.Done()
		retryInterval := time.Millisecond // Initial retry interval (1ms)
		bumpCount := 0                    // Number of times the fee has been bumped
		bumpTime := time.Now()
		var wg sync.WaitGroup

		// Create a timer that fires immediately
		retryTimer := time.NewTimer(0)
		defer retryTimer.Stop()

		for {
			select {
			case <-ctx.Done():
				// Wait for any ongoing retries to finish before exiting
				wg.Wait()
				txm.lggr.Debugw("stopped tx retry", "id", msg.id, "signatures", sigs.List(), "err", context.Cause(ctx))
				return
			case <-retryTimer.C:
				// Determine if we should bump the fee and/or update the blockhash
				shouldBump, needBlockhashUpdate, newBumpTime, err := txm.bumpFeeAndUpdateBlockhash(ctx, msg.id, bumpTime)
				if err != nil {
					txm.lggr.Errorw("error determining if should bump fee", "error", err)
					return
				}
				bumpTime = newBumpTime

				if shouldBump {
					bumpCount++
					// Prepare the retry transaction with a bumped fee and updated blockhash if needed
					currentTx, err = txm.prepareRetryTx(ctx, baseTx, key, &msg, bumpCount, needBlockhashUpdate)
					if err != nil {
						txm.lggr.Errorw("failed to prepare retry transaction", "error", err, "id", msg.id)
						return
					}
					ind := sigs.Allocate()
					if ind != bumpCount {
						txm.lggr.Errorw("INVARIANT VIOLATION: index (%d) != bumpCount (%d)", ind, bumpCount)
						return
					}
				}

				wg.Add(1)
				go func(bump bool, count int, retryTx solanaGo.Transaction) {
					defer wg.Done()
					// Send the retry transaction in a separate goroutine
					txm.sendRetryTx(ctx, bump, count, retryTx, msg.id, &sigs)
				}(shouldBump, bumpCount, currentTx)
			}

			// Calculate the next retry interval with exponential backoff. Capped at MaxRetryTimeMs.
			retryInterval = getNextRetryInterval(retryInterval)
			resetTimer(retryTimer, retryInterval)
		}
	}(ctx, baseTx, initTx)

	// return signed tx, id, signature for use in simulation
	return initTx, msg.id, sig, nil
}

// buildTx constructs a new transaction with the specified fee and signs it.
// It updates the compute unit price and appends the signature to the transaction.
func (txm *Txm) buildTx(ctx context.Context, baseTx solanaGo.Transaction, key string, retryCount int) (solanaGo.Transaction, error) {
	// Set fee
	if computeUnitErr := fees.SetComputeUnitPrice(&baseTx, txm.getFee(retryCount)); computeUnitErr != nil {
		return solanaGo.Transaction{}, computeUnitErr
	}

	// Serialize the transaction message for signing
	txMsg, marshalErr := baseTx.Message.MarshalBinary()
	if marshalErr != nil {
		return solanaGo.Transaction{}, fmt.Errorf("error in MarshalBinary: %w", marshalErr)
	}

	// Sign the transaction message using the key
	sigBytes, signErr := txm.ks.Sign(ctx, key, txMsg)
	if signErr != nil {
		return solanaGo.Transaction{}, fmt.Errorf("error in Sign: %w", signErr)
	}

	// Append the signature to the transaction
	var finalSig [64]byte
	copy(finalSig[:], sigBytes)
	baseTx.Signatures = append(baseTx.Signatures, finalSig)

	return baseTx, nil
}

// getFee calculates the compute unit price for a transaction based on the number of retries.
func (txm *Txm) getFee(count int) fees.ComputeUnitPrice {
	// base compute unit price should only be calculated once
	// prevent underlying base changing when bumping (could occur with RPC based estimation)
	fee := fees.CalculateFee(
		txm.cfg.ComputeUnitPriceDefault(),
		txm.cfg.ComputeUnitPriceMax(),
		txm.cfg.ComputeUnitPriceMin(),
		uint(count), //nolint:gosec // reasonable number of bumps should never cause overflow
	)
	return fees.ComputeUnitPrice(fee)
}

// bumpFeeAndUpdateBlockhash determines whether the transaction fee should be bumped
// and whether the blockhash needs to be updated based on the configured criteria.
func (txm *Txm) bumpFeeAndUpdateBlockhash(ctx context.Context, txID string, bumpTime time.Time) (shouldBump bool, needBlockhashUpdate bool, newBumpTime time.Time, err error) {
	// helper function to get current height
	getCurrentHeight := func(ctx context.Context) (uint64, error) {
		client, errClient := txm.client.Get()
		if errClient != nil {
			return 0, errClient
		}

		currHeight, errSlot := client.SlotHeight(ctx)
		if errSlot != nil {
			return 0, errSlot
		}

		return currHeight, nil
	}

	newBumpTime = bumpTime
	currHeight, err := getCurrentHeight(ctx)
	if err != nil {
		return false, false, bumpTime, fmt.Errorf("failed to get current height: %w", err)
	}

	// Check if fee bumping is enabled and decide based on the criteria
	if txm.cfg.FeeBumpPeriod() > 0 {
		switch txm.cfg.FeeBumpCriteria() {
		case FeeBumpCriteriaFixedIntervals:
			// Bump the fee at fixed intervals. We may want to bump without blockhash being expired.
			if time.Since(bumpTime) >= txm.cfg.FeeBumpPeriod() {
				shouldBump = true
				newBumpTime = time.Now()
			}

			// in case blockhash has expired, we need to update it.
			if txm.txs.IsBlockhashExpired(txID, currHeight) {
				needBlockhashUpdate = true
			}
		case FeeBumpCriteriaExpiration:
			// Bump the fee only if the blockhash has expired
			if txm.txs.IsBlockhashExpired(txID, currHeight) {
				shouldBump = true
				needBlockhashUpdate = true
			}
		case FeeBumpCriteriaNone:
			// Do not bump the fee
			shouldBump = false
		default:
			txm.lggr.Errorw("unknown fee bump criteria", "criteria", txm.cfg.FeeBumpCriteria())
		}
	}

	return shouldBump, needBlockhashUpdate, newBumpTime, nil
}

// prepareRetryTx prepares a new transaction for retrying by optionally updating the blockhash
// and rebuilding the transaction with a bumped fee.
func (txm *Txm) prepareRetryTx(ctx context.Context, baseTx solanaGo.Transaction, key string, msg *pendingTx, retryCount int, needBlockhashUpdate bool) (solanaGo.Transaction, error) {
	// helper function to get latest blockhash
	getLatestBlockhash := func(ctx context.Context) (*rpc.GetLatestBlockhashResult, error) {
		client, err := txm.client.Get()
		if err != nil {
			return nil, fmt.Errorf("failed to get client: %w", err)
		}
		blockhashResult, err := client.LatestBlockhash(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get latest blockhash: %w", err)
		}

		return blockhashResult, nil
	}

	// We need to update blockhash in retryTx and pendingTx context if it has expired
	if needBlockhashUpdate {
		// Updates Blockhash and LastValidBlockHeight in the retry transaction
		blockhashResult, err := getLatestBlockhash(ctx)
		if err != nil {
			return solanaGo.Transaction{}, fmt.Errorf("failed to get latest blockhash: %w", err)
		}
		baseTx.Message.RecentBlockhash = blockhashResult.Value.Blockhash
		msg.lastValidBlockHeight = blockhashResult.Value.LastValidBlockHeight

		// updates Blockhash and LastValidBlockHeight in txPending context.
		err = txm.txs.UpdateBlockhash(msg.id, blockhashResult)
		if err != nil {
			return solanaGo.Transaction{}, fmt.Errorf("failed to update blockhash in txPending context: %w", err)
		}
	}

	// return new transaction with bumped fee and blockhash updated if needed.
	return txm.buildTx(ctx, baseTx, key, retryCount)
}

// sendRetryTx sends a retry transaction with an optional fee bump.
// It handles sending the transaction and updating the signature tracking.
func (txm *Txm) sendRetryTx(ctx context.Context, bump bool, count int, retryTx solanaGo.Transaction, txID string, sigs *signatureList) {
	retrySig, retrySendErr := txm.sendTx(ctx, &retryTx)
	// Handle send errors
	if retrySendErr != nil {
		if strings.Contains(retrySendErr.Error(), "context canceled") || strings.Contains(retrySendErr.Error(), "context deadline exceeded") {
			txm.lggr.Debugw("ctx error on send retry transaction", "error", retrySendErr, "signatures", sigs.List(), "id", txID)
		} else {
			txm.lggr.Warnw("failed to send retry transaction", "error", retrySendErr, "signatures", sigs.List(), "id", txID)
		}
		return
	}

	// If the fee was bumped, save the new signature and update tracking
	if bump {
		if retryStoreErr := txm.txs.AddSignature(txID, retrySig); retryStoreErr != nil {
			txm.lggr.Warnw("error in adding retry transaction", "error", retryStoreErr, "id", txID)
			return
		}
		if setErr := sigs.Set(count, retrySig); setErr != nil {
			txm.lggr.Errorw("INVARIANT VIOLATION", "error", setErr)
		}
		txm.lggr.Debugw("tx rebroadcast with bumped fee", "id", txID, "fee", txm.getFee(count), "signatures", sigs.List())
	}

	// Wait for signature synchronization. Prevent locking on waitgroup when ctx is closed
	wait := make(chan struct{})
	go func() {
		defer close(wait)
		sigs.Wait(count)
	}()
	select {
	case <-ctx.Done():
		return
	case <-wait:
	}

	// Check for invariant violation. Verify that the fetched signature matches the expected one
	if fetchedSig, fetchErr := sigs.Get(count); fetchErr != nil || retrySig != fetchedSig {
		txm.lggr.Errorw("original signature does not match retry signature", "expectedSignatures", sigs.List(), "receivedSignature", retrySig, "error", fetchErr)
	}
}

// getNextRetryInterval calculates the next retry interval using exponential backoff.
// It doubles the current interval up to a maximum defined by MaxRetryTimeMs.
func getNextRetryInterval(currentInterval time.Duration) time.Duration {
	nextInterval := currentInterval * 2
	maxInterval := time.Duration(MaxRetryTimeMs) * time.Millisecond
	if nextInterval > maxInterval {
		return maxInterval
	}
	return nextInterval
}

// resetTimer safely resets a time.Timer to the new interval.
// It ensures that the timer is stopped and the channel is drained before resetting.
func resetTimer(timer *time.Timer, newInterval time.Duration) {
	if !timer.Stop() {
		// If the timer has already expired, drain the channel to prevent race conditions.
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(newInterval)
}

// goroutine that polls to confirm implementation
// cancels the exponential retry once confirmed
func (txm *Txm) confirm() {
	defer txm.done.Done()
	ctx, cancel := txm.chStop.NewCtx()
	defer cancel()

	tick := time.After(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			// get list of tx signatures to confirm
			sigs := txm.txs.ListAll()

			// exit switch if not txs to confirm
			if len(sigs) == 0 {
				break
			}

			// get client
			client, err := txm.client.Get()
			if err != nil {
				txm.lggr.Errorw("failed to get client in soltxm.confirm", "error", err)
				break // exit switch
			}

			// batch sigs no more than MaxSigsToConfirm each
			sigsBatch, err := utils.BatchSplit(sigs, MaxSigsToConfirm)
			if err != nil { // this should never happen
				txm.lggr.Fatalw("failed to batch signatures", "error", err)
				break // exit switch
			}

			// process signatures
			processSigs := func(s []solanaGo.Signature, res []*rpc.SignatureStatusesResult) {
				// sort signatures and results process successful first
				s, res, err := SortSignaturesAndResults(s, res)
				if err != nil {
					txm.lggr.Errorw("sorting error", "error", err)
					return
				}

				for i := 0; i < len(res); i++ {
					// if status is nil (sig not found), continue polling
					// sig not found could mean invalid tx or not picked up yet
					if res[i] == nil {
						txm.lggr.Debugw("tx state: not found",
							"signature", s[i],
						)

						// check confirm timeout exceeded
						if txm.txs.Expired(s[i], txm.cfg.TxConfirmTimeout()) {
							id, err := txm.txs.OnError(s[i], txm.cfg.TxRetentionTimeout(), TxFailDrop)
							if err != nil {
								txm.lggr.Infow("failed to mark transaction as errored", "id", id, "signature", s[i], "timeoutSeconds", txm.cfg.TxConfirmTimeout(), "error", err)
							} else {
								txm.lggr.Infow("failed to find transaction within confirm timeout", "id", id, "signature", s[i], "timeoutSeconds", txm.cfg.TxConfirmTimeout())
							}
						}
						continue
					}

					// if signature has an error, end polling
					if res[i].Err != nil {
						id, err := txm.txs.OnError(s[i], txm.cfg.TxRetentionTimeout(), TxFailRevert)
						if err != nil {
							txm.lggr.Infow("failed to mark transaction as errored", "id", id, "signature", s[i], "error", err)
						} else {
							txm.lggr.Debugw("tx state: failed", "id", id, "signature", s[i], "error", res[i].Err, "status", res[i].ConfirmationStatus)
						}
						continue
					}

					// if signature is processed, keep polling for confirmed or finalized status
					if res[i].ConfirmationStatus == rpc.ConfirmationStatusProcessed {
						// update transaction state in local memory
						id, err := txm.txs.OnProcessed(s[i])
						if err != nil && !errors.Is(err, ErrAlreadyInExpectedState) {
							txm.lggr.Errorw("failed to mark transaction as processed", "signature", s[i], "error", err)
						} else if err == nil {
							txm.lggr.Debugw("marking transaction as processed", "id", id, "signature", s[i])
						}
						// check confirm timeout exceeded if TxConfirmTimeout set
						if txm.cfg.TxConfirmTimeout() != 0*time.Second && txm.txs.Expired(s[i], txm.cfg.TxConfirmTimeout()) {
							id, err := txm.txs.OnError(s[i], txm.cfg.TxRetentionTimeout(), TxFailDrop)
							if err != nil {
								txm.lggr.Infow("failed to mark transaction as errored", "id", id, "signature", s[i], "timeoutSeconds", txm.cfg.TxConfirmTimeout(), "error", err)
							} else {
								txm.lggr.Debugw("tx failed to move beyond 'processed' within confirm timeout", "id", id, "signature", s[i], "timeoutSeconds", txm.cfg.TxConfirmTimeout())
							}
						}
						continue
					}

					// if signature is confirmed, keep polling for finalized status
					if res[i].ConfirmationStatus == rpc.ConfirmationStatusConfirmed {
						id, err := txm.txs.OnConfirmed(s[i])
						if err != nil && !errors.Is(err, ErrAlreadyInExpectedState) {
							txm.lggr.Errorw("failed to mark transaction as confirmed", "id", id, "signature", s[i], "error", err)
						} else if err == nil {
							txm.lggr.Debugw("marking transaction as confirmed", "id", id, "signature", s[i])
						}
						continue
					}

					// if signature is finalized, end polling
					if res[i].ConfirmationStatus == rpc.ConfirmationStatusFinalized {
						id, err := txm.txs.OnFinalized(s[i], txm.cfg.TxRetentionTimeout())
						if err != nil {
							txm.lggr.Errorw("failed to mark transaction as finalized", "id", id, "signature", s[i], "error", err)
						} else {
							txm.lggr.Debugw("marking transaction as finalized", "id", id, "signature", s[i])
						}
						continue
					}
				}
			}

			// waitgroup for processing
			var wg sync.WaitGroup

			// loop through batch
			for i := 0; i < len(sigsBatch); i++ {
				// fetch signature statuses
				statuses, err := client.SignatureStatuses(ctx, sigsBatch[i])
				if err != nil {
					txm.lggr.Errorw("failed to get signature statuses in soltxm.confirm", "error", err)
					break // exit for loop
				}

				wg.Add(1)
				// nonblocking: process batches as soon as they come in
				go func(index int) {
					defer wg.Done()
					processSigs(sigsBatch[index], statuses)
				}(i)
			}
			wg.Wait() // wait for processing to finish
		}
		tick = time.After(utils.WithJitter(txm.cfg.ConfirmPollPeriod()))
	}
}

// goroutine that simulates tx (use a bounded number of goroutines to pick from queue?)
// simulate can cancel the send retry function early in the tx management process
// additionally, it can provide reasons for why a tx failed in the logs
func (txm *Txm) simulate() {
	defer txm.done.Done()
	ctx, cancel := txm.chStop.NewCtx()
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-txm.chSim:
			res, err := txm.simulateTx(ctx, &msg.tx)
			if err != nil {
				// this error can occur if endpoint goes down or if invalid signature (invalid signature should occur further upstream in sendWithRetry)
				// allow retry to continue in case temporary endpoint failure (if still invalid, confirmation or timeout will cleanup)
				txm.lggr.Debugw("failed to simulate tx", "id", msg.id, "signatures", msg.signatures, "error", err)
				continue
			}

			// continue if simulation does not return error continue
			if res.Err == nil {
				continue
			}

			// Transaction has to have a signature if simulation succeeded but added check for belt and braces approach
			if len(msg.signatures) > 0 {
				txm.processSimulationError(msg.id, msg.signatures[0], res)
			}
		}
	}
}

// reap is a goroutine that periodically checks whether finalized and errored transactions have reached
// their retention threshold and purges them from the in-memory storage if they have
func (txm *Txm) reap() {
	defer txm.done.Done()
	ctx, cancel := txm.chStop.NewCtx()
	defer cancel()

	tick := time.After(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			txm.txs.TrimFinalizedErroredTxs()
		}
		tick = time.After(utils.WithJitter(TxReapInterval))
	}
}

// Enqueue enqueues a msg destined for the solana chain.
func (txm *Txm) Enqueue(ctx context.Context, accountID string, tx *solanaGo.Transaction, txID *string, LastValidBlockHeight uint64, txCfgs ...SetTxConfig) error {
	if err := txm.Ready(); err != nil {
		return fmt.Errorf("error in soltxm.Enqueue: %w", err)
	}

	// validate nil pointer
	if tx == nil {
		return errors.New("error in soltxm.Enqueue: tx is nil pointer")
	}
	// validate account keys slice
	if len(tx.Message.AccountKeys) == 0 {
		return errors.New("error in soltxm.Enqueue: not enough account keys in tx")
	}

	// validate expected key exists by trying to sign with it
	// fee payer account is index 0 account
	// https://github.com/gagliardetto/solana-go/blob/main/transaction.go#L252
	_, err := txm.ks.Sign(ctx, tx.Message.AccountKeys[0].String(), nil)
	if err != nil {
		return fmt.Errorf("error in soltxm.Enqueue.GetKey: %w", err)
	}

	// apply changes to default config
	cfg := txm.defaultTxConfig()
	for _, v := range txCfgs {
		v(&cfg)
	}

	if cfg.EstimateComputeUnitLimit {
		computeUnitLimit, err := txm.EstimateComputeUnitLimit(ctx, tx)
		if err != nil {
			return fmt.Errorf("transaction failed simulation: %w", err)
		}
		// If estimation returns 0 compute unit limit without error, fallback to original config
		if computeUnitLimit != 0 {
			cfg.ComputeUnitLimit = computeUnitLimit
		}
	}

	// Use transaction ID provided by caller if set
	id := uuid.New().String()
	if txID != nil && *txID != "" {
		id = *txID
	}
	msg := pendingTx{
		tx:                   *tx,
		cfg:                  cfg,
		id:                   id,
		lastValidBlockHeight: LastValidBlockHeight,
	}

	select {
	case txm.chSend <- msg:
	default:
		txm.lggr.Errorw("failed to enqeue tx", "queueFull", len(txm.chSend) == MaxQueueLen, "tx", msg)
		return fmt.Errorf("failed to enqueue transaction for %s", accountID)
	}
	return nil
}

// GetTransactionStatus translates internal TXM transaction statuses to chainlink common statuses
func (txm *Txm) GetTransactionStatus(ctx context.Context, transactionID string) (commontypes.TransactionStatus, error) {
	state, err := txm.txs.GetTxState(transactionID)
	if err != nil {
		return commontypes.Unknown, fmt.Errorf("failed to find transaction with id %s: %w", transactionID, err)
	}

	switch state {
	case Broadcasted:
		return commontypes.Pending, nil
	case Processed, Confirmed:
		return commontypes.Unconfirmed, nil
	case Finalized:
		return commontypes.Finalized, nil
	case Errored:
		return commontypes.Failed, nil
	default:
		return commontypes.Unknown, fmt.Errorf("found unknown transaction state: %s", state.String())
	}
}

// EstimateComputeUnitLimit estimates the compute unit limit needed for a transaction.
// It simulates the provided transaction to determine the used compute and applies a buffer to it.
func (txm *Txm) EstimateComputeUnitLimit(ctx context.Context, tx *solanaGo.Transaction) (uint32, error) {
	res, err := txm.simulateTx(ctx, tx)
	if err != nil {
		return 0, err
	}

	// Return error if response err is non-nil to avoid broadcasting a tx destined to fail
	if res.Err != nil {
		sig := solanaGo.Signature{}
		if len(tx.Signatures) > 0 {
			sig = tx.Signatures[0]
		}
		txm.processSimulationError("", sig, res)
		return 0, fmt.Errorf("simulated tx returned error: %v", res.Err)
	}

	if res.UnitsConsumed == nil || *res.UnitsConsumed == 0 {
		txm.lggr.Debug("failed to get units consumed for tx")
		// Do not return error to allow falling back to default compute unit limit
		return 0, nil
	}

	unitsConsumed := *res.UnitsConsumed

	// Add buffer to the used compute estimate
	unitsConsumed = bigmath.AddPercentage(new(big.Int).SetUint64(unitsConsumed), EstimateComputeUnitLimitBuffer).Uint64()

	if unitsConsumed > math.MaxUint32 {
		txm.lggr.Debug("compute units used with buffer greater than uint32 max", "unitsConsumed", unitsConsumed)
		// Do not return error to allow falling back to default compute unit limit
		return 0, nil
	}

	return uint32(unitsConsumed), nil
}

// simulateTx simulates transactions using the SimulateTx client method
func (txm *Txm) simulateTx(ctx context.Context, tx *solanaGo.Transaction) (res *rpc.SimulateTransactionResult, err error) {
	// get client
	client, err := txm.client.Get()
	if err != nil {
		txm.lggr.Errorw("failed to get client", "error", err)
		return
	}

	res, err = client.SimulateTx(ctx, tx, nil) // use default options (does not verify signatures)
	if err != nil {
		// This error can occur if endpoint goes down or if invalid signature
		txm.lggr.Errorw("failed to simulate tx", "error", err)
		return
	}
	return
}

// processSimulationError parses and handles relevant errors found in simulation results
func (txm *Txm) processSimulationError(id string, sig solanaGo.Signature, res *rpc.SimulateTransactionResult) {
	if res.Err != nil {
		// handle various errors
		// https://github.com/solana-labs/solana/blob/master/sdk/src/transaction/error.rs
		errStr := fmt.Sprintf("%v", res.Err) // convert to string to handle various interfaces
		logValues := []interface{}{
			"id", id,
			"signature", sig,
			"result", res,
		}
		switch {
		// blockhash not found when simulating, occurs when network bank has not seen the given blockhash or tx is too old
		// let confirmation process clean up
		case strings.Contains(errStr, "BlockhashNotFound"):
			txm.lggr.Debugw("simulate: BlockhashNotFound", logValues...)
		// transaction will encounter execution error/revert, mark as reverted to remove from confirmation + retry
		case strings.Contains(errStr, "InstructionError"):
			_, err := txm.txs.OnError(sig, txm.cfg.TxRetentionTimeout(), TxFailSimRevert) // cancel retry
			if err != nil {
				logValues = append(logValues, "stateTransitionErr", err)
			}
			txm.lggr.Debugw("simulate: InstructionError", logValues...)
		// transaction is already processed in the chain, letting txm confirmation handle
		case strings.Contains(errStr, "AlreadyProcessed"):
			txm.lggr.Debugw("simulate: AlreadyProcessed", logValues...)
		// unrecognized errors (indicates more concerning failures)
		default:
			_, err := txm.txs.OnError(sig, txm.cfg.TxRetentionTimeout(), TxFailSimOther) // cancel retry
			if err != nil {
				logValues = append(logValues, "stateTransitionErr", err)
			}
			txm.lggr.Errorw("simulate: unrecognized error", logValues...)
		}
	}
}

func (txm *Txm) InflightTxs() int {
	return len(txm.txs.ListAll())
}

// Close close service
func (txm *Txm) Close() error {
	return txm.StopOnce("Txm", func() error {
		close(txm.chStop)
		txm.done.Wait()
		return txm.fee.Close()
	})
}
func (txm *Txm) Name() string { return txm.lggr.Name() }

func (txm *Txm) HealthReport() map[string]error { return map[string]error{txm.Name(): txm.Healthy()} }

func (txm *Txm) defaultTxConfig() TxConfig {
	return TxConfig{
		Timeout:                  txm.cfg.TxRetryTimeout(),
		FeeBumpCriteria:          txm.cfg.FeeBumpCriteria(),
		FeeBumpPeriod:            txm.cfg.FeeBumpPeriod(),
		BaseComputeUnitPrice:     txm.fee.BaseComputeUnitPrice(),
		ComputeUnitPriceMin:      txm.cfg.ComputeUnitPriceMin(),
		ComputeUnitPriceMax:      txm.cfg.ComputeUnitPriceMax(),
		ComputeUnitLimit:         txm.cfg.ComputeUnitLimitDefault(),
		EstimateComputeUnitLimit: txm.cfg.EstimateComputeUnitLimit(),
	}
}

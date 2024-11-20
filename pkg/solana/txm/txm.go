package txm

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/smartcontractkit/chainlink-common/pkg/utils/mathutil"

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
	MaxComputeUnitLimit            = 1_400_000        // max compute unit limit a transaction can have
)

var _ services.Service = (*Txm)(nil)

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
	FeeBumpPeriod        time.Duration // how often to bump fee
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
		// determine estimator type
		var estimator fees.Estimator
		var err error
		switch strings.ToLower(txm.cfg.FeeEstimatorMode()) {
		case "fixed":
			estimator, err = fees.NewFixedPriceEstimator(txm.cfg)
		case "blockhistory":
			estimator, err = fees.NewBlockHistoryEstimator(txm.client, txm.cfg, txm.lggr)
		default:
			err = fmt.Errorf("unknown solana fee estimator type: %s", txm.cfg.FeeEstimatorMode())
		}
		if err != nil {
			return err
		}
		txm.fee = estimator
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

func (txm *Txm) sendWithRetry(ctx context.Context, msg pendingTx) (solanaGo.Transaction, string, solanaGo.Signature, error) {
	// get key
	// fee payer account is index 0 account
	// https://github.com/gagliardetto/solana-go/blob/main/transaction.go#L252
	key := msg.tx.Message.AccountKeys[0].String()

	// base compute unit price should only be calculated once
	// prevent underlying base changing when bumping (could occur with RPC based estimation)
	getFee := func(count int) fees.ComputeUnitPrice {
		fee := fees.CalculateFee(
			msg.cfg.BaseComputeUnitPrice,
			msg.cfg.ComputeUnitPriceMax,
			msg.cfg.ComputeUnitPriceMin,
			uint(count), //nolint:gosec // reasonable number of bumps should never cause overflow
		)
		return fees.ComputeUnitPrice(fee)
	}

	baseTx := msg.tx

	// add compute unit limit instruction - static for the transaction
	// skip if compute unit limit = 0 (otherwise would always fail)
	if msg.cfg.ComputeUnitLimit != 0 {
		if computeUnitLimitErr := fees.SetComputeUnitLimit(&baseTx, fees.ComputeUnitLimit(msg.cfg.ComputeUnitLimit)); computeUnitLimitErr != nil {
			return solanaGo.Transaction{}, "", solanaGo.Signature{}, fmt.Errorf("failed to add compute unit limit instruction: %w", computeUnitLimitErr)
		}
	}

	buildTx := func(ctx context.Context, base solanaGo.Transaction, retryCount int) (solanaGo.Transaction, error) {
		newTx := base // make copy

		// set fee
		// fee bumping can be enabled by moving the setting & signing logic to the broadcaster
		if computeUnitErr := fees.SetComputeUnitPrice(&newTx, getFee(retryCount)); computeUnitErr != nil {
			return solanaGo.Transaction{}, computeUnitErr
		}

		// sign tx
		txMsg, marshalErr := newTx.Message.MarshalBinary()
		if marshalErr != nil {
			return solanaGo.Transaction{}, fmt.Errorf("error in soltxm.SendWithRetry.MarshalBinary: %w", marshalErr)
		}
		sigBytes, signErr := txm.ks.Sign(ctx, key, txMsg)
		if signErr != nil {
			return solanaGo.Transaction{}, fmt.Errorf("error in soltxm.SendWithRetry.Sign: %w", signErr)
		}
		var finalSig [64]byte
		copy(finalSig[:], sigBytes)
		newTx.Signatures = append(newTx.Signatures, finalSig)

		return newTx, nil
	}

	initTx, initBuildErr := buildTx(ctx, baseTx, 0)
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

	txm.lggr.Debugw("tx initial broadcast", "id", msg.id, "fee", getFee(0), "signature", sig)

	txm.done.Add(1)
	// retry with exponential backoff
	// until context cancelled by timeout or called externally
	// pass in copy of baseTx (used to build new tx with bumped fee) and broadcasted tx == initTx (used to retry tx without bumping)
	go func(ctx context.Context, baseTx, currentTx solanaGo.Transaction) {
		defer txm.done.Done()
		deltaT := 1 // ms
		tick := time.After(0)
		bumpCount := 0
		bumpTime := time.Now()
		var wg sync.WaitGroup

		for {
			select {
			case <-ctx.Done():
				// stop sending tx after retry tx ctx times out (does not stop confirmation polling for tx)
				wg.Wait()
				txm.lggr.Debugw("stopped tx retry", "id", msg.id, "signatures", sigs.List(), "err", context.Cause(ctx))
				return
			case <-tick:
				var shouldBump bool
				// bump if period > 0 and past time
				if msg.cfg.FeeBumpPeriod != 0 && time.Since(bumpTime) > msg.cfg.FeeBumpPeriod {
					bumpCount++
					bumpTime = time.Now()
					shouldBump = true
				}

				// if fee should be bumped, build new tx and replace currentTx
				if shouldBump {
					var retryBuildErr error
					currentTx, retryBuildErr = buildTx(ctx, baseTx, bumpCount)
					if retryBuildErr != nil {
						txm.lggr.Errorw("failed to build bumped retry tx", "error", retryBuildErr, "id", msg.id)
						return // exit func if cannot build tx for retrying
					}
					ind := sigs.Allocate()
					if ind != bumpCount {
						txm.lggr.Errorw("INVARIANT VIOLATION: index (%d) != bumpCount (%d)", ind, bumpCount)
						return
					}
				}

				// take currentTx and broadcast, if bumped fee -> save signature to list
				wg.Add(1)
				go func(bump bool, count int, retryTx solanaGo.Transaction) {
					defer wg.Done()

					retrySig, retrySendErr := txm.sendTx(ctx, &retryTx)
					// this could occur if endpoint goes down or if ctx cancelled
					if retrySendErr != nil {
						if strings.Contains(retrySendErr.Error(), "context canceled") || strings.Contains(retrySendErr.Error(), "context deadline exceeded") {
							txm.lggr.Debugw("ctx error on send retry transaction", "error", retrySendErr, "signatures", sigs.List(), "id", msg.id)
						} else {
							txm.lggr.Warnw("failed to send retry transaction", "error", retrySendErr, "signatures", sigs.List(), "id", msg.id)
						}
						return
					}

					// save new signature if fee bumped
					if bump {
						if retryStoreErr := txm.txs.AddSignature(msg.id, retrySig); retryStoreErr != nil {
							txm.lggr.Warnw("error in adding retry transaction", "error", retryStoreErr, "id", msg.id)
							return
						}
						if setErr := sigs.Set(count, retrySig); setErr != nil {
							// this should never happen
							txm.lggr.Errorw("INVARIANT VIOLATION", "error", setErr)
						}
						txm.lggr.Debugw("tx rebroadcast with bumped fee", "id", msg.id, "fee", getFee(count), "signatures", sigs.List())
					}

					// prevent locking on waitgroup when ctx is closed
					wait := make(chan struct{})
					go func() {
						defer close(wait)
						sigs.Wait(count) // wait until bump tx has set the tx signature to compare rebroadcast signatures
					}()
					select {
					case <-ctx.Done():
						return
					case <-wait:
					}

					// this should never happen (should match the signature saved to sigs)
					if fetchedSig, fetchErr := sigs.Get(count); fetchErr != nil || retrySig != fetchedSig {
						txm.lggr.Errorw("original signature does not match retry signature", "expectedSignatures", sigs.List(), "receivedSignature", retrySig, "error", fetchErr)
					}
				}(shouldBump, bumpCount, currentTx)
			}

			// exponential increase in wait time, capped at 250ms
			deltaT *= 2
			if deltaT > MaxRetryTimeMs {
				deltaT = MaxRetryTimeMs
			}
			tick = time.After(time.Duration(deltaT) * time.Millisecond)
		}
	}(ctx, baseTx, initTx)

	// return signed tx, id, signature for use in simulation
	return initTx, msg.id, sig, nil
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
func (txm *Txm) Enqueue(ctx context.Context, accountID string, tx *solanaGo.Transaction, txID *string, txCfgs ...SetTxConfig) error {
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
		tx:  *tx,
		cfg: cfg,
		id:  id,
	}

	select {
	case txm.chSend <- msg:
	default:
		txm.lggr.Errorw("failed to enqueue tx", "queueFull", len(txm.chSend) == MaxQueueLen, "tx", msg)
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
	txCopy := *tx

	// Set max compute unit limit when simulating a transaction to avoid getting an error for exceeding the default 200k compute unit limit
	if computeUnitLimitErr := fees.SetComputeUnitLimit(&txCopy, fees.ComputeUnitLimit(MaxComputeUnitLimit)); computeUnitLimitErr != nil {
		txm.lggr.Errorw("failed to set compute unit limit when simulating tx", "error", computeUnitLimitErr)
		return 0, computeUnitLimitErr
	}

	// Sign and set signature in tx copy for simulation
	txMsg, marshalErr := txCopy.Message.MarshalBinary()
	if marshalErr != nil {
		return 0, fmt.Errorf("failed to marshal tx message: %w", marshalErr)
	}
	sigBytes, signErr := txm.ks.Sign(ctx, txCopy.Message.AccountKeys[0].String(), txMsg)
	if signErr != nil {
		return 0, fmt.Errorf("failed to sign transaction: %w", signErr)
	}
	var sig [64]byte
	copy(sig[:], sigBytes)
	txCopy.Signatures = append(txCopy.Signatures, sig)

	res, err := txm.simulateTx(ctx, &txCopy)
	if err != nil {
		return 0, err
	}

	// Return error if response err is non-nil to avoid broadcasting a tx destined to fail
	if res.Err != nil {
		sig := solanaGo.Signature{}
		if len(txCopy.Signatures) > 0 {
			sig = txCopy.Signatures[0]
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

	// Ensure unitsConsumed does not exceed the max compute unit limit for a transaction after adding buffer
	unitsConsumed = mathutil.Min(unitsConsumed, MaxComputeUnitLimit)

	return uint32(unitsConsumed), nil //nolint // unitsConsumed can only be a maximum of 1.4M
}

// simulateTx simulates transactions using the SimulateTx client method
func (txm *Txm) simulateTx(ctx context.Context, tx *solanaGo.Transaction) (res *rpc.SimulateTransactionResult, err error) {
	// get client
	client, err := txm.client.Get()
	if err != nil {
		txm.lggr.Errorw("failed to get client", "error", err)
		return
	}

	// Simulate with signature verification enabled since it can have an impact on the compute units used
	res, err = client.SimulateTx(ctx, tx, &rpc.SimulateTransactionOpts{SigVerify: true, Commitment: txm.cfg.Commitment()})
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
		FeeBumpPeriod:            txm.cfg.FeeBumpPeriod(),
		BaseComputeUnitPrice:     txm.fee.BaseComputeUnitPrice(),
		ComputeUnitPriceMin:      txm.cfg.ComputeUnitPriceMin(),
		ComputeUnitPriceMax:      txm.cfg.ComputeUnitPriceMax(),
		ComputeUnitLimit:         txm.cfg.ComputeUnitLimitDefault(),
		EstimateComputeUnitLimit: txm.cfg.EstimateComputeUnitLimit(),
	}
}

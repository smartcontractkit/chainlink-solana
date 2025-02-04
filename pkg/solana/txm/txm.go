package txm

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
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
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

const (
	MaxQueueLen                    = 1000
	MaxRetryTimeMs                 = 250              // max tx retry time (exponential retry will taper to retry every 0.25s)
	MaxSigsToConfirm               = 256              // max number of signatures in GetSignatureStatus call
	EstimateComputeUnitLimitBuffer = 10               // percent buffer added on top of estimated compute unit limits to account for any variance
	TxReapInterval                 = 10 * time.Second // interval of time between reaping transactions that have met the retention threshold
	MaxComputeUnitLimit            = 1_400_000        // max compute unit limit a transaction can have
)

type SimpleKeystore interface {
	Sign(ctx context.Context, account string, data []byte) (signature []byte, err error)
	Accounts(ctx context.Context) (accounts []string, err error)
}

var _ loop.Keystore = (SimpleKeystore)(nil)

type TxManager interface {
	services.Service
	Enqueue(ctx context.Context, accountID string, tx *solanaGo.Transaction, txID *string, txLastValidBlockHeight uint64, txCfgs ...txmutils.SetTxConfig) error
	GetTransactionStatus(ctx context.Context, transactionID string) (commontypes.TransactionStatus, error)
}

var _ TxManager = (*Txm)(nil)

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

// run is a goroutine that continuously processes transactions from the chSend channel.
// It attempts to send each transaction with retry logic and, upon success, enqueues the transaction for simulation.
// If a transaction fails to send, it logs the error and resets the client to handle potential bad RPCs.
// The function runs until the chStop channel signals to stop.
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

// sendWithRetry attempts to send a transaction with exponential backoff retry logic.
// It builds, signs, sends the initial tx, and starts a retry routine with fee bumping if needed.
// The function returns the signed transaction, its ID, and the initial signature for use in simulation.
func (txm *Txm) sendWithRetry(ctx context.Context, msg pendingTx) (solanaGo.Transaction, string, solanaGo.Signature, error) {
	// Build and sign initial transaction setting compute unit price and limit
	initTx, err := txm.buildTx(ctx, msg, 0)
	if err != nil {
		return solanaGo.Transaction{}, "", solanaGo.Signature{}, err
	}

	// Send initial transaction
	ctx, cancel := context.WithTimeout(ctx, msg.cfg.Timeout)
	sig, initSendErr := txm.sendTx(ctx, &initTx)
	if initSendErr != nil {
		// Do not retry and exit early if fails
		cancel()
		stateTransitionErr := txm.txs.OnPrebroadcastError(msg.id, txm.cfg.TxRetentionTimeout(), txmutils.Errored, TxFailReject)
		return solanaGo.Transaction{}, "", solanaGo.Signature{}, fmt.Errorf("tx failed initial transmit: %w", errors.Join(initSendErr, stateTransitionErr))
	}

	// Create new transaction in memory
	if err := txm.txs.New(msg); err != nil {
		cancel()
		return solanaGo.Transaction{}, "", solanaGo.Signature{}, fmt.Errorf("failed to create new transaction: %w", err)
	}

	// Associate initial signature and cancel func to tx
	if err := txm.txs.AddSignature(cancel, msg.id, sig); err != nil {
		cancel()
		return solanaGo.Transaction{}, "", solanaGo.Signature{}, fmt.Errorf("failed to save initial signature (%s) to inflight txs: %w", sig, err)
	}

	txm.lggr.Debugw("tx initial broadcast", "id", msg.id, "fee", msg.cfg.BaseComputeUnitPrice, "signature", sig, "lastValidBlockHeight", msg.lastValidBlockHeight)

	// pass in copy of msg (to build new tx with bumped fee) and broadcasted tx == initTx (to retry tx without bumping)
	txm.done.Add(1)
	go func() {
		defer txm.done.Done()
		txm.retryTx(ctx, cancel, msg, initTx, sig)
	}()

	// Return signed tx, id, signature for use in simulation
	return initTx, msg.id, sig, nil
}

// buildTx builds and signs the transaction with the appropriate compute unit price.
func (txm *Txm) buildTx(ctx context.Context, msg pendingTx, retryCount int) (solanaGo.Transaction, error) {
	// work with a copy
	newTx := msg.tx

	// Set compute unit limit if specified
	if msg.cfg.ComputeUnitLimit != 0 {
		if err := fees.SetComputeUnitLimit(&newTx, fees.ComputeUnitLimit(msg.cfg.ComputeUnitLimit)); err != nil {
			return solanaGo.Transaction{}, fmt.Errorf("failed to add compute unit limit instruction: %w", err)
		}
	}

	// Set compute unit price (fee)
	fee := fees.ComputeUnitPrice(
		fees.CalculateFee(
			msg.cfg.BaseComputeUnitPrice,
			msg.cfg.ComputeUnitPriceMax,
			msg.cfg.ComputeUnitPriceMin,
			uint(retryCount), //nolint:gosec // reasonable number of bumps should never cause overflow
		))
	if err := fees.SetComputeUnitPrice(&newTx, fee); err != nil {
		return solanaGo.Transaction{}, err
	}

	// Sign transaction
	// NOTE: fee payer account is index 0 account. https://github.com/gagliardetto/solana-go/blob/main/transaction.go#L252
	txMsg, err := newTx.Message.MarshalBinary()
	if err != nil {
		return solanaGo.Transaction{}, fmt.Errorf("error in MarshalBinary: %w", err)
	}
	sigBytes, err := txm.ks.Sign(ctx, msg.tx.Message.AccountKeys[0].String(), txMsg)
	if err != nil {
		return solanaGo.Transaction{}, fmt.Errorf("error in Sign: %w", err)
	}
	var finalSig [64]byte
	copy(finalSig[:], sigBytes)
	newTx.Signatures = append(newTx.Signatures, finalSig)

	return newTx, nil
}

// retryTx contains the logic for retrying the transaction, including exponential backoff and fee bumping.
// Retries until context cancelled by timeout or called externally.
// It uses handleRetry helper function to handle each retry attempt.
func (txm *Txm) retryTx(ctx context.Context, cancel context.CancelFunc, msg pendingTx, currentTx solanaGo.Transaction, sig solanaGo.Signature) {
	// Initialize signature list with initialTx signature. This list will be used to add new signatures and track retry attempts.
	sigs := &txmutils.SignatureList{}
	sigs.Allocate()
	if initSetErr := sigs.Set(0, sig); initSetErr != nil {
		cancel()
		txm.lggr.Errorw("failed to save initial signature in signature list", "error", initSetErr)
		return
	}

	deltaT := 1 // initial delay in ms
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
			// determines whether the fee should be bumped based on the fee bump period.
			shouldBump := msg.cfg.FeeBumpPeriod != 0 && time.Since(bumpTime) > msg.cfg.FeeBumpPeriod
			if shouldBump {
				bumpCount++
				bumpTime = time.Now()
				// Build new transaction with bumped fee and replace current tx
				var err error
				currentTx, err = txm.buildTx(ctx, msg, bumpCount)
				if err != nil {
					// Exit if unable to build transaction for retrying
					txm.lggr.Errorw("failed to build bumped retry tx", "error", err, "id", msg.id)
					return
				}
				// allocates space for new signature that will be introduced in handleRetry if needs bumping.
				index := sigs.Allocate()
				if index != bumpCount {
					txm.lggr.Errorw("invariant violation: index does not match bumpCount", "index", index, "bumpCount", bumpCount)
					return
				}
			}

			// Start a goroutine to handle the retry attempt
			// takes currentTx and rebroadcast. If needs bumping it will new signature to already allocated space in txmutils.SignatureList.
			wg.Add(1)
			go func(bump bool, count int, retryTx solanaGo.Transaction) {
				defer wg.Done()
				txm.handleRetry(ctx, cancel, msg, bump, count, retryTx, sigs)
			}(shouldBump, bumpCount, currentTx)
		}

		// updates the exponential backoff delay up to a maximum limit.
		deltaT = deltaT * 2
		if deltaT > MaxRetryTimeMs {
			deltaT = MaxRetryTimeMs
		}
		tick = time.After(time.Duration(deltaT) * time.Millisecond)
	}
}

// handleRetry handles the logic for each retry attempt, including sending the transaction, updating signatures, and logging.
func (txm *Txm) handleRetry(ctx context.Context, cancel context.CancelFunc, msg pendingTx, bump bool, count int, retryTx solanaGo.Transaction, sigs *txmutils.SignatureList) {
	// send retry transaction
	retrySig, err := txm.sendTx(ctx, &retryTx)
	if err != nil {
		// this could occur if endpoint goes down or if ctx cancelled
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			txm.lggr.Debugw("ctx error on send retry transaction", "error", err, "signatures", sigs.List(), "id", msg.id)
		} else {
			txm.lggr.Warnw("failed to send retry transaction", "error", err, "signatures", sigs.List(), "id", msg.id)
		}
		return
	}

	// if bump is true, update signature list and set new signature in space already allocated.
	if bump {
		if err := txm.txs.AddSignature(cancel, msg.id, retrySig); err != nil {
			txm.lggr.Warnw("error in adding retry transaction", "error", err, "id", msg.id)
			return
		}
		if err := sigs.Set(count, retrySig); err != nil {
			// this should never happen
			txm.lggr.Errorw("INVARIANT VIOLATION: failed to set signature", "error", err, "id", msg.id)
			return
		}
		txm.lggr.Debugw("tx rebroadcast with bumped fee", "id", msg.id, "retryCount", count, "fee", msg.cfg.BaseComputeUnitPrice, "signatures", sigs.List())
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
	if fetchedSig, err := sigs.Get(count); err != nil || retrySig != fetchedSig {
		txm.lggr.Errorw("original signature does not match retry signature", "expectedSignatures", sigs.List(), "receivedSignature", retrySig, "error", err)
	}
}

// confirm is a goroutine that continuously polls for transaction confirmations. It also handles reorgs and expired transactions rebroadcasting.
// The function runs until the chStop channel signals to stop.
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
			// If no signatures to confirm, we can break loop as there's nothing to process.
			if txm.InflightTxs() == 0 {
				break
			}

			client, err := txm.client.Get()
			if err != nil {
				txm.lggr.Errorw("failed to get client in txm.confirm", "error", err)
				break
			}
			txm.processConfirmations(ctx, client)
			if txm.cfg.TxExpirationRebroadcast() {
				txm.rebroadcastExpiredTxs(ctx, client)
			}
		}
		tick = time.After(utils.WithJitter(txm.cfg.ConfirmPollPeriod()))
	}
}

// processConfirmations checks the on-chain status of transaction signatures and updates their in-memory state accordingly.
// The function splits the signatures into batches, retrieves their statuses using RPC calls, and processes each status.
// It handles various scenarios including expirations, errors, and state transitions (broadcasted, processed, confirmed, finalized).
// Additionally, it detects and manages re-orgs by removing or rebroadcasting transactions as necessary and determines when to end polling cancelling retry loops.
func (txm *Txm) processConfirmations(ctx context.Context, client client.ReaderWriter) {
	sigsBatch, err := utils.BatchSplit(txm.txs.ListAllSigs(), MaxSigsToConfirm)
	if err != nil { // this should never happen
		txm.lggr.Fatalw("failed to batch signatures", "error", err)
		return
	}

	var wg sync.WaitGroup
	for i := 0; i < len(sigsBatch); i++ {
		statuses, err := client.SignatureStatuses(ctx, sigsBatch[i])
		if err != nil {
			txm.lggr.Errorw("failed to get signature statuses in txm.confirm", "error", err)
			break
		}

		wg.Add(1)
		// nonblocking: process batches as soon as they come in
		go func(index int) {
			defer wg.Done()

			// to process successful first
			sortedSigs, sortedRes, err := txmutils.SortSignaturesAndResults(sigsBatch[i], statuses)
			if err != nil {
				txm.lggr.Errorw("sorting error", "error", err)
				return
			}

			for j := 0; j < len(sortedRes); j++ {
				sig, status := sortedSigs[j], sortedRes[j]
				if status == nil {
					// sig not found could mean invalid tx or not picked up yet, keep polling
					// we also need to check if a re-org has occurred for this sig and handle it
					txm.handleReorg(ctx, client, sig, status)
					txm.handleNotFoundSignatureStatus(sig)
					continue
				}

				// if signature has an error, end polling unless blockhash not found and expiration rebroadcast is enabled
				if status.Err != nil {
					txm.handleErrorSignatureStatus(sig, status)
					continue
				}

				switch status.ConfirmationStatus {
				case rpc.ConfirmationStatusProcessed:
					// if signature is processed, keep polling for confirmed or finalized status
					// we also need to check if a re-org has occurred for this sig and handle it
					txm.handleReorg(ctx, client, sig, status)
					txm.handleProcessedSignatureStatus(sig)
				case rpc.ConfirmationStatusConfirmed:
					// if signature is confirmed, keep polling for finalized status
					txm.handleConfirmedSignatureStatus(sig)
				case rpc.ConfirmationStatusFinalized:
					// if signature is finalized, end polling
					txm.handleFinalizedSignatureStatus(sig)
				default:
					txm.lggr.Warnw("unknown confirmation status", "signature", sig, "status", status.ConfirmationStatus)
				}
			}
		}(i)
	}
	wg.Wait() // wait for processing to finish
}

// handleNotFoundSignatureStatus handles the case where a transaction signature is not found on-chain.
// If the confirmation timeout has been exceeded it marks the transaction as errored.
func (txm *Txm) handleNotFoundSignatureStatus(sig solanaGo.Signature) {
	txm.lggr.Debugw("tx state: not found", "signature", sig)
	if txm.cfg.TxConfirmTimeout() != 0*time.Second && txm.txs.Expired(sig, txm.cfg.TxConfirmTimeout()) {
		id, err := txm.txs.OnError(sig, txm.cfg.TxRetentionTimeout(), txmutils.Errored, TxFailDrop)
		if err != nil {
			txm.lggr.Infow("failed to mark transaction as errored", "id", id, "signature", sig, "timeoutSeconds", txm.cfg.TxConfirmTimeout(), "error", err)
		} else {
			txm.lggr.Debugw("failed to find transaction within confirm timeout", "id", id, "signature", sig, "timeoutSeconds", txm.cfg.TxConfirmTimeout())
		}
	}
}

// handleErrorSignatureStatus handles the case where a transaction signature has an error on-chain.
// If the error is BlockhashNotFound and expiration rebroadcast is enabled, it skips error handling to allow rebroadcasting.
// Otherwise, it marks the transaction as errored.
func (txm *Txm) handleErrorSignatureStatus(sig solanaGo.Signature, status *rpc.SignatureStatusesResult) {
	// We want to rebroadcast rather than drop tx if expiration rebroadcast is enabled when blockhash was not found.
	// converting error to string so we are able to check if it contains the error message.
	if status.Err != nil && strings.Contains(fmt.Sprintf("%v", status.Err), "BlockhashNotFound") && txm.cfg.TxExpirationRebroadcast() {
		return
	}

	// Process error to determine the corresponding state and type.
	// Skip marking as errored if error considered to not be a failure.
	if txState, errType := txm.ProcessError(sig, status.Err, false); errType != NoFailure {
		id, err := txm.txs.OnError(sig, txm.cfg.TxRetentionTimeout(), txState, errType)
		if err != nil {
			txm.lggr.Infow(fmt.Sprintf("failed to mark transaction as %s", txState.String()), "id", id, "signature", sig, "error", err)
		} else {
			txm.lggr.Debugw(fmt.Sprintf("marking transaction as %s", txState.String()), "id", id, "signature", sig, "error", status.Err, "status", status.ConfirmationStatus)
		}
	}
}

// handleReorg detects and manages state regressions (re-orgs) for a given signature.
//
// A re-org occurs when the on-chain state of a signature regresses as follows:
// - Confirmed -> Processed || Not Found
// - Processed -> Not Found
//
// When a signature re-org is detected, the following steps are taken:
// - Remove the prior transaction, along with all associated signatures, and cancel the prior context.
// - Rebroadcast the prior transaction with a new blockhash and an updated compute unit price.
func (txm *Txm) handleReorg(ctx context.Context, client client.ReaderWriter, sig solanaGo.Signature, status *rpc.SignatureStatusesResult) {
	// Determine if a re-org has occurred
	sigState := txmutils.ConvertStatus(status)
	txID, hasReorg := txm.txs.IsTxReorged(sig, sigState)
	if !hasReorg {
		return
	}

	// At this point, we have detected a re-org. We need to rebroadcast the tx.
	txm.lggr.Debugw("re-org detected for transaction", "txID", txID, "signature", sig)
	pTx, err := txm.getPendingTx(txID)
	if err != nil {
		txm.lggr.Errorw("failed to get pending tx for rebroadcast", "txID", txID, "error", err)
		return
	}

	// The previous blockhash is invalid. We need to request a new one and rebroadcast the tx with it.
	blockhash, err := client.LatestBlockhash(ctx)
	if err != nil {
		txm.lggr.Errorw("failed to getLatestBlockhash for rebroadcast", "error", err)
		return
	}
	if blockhash == nil || blockhash.Value == nil {
		txm.lggr.Errorw("nil pointer returned from getLatestBlockhash for rebroadcast")
		return
	}

	// Rebroadcasts tx with new blockhash after removing prior tx and signatures associated with it, cancelling prior ctx and updating compute unit price.
	newSig, err := txm.rebroadcastWithGivenBlockhash(ctx, pTx, blockhash.Value.Blockhash, blockhash.Value.LastValidBlockHeight)
	if err != nil {
		return // logging handled inside the func
	}

	txm.lggr.Debugw("re-orged tx was rebroadcasted successfully", "id", pTx.id, "newSig", newSig)
}

// handleProcessedSignatureStatus handles the case where a transaction signature is in the "processed" state on-chain.
// It updates the transaction state in the local memory and checks if the confirmation timeout has been exceeded.
// If the timeout is exceeded, it marks the transaction as errored.
func (txm *Txm) handleProcessedSignatureStatus(sig solanaGo.Signature) {
	// update transaction state in local memory
	id, err := txm.txs.OnProcessed(sig)
	if err != nil && !errors.Is(err, ErrAlreadyInExpectedState) {
		txm.lggr.Errorw("failed to mark transaction as processed", "signature", sig, "error", err)
	} else if err == nil {
		txm.lggr.Debugw("marking transaction as processed", "id", id, "signature", sig)
	}
	// check confirm timeout exceeded if TxConfirmTimeout set
	if txm.cfg.TxConfirmTimeout() != 0*time.Second && txm.txs.Expired(sig, txm.cfg.TxConfirmTimeout()) {
		id, err := txm.txs.OnError(sig, txm.cfg.TxRetentionTimeout(), txmutils.Errored, TxFailDrop)
		if err != nil {
			txm.lggr.Infow("failed to mark transaction as errored", "id", id, "signature", sig, "timeoutSeconds", txm.cfg.TxConfirmTimeout(), "error", err)
		} else {
			txm.lggr.Debugw("tx failed to move beyond 'processed' within confirm timeout", "id", id, "signature", sig, "timeoutSeconds", txm.cfg.TxConfirmTimeout())
		}
	}
}

// handleConfirmedSignatureStatus handles the case where a transaction signature is in the "confirmed" state on-chain.
// It updates the transaction state in the local memory.
func (txm *Txm) handleConfirmedSignatureStatus(sig solanaGo.Signature) {
	id, err := txm.txs.OnConfirmed(sig)
	if err != nil && !errors.Is(err, ErrAlreadyInExpectedState) {
		txm.lggr.Errorw("failed to mark transaction as confirmed", "id", id, "signature", sig, "error", err)
	} else if err == nil {
		txm.lggr.Debugw("marking transaction as confirmed", "id", id, "signature", sig)
	}
}

// handleFinalizedSignatureStatus handles the case where a transaction signature is in the "finalized" state on-chain.
// It updates the transaction state in the local memory.
func (txm *Txm) handleFinalizedSignatureStatus(sig solanaGo.Signature) {
	id, err := txm.txs.OnFinalized(sig, txm.cfg.TxRetentionTimeout())
	if err != nil {
		txm.lggr.Errorw("failed to mark transaction as finalized", "id", id, "signature", sig, "error", err)
	} else {
		txm.lggr.Debugw("marking transaction as finalized", "id", id, "signature", sig)
	}
}

// rebroadcastExpiredTxs attempts to rebroadcast all transactions that are in broadcasted state and have expired.
// An expired tx is one where it's blockhash lastValidBlockHeight (last valid block number) is smaller than the current block height (block number).
// If any error occurs during rebroadcast attempt, they are discarded, and the function continues with the next transaction.
func (txm *Txm) rebroadcastExpiredTxs(ctx context.Context, client client.ReaderWriter) {
	blockHeight, err := client.GetLatestBlockHeight(ctx)
	if err != nil || blockHeight == 0 {
		txm.lggr.Errorw("failed to get current block height", "error", err)
		return
	}

	// Get all expired broadcasted transactions at current block number. Safe to quit if no txes are found.
	expiredBroadcastedTxes := txm.txs.ListAllExpiredBroadcastedTxs(blockHeight)
	if len(expiredBroadcastedTxes) == 0 {
		return
	}

	blockhash, err := client.LatestBlockhash(ctx)
	if err != nil {
		txm.lggr.Errorw("failed to getLatestBlockhash for rebroadcast", "error", err)
		return
	}
	if blockhash == nil || blockhash.Value == nil {
		txm.lggr.Errorw("nil pointer returned from getLatestBlockhash for rebroadcast")
		return
	}

	// rebroadcast each expired tx
	for _, expiredTx := range expiredBroadcastedTxes {
		txm.lggr.Debugw("transaction expired, rebroadcasting", "id", expiredTx.id, "signature", expiredTx.signatures, "lastValidBlockHeight", expiredTx.lastValidBlockHeight, "currentBlockHeight", blockHeight)
		newSig, err := txm.rebroadcastWithGivenBlockhash(ctx, expiredTx, blockhash.Value.Blockhash, blockhash.Value.LastValidBlockHeight)
		if err != nil {
			continue // logging handled inside the func
		}

		txm.lggr.Debugw("expired tx was rebroadcasted successfully", "id", expiredTx.id, "newSig", newSig)
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
			if len(msg.signatures) == 0 {
				continue
			}
			// Process error to determine the corresponding state and type.
			// Certain errors can be considered not to be failures during simulation to allow the process to continue
			if txState, errType := txm.ProcessError(msg.signatures[0], res.Err, true); errType != NoFailure {
				id, err := txm.txs.OnError(msg.signatures[0], txm.cfg.TxRetentionTimeout(), txState, errType)
				if err != nil {
					txm.lggr.Errorw(fmt.Sprintf("failed to mark transaction as %s", txState.String()), "id", id, "err", err)
				} else {
					txm.lggr.Debugw(fmt.Sprintf("marking transaction as %s", txState.String()), "id", id, "signature", msg.signatures[0], "error", res.Err)
				}
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
			reapCount := txm.txs.TrimFinalizedErroredTxs()
			if reapCount > 0 {
				txm.lggr.Debugf("Reaped %d finalized or errored transactions", reapCount)
			}
		}
		tick = time.After(utils.WithJitter(TxReapInterval))
	}
}

func (txm *Txm) FeeEstimator() fees.Estimator {
	return txm.fee
}

// Enqueue enqueues a msg destined for the solana chain.
func (txm *Txm) Enqueue(ctx context.Context, accountID string, tx *solanaGo.Transaction, txID *string, txLastValidBlockHeight uint64, txCfgs ...txmutils.SetTxConfig) error {
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

	// Use transaction ID provided by caller if set
	id := uuid.New().String()
	if txID != nil && *txID != "" {
		id = *txID
	}

	// Perform compute unit limit estimation after storing transaction
	// If error found during simulation, transaction should be in storage to mark accordingly
	if cfg.EstimateComputeUnitLimit {
		computeUnitLimit, err := txm.EstimateComputeUnitLimit(ctx, tx, id)
		if err != nil {
			return fmt.Errorf("transaction failed simulation: %w", err)
		}
		// If estimation returns 0 compute unit limit without error, fallback to original config
		if computeUnitLimit != 0 {
			cfg.ComputeUnitLimit = computeUnitLimit
		}
	}

	msg := pendingTx{
		id:                   id,
		tx:                   *tx,
		cfg:                  cfg,
		lastValidBlockHeight: txLastValidBlockHeight,
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
	case txmutils.Broadcasted:
		return commontypes.Pending, nil
	case txmutils.Processed, txmutils.Confirmed:
		return commontypes.Unconfirmed, nil
	case txmutils.Finalized:
		return commontypes.Finalized, nil
	case txmutils.Errored:
		return commontypes.Failed, nil
	case txmutils.FatallyErrored:
		return commontypes.Fatal, nil
	default:
		return commontypes.Unknown, fmt.Errorf("found unknown transaction state: %s", state.String())
	}
}

// EstimateComputeUnitLimit estimates the compute unit limit needed for a transaction.
// It simulates the provided transaction to determine the used compute and applies a buffer to it.
func (txm *Txm) EstimateComputeUnitLimit(ctx context.Context, tx *solanaGo.Transaction, id string) (uint32, error) {
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
		// Process error to determine the corresponding state and type.
		// Certain errors can be considered not to be failures during simulation to allow the process to continue
		if txState, errType := txm.ProcessError(sig, res.Err, true); errType != NoFailure {
			err := txm.txs.OnPrebroadcastError(id, txm.cfg.TxRetentionTimeout(), txState, errType)
			if err != nil {
				return 0, fmt.Errorf("failed to process error %v for tx ID %s: %w", res.Err, id, err)
			}
		}
		return 0, fmt.Errorf("simulated tx returned error: %v", res.Err)
	}

	if res.UnitsConsumed == nil || *res.UnitsConsumed == 0 {
		txm.lggr.Debug("failed to get units consumed for tx")
		// Do not return error to allow falling back to default compute unit limit
		return 0, nil
	}

	unitsConsumed := *res.UnitsConsumed
	// Add buffer to the used compute estimate
	computeUnitLimit := bigmath.AddPercentage(new(big.Int).SetUint64(unitsConsumed), EstimateComputeUnitLimitBuffer).Uint64()
	// Ensure computeUnitLimit does not exceed the max compute unit limit for a transaction after adding buffer
	computeUnitLimit = mathutil.Min(computeUnitLimit, MaxComputeUnitLimit)

	return uint32(computeUnitLimit), nil //nolint // computeUnitLimit can only be a maximum of 1.4M
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

// ProcessError parses and handles relevant errors found in simulation results
func (txm *Txm) ProcessError(sig solanaGo.Signature, resErr interface{}, simulation bool) (txState txmutils.TxState, errType TxErrType) {
	if resErr != nil {
		// handle various errors
		// https://github.com/solana-labs/solana/blob/master/sdk/src/transaction/error.rs
		errStr := fmt.Sprintf("%v", resErr) // convert to string to handle various interfaces
		txm.lggr.Info(errStr)
		logValues := []interface{}{
			"signature", sig,
			"error", resErr,
		}
		// return TxFailRevert on any error if when processing error during confirmation
		errType := TxFailRevert
		// return TxFailSimRevert on any known error when processing simulation error
		if simulation {
			errType = TxFailSimRevert
		}
		switch {
		// blockhash not found when simulating, occurs when network bank has not seen the given blockhash or tx is too old
		// let confirmation process clean up
		case strings.Contains(errStr, "BlockhashNotFound"):
			txm.lggr.Debugw("BlockhashNotFound", logValues...)
			// return no failure for this error when simulating to allow later send/retry code to assign a proper blockhash
			// in case the one provided by the caller is outdated
			if simulation {
				return txState, NoFailure
			}
			return txmutils.Errored, errType
		// transaction is already processed in the chain
		case strings.Contains(errStr, "AlreadyProcessed"):
			txm.lggr.Debugw("AlreadyProcessed", logValues...)
			// return no failure for this error when simulating in case there is a race between broadcast and simulation
			// when doing both in parallel
			if simulation {
				return txState, NoFailure
			}
			return txmutils.Errored, errType
		// transaction will encounter execution error/revert
		case strings.Contains(errStr, "InstructionError"):
			txm.lggr.Debugw("InstructionError", logValues...)
			return txmutils.FatallyErrored, errType
		// transaction contains an invalid account reference
		case strings.Contains(errStr, "InvalidAccountIndex"):
			txm.lggr.Debugw("InvalidAccountIndex", logValues...)
			return txmutils.FatallyErrored, errType
		// transaction loads a writable account that cannot be written
		case strings.Contains(errStr, "InvalidWritableAccount"):
			txm.lggr.Debugw("InvalidWritableAccount", logValues...)
			return txmutils.FatallyErrored, errType
		// address lookup table not found
		case strings.Contains(errStr, "AddressLookupTableNotFound"):
			txm.lggr.Debugw("AddressLookupTableNotFound", logValues...)
			return txmutils.FatallyErrored, errType
		// attempted to lookup addresses from an invalid account
		case strings.Contains(errStr, "InvalidAddressLookupTableData"):
			txm.lggr.Debugw("InvalidAddressLookupTableData", logValues...)
			return txmutils.FatallyErrored, errType
		// address table lookup uses an invalid index
		case strings.Contains(errStr, "InvalidAddressLookupTableIndex"):
			txm.lggr.Debugw("InvalidAddressLookupTableIndex", logValues...)
			return txmutils.FatallyErrored, errType
		// attempt to debit an account but found no record of a prior credit.
		case strings.Contains(errStr, "AccountNotFound"):
			txm.lggr.Debugw("AccountNotFound", logValues...)
			return txmutils.FatallyErrored, errType
		// attempt to load a program that does not exist
		case strings.Contains(errStr, "ProgramAccountNotFound"):
			txm.lggr.Debugw("ProgramAccountNotFound", logValues...)
			return txmutils.FatallyErrored, errType
		// unrecognized errors (indicates more concerning failures)
		default:
			// if simulating, return TxFailSimOther if error unknown
			if simulation {
				errType = TxFailSimOther
			}
			txm.lggr.Errorw("unrecognized error", logValues...)
			return txmutils.Errored, errType
		}
	}
	return
}

// InflightTxs returns the number of signatures being tracked for all transactions not yet finalized or errored
func (txm *Txm) InflightTxs() int {
	return len(txm.txs.ListAllSigs())
}

// rebroadcastWithGivenBlockhash attempts to rebroadcast a pending tx with a new blockhash.
// Removes all signatures associated with the prior tx, cancels prior ctx, updates compute unit price and sets given blockhash for rebroadcasting.
// Calls sendWithRetry directly to avoid enqueuing the transaction. It logs the error when rebroadcast fails and returns the new signature when successful.
func (txm *Txm) rebroadcastWithGivenBlockhash(ctx context.Context, pTx pendingTx, blockhash solana.Hash, lastValidBlockHeight uint64) (solana.Signature, error) {
	// Remove the previous tx from state
	_, err := txm.txs.Remove(pTx.id)
	if err != nil {
		txm.lggr.Errorw("failed to remove tx", "id", pTx.id, "error", err)
		return solana.Signature{}, err
	}

	// Set new blockhash, lastValidBlockHeight and update compute unit price for rebroadcast
	pTx.tx.Message.RecentBlockhash = blockhash
	pTx.cfg.BaseComputeUnitPrice = txm.fee.BaseComputeUnitPrice()
	pTx.lastValidBlockHeight = lastValidBlockHeight

	// call sendWithRetry directly to avoid enqueuing
	_, _, newSig, sendErr := txm.sendWithRetry(ctx, pTx)
	if sendErr != nil {
		stateTransitionErr := txm.txs.OnPrebroadcastError(pTx.id, txm.cfg.TxRetentionTimeout(), txmutils.Errored, TxFailReject)
		combinedErr := errors.Join(sendErr, stateTransitionErr)
		txm.lggr.Errorw("failed to rebroadcast tx with new blockhash", "id", pTx.id, "error", combinedErr)
		return solana.Signature{}, combinedErr
	}

	return newSig, nil
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

func (txm *Txm) defaultTxConfig() txmutils.TxConfig {
	return txmutils.TxConfig{
		Timeout:                  txm.cfg.TxRetryTimeout(),
		FeeBumpPeriod:            txm.cfg.FeeBumpPeriod(),
		BaseComputeUnitPrice:     txm.fee.BaseComputeUnitPrice(),
		ComputeUnitPriceMin:      txm.cfg.ComputeUnitPriceMin(),
		ComputeUnitPriceMax:      txm.cfg.ComputeUnitPriceMax(),
		ComputeUnitLimit:         txm.cfg.ComputeUnitLimitDefault(),
		EstimateComputeUnitLimit: txm.cfg.EstimateComputeUnitLimit(),
	}
}

func (txm *Txm) getPendingTx(txID string) (pendingTx, error) {
	return txm.txs.GetPendingTx(txID)
}

package txm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"

	solanaGo "github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

const (
	// DefaultInstructionComputeUnitLimit mirrors the runtime default compute unit limit applied per instruction
	// when a legacy transaction does not request one. V1 transactions default to 0 compute units instead.
	DefaultInstructionComputeUnitLimit = 200_000
	microLamportsPerLamport            = 1_000_000
)

// EnqueueV1 enqueues a msg destined for the solana chain encoded as a V1 (SIMD-0385) transaction.
// It behaves like Enqueue, except the compute unit limit and price are set through the message's inline
// TransactionConfig instead of ComputeBudget instructions. The tx must not use address lookup tables or
// contain ComputeBudget instructions. The caller's tx is not modified.
func (txm *Txm) EnqueueV1(ctx context.Context, accountID string, tx *solanaGo.Transaction, txID *string, txLastValidBlockHeight uint64, txCfgs ...txmutils.SetTxConfig) error {
	if err := txm.Ready(); err != nil {
		return fmt.Errorf("error in soltxm.EnqueueV1: %w", err)
	}

	if tx == nil {
		return errors.New("error in soltxm.EnqueueV1: tx is nil pointer")
	}

	v1Tx, err := toV1Tx(*tx)
	if err != nil {
		return fmt.Errorf("error in soltxm.EnqueueV1: %w", err)
	}

	return txm.enqueue(ctx, accountID, v1Tx, txID, txLastValidBlockHeight, txCfgs...)
}

// toV1Tx returns a copy of tx with its message set to the V1 format.
func toV1Tx(tx solanaGo.Transaction) (solanaGo.Transaction, error) {
	v1Tx := utils.DeepCopyTx(tx)
	for i, ix := range v1Tx.Message.Instructions {
		if int(ix.ProgramIDIndex) < len(v1Tx.Message.AccountKeys) && v1Tx.Message.AccountKeys[ix.ProgramIDIndex].Equals(fees.ComputeBudgetProgram) {
			return solanaGo.Transaction{}, fmt.Errorf("instruction %d: ComputeBudget instructions are not supported in v1 transactions", i)
		}
	}
	if _, err := v1Tx.Message.SetVersion(solanaGo.MessageVersionV1); err != nil {
		return solanaGo.Transaction{}, err
	}
	if err := v1Tx.Message.Sanitize(); err != nil {
		return solanaGo.Transaction{}, fmt.Errorf("invalid v1 message: %w", err)
	}
	return v1Tx, nil
}

func isV1Tx(tx *solanaGo.Transaction) bool {
	return tx.Message.GetVersion() == solanaGo.MessageVersionV1
}

// setV1ComputeBudget sets the compute unit limit and the priority fee derived from the compute unit price on a V1 tx.
// A computeUnitLimit of 0 falls back to the legacy runtime default since V1 treats an unset limit as 0 compute units.
func setV1ComputeBudget(tx *solanaGo.Transaction, computeUnitLimit uint32, computeUnitPrice fees.ComputeUnitPrice) {
	if computeUnitLimit == 0 {
		computeUnitLimit = defaultV1ComputeUnitLimit(tx)
	}
	tx.Message.TransactionConfig = tx.Message.TransactionConfig.
		WithComputeUnitLimit(computeUnitLimit).
		WithPriorityFee(v1PriorityFee(computeUnitPrice, computeUnitLimit)).WithLoadedAccountsDataSizeLimit(64 * 1024 * 1024)
}

func defaultV1ComputeUnitLimit(tx *solanaGo.Transaction) uint32 {
	limit := uint64(DefaultInstructionComputeUnitLimit) * uint64(len(tx.Message.Instructions))
	return uint32(min(limit, MaxComputeUnitLimit)) //nolint:gosec // capped at MaxComputeUnitLimit
}

// v1PriorityFee converts a compute unit price in micro-lamports into the total priority fee in lamports,
// rounding up the same way the runtime does for legacy compute unit prices.
func v1PriorityFee(computeUnitPrice fees.ComputeUnitPrice, computeUnitLimit uint32) uint64 {
	fee := new(big.Int).Mul(new(big.Int).SetUint64(uint64(computeUnitPrice)), new(big.Int).SetUint64(uint64(computeUnitLimit)))
	fee.Add(fee, big.NewInt(microLamportsPerLamport-1))
	fee.Quo(fee, big.NewInt(microLamportsPerLamport))
	if !fee.IsUint64() {
		return math.MaxUint64
	}
	return fee.Uint64()
}

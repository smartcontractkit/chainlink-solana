package txm

import (
	"fmt"
	"math"
	"math/big"

	solanaGo "github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

const (
	// DefaultInstructionComputeUnitLimit mirrors the runtime default compute unit limit applied per instruction
	// when a legacy transaction does not request one. V1 transactions default to 0 compute units instead.
	DefaultInstructionComputeUnitLimit = 200_000
	microLamportsPerLamport            = 1_000_000
	// MaxLoadedAccountsDataSizeLimit is the runtime cap. V1 treats an unset limit as 0, which
	// fails with MaxLoadedAccountsDataSizeExceeded, so TXM defaults to this for v1 txs.
	MaxLoadedAccountsDataSizeLimit = 64 * 1024 * 1024
)

func isV1Tx(tx *solanaGo.Transaction) bool {
	return tx.Message.GetVersion() == solanaGo.MessageVersionV1
}

// validateV1Tx rejects ComputeBudget instructions and address lookup tables.
// Enqueue uses this so a v1-encoded tx can share the existing enqueue path
// without a separate EnqueueV1 entrypoint.
func validateV1Tx(tx solanaGo.Transaction) error {
	if len(tx.Message.AddressTableLookups) > 0 {
		return fmt.Errorf("address table lookups are not supported in v1 transactions")
	}
	for i, ix := range tx.Message.Instructions {
		if int(ix.ProgramIDIndex) < len(tx.Message.AccountKeys) && tx.Message.AccountKeys[ix.ProgramIDIndex].Equals(fees.ComputeBudgetProgram) {
			return fmt.Errorf("instruction %d: ComputeBudget instructions are not supported in v1 transactions", i)
		}
	}
	if err := tx.Message.Sanitize(); err != nil {
		return fmt.Errorf("invalid v1 message: %w", err)
	}
	return nil
}

// toV1Tx returns a copy of tx with its message set to the V1 format.
func toV1Tx(tx solanaGo.Transaction) (solanaGo.Transaction, error) {
	v1Tx := utils.DeepCopyTx(tx)
	if _, err := v1Tx.Message.SetVersion(solanaGo.MessageVersionV1); err != nil {
		return solanaGo.Transaction{}, err
	}
	if err := validateV1Tx(v1Tx); err != nil {
		return solanaGo.Transaction{}, err
	}
	return v1Tx, nil
}

// applyComputeBudget writes the compute unit limit and fee onto tx.
// Legacy/v0 txs get ComputeBudget instructions; v1 txs get inline TransactionConfig.
func applyComputeBudget(tx *solanaGo.Transaction, computeUnitLimit uint32, computeUnitPrice fees.ComputeUnitPrice) error {
	if isV1Tx(tx) {
		setV1ComputeBudget(tx, computeUnitLimit, computeUnitPrice)
		return nil
	}
	if computeUnitLimit != 0 {
		if err := fees.SetComputeUnitLimit(tx, fees.ComputeUnitLimit(computeUnitLimit)); err != nil {
			return fmt.Errorf("failed to add compute unit limit instruction: %w", err)
		}
	}
	if err := fees.SetComputeUnitPrice(tx, computeUnitPrice); err != nil {
		return err
	}
	return nil
}

// prepareTxForSimulation sets the runtime-max compute budget so simulation is not
// capped by the legacy 200k default (v1: unset CU limit and data-size limit are both 0).
func prepareTxForSimulation(tx *solanaGo.Transaction) error {
	if isV1Tx(tx) {
		cfg := tx.Message.TransactionConfig.WithComputeUnitLimit(MaxComputeUnitLimit)
		if cfg.LoadedAccountsDataSizeLimit == nil {
			cfg = cfg.WithLoadedAccountsDataSizeLimit(MaxLoadedAccountsDataSizeLimit)
		}
		tx.Message.TransactionConfig = cfg
		return nil
	}
	if err := fees.SetComputeUnitLimit(tx, fees.ComputeUnitLimit(MaxComputeUnitLimit)); err != nil {
		return fmt.Errorf("failed to set compute unit limit when simulating tx: %w", err)
	}
	return nil
}

// setV1ComputeBudget sets the compute unit limit and the priority fee derived from the compute unit price on a V1 tx.
// A computeUnitLimit of 0 falls back to the legacy runtime default since V1 treats an unset limit as 0 compute units.
func setV1ComputeBudget(tx *solanaGo.Transaction, computeUnitLimit uint32, computeUnitPrice fees.ComputeUnitPrice) {
	if computeUnitLimit == 0 {
		computeUnitLimit = defaultV1ComputeUnitLimit(tx)
	}
	cfg := tx.Message.TransactionConfig.
		WithComputeUnitLimit(computeUnitLimit).
		WithPriorityFee(v1PriorityFee(computeUnitPrice, computeUnitLimit))
	if cfg.LoadedAccountsDataSizeLimit == nil {
		cfg = cfg.WithLoadedAccountsDataSizeLimit(MaxLoadedAccountsDataSizeLimit)
	}
	tx.Message.TransactionConfig = cfg
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

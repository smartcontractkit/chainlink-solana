package fees

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// returns new fee based on number of times bumped
func CalculateFee(base, maxFee, minFee uint64, count uint) uint64 {
	amount := base

	for i := uint(0); i < count; i++ {
		if base == 0 && i == 0 {
			amount = 1
		} else {
			next := amount + amount
			if next <= amount {
				// overflowed
				amount = maxFee
				break
			}
			amount = next
		}
	}

	// respect bounds
	if amount < minFee {
		return minFee
	}
	if amount > maxFee {
		return maxFee
	}
	return amount
}

type BlockData struct {
	Fees   []uint64           // total fee
	Prices []ComputeUnitPrice // price per unit
}

// ParseBlock parses the fee calculations from all the transactions within a block
func ParseBlock(res *rpc.GetBlockResult) (out BlockData, err error) {
	if res == nil {
		return out, fmt.Errorf("GetBlockResult was nil")
	}

	for _, tx := range res.Transactions {
		if tx.Meta == nil {
			continue
		}
		baseTx, getTxErr := tx.GetTransaction()
		if getTxErr != nil {
			// exit on GetTransaction error
			// if this occurs, solana-go was unable to parse a transaction
			// further investigation is required to determine if there is incompatibility
			return out, fmt.Errorf("failed to GetTransaction (blockhash: %s): %w", res.Blockhash, getTxErr)
		}
		if baseTx == nil {
			continue
		}

		if isConsensusVoteTX(baseTx) {
			continue
		}

		var price ComputeUnitPrice
		switch baseTx.Message.GetVersion() {
		case solana.MessageVersionLegacy, solana.MessageVersionV0:
			//handle here
			price = parsePriceFromTransactionV0(baseTx)
		case solana.MessageVersionV1:
			price = parsePriceFromTransactionV1(baseTx)
		default:
			return out, fmt.Errorf("unknown message version %d", baseTx.Message.GetVersion())
		}

		out.Prices = append(out.Prices, price)
		out.Fees = append(out.Fees, tx.Meta.Fee)
	}
	return out, nil
}

func isConsensusVoteTX(baseTx *solana.Transaction) bool {
	// filter out consensus vote transactions
	// consensus messages are included as txs within blocks
	// validate AccountKeys has enough elements to index into ProgramIDIndex
	if len(baseTx.Message.Instructions) == 1 &&
		len(baseTx.Message.AccountKeys) > int(baseTx.Message.Instructions[0].ProgramIDIndex) &&
		baseTx.Message.AccountKeys[baseTx.Message.Instructions[0].ProgramIDIndex] == solana.VoteProgramID {
		return true
	}

	return false
}

func parsePriceFromTransactionV0(baseTx *solana.Transaction) ComputeUnitPrice {
	for _, instruction := range baseTx.Message.Instructions {
		// find instructions for compute budget program
		// validate AccountKeys has enough elements to index into ProgramIDIndex
		if len(baseTx.Message.AccountKeys) > int(instruction.ProgramIDIndex) &&
			baseTx.Message.AccountKeys[instruction.ProgramIDIndex] == ComputeBudgetProgram {
			parsed, parseErr := ParseComputeUnitPrice(instruction.Data)
			// if compute unit price found, break instruction loop
			// only one compute unit price tx is allowed
			// err returned if not SetComputeUnitPrice instruction
			if parseErr == nil {
				return parsed
			}
		}
	}

	return ComputeUnitPrice(0)
}
func parsePriceFromTransactionV1(baseTx *solana.Transaction) ComputeUnitPrice {
	computeUnitLimit := baseTx.Message.TransactionConfig.ComputeUnitLimit
	priorityFee := baseTx.Message.TransactionConfig.PriorityFee
	if computeUnitLimit == nil || *computeUnitLimit == 0 || priorityFee == nil {
		return ComputeUnitPrice(0)
	}

	return ComputeUnitPrice(*priorityFee * 1_000_000 / uint64(*computeUnitLimit))
}

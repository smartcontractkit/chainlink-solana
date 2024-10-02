package solana

import (
	"github.com/gagliardetto/solana-go"
	mn "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode"
	"strings"
)

// ClassifySendError returns the corresponding SendTxReturnCode based on the error.
// Errors derived from anza-xyz/agave@master/sdk/src/transaction/error.rs
func ClassifySendError(tx *solana.Transaction, err error) mn.SendTxReturnCode {
	if err == nil {
		return mn.Successful
	}

	errMsg := err.Error()

	// TODO: Ensure correct error classification for each error message.
	// TODO: is strings.Contains good enough for error classification?
	switch {
	case strings.Contains(errMsg, "Account in use"):
		return mn.TransactionAlreadyKnown
	case strings.Contains(errMsg, "Account loaded twice"):
		return mn.Retryable
	case strings.Contains(errMsg, "Attempt to debit an account but found no record of a prior credit"):
		return mn.Retryable
	case strings.Contains(errMsg, "Attempt to load a program that does not exist"):
		return mn.Fatal
	case strings.Contains(errMsg, "Insufficient funds for fee"):
		return mn.InsufficientFunds
	case strings.Contains(errMsg, "This account may not be used to pay transaction fees"):
		return mn.Unsupported
	case strings.Contains(errMsg, "This transaction has already been processed"):
		return mn.TransactionAlreadyKnown
	case strings.Contains(errMsg, "Blockhash not found"):
		return mn.Retryable
	case strings.Contains(errMsg, "Error processing Instruction"):
		return mn.Retryable
	case strings.Contains(errMsg, "Loader call chain is too deep"):
		return mn.Retryable
	case strings.Contains(errMsg, "Transaction requires a fee but has no signature present"):
		return mn.Retryable
	case strings.Contains(errMsg, "Transaction contains an invalid account reference"):
		return mn.Unsupported
	case strings.Contains(errMsg, "Transaction did not pass signature verification"):
		return mn.Fatal
	case strings.Contains(errMsg, "This program may not be used for executing instructions"):
		return mn.Unsupported
	case strings.Contains(errMsg, "Transaction failed to sanitize accounts offsets correctly"):
		return mn.Fatal
	case strings.Contains(errMsg, "Transactions are currently disabled due to cluster maintenance"):
		return mn.Retryable
	case strings.Contains(errMsg, "Transaction processing left an account with an outstanding borrowed reference"):
		return mn.Fatal
	case strings.Contains(errMsg, "Transaction would exceed max Block Cost Limit"):
		return mn.ExceedsMaxFee
	case strings.Contains(errMsg, "Transaction version is unsupported"):
		return mn.Unsupported
	case strings.Contains(errMsg, "Transaction loads a writable account that cannot be written"):
		return mn.Unsupported
	case strings.Contains(errMsg, "Transaction would exceed max account limit within the block"):
		return mn.ExceedsMaxFee
	case strings.Contains(errMsg, "Transaction would exceed account data limit within the block"):
		return mn.ExceedsMaxFee
	case strings.Contains(errMsg, "Transaction locked too many accounts"):
		return mn.Fatal
	case strings.Contains(errMsg, "Transaction loads an address table account that doesn't exist"):
		return mn.Unsupported
	case strings.Contains(errMsg, "Transaction loads an address table account with an invalid owner"):
		return mn.Unsupported
	case strings.Contains(errMsg, "Transaction loads an address table account with invalid data"):
		return mn.Unsupported
	case strings.Contains(errMsg, "Transaction address table lookup uses an invalid index"):
		return mn.Unsupported
	case strings.Contains(errMsg, "Transaction leaves an account with a lower balance than rent-exempt minimum"):
		return mn.Fatal
	case strings.Contains(errMsg, "Transaction would exceed max Vote Cost Limit"):
		return mn.ExceedsMaxFee
	case strings.Contains(errMsg, "Transaction would exceed total account data limit"):
		return mn.ExceedsMaxFee
	case strings.Contains(errMsg, "Transaction contains a duplicate instruction"):
		return mn.Fatal
	case strings.Contains(errMsg, "Transaction results in an account with insufficient funds for rent"):
		return mn.InsufficientFunds
	case strings.Contains(errMsg, "Transaction exceeded max loaded accounts data size cap"):
		return mn.Unsupported
	case strings.Contains(errMsg, "LoadedAccountsDataSizeLimit set for transaction must be greater than 0"):
		return mn.Fatal
	case strings.Contains(errMsg, "Sanitized transaction differed before/after feature activation"):
		return mn.Fatal
	case strings.Contains(errMsg, "Execution of the program referenced by account at index is temporarily restricted"):
		return mn.Unsupported
	case strings.Contains(errMsg, "Sum of account balances before and after transaction do not match"):
		return mn.Fatal
	case strings.Contains(errMsg, "Program cache hit max limit"):
		return mn.Retryable
	default:
		return mn.Retryable
	}
}

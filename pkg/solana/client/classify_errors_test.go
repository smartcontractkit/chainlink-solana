package client

import (
	"errors"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"

	mn "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode"
)

func TestClassifySendError(t *testing.T) {
	tests := []struct {
		errMsg       string
		expectedCode mn.SendTxReturnCode
	}{
		// Static error cases
		{"Account in use", mn.Retryable},
		{"Account loaded twice", mn.Retryable},
		{"Attempt to debit an account but found no record of a prior credit.", mn.Retryable},
		{"Attempt to load a program that does not exist", mn.Fatal},
		{"Insufficient funds for fee", mn.InsufficientFunds},
		{"This account may not be used to pay transaction fees", mn.Unsupported},
		{"This transaction has already been processed", mn.TransactionAlreadyKnown},
		{"Blockhash not found", mn.Retryable},
		{"Loader call chain is too deep", mn.Retryable},
		{"Transaction requires a fee but has no signature present", mn.Retryable},
		{"Transaction contains an invalid account reference", mn.Retryable},
		{"Transaction did not pass signature verification", mn.Fatal},
		{"This program may not be used for executing instructions", mn.Retryable},
		{"Transaction failed to sanitize accounts offsets correctly", mn.Fatal},
		{"Transactions are currently disabled due to cluster maintenance", mn.Retryable},
		{"Transaction processing left an account with an outstanding borrowed reference", mn.Retryable},
		{"Transaction would exceed max Block Cost Limit", mn.ExceedsMaxFee},
		{"Transaction version is unsupported", mn.Unsupported},
		{"Transaction loads a writable account that cannot be written", mn.Retryable},
		{"Transaction would exceed max account limit within the block", mn.ExceedsMaxFee},
		{"Transaction would exceed account data limit within the block", mn.ExceedsMaxFee},
		{"Transaction locked too many accounts", mn.Retryable},
		{"Address lookup table not found", mn.Retryable},
		{"Attempted to lookup addresses from an account owned by the wrong program", mn.Retryable},
		{"Attempted to lookup addresses from an invalid account", mn.Retryable},
		{"Address table lookup uses an invalid index", mn.Retryable},
		{"Transaction leaves an account with a lower balance than rent-exempt minimum", mn.Retryable},
		{"Transaction would exceed max Vote Cost Limit", mn.Retryable},
		{"Transaction would exceed total account data limit", mn.Retryable},
		{"Transaction contains a duplicate instruction", mn.Retryable},
		{"Transaction exceeded max loaded accounts data size cap", mn.Retryable},
		{"LoadedAccountsDataSizeLimit set for transaction must be greater than 0.", mn.Retryable},
		{"Sanitized transaction differed before/after feature activation. Needs to be resanitized.", mn.Retryable},
		{"Program cache hit max limit", mn.Retryable},

		// Dynamic error cases
		{"Transaction results in an account (123) with insufficient funds for rent", mn.InsufficientFunds},
		{"Error processing Instruction 2: Some error details", mn.Retryable},
		{"Execution of the program referenced by account at index 3 is temporarily restricted.", mn.Retryable},

		// Edge cases
		{"Unknown error message", mn.Retryable},
		{"", mn.Retryable}, // Empty message
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			tx := &solana.Transaction{}  // Dummy transaction
			err := errors.New(tt.errMsg) // Create a standard Go error with the message
			result := ClassifySendError(tx, err)
			assert.Equal(t, tt.expectedCode, result, "Expected %v but got %v for error message: %s", tt.expectedCode, result, tt.errMsg)
		})
	}
}

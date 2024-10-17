package client

import (
	"regexp"

	"github.com/gagliardetto/solana-go"

	mn "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode"
)

// Solana error patters
// https://github.com/anza-xyz/agave/blob/master/sdk/src/transaction/error.rs
var (
	ErrAccountInUse                          = regexp.MustCompile(`Account in use`)
	ErrAccountLoadedTwice                    = regexp.MustCompile(`Account loaded twice`)
	ErrAccountNotFound                       = regexp.MustCompile(`Attempt to debit an account but found no record of a prior credit\.`)
	ErrProgramAccountNotFound                = regexp.MustCompile(`Attempt to load a program that does not exist`)
	ErrInsufficientFundsForFee               = regexp.MustCompile(`Insufficient funds for fee`)
	ErrInvalidAccountForFee                  = regexp.MustCompile(`This account may not be used to pay transaction fees`)
	ErrAlreadyProcessed                      = regexp.MustCompile(`This transaction has already been processed`)
	ErrBlockhashNotFound                     = regexp.MustCompile(`Blockhash not found`)
	ErrInstructionError                      = regexp.MustCompile(`Error processing Instruction \d+: .+`)
	ErrCallChainTooDeep                      = regexp.MustCompile(`Loader call chain is too deep`)
	ErrMissingSignatureForFee                = regexp.MustCompile(`Transaction requires a fee but has no signature present`)
	ErrInvalidAccountIndex                   = regexp.MustCompile(`Transaction contains an invalid account reference`)
	ErrSignatureFailure                      = regexp.MustCompile(`Transaction did not pass signature verification`)
	ErrInvalidProgramForExecution            = regexp.MustCompile(`This program may not be used for executing instructions`)
	ErrSanitizeFailure                       = regexp.MustCompile(`Transaction failed to sanitize accounts offsets correctly`)
	ErrClusterMaintenance                    = regexp.MustCompile(`Transactions are currently disabled due to cluster maintenance`)
	ErrAccountBorrowOutstanding              = regexp.MustCompile(`Transaction processing left an account with an outstanding borrowed reference`)
	ErrWouldExceedMaxBlockCostLimit          = regexp.MustCompile(`Transaction would exceed max Block Cost Limit`)
	ErrUnsupportedVersion                    = regexp.MustCompile(`Transaction version is unsupported`)
	ErrInvalidWritableAccount                = regexp.MustCompile(`Transaction loads a writable account that cannot be written`)
	ErrWouldExceedMaxAccountCostLimit        = regexp.MustCompile(`Transaction would exceed max account limit within the block`)
	ErrWouldExceedAccountDataBlockLimit      = regexp.MustCompile(`Transaction would exceed account data limit within the block`)
	ErrTooManyAccountLocks                   = regexp.MustCompile(`Transaction locked too many accounts`)
	ErrAddressLookupTableNotFound            = regexp.MustCompile(`Transaction loads an address table account that doesn't exist`)
	ErrInvalidAddressLookupTableOwner        = regexp.MustCompile(`Transaction loads an address table account with an invalid owner`)
	ErrInvalidAddressLookupTableData         = regexp.MustCompile(`Transaction loads an address table account with invalid data`)
	ErrInvalidAddressLookupTableIndex        = regexp.MustCompile(`Transaction address table lookup uses an invalid index`)
	ErrInvalidRentPayingAccount              = regexp.MustCompile(`Transaction leaves an account with a lower balance than rent-exempt minimum`)
	ErrWouldExceedMaxVoteCostLimit           = regexp.MustCompile(`Transaction would exceed max Vote Cost Limit`)
	ErrWouldExceedAccountDataTotalLimit      = regexp.MustCompile(`Transaction would exceed total account data limit`)
	ErrDuplicateInstruction                  = regexp.MustCompile(`Transaction contains a duplicate instruction \(\d+\) that is not allowed`)
	ErrInsufficientFundsForRent              = regexp.MustCompile(`Transaction results in an account \(\d+\) with insufficient funds for rent`)
	ErrMaxLoadedAccountsDataSizeExceeded     = regexp.MustCompile(`Transaction exceeded max loaded accounts data size cap`)
	ErrInvalidLoadedAccountsDataSizeLimit    = regexp.MustCompile(`LoadedAccountsDataSizeLimit set for transaction must be greater than 0\.`)
	ErrResanitizationNeeded                  = regexp.MustCompile(`Sanitized transaction differed before/after feature activation\. Needs to be resanitized\.`)
	ErrProgramExecutionTemporarilyRestricted = regexp.MustCompile(`Execution of the program referenced by account at index \d+ is temporarily restricted\.`)
	ErrUnbalancedTransaction                 = regexp.MustCompile(`Sum of account balances before and after transaction do not match`)
	ErrProgramCacheHitMaxLimit               = regexp.MustCompile(`Program cache hit max limit`)
)

// errCodes maps regex patterns to corresponding return code
var errCodes = map[*regexp.Regexp]mn.SendTxReturnCode{
	ErrAccountInUse:                          mn.Retryable,
	ErrAccountLoadedTwice:                    mn.Retryable,
	ErrAccountNotFound:                       mn.Retryable,
	ErrProgramAccountNotFound:                mn.Fatal,
	ErrInsufficientFundsForFee:               mn.InsufficientFunds,
	ErrInvalidAccountForFee:                  mn.Unsupported,
	ErrAlreadyProcessed:                      mn.TransactionAlreadyKnown,
	ErrBlockhashNotFound:                     mn.Retryable,
	ErrInstructionError:                      mn.Retryable,
	ErrCallChainTooDeep:                      mn.Retryable,
	ErrMissingSignatureForFee:                mn.Retryable,
	ErrInvalidAccountIndex:                   mn.Retryable,
	ErrSignatureFailure:                      mn.Fatal,
	ErrInvalidProgramForExecution:            mn.Retryable,
	ErrSanitizeFailure:                       mn.Fatal,
	ErrClusterMaintenance:                    mn.Retryable,
	ErrAccountBorrowOutstanding:              mn.Retryable,
	ErrWouldExceedMaxBlockCostLimit:          mn.ExceedsMaxFee,
	ErrUnsupportedVersion:                    mn.Unsupported,
	ErrInvalidWritableAccount:                mn.Retryable,
	ErrWouldExceedMaxAccountCostLimit:        mn.ExceedsMaxFee,
	ErrWouldExceedAccountDataBlockLimit:      mn.ExceedsMaxFee,
	ErrTooManyAccountLocks:                   mn.Retryable,
	ErrAddressLookupTableNotFound:            mn.Retryable,
	ErrInvalidAddressLookupTableOwner:        mn.Retryable,
	ErrInvalidAddressLookupTableData:         mn.Retryable,
	ErrInvalidAddressLookupTableIndex:        mn.Retryable,
	ErrInvalidRentPayingAccount:              mn.Retryable,
	ErrWouldExceedMaxVoteCostLimit:           mn.Retryable,
	ErrWouldExceedAccountDataTotalLimit:      mn.Retryable,
	ErrMaxLoadedAccountsDataSizeExceeded:     mn.Retryable,
	ErrInvalidLoadedAccountsDataSizeLimit:    mn.Retryable,
	ErrResanitizationNeeded:                  mn.Retryable,
	ErrUnbalancedTransaction:                 mn.Retryable,
	ErrProgramCacheHitMaxLimit:               mn.Retryable,
	ErrInsufficientFundsForRent:              mn.InsufficientFunds,
	ErrDuplicateInstruction:                  mn.Fatal,
	ErrProgramExecutionTemporarilyRestricted: mn.Retryable,
}

// ClassifySendError returns the corresponding return code based on the error.
func ClassifySendError(_ *solana.Transaction, err error) mn.SendTxReturnCode {
	if err == nil {
		return mn.Successful
	}

	errMsg := err.Error()
	for pattern, code := range errCodes {
		if pattern.MatchString(errMsg) {
			return code
		}
	}
	return mn.Retryable
}

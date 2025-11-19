package chainwriterutils

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

// SubmitTransactionParams encapsulates all parameters needed for submitting a transaction.
// This reduces the number of function parameters and makes the API more maintainable.
type SubmitTransactionParams struct {
	// ProgramConfig contains the program configuration with methods and IDL
	ProgramConfig ProgramConfig
	// ContractName identifies which Solana program config to use
	ContractName string
	// Method specifies which method config to invoke
	Method string
	// Args are the arguments that are encoded into the transaction payload
	Args any
	// TransactionID is a unique identifier for tracking
	TransactionID string
	// ToAddress is the on-chain address (program ID)
	ToAddress string
	// Client is the multi-client for RPC calls
	Client client.MultiClient
	// TxManager is the transaction manager for enqueueing
	TxManager txm.TxManager
	// Encoder is the types encoder for payload encoding
	Encoder types.Encoder
	// Lggr is the logger instance
	Lggr logger.Logger
}

// SubmitTransactionImpl encapsulates the full logic for building, encoding, and enqueuing
// a transaction. This is used by both the chain-level and chainwriter-level implementations
// to avoid code duplication.
//
// Returns an error if any stage fails, nil if successfully enqueued.
func SubmitTransactionImpl(ctx context.Context, params SubmitTransactionParams) error {
	programConfig := params.ProgramConfig
	contractName := params.ContractName
	method := params.Method
	args := params.Args
	transactionID := params.TransactionID
	toAddress := params.ToAddress
	client := params.Client
	txManager := params.TxManager
	encoder := params.Encoder
	lggr := params.Lggr
	methodConfig, exists := programConfig.Methods[method]
	if !exists {
		return fmt.Errorf("failed to find method config for method: %s", method)
	}

	// Configure debug ID
	debugID := ""
	if methodConfig.DebugIDLocation != "" {
		var err error
		debugID, err = GetDebugIDAtLocation(args, methodConfig.DebugIDLocation)
		if err != nil {
			return ErrorWithDebugID(fmt.Errorf("error getting debug ID from input args: %w", err), debugID)
		}
	}

	// Fetch derived and static table maps
	derivedTableMap, staticTableMap, err := ResolveLookupTables(ctx, args, methodConfig.LookupTables, client)
	if err != nil {
		return ErrorWithDebugID(fmt.Errorf("error getting lookup tables: %w", err), debugID)
	}

	lggr.Debugw("Resolving account addresses", "contract", contractName, "method", method, "tx", transactionID, "debugID", debugID)
	// Resolve account metas
	accounts, err := GetAddresses(ctx, args, methodConfig.Accounts, derivedTableMap, client)
	if err != nil {
		return ErrorWithDebugID(fmt.Errorf("error resolving account addresses: %w", err), debugID)
	}

	feePayer, err := solana.PublicKeyFromBase58(methodConfig.FromAddress)
	if err != nil {
		return ErrorWithDebugID(fmt.Errorf("error parsing fee payer address: %w", err), debugID)
	}

	options := []txmutils.SetTxConfig{}
	// Transform args if necessary
	if methodConfig.ArgsTransform != "" {
		transformFunc, tfErr := FindTransform(methodConfig.ArgsTransform)
		if tfErr != nil {
			return ErrorWithDebugID(fmt.Errorf("error finding transform function: %w", tfErr), debugID)
		}
		lggr.Debugw("Applying args transformation", "contract", contractName, "method", method, "tx", transactionID, "debugID", debugID)
		args, accounts, staticTableMap, options, err = transformFunc(ctx, client, lggr, args, accounts, staticTableMap, derivedTableMap, feePayer, toAddress, methodConfig.ComputeUnitLimitOverhead, options, debugID)
		if err != nil {
			return ErrorWithDebugID(fmt.Errorf("error transforming args: %w", err), debugID)
		}
	}

	if len(methodConfig.ATAs) > 0 {
		lggr.Debugw("Creating ATAs", "contract", contractName, "method", method, "tx", transactionID, "debugID", debugID)
		createATAInstructions, ataErr := CreateATAs(ctx, args, methodConfig.ATAs, derivedTableMap, client, feePayer, lggr)
		if ataErr != nil {
			return ErrorWithDebugID(fmt.Errorf("error resolving account addresses: %w", ataErr), debugID)
		}
		var ataUUID string
		if ataUUID, err = HandleATACreation(ctx, createATAInstructions, methodConfig, contractName, method, feePayer, client, txManager, lggr); err != nil {
			return ErrorWithDebugID(fmt.Errorf("error creating ATAs: %w", err), debugID)
		}
		if ataUUID != "" {
			// Wait till ATA creation is finalized before proceeding with the main transaction
			options = append(options, txmutils.AppendDependencyTxs([]txmutils.DependencyTx{{TxID: ataUUID, DesiredStatus: types.Finalized}}))
		}
	}

	lggr.Debugw("Filtering lookup table addresses", "contract", contractName, "method", method, "tx", transactionID, "debugID", debugID)
	// Filter the lookup table addresses based on which accounts are actually used
	filteredLookupTableMap := FilterLookupTableAddresses(accounts, derivedTableMap, staticTableMap)

	// Prepare transaction
	programID, err := solana.PublicKeyFromBase58(toAddress)
	if err != nil {
		return ErrorWithDebugID(fmt.Errorf("error parsing program ID: %w", err), debugID)
	}

	encodedPayload, err := EncodePayload(ctx, args, methodConfig, contractName, method, lggr, encoder)
	if err != nil {
		return ErrorWithDebugID(fmt.Errorf("error encoding transaction payload: %w", err), debugID)
	}

	// Fetch latest blockhash
	blockhash, err := client.LatestBlockhash(ctx)
	if err != nil {
		return ErrorWithDebugID(fmt.Errorf("error fetching latest blockhash: %w", err), debugID)
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{solana.NewInstruction(programID, accounts, encodedPayload)},
		blockhash.Value.Blockhash,
		solana.TransactionPayer(feePayer),
		solana.TransactionAddressTables(filteredLookupTableMap),
	)
	if err != nil {
		return ErrorWithDebugID(fmt.Errorf("error constructing transaction: %w", err), debugID)
	}

	txSize, err := CalculateTxSize(tx)
	if err != nil {
		return ErrorWithDebugID(fmt.Errorf("failed to calculate tx size: %w", err), debugID)
	}

	if txSize > MaxSolanaTxSize {
		lggr.Debugw("Transaction size exceeds the Solana max", "size", txSize, "max", MaxSolanaTxSize, "tx", transactionID, "debugID", debugID)
		// Return error if transaction too large and method to write to buffer is not provided
		if methodConfig.BufferPayloadMethod == "" {
			return ErrorWithDebugID(fmt.Errorf("transaction size %d exceeds limit %d with no buffer payload method set", txSize, MaxSolanaTxSize), debugID)
		}
		if bufferErr := HandleTxBuffering(ctx, methodConfig, contractName, method, transactionID, debugID, accounts, programID, feePayer, args, options, filteredLookupTableMap, client, txManager, lggr, encoder); bufferErr != nil {
			return ErrorWithDebugID(fmt.Errorf("error handling transaction buffering: %w", bufferErr), debugID)
		}
		// handleTxBuffering takes care of queueing the main transaction in the correct order of dependencies so we should exit early
		return nil
	}

	lggr.Debugw("Sending main transaction", "contract", contractName, "method", method, "tx", transactionID, "debugID", debugID)

	// Enqueue transaction
	if err = txManager.Enqueue(ctx, methodConfig.FromAddress, tx, &transactionID, blockhash.Value.LastValidBlockHeight, options...); err != nil {
		return ErrorWithDebugID(fmt.Errorf("error enqueuing transaction: %w", err), debugID)
	}

	return nil
}

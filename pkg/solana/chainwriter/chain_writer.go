package chainwriter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	anchoridl "github.com/gagliardetto/anchor-go/idl"
	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/rpc"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	codecv2 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v2"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

const (
	ServiceName     = "SolanaChainWriter"
	MaxSolanaTxSize = 1232
)

type SolanaChainWriterService struct {
	lggr   logger.Logger
	client client.MultiClient
	txm    txm.TxManager
	ge     fees.Estimator
	config ChainWriterConfig

	parsed  *solcommoncodec.ParsedTypes
	encoder types.Encoder

	services.StateMachine
}

var (
	_ services.Service     = &SolanaChainWriterService{}
	_ types.ContractWriter = &SolanaChainWriterService{}
)

// nolint // ignoring naming suggestion
type ChainWriterConfig struct {
	Programs map[string]ProgramConfig `json:"programs"`
}

type ProgramConfig struct {
	Methods map[string]MethodConfig `json:"methods"`
	IDL     string                  `json:"idl"`
}

type MethodConfig struct {
	FromAddress        string                      `json:"fromAddress"`
	InputModifications commoncodec.ModifiersConfig `json:"inputModifications,omitempty"`
	ChainSpecificName  string                      `json:"chainSpecificName"`
	LookupTables       LookupTables                `json:"lookupTables,omitempty"`
	Accounts           []Lookup                    `json:"accounts"`
	ATAs               []ATALookup                 `json:"atas,omitempty"`
	// Location in the args where the debug ID is stored
	DebugIDLocation string `json:"debugIDLocation,omitempty"`
	ArgsTransform   string `json:"argsTransform,omitempty"`
	// Overhead added to calculated compute units in the args transform
	ComputeUnitLimitOverhead uint32 `json:"ComputeUnitLimitOverhead,omitempty"`
	// Configs for buffering payloads to support larger transaction sizes for this method
	BufferPayloadMethod string `json:"bufferPayloadMethod,omitempty"`
}

func NewSolanaChainWriterService(logger logger.Logger, client client.MultiClient, txm txm.TxManager, ge fees.Estimator, config ChainWriterConfig) (*SolanaChainWriterService, error) {
	w := SolanaChainWriterService{
		lggr:   logger,
		client: client,
		txm:    txm,
		ge:     ge,
		config: config,
		parsed: &solcommoncodec.ParsedTypes{EncoderDefs: map[string]solcommoncodec.Entry{}, DecoderDefs: map[string]solcommoncodec.Entry{}},
	}

	if err := w.parsePrograms(config); err != nil {
		return nil, fmt.Errorf("failed to parse programs: %w", err)
	}

	var err error
	if w.encoder, err = w.parsed.ToCodec(); err != nil {
		return nil, fmt.Errorf("%w: failed to create codec", err)
	}

	w.lggr.Info("SolanaChainWriterService initialized")
	return &w, nil
}

func (s *SolanaChainWriterService) parsePrograms(config ChainWriterConfig) error {
	for program, programConfig := range config.Programs {
		// Try to unmarshal as codecv2 IDL first
		var codecv2IDL anchoridl.Idl
		if err := json.Unmarshal([]byte(programConfig.IDL), &codecv2IDL); err == nil {
			// Successfully unmarshaled as codecv2 IDL
			if err := s.parseProgramCodecv2(program, programConfig, codecv2IDL); err != nil {
				return err
			}
		} else {
			// Fall back to codec IDL
			var codecIDL codecv1.IDL
			if err := json.Unmarshal([]byte(programConfig.IDL), &codecIDL); err != nil {
				return fmt.Errorf("failed to unmarshal IDL for program %s (tried both codecv2 and codec), error: %w", program, err)
			}
			if err := s.parseProgramCodec(program, programConfig, codecIDL); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *SolanaChainWriterService) parseProgramCodec(program string, programConfig ProgramConfig, idl codecv1.IDL) error {
	for method, methodConfig := range programConfig.Methods {
		utils.InjectAddressModifier(methodConfig.InputModifications, nil)
		idlDef, err := codecv1.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeInstructionDef, methodConfig.ChainSpecificName, idl)
		if err != nil {
			return err
		}

		inputMod, err := methodConfig.InputModifications.ToModifier(solcommoncodec.DecoderHooks...)
		if err != nil {
			return fmt.Errorf("failed to create input modifications for method %s.%s, error: %w", program, method, err)
		}

		input, err := codecv1.CreateCodecEntry(idlDef, methodConfig.ChainSpecificName, idl, inputMod)
		if err != nil {
			return fmt.Errorf("failed to create codec entry for method %s.%s, error: %w", program, method, err)
		}

		s.parsed.EncoderDefs[solcommoncodec.WrapItemType(true, program, method)] = input
	}
	return nil
}

func (s *SolanaChainWriterService) parseProgramCodecv2(program string, programConfig ProgramConfig, idl anchoridl.Idl) error {
	for method, methodConfig := range programConfig.Methods {
		utils.InjectAddressModifier(methodConfig.InputModifications, nil)
		idlDef, err := codecv2.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeInstructionDef, methodConfig.ChainSpecificName, idl)
		if err != nil {
			return err
		}

		inputMod, err := methodConfig.InputModifications.ToModifier(solcommoncodec.DecoderHooks...)
		if err != nil {
			return fmt.Errorf("failed to create input modifications for method %s.%s, error: %w", program, method, err)
		}

		input, err := codecv2.CreateCodecEntry(idlDef, methodConfig.ChainSpecificName, idl, inputMod)
		if err != nil {
			return fmt.Errorf("failed to create codec entry for method %s.%s, error: %w", program, method, err)
		}

		s.parsed.EncoderDefs[solcommoncodec.WrapItemType(true, program, method)] = input
	}
	return nil
}

/*
GetAddresses resolves account addresses from various `Lookup` configurations to build the required `solana.AccountMeta` list
for Solana transactions. It handles constant addresses, dynamic lookups, program-derived addresses (PDAs), and lookup tables.

### Parameters:
- `ctx`: Context for request lifecycle management.
- `args`: Input arguments used for dynamic lookups.
- `accounts`: List of `Lookup` configurations specifying how addresses are derived.
- `derivedTableMap`: Map of pre-loaded lookup table addresses.
- `debugID`: Debug identifier for tracing errors.

### Return:
- A slice of `solana.AccountMeta` containing derived addresses and associated metadata.

### Account Types:
1. **AccountConstant**:
  - A fixed address, provided in Base58 format, converted into a `solana.PublicKey`.
  - Example: A pre-defined fee payer or system account.

2. **AccountLookup**:
  - Dynamically derived from input args using a specified location path (e.g., `user.walletAddress`).
  - If the lookup table is pre-loaded, the address is fetched from `derivedTableMap`.

3. **PDALookups**:
  - Generates Program Derived Addresses (PDA) by combining a derived public key with one or more seeds.
  - Seeds can be `AddressSeeds` (public keys from the input args) or `ValueSeeds` (byte arrays).
  - Ensures there is only one public key if multiple seeds are provided.

### Error Handling:
- Errors are wrapped with the `debugID` for easier tracing.
*/
func GetAddresses(ctx context.Context, args any, accounts []Lookup, derivedTableMap map[string]map[string][]*solana.AccountMeta, client client.MultiClient) ([]*solana.AccountMeta, error) {
	var addresses []*solana.AccountMeta
	for _, accountConfig := range accounts {
		meta, err := accountConfig.Resolve(ctx, args, derivedTableMap, client)
		if accountConfig.Optional && err != nil && isIgnorableError(err) {
			// skip optional accounts if they are not found
			continue
		}
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, meta...)
	}
	return addresses, nil
}

// These errors are ignorable if the lookup is optional.
func isIgnorableError(err error) bool {
	return errors.Is(err, ErrLookupNotFoundAtLocation) ||
		errors.Is(err, ErrLookupTableNotFound) ||
		errors.Is(err, ErrGettingSeedAtLocation)
}

// FilterLookupTableAddresses takes a list of accounts and two lookup table maps
// (one for derived tables, one for static tables) and filters out any addresses that are
// not used by the accounts. It returns a map of only those lookup table
// addresses that match entries in `accounts`.
func (s *SolanaChainWriterService) FilterLookupTableAddresses(
	accounts []*solana.AccountMeta,
	derivedTableMap map[string]map[string][]*solana.AccountMeta,
	staticTableMap map[solana.PublicKey]solana.PublicKeySlice,
) map[solana.PublicKey]solana.PublicKeySlice {
	filteredLookupTables := make(map[solana.PublicKey]solana.PublicKeySlice)

	// Build a hash set of account public keys for fast lookup
	usedAccounts := make(map[string]struct{})
	for _, account := range accounts {
		if account != nil {
			usedAccounts[account.PublicKey.String()] = struct{}{}
		}
	}

	// Filter derived lookup tables
	for _, innerMap := range derivedTableMap {
		for innerIdentifier, metas := range innerMap {
			tableKey, err := solana.PublicKeyFromBase58(innerIdentifier)
			if err != nil {
				continue
			}

			tableAddresses := make(solana.PublicKeySlice, 0, len(metas))
			foundUsedAddress := false
			// Parse metas into public keys for filtered lookup table map
			for _, meta := range metas {
				if meta == nil {
					continue
				}
				tableAddresses = append(tableAddresses, meta.PublicKey)
				if _, exists := usedAccounts[meta.PublicKey.String()]; exists {
					foundUsedAddress = true
				}
			}

			// Add lookup table to the filtered map if it contains an address used for the tx
			if foundUsedAddress {
				filteredLookupTables[tableKey] = tableAddresses
			}
		}
	}

	// Filter static lookup tables
	for tableKey, addresses := range staticTableMap {
		foundUsedAddress := false
		for _, staticAddress := range addresses {
			if _, exists := usedAccounts[staticAddress.String()]; exists {
				foundUsedAddress = true
				break
			}
		}

		// Add lookup table to the filtered map if it contains an address used for the tx
		if foundUsedAddress {
			filteredLookupTables[tableKey] = addresses
		}
	}

	return filteredLookupTables
}

// SubmitTransaction builds, encodes, and enqueues a transaction using the provided program
// configuration and method details. It relies on the configured IDL, account lookups, and
// lookup tables to gather the necessary accounts and data. The function retrieves the latest
// blockhash and assigns it to the transaction, so callers do not need to provide one.
//
// Submissions and retries are handled by the underlying transaction manager. If a “debug ID”
// location is configured, SubmitTransaction extracts it from the provided `args` and attaches
// it to errors for easier troubleshooting. Only the first debug ID it encounters will be used.
//
// Parameters:
//   - ctx: The context for cancellation and timeouts.
//   - contractName: Identifies which Solana program config to use from `s.config.Programs`.
//   - method: Specifies which method config to invoke within the chosen program config.
//   - args: Arbitrary arguments that are encoded into the transaction payload and/or used for dynamic address lookups.
//   - transactionID: A unique identifier for the transaction, used for tracking within the transaction manager.
//   - toAddress: The on-chain address (program ID) to which the transaction is directed.
//   - meta: Currently unused; included for interface compatibility.
//   - value: Currently unused; included for interface compatibility.
//
// Returns:
//
//	An error if any stage of the transaction preparation or enqueueing fails. A nil return
//	indicates that the transaction was successfully submitted to the transaction manager.
func (s *SolanaChainWriterService) SubmitTransaction(ctx context.Context, contractName, method string, args any, transactionID string, toAddress string, _ *types.TxMeta, _ *big.Int) error {
	programConfig, exists := s.config.Programs[contractName]
	if !exists {
		return fmt.Errorf("failed to find program config for contract name: %s", contractName)
	}
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
			return errorWithDebugID(fmt.Errorf("error getting debug ID from input args: %w", err), debugID)
		}
	}

	// Fetch derived and static table maps
	derivedTableMap, staticTableMap, err := s.ResolveLookupTables(ctx, args, methodConfig.LookupTables)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error getting lookup tables: %w", err), debugID)
	}

	s.lggr.Debugw("Resolving account addresses", "contract", contractName, "method", method, "tx", transactionID, "debugID", debugID)
	// Resolve account metas
	accounts, err := GetAddresses(ctx, args, methodConfig.Accounts, derivedTableMap, s.client)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error resolving account addresses: %w", err), debugID)
	}

	feePayer, err := solana.PublicKeyFromBase58(methodConfig.FromAddress)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error parsing fee payer address: %w", err), debugID)
	}

	options := []txmutils.SetTxConfig{}
	// Transform args if necessary
	if methodConfig.ArgsTransform != "" {
		transformFunc, tfErr := FindTransform(methodConfig.ArgsTransform)
		if tfErr != nil {
			return errorWithDebugID(fmt.Errorf("error finding transform function: %w", tfErr), debugID)
		}
		s.lggr.Debugw("Applying args transformation", "contract", contractName, "method", method, "tx", transactionID, "debugID", debugID)
		args, accounts, staticTableMap, options, err = transformFunc(ctx, s.client, s.lggr, args, accounts, staticTableMap, derivedTableMap, feePayer, toAddress, methodConfig.ComputeUnitLimitOverhead, options, debugID)
		if err != nil {
			return errorWithDebugID(fmt.Errorf("error transforming args: %w", err), debugID)
		}
	}

	if len(methodConfig.ATAs) > 0 {
		s.lggr.Debugw("Creating ATAs", "contract", contractName, "method", method, "tx", transactionID, "debugID", debugID)
		createATAInstructions, ataErr := CreateATAs(ctx, args, methodConfig.ATAs, derivedTableMap, s.client, feePayer, s.lggr)
		if ataErr != nil {
			return errorWithDebugID(fmt.Errorf("error resolving account addresses: %w", err), debugID)
		}
		var ataUUID string
		if ataUUID, err = s.handleATACreation(ctx, createATAInstructions, methodConfig, contractName, method, feePayer); err != nil {
			return errorWithDebugID(fmt.Errorf("error creating ATAs: %w", err), debugID)
		}
		if ataUUID != "" {
			// Wait till ATA creation is finalized before proceeding with the main transaction
			options = append(options, txmutils.AppendDependencyTxs([]txmutils.DependencyTx{{TxID: ataUUID, DesiredStatus: types.Finalized}}))
		}
	}

	s.lggr.Debugw("Filtering lookup table addresses", "contract", contractName, "method", method, "tx", transactionID, "debugID", debugID)
	// Filter the lookup table addresses based on which accounts are actually used
	filteredLookupTableMap := s.FilterLookupTableAddresses(accounts, derivedTableMap, staticTableMap)

	// Prepare transaction
	programID, err := solana.PublicKeyFromBase58(toAddress)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error parsing program ID: %w", err), debugID)
	}

	encodedPayload, err := s.EncodePayload(ctx, args, methodConfig, contractName, method)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error encoding transaction payload: %w", err), debugID)
	}

	// Fetch latest blockhash
	blockhash, err := s.client.LatestBlockhash(ctx)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error fetching latest blockhash: %w", err), debugID)
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{solana.NewInstruction(programID, accounts, encodedPayload)},
		blockhash.Value.Blockhash,
		solana.TransactionPayer(feePayer),
		solana.TransactionAddressTables(filteredLookupTableMap),
	)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error constructing transaction: %w", err), debugID)
	}

	// Calculate the transaction size to validate it fits within Solana limit
	// Includes the compute unit price and limit instructions in size to allow room for those to be added downstream in the TXM
	txSize, err := CalculateTxSize(tx)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("failed to calculate tx size: %w", err), debugID)
	}

	if txSize > MaxSolanaTxSize {
		s.lggr.Debugw("Transaction size exceeds the Solana max", "size", txSize, "max", MaxSolanaTxSize, "tx", transactionID, "debugID", debugID)
		// Return error if transaction too large and method to write to buffer is not provided
		if methodConfig.BufferPayloadMethod == "" {
			return errorWithDebugID(fmt.Errorf("transaction size %d exceeds limit %d with no buffer payload method set", txSize, MaxSolanaTxSize), debugID)
		}
		if bufferErr := s.handleTxBuffering(ctx, methodConfig, contractName, method, transactionID, debugID, accounts, programID, feePayer, args, options, filteredLookupTableMap); bufferErr != nil {
			return errorWithDebugID(fmt.Errorf("error handling transaction buffering: %w", bufferErr), debugID)
		}
		// handleTxBuffering takes care of queueing the main transaction in the correct order of dependencies so we should exit early
		return nil
	}

	s.lggr.Debugw("Sending main transaction", "contract", contractName, "method", method, "tx", transactionID, "debugID", debugID)

	// Enqueue transaction
	if err = s.txm.Enqueue(ctx, methodConfig.FromAddress, tx, &transactionID, blockhash.Value.LastValidBlockHeight, options...); err != nil {
		return errorWithDebugID(fmt.Errorf("error enqueuing transaction: %w", err), debugID)
	}

	return nil
}

// GetTransactionStatus returns the current status of a transaction in the underlying chain's TXM.
func (s *SolanaChainWriterService) GetTransactionStatus(ctx context.Context, transactionID string) (types.TransactionStatus, error) {
	status, err := s.txm.GetTransactionStatus(ctx, transactionID)
	s.lggr.Debugw("Fetching transaction status", "tx", transactionID, "status", status)
	return status, err
}

// GetFeeComponents retrieves the associated gas costs for executing a transaction.
func (s *SolanaChainWriterService) GetFeeComponents(ctx context.Context) (*types.ChainFeeComponents, error) {
	if s.ge == nil {
		return nil, fmt.Errorf("gas estimator not available")
	}

	fee := s.ge.BaseComputeUnitPrice()
	s.lggr.Debugw("Fetched fee components", "executionFee", fee, "dataAvailabilityFee", 0)

	return &types.ChainFeeComponents{
		ExecutionFee:        new(big.Int).SetUint64(fee),
		DataAvailabilityFee: big.NewInt(0), // required field so return 0 instead of nil
	}, nil
}

func (s *SolanaChainWriterService) GetEstimateFee(ctx context.Context, contract, method string, args any, toAddress string, meta *types.TxMeta, val *big.Int) (types.EstimateFee, error) {
	return types.EstimateFee{}, errors.New("estimate fee is not implemented for solana")
}

func (s *SolanaChainWriterService) ResolveLookupTables(ctx context.Context, args any, lookupTables LookupTables) (map[string]map[string][]*solana.AccountMeta, map[solana.PublicKey]solana.PublicKeySlice, error) {
	derivedTableMap := make(map[string]map[string][]*solana.AccountMeta)
	staticTableMap := make(map[solana.PublicKey]solana.PublicKeySlice)

	// Read derived lookup tables
	for _, derivedLookup := range lookupTables.DerivedLookupTables {
		// Load the lookup table - note: This could be multiple tables if the lookup is a PDALookups that resolves to more
		// than one address
		lookupTableMap, err := s.loadTable(ctx, args, derivedLookup)
		if derivedLookup.Optional && err != nil && isIgnorableError(err) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("error loading derived lookup table: %w", err)
		}

		// Merge the loaded table map into the result
		for tableName, innerMap := range lookupTableMap {
			if derivedTableMap[tableName] == nil {
				derivedTableMap[tableName] = make(map[string][]*solana.AccountMeta)
			}
			for accountKey, metas := range innerMap {
				derivedTableMap[tableName][accountKey] = metas
			}
		}
	}

	// Read static lookup tables
	for _, staticTable := range lookupTables.StaticLookupTables {
		addressses, err := getLookupTableAddresses(ctx, s.client, staticTable)
		if err != nil {
			return nil, nil, fmt.Errorf("error fetching static lookup table address: %w", err)
		}
		staticTableMap[staticTable] = addressses
	}

	return derivedTableMap, staticTableMap, nil
}

func (s *SolanaChainWriterService) loadTable(ctx context.Context, args any, rlt DerivedLookupTable) (map[string]map[string][]*solana.AccountMeta, error) {
	// Resolve all addresses specified by the identifier
	lookupTableAddresses, err := GetAddresses(ctx, args, []Lookup{rlt.Accounts}, nil, s.client)
	if err != nil {
		return nil, fmt.Errorf("error resolving addresses for lookup table: %w", err)
	}

	// Nested map in case the lookup table resolves to multiple addresses
	resultMap := make(map[string]map[string][]*solana.AccountMeta)

	// Iterate over each address of the lookup table
	for _, addressMeta := range lookupTableAddresses {
		// Read the full list of addresses from the lookup table
		addresses, err := getLookupTableAddresses(ctx, s.client, addressMeta.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("error fetching lookup table address: %s, error: %w", addressMeta.PublicKey, err)
		}

		// Create the inner map for this lookup table
		if resultMap[rlt.Name] == nil {
			resultMap[rlt.Name] = make(map[string][]*solana.AccountMeta)
		}

		// Populate the inner map (keyed by the account public key)
		for _, addr := range addresses {
			resultMap[rlt.Name][addressMeta.PublicKey.String()] = append(resultMap[rlt.Name][addressMeta.PublicKey.String()], &solana.AccountMeta{
				PublicKey:  addr,
				IsSigner:   addressMeta.IsSigner,
				IsWritable: addressMeta.IsWritable,
			})
		}
	}

	return resultMap, nil
}

func (s *SolanaChainWriterService) EncodePayload(ctx context.Context, args any, methodConfig MethodConfig, contractName, method string) ([]byte, error) {
	s.lggr.Debugw("Encoding transaction payload", "contract", contractName, "method", method)
	encodedPayload, err := s.encoder.Encode(ctx, args, solcommoncodec.WrapItemType(true, contractName, method))
	if err != nil {
		return nil, fmt.Errorf("error encoding transaction payload: %w", err)
	}

	discriminator := solcommoncodec.NewMethodDiscriminatorHashPrefix(methodConfig.ChainSpecificName)
	encodedPayload = append(discriminator[:], encodedPayload...)
	return encodedPayload, nil
}

// handleTxBuffering handles the creation, queuing, and dependency tracking for transactions that require writing their payload to a buffer
// - Creates and queues transactions to write to the buffer
// - Creates and queues the main transaction with the new accounts list and transformed args
// - Marks the main transaction as dependent on all buffer transactions to ensure buffer is completely written before broadcast
// - Creates and queues a close buffer transaction dependent on the failure of the main transaction or buffer transactions. If the main transaction succeeds, the close transasction is quietly dropped.
func (s *SolanaChainWriterService) handleTxBuffering(
	ctx context.Context,
	methodConfig MethodConfig,
	contractName, method, transactionID, debugID string,
	accounts solana.AccountMetaSlice,
	programID, feePayer solana.PublicKey,
	args any,
	options []txmutils.SetTxConfig,
	lookupTableMap map[solana.PublicKey]solana.PublicKeySlice,
) error {
	// Check registry for method to create buffer intstructions
	createBufferIxs, err := FindCreateBufferInstructionsMethod(methodConfig.BufferPayloadMethod)
	if err != nil {
		return fmt.Errorf("error finding buffer method for name %s: %w", methodConfig.BufferPayloadMethod, err)
	}
	// Use method to create the instructions to write to the on-chain buffer
	var bufferIxs []solana.Instruction
	var closeBufferIx solana.Instruction
	bufferIxs, closeBufferIx, accounts, args, err = createBufferIxs(ctx, args, accounts, programID, feePayer)
	if err != nil {
		return fmt.Errorf("error creating buffer instructions: %w", err)
	}
	// Send the buffer transactions and track the IDs to mark the main transaction as dependent
	err = s.sendBufferInstructions(ctx, bufferIxs, closeBufferIx, methodConfig, contractName, method, transactionID, debugID, programID, feePayer, accounts, args, options, lookupTableMap)
	if err != nil {
		return fmt.Errorf("error enqueuing buffer transactions: %w", err)
	}

	return nil
}

func getLookupTableAddresses(ctx context.Context, client client.MultiClient, tableAddress solana.PublicKey) (solana.PublicKeySlice, error) {
	// Fetch the account info for the static table
	accountInfo, err := client.GetAccountInfoWithOpts(ctx, tableAddress, &rpc.GetAccountInfoOpts{
		Encoding:   "base64",
		Commitment: rpc.CommitmentFinalized,
	})

	if err != nil || accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("error fetching account info for table: %s, error: %w", tableAddress.String(), err)
	}
	alt, err := addresslookuptable.DecodeAddressLookupTableState(accountInfo.GetBinary())
	if err != nil {
		return nil, fmt.Errorf("error decoding address lookup table state: %w", err)
	}
	return alt.Addresses, nil
}

func CalculateTxSize(tx *solana.Transaction) (int, error) {
	if tx == nil {
		return 0, errors.New("tx is nulll")
	}
	copyTx := utils.DeepCopyTx(*tx)

	// Set instructions and fields that are added further downstream with arbitrary values to get an accurate tx size
	err := fees.SetComputeUnitPrice(&copyTx, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to set compute unit price instruction: %w", err)
	}
	err = fees.SetComputeUnitLimit(&copyTx, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to set compute unit limit instruction: %w", err)
	}
	copyTx.Signatures = append(copyTx.Signatures, solana.Signature{})

	// Get the transaction bytes with all releavnt fields added
	txBytes, err := copyTx.MarshalBinary()
	if err != nil {
		return 0, fmt.Errorf("error marshaling transaction: %w", err)
	}
	return len(txBytes), nil
}

func (s *SolanaChainWriterService) Start(_ context.Context) error {
	return s.StartOnce(ServiceName, func() error {
		return nil
	})
}

func (s *SolanaChainWriterService) Close() error {
	return s.StopOnce(ServiceName, func() error {
		return nil
	})
}

func (s *SolanaChainWriterService) HealthReport() map[string]error {
	return map[string]error{s.Name(): s.Healthy()}
}

func (s *SolanaChainWriterService) Name() string {
	return s.lggr.Name()
}

func (s *SolanaChainWriterService) Ready() error {
	return s.StateMachine.Ready()
}

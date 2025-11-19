package chainwriterutils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/rpc"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

const MaxSolanaTxSize = 1232

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
		if accountConfig.Optional && err != nil && IsIgnorableError(err) {
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
func IsIgnorableError(err error) bool {
	return errors.Is(err, ErrLookupNotFoundAtLocation) ||
		errors.Is(err, ErrLookupTableNotFound) ||
		errors.Is(err, ErrGettingSeedAtLocation)
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

func GetLookupTableAddresses(ctx context.Context, client client.MultiClient, tableAddress solana.PublicKey) (solana.PublicKeySlice, error) {
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

func ResolveLookupTables(ctx context.Context, args any, lookupTables LookupTables, client client.MultiClient) (map[string]map[string][]*solana.AccountMeta, map[solana.PublicKey]solana.PublicKeySlice, error) {
	derivedTableMap := make(map[string]map[string][]*solana.AccountMeta)
	staticTableMap := make(map[solana.PublicKey]solana.PublicKeySlice)

	// Read derived lookup tables
	for _, derivedLookup := range lookupTables.DerivedLookupTables {
		// Load the lookup table - note: This could be multiple tables if the lookup is a PDALookups that resolves to more
		// than one address
		lookupTableMap, err := loadTable(ctx, args, derivedLookup, client)
		if derivedLookup.Optional && err != nil && IsIgnorableError(err) {
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
		addressses, err := GetLookupTableAddresses(ctx, client, staticTable)
		if err != nil {
			return nil, nil, fmt.Errorf("error fetching static lookup table address: %w", err)
		}
		staticTableMap[staticTable] = addressses
	}

	return derivedTableMap, staticTableMap, nil
}

func loadTable(ctx context.Context, args any, rlt DerivedLookupTable, client client.MultiClient) (map[string]map[string][]*solana.AccountMeta, error) {
	// Resolve all addresses specified by the identifier
	lookupTableAddresses, err := GetAddresses(ctx, args, []Lookup{rlt.Accounts}, nil, client)
	if err != nil {
		return nil, fmt.Errorf("error resolving addresses for lookup table: %w", err)
	}

	// Nested map in case the lookup table resolves to multiple addresses
	resultMap := make(map[string]map[string][]*solana.AccountMeta)

	// Iterate over each address of the lookup table
	for _, addressMeta := range lookupTableAddresses {
		// Read the full list of addresses from the lookup table
		addresses, err := GetLookupTableAddresses(ctx, client, addressMeta.PublicKey)
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

// FilterLookupTableAddresses takes a list of accounts and two lookup table maps
// (one for derived tables, one for static tables) and filters out any addresses that are
// not used by the accounts. It returns a map of only those lookup table
// addresses that match entries in `accounts`.
func FilterLookupTableAddresses(
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

func EncodePayload(ctx context.Context, args any, methodConfig MethodConfig, contractName, method string, lggr logger.Logger, encoder types.Encoder) ([]byte, error) {
	lggr.Debugw("Encoding transaction payload", "contract", contractName, "method", method)
	encodedPayload, err := encoder.Encode(ctx, args, codec.WrapItemType(true, contractName, method))
	if err != nil {
		return nil, fmt.Errorf("error encoding transaction payload: %w", err)
	}

	discriminator := GetDiscriminator(methodConfig.ChainSpecificName)
	encodedPayload = append(discriminator[:], encodedPayload...)
	return encodedPayload, nil
}

// handleTxBuffering handles the creation, queuing, and dependency tracking for transactions that require writing their payload to a buffer
// - Creates and queues transactions to write to the buffer
// - Creates and queues the main transaction with the new accounts list and transformed args
// - Marks the main transaction as dependent on all buffer transactions to ensure buffer is completely written before broadcast
// - Creates and queues a close buffer transaction dependent on the failure of the main transaction or buffer transactions. If the main transaction succeeds, the close transasction is quietly dropped.
func HandleTxBuffering(
	ctx context.Context,
	methodConfig MethodConfig,
	contractName, method, transactionID, debugID string,
	accounts solana.AccountMetaSlice,
	programID, feePayer solana.PublicKey,
	args any,
	options []txmutils.SetTxConfig,
	lookupTableMap map[solana.PublicKey]solana.PublicKeySlice,
	client client.MultiClient,
	txm txm.TxManager,
	lggr logger.Logger,
	encoder types.Encoder,
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
	err = SendBufferInstructions(ctx, bufferIxs, closeBufferIx, methodConfig, contractName, method, transactionID, debugID, programID, feePayer, accounts, args, options, lookupTableMap, client, txm, lggr, encoder)
	if err != nil {
		return fmt.Errorf("error enqueuing buffer transactions: %w", err)
	}

	return nil
}

// ParseProgramsToCodec parses program configurations and creates a codec with encoder definitions
// for all methods defined in the programs. This is used by both chain and chainwriter to prepare
// the encoder for transaction payload encoding.
func ParseProgramsToCodec(programs map[string]ProgramConfig) (*codec.ParsedTypes, types.Encoder, error) {
	parsed := &codec.ParsedTypes{EncoderDefs: map[string]codec.Entry{}, DecoderDefs: map[string]codec.Entry{}}

	for program, programConfig := range programs {
		var idl codec.IDL
		if err := json.Unmarshal([]byte(programConfig.IDL), &idl); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal IDL for program: %s, error: %w", program, err)
		}
		for method, methodConfig := range programConfig.Methods {
			utils.InjectAddressModifier(methodConfig.InputModifications, nil)
			idlDef, err := codec.FindDefinitionFromIDL(codec.ChainConfigTypeInstructionDef, methodConfig.ChainSpecificName, idl)
			if err != nil {
				return nil, nil, err
			}

			inputMod, err := methodConfig.InputModifications.ToModifier(codec.DecoderHooks...)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create input modifications for method %s.%s, error: %w", program, method, err)
			}

			input, err := codec.CreateCodecEntry(idlDef, methodConfig.ChainSpecificName, idl, inputMod)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create codec entry for method %s.%s, error: %w", program, method, err)
			}

			parsed.EncoderDefs[codec.WrapItemType(true, program, method)] = input
		}
	}

	encoder, err := parsed.ToCodec()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create codec: %w", err)
	}

	return parsed, encoder, nil
}

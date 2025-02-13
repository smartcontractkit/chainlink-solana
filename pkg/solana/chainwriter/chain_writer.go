package chainwriter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/tokens"
	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

const ServiceName = "SolanaChainWriter"

type SolanaChainWriterService struct {
	lggr   logger.Logger
	client client.MultiClient
	txm    txm.TxManager
	ge     fees.Estimator
	config ChainWriterConfig

	parsed  *codec.ParsedTypes
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
}

func NewSolanaChainWriterService(logger logger.Logger, client client.MultiClient, txm txm.TxManager, ge fees.Estimator, config ChainWriterConfig) (*SolanaChainWriterService, error) {
	w := SolanaChainWriterService{
		lggr:   logger,
		client: client,
		txm:    txm,
		ge:     ge,
		config: config,
		parsed: &codec.ParsedTypes{EncoderDefs: map[string]codec.Entry{}, DecoderDefs: map[string]codec.Entry{}},
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
		var idl codec.IDL
		if err := json.Unmarshal([]byte(programConfig.IDL), &idl); err != nil {
			return fmt.Errorf("failed to unmarshal IDL for program: %s, error: %w", program, err)
		}
		for method, methodConfig := range programConfig.Methods {
			utils.InjectAddressModifier(methodConfig.InputModifications, nil)
			idlDef, err := codec.FindDefinitionFromIDL(codec.ChainConfigTypeInstructionDef, methodConfig.ChainSpecificName, idl)
			if err != nil {
				return err
			}

			inputMod, err := methodConfig.InputModifications.ToModifier(codec.DecoderHooks...)
			if err != nil {
				return fmt.Errorf("failed to create input modifications for method %s.%s, error: %w", program, method, err)
			}

			input, err := codec.CreateCodecEntry(idlDef, methodConfig.ChainSpecificName, idl, inputMod)
			if err != nil {
				return fmt.Errorf("failed to create codec entry for method %s.%s, error: %w", program, method, err)
			}

			s.parsed.EncoderDefs[codec.WrapItemType(true, program, method)] = input
		}
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
		if accountConfig.Optional && err != nil {
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
		usedAccounts[account.PublicKey.String()] = struct{}{}
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

// CreateATAs first checks if a specified location exists, then checks if the accounts derived from the
// ATALookups in the ChainWriter's configuration exist on-chain and creates them if they do not.
func CreateATAs(ctx context.Context, args any, lookups []ATALookup, derivedTableMap map[string]map[string][]*solana.AccountMeta, client client.MultiClient, idl string, feePayer solana.PublicKey, logger logger.Logger) ([]solana.Instruction, error) {
	createATAInstructions := []solana.Instruction{}
	for _, lookup := range lookups {
		// Check if location exists
		if lookup.Location != "" {
			_, err := GetValuesAtLocation(args, lookup.Location)
			if err != nil {
				// field doesn't exist, so ignore ATA creation
				if errors.Is(err, errFieldNotFound) {
					logger.Debugw("field not found, skipping ATA creation", "location", lookup.Location)
					continue
				}
				return nil, fmt.Errorf("error getting values at location: %w", err)
			}
		}
		walletAddresses, err := GetAddresses(ctx, args, []Lookup{lookup.WalletAddress}, derivedTableMap, client)
		if err != nil {
			return nil, fmt.Errorf("error resolving wallet address: %w", err)
		}
		if len(walletAddresses) != 1 {
			return nil, fmt.Errorf("expected exactly one wallet address, got %d", len(walletAddresses))
		}
		wallet := walletAddresses[0].PublicKey

		tokenPrograms, err := GetAddresses(ctx, args, []Lookup{lookup.TokenProgram}, derivedTableMap, client)
		if err != nil {
			return nil, fmt.Errorf("error resolving token program address: %w", err)
		}

		mints, err := GetAddresses(ctx, args, []Lookup{lookup.MintAddress}, derivedTableMap, client)
		if err != nil {
			return nil, fmt.Errorf("error resolving mint address: %w", err)
		}
		if len(tokenPrograms) != len(mints) {
			return nil, fmt.Errorf("expected equal number of token programs and mints, got %d tokenPrograms and %d mints", len(tokenPrograms), len(mints))
		}

		for i := range tokenPrograms {
			tokenProgram := tokenPrograms[i].PublicKey
			mint := mints[i].PublicKey

			ataAddress, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, mint, wallet)
			if err != nil {
				return nil, fmt.Errorf("error deriving ATA: %w", err)
			}

			_, err = client.GetAccountInfoWithOpts(ctx, ataAddress, &rpc.GetAccountInfoOpts{
				Encoding:   "base64",
				Commitment: rpc.CommitmentFinalized,
			})
			if err == nil {
				logger.Info("ATA already exists, skipping creation.", lookup.Location)
				continue
			}
			if !strings.Contains(err.Error(), "not found") {
				return nil, fmt.Errorf("error reading account info for ATA: %w", err)
			}

			ins, _, err := tokens.CreateAssociatedTokenAccount(tokenProgram, mint, wallet, feePayer)
			if err != nil {
				return nil, fmt.Errorf("error creating associated token account: %w", err)
			}
			createATAInstructions = append(createATAInstructions, ins)
		}
	}

	return createATAInstructions, nil
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

	s.lggr.Debug("Resolving account addresses", "contract", contractName, "method", method)
	// Resolve account metas
	accounts, err := GetAddresses(ctx, args, methodConfig.Accounts, derivedTableMap, s.client)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error resolving account addresses: %w", err), debugID)
	}

	feePayer, err := solana.PublicKeyFromBase58(methodConfig.FromAddress)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error parsing fee payer address: %w", err), debugID)
	}

	s.lggr.Debug("Creating ATAs", "contract", contractName, "method", method)
	createATAinstructions, err := CreateATAs(ctx, args, methodConfig.ATAs, derivedTableMap, s.client, programConfig.IDL, feePayer, s.lggr)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error resolving account addresses: %w", err), debugID)
	}

	s.lggr.Debug("Filtering lookup table addresses", "contract", contractName, "method", method)
	// Filter the lookup table addresses based on which accounts are actually used
	filteredLookupTableMap := s.FilterLookupTableAddresses(accounts, derivedTableMap, staticTableMap)

	// Transform args if necessary
	if methodConfig.ArgsTransform != "" {
		transformFunc, tfErr := FindTransform(methodConfig.ArgsTransform)
		if tfErr != nil {
			return errorWithDebugID(fmt.Errorf("error finding transform function: %w", tfErr), debugID)
		}
		s.lggr.Debug("Applying args transformation", "contract", contractName, "method", method)
		args, err = transformFunc(ctx, s, args, accounts, toAddress)
		if err != nil {
			return errorWithDebugID(fmt.Errorf("error transforming args: %w", err), debugID)
		}
	}

	// Prepare transaction
	programID, err := solana.PublicKeyFromBase58(toAddress)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error parsing program ID: %w", err), debugID)
	}

	s.lggr.Debug("Encoding transaction payload", "contract", contractName, "method", method)
	encodedPayload, err := s.encoder.Encode(ctx, args, codec.WrapItemType(true, contractName, method))

	if err != nil {
		return errorWithDebugID(fmt.Errorf("error encoding transaction payload: %w", err), debugID)
	}

	discriminator := GetDiscriminator(methodConfig.ChainSpecificName)
	encodedPayload = append(discriminator[:], encodedPayload...)

	if len(createATAinstructions) > 0 {
		err = s.handleATACreation(ctx, createATAinstructions, methodConfig, contractName, method, feePayer)
		if err != nil {
			return errorWithDebugID(fmt.Errorf("error creating ATAs: %w", err), debugID)
		}
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

	s.lggr.Debug("Sending main transaction", "contract", contractName, "method", method)
	// Enqueue transaction
	if err = s.txm.Enqueue(ctx, methodConfig.FromAddress, tx, &transactionID, blockhash.Value.LastValidBlockHeight); err != nil {
		return errorWithDebugID(fmt.Errorf("error enqueuing transaction: %w", err), debugID)
	}

	return nil
}

// GetTransactionStatus returns the current status of a transaction in the underlying chain's TXM.
func (s *SolanaChainWriterService) GetTransactionStatus(ctx context.Context, transactionID string) (types.TransactionStatus, error) {
	s.lggr.Debug("Fetching transaction status", "transactionID", transactionID)
	return s.txm.GetTransactionStatus(ctx, transactionID)
}

// GetFeeComponents retrieves the associated gas costs for executing a transaction.
func (s *SolanaChainWriterService) GetFeeComponents(ctx context.Context) (*types.ChainFeeComponents, error) {
	if s.ge == nil {
		return nil, fmt.Errorf("gas estimator not available")
	}

	s.lggr.Debug("Fetching fee components")
	fee := s.ge.BaseComputeUnitPrice()
	return &types.ChainFeeComponents{
		ExecutionFee:        new(big.Int).SetUint64(fee),
		DataAvailabilityFee: nil,
	}, nil
}

func (s *SolanaChainWriterService) ResolveLookupTables(ctx context.Context, args any, lookupTables LookupTables) (map[string]map[string][]*solana.AccountMeta, map[solana.PublicKey]solana.PublicKeySlice, error) {
	derivedTableMap := make(map[string]map[string][]*solana.AccountMeta)
	staticTableMap := make(map[solana.PublicKey]solana.PublicKeySlice)

	// Read derived lookup tables
	for _, derivedLookup := range lookupTables.DerivedLookupTables {
		// Load the lookup table - note: This could be multiple tables if the lookup is a PDALookups that resolves to more
		// than one address
		lookupTableMap, err := s.loadTable(ctx, args, derivedLookup)
		if derivedLookup.Optional && err != nil {
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

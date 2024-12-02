package chainwriter

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/gagliardetto/solana-go"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
)

type SolanaChainWriterService struct {
	reader client.Reader
	txm    txm.TxManager
	ge     fees.Estimator
	config ChainWriterConfig
	codecs map[string]types.Codec
}

type ChainWriterConfig struct {
	Programs map[string]ProgramConfig
}

type ProgramConfig struct {
	Methods map[string]MethodConfig
	IDL     string
}

type MethodConfig struct {
	FromAddress        string
	InputModifications commoncodec.ModifiersConfig
	ChainSpecificName  string
	LookupTables       LookupTables
	Accounts           []Lookup
	// Location in the args where the debug ID is stored
	DebugIDLocation string
}

func NewSolanaChainWriterService(reader client.Reader, txm txm.TxManager, ge fees.Estimator, config ChainWriterConfig) (*SolanaChainWriterService, error) {
	codecs, err := parseIDLCodecs(config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse IDL codecs: %w", err)
	}

	return &SolanaChainWriterService{
		reader: reader,
		txm:    txm,
		ge:     ge,
		config: config,
		codecs: codecs,
	}, nil
}

func parseIDLCodecs(config ChainWriterConfig) (map[string]types.Codec, error) {
	codecs := make(map[string]types.Codec)
	for program, programConfig := range config.Programs {
		var idl codec.IDL
		if err := json.Unmarshal([]byte(programConfig.IDL), &idl); err != nil {
			return nil, fmt.Errorf("failed to unmarshal IDL: %w", err)
		}
		idlCodec, err := codec.NewIDLInstructionsCodec(idl, binary.LittleEndian())
		if err != nil {
			return nil, fmt.Errorf("failed to create codec from IDL: %w", err)
		}
		for method, methodConfig := range programConfig.Methods {
			if methodConfig.InputModifications != nil {
				modConfig, err := methodConfig.InputModifications.ToModifier(codec.DecoderHooks...)
				if err != nil {
					return nil, fmt.Errorf("failed to create input modifications: %w", err)
				}
				// add mods to codec
				idlCodec, err = codec.NewNamedModifierCodec(idlCodec, method, modConfig)
				if err != nil {
					return nil, fmt.Errorf("failed to create named codec: %w", err)
				}
			}
		}
		codecs[program] = idlCodec
	}
	return codecs, nil
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
// GetAddresses resolves account addresses from various `Lookup` configurations to build the required `solana.AccountMeta` list
// for Solana transactions.
func GetAddresses(ctx context.Context, args any, accounts []Lookup, derivedTableMap map[string]map[string][]*solana.AccountMeta, reader client.Reader) ([]*solana.AccountMeta, error) {
	var addresses []*solana.AccountMeta
	for _, accountConfig := range accounts {
		meta, err := accountConfig.Resolve(ctx, args, derivedTableMap, reader)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, meta...)
	}
	return addresses, nil
}

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
				fmt.Errorf("error parsing lookup table key: %w", err)
			}

			// Collect public keys that are actually used
			var usedAddresses solana.PublicKeySlice
			for _, meta := range metas {
				if _, exists := usedAccounts[meta.PublicKey.String()]; exists {
					usedAddresses = append(usedAddresses, meta.PublicKey)
				}
			}

			// Add to the filtered map if there are any used addresses
			if len(usedAddresses) > 0 {
				filteredLookupTables[tableKey] = usedAddresses
			}
		}
	}

	// Filter static lookup tables
	for tableKey, addresses := range staticTableMap {
		var usedAddresses solana.PublicKeySlice
		for _, staticAddress := range addresses {
			if _, exists := usedAccounts[staticAddress.String()]; exists {
				usedAddresses = append(usedAddresses, staticAddress)
			}
		}

		// Add to the filtered map if there are any used addresses
		if len(usedAddresses) > 0 {
			filteredLookupTables[tableKey] = usedAddresses
		}
	}

	return filteredLookupTables
}

func (s *SolanaChainWriterService) SubmitTransaction(ctx context.Context, contractName, method string, args any, transactionID string, toAddress string, meta *types.TxMeta, value *big.Int) error {
	programConfig := s.config.Programs[contractName]
	methodConfig := programConfig.Methods[method]

	// Configure debug ID
	debugID := ""
	if methodConfig.DebugIDLocation != "" {
		debugID, err := GetDebugIDAtLocation(args, methodConfig.DebugIDLocation)
		if err != nil {
			return errorWithDebugID(fmt.Errorf("error getting debug ID from input args: %w", err), debugID)
		}
	}

	// Fetch derived and static table maps
	derivedTableMap, staticTableMap, err := s.ResolveLookupTables(ctx, args, methodConfig.LookupTables)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error getting lookup tables: %w", err), debugID)
	}

	// Resolve account metas
	accounts, err := GetAddresses(ctx, args, methodConfig.Accounts, derivedTableMap, s.reader)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error resolving account addresses: %w", err), debugID)
	}

	// Filter the lookup table addresses based on which accounts are actually used
	filteredLookupTableMap := s.FilterLookupTableAddresses(accounts, derivedTableMap, staticTableMap)

	// Fetch latest blockhash
	blockhash, err := s.reader.LatestBlockhash(ctx)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error fetching latest blockhash: %w", err), debugID)
	}

	// Prepare transaction
	programId, err := solana.PublicKeyFromBase58(contractName)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error parsing program ID: %w", err), debugID)
	}

	feePayer, err := solana.PublicKeyFromBase58(methodConfig.FromAddress)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error parsing fee payer address: %w", err), debugID)
	}

	codec := s.codecs[contractName]
	encodedPayload, err := codec.Encode(ctx, args, method)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error encoding transaction payload: %w", err), debugID)
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			solana.NewInstruction(programId, accounts, encodedPayload),
		},
		blockhash.Value.Blockhash,
		solana.TransactionPayer(feePayer),
		solana.TransactionAddressTables(filteredLookupTableMap),
	)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error constructing transaction: %w", err), debugID)
	}

	// Enqueue transaction
	if err = s.txm.Enqueue(ctx, accounts[0].PublicKey.String(), tx, &transactionID); err != nil {
		return errorWithDebugID(fmt.Errorf("error enqueuing transaction: %w", err), debugID)
	}

	return nil
}

var (
	_ services.Service  = &SolanaChainWriterService{}
	_ types.ChainWriter = &SolanaChainWriterService{}
)

// GetTransactionStatus returns the current status of a transaction in the underlying chain's TXM.
func (s *SolanaChainWriterService) GetTransactionStatus(ctx context.Context, transactionID string) (types.TransactionStatus, error) {
	return s.txm.GetTransactionStatus(ctx, transactionID)
}

// GetFeeComponents retrieves the associated gas costs for executing a transaction.
func (s *SolanaChainWriterService) GetFeeComponents(ctx context.Context) (*types.ChainFeeComponents, error) {
	if s.ge == nil {
		return nil, fmt.Errorf("gas estimator not available")
	}

	fee := s.ge.BaseComputeUnitPrice()
	return &types.ChainFeeComponents{
		ExecutionFee:        big.NewInt(int64(fee)),
		DataAvailabilityFee: nil,
	}, nil
}

func (s *SolanaChainWriterService) Start(context.Context) error {
	return nil
}

func (s *SolanaChainWriterService) Close() error {
	return nil
}

func (s *SolanaChainWriterService) HealthReport() map[string]error {
	return nil
}

func (s *SolanaChainWriterService) Name() string {
	return ""
}

func (s *SolanaChainWriterService) Ready() error {
	return nil
}

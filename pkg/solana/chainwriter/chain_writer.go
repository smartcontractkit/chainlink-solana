package chainwriter

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

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
	txm    txm.Txm
	ge     fees.Estimator
	codec  types.Codec
	config ChainWriterConfig
}

type ChainWriterConfig struct {
	Programs map[string]ProgramConfig
}

type ProgramConfig struct {
	Methods map[string]MethodConfig
	IDL     string
}

type MethodConfig struct {
	InputModifications   commoncodec.ModifiersConfig
	EncodedTypeIDL       string
	DataType             reflect.Type
	DecodedTypeName      string
	ChainSpecificName    string
	ReadableLookupTables []ReadableLookupTable
	Accounts             []Lookup
	LookupTables         []LookupTable
	// Location in the decoded data where the debug ID is stored
	DebugIDLocation string
}

type Lookup interface {
}

type AccountConstant struct {
	Name       string
	Address    string
	IsSigner   bool
	IsWritable bool
}

type AccountLookup struct {
	Name       string
	Location   string
	IsSigner   bool
	IsWritable bool
}

type PDALookup struct {
	Name         string
	PublicKey    Lookup
	AddressSeeds []Lookup
	ValueSeeds   []ValueLookup
	IsSigner     bool
	IsWritable   bool
}

type ValueLookup struct {
	Location string
}

type LookupTable struct {
	Name       string
	Address    solana.PublicKey
	Identifier Lookup
}

type ReadableLookupTable struct {
	Name           string
	Address        solana.PublicKey
	Identifier     Lookup
	EncodedTypeIDL string
	Locations      []AccountLookup
	DecodedType    reflect.Type
}

func NewSolanaChainWriterService(reader client.Reader, txm txm.Txm, ge fees.Estimator, config ChainWriterConfig) *SolanaChainWriterService {
	return &SolanaChainWriterService{
		reader: reader,
		txm:    txm,
		ge:     ge,
		config: config,
	}
}

/*
GetAddresses resolves account addresses from various `Lookup` configurations to build the required `solana.AccountMeta` list
for Solana transactions. It handles constant addresses, dynamic lookups, program-derived addresses (PDAs), and lookup tables.

### Parameters:
- `ctx`: Context for request lifecycle management.
- `decoded`: Decoded data used for dynamic lookups.
- `accounts`: List of `Lookup` configurations specifying how addresses are derived.
- `readableTableMap`: Map of pre-loaded lookup table addresses.
- `debugID`: Debug identifier for tracing errors.

### Return:
- A slice of `solana.AccountMeta` containing derived addresses and associated metadata.

### Account Types:
1. **AccountConstant**:
  - A fixed address, provided in Base58 format, converted into a `solana.PublicKey`.
  - Example: A pre-defined fee payer or system account.

2. **AccountLookup**:
  - Dynamically derived from decoded data using a specified location path (e.g., `user.walletAddress`).
  - If the lookup table is pre-loaded, the address is fetched from `readableTableMap`.

3. **PDALookup**:
  - Generates Program Derived Addresses (PDA) by combining a derived public key with one or more seeds.
  - Seeds can be `AddressSeeds` (public keys from the decoded data) or `ValueSeeds` (byte arrays).
  - Ensures there is only one public key if multiple seeds are provided.

### Error Handling:
- Errors are wrapped with the `debugID` for easier tracing.
*/
// GetAddresses resolves account addresses from various `Lookup` configurations to build the required `solana.AccountMeta` list
// for Solana transactions.
func (s *SolanaChainWriterService) GetAddresses(ctx context.Context, decoded any, accounts []Lookup, readableTableMap map[string][]*solana.AccountMeta, debugID string) ([]*solana.AccountMeta, error) {
	var addresses []*solana.AccountMeta
	for _, accountConfig := range accounts {
		meta, err := s.getAccountMeta(ctx, decoded, accountConfig, readableTableMap, debugID)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, meta...)
	}
	return addresses, nil
}

// getAccountMeta processes a single account configuration and returns the corresponding `solana.AccountMeta` slice.
func (s *SolanaChainWriterService) getAccountMeta(ctx context.Context, decoded any, accountConfig Lookup, readableTableMap map[string][]*solana.AccountMeta, debugID string) ([]*solana.AccountMeta, error) {
	switch lookup := accountConfig.(type) {
	case AccountConstant:
		return s.handleAccountConstant(lookup, debugID)
	case AccountLookup:
		return s.handleAccountLookup(decoded, lookup, readableTableMap, debugID)
	case PDALookup:
		return s.handlePDALookup(ctx, decoded, lookup, readableTableMap, debugID)
	default:
		return nil, errorWithDebugID(fmt.Errorf("unsupported account type: %T", lookup), debugID)
	}
}

// handleAccountConstant processes an `AccountConstant` and returns the corresponding `solana.AccountMeta`.
func (s *SolanaChainWriterService) handleAccountConstant(lookup AccountConstant, debugID string) ([]*solana.AccountMeta, error) {
	address, err := solana.PublicKeyFromBase58(lookup.Address)
	if err != nil {
		return nil, errorWithDebugID(fmt.Errorf("error getting account from constant: %w", err), debugID)
	}
	return []*solana.AccountMeta{
		{
			PublicKey:  address,
			IsSigner:   lookup.IsSigner,
			IsWritable: lookup.IsWritable,
		},
	}, nil
}

// handleAccountLookup processes an `AccountLookup` by either fetching from the lookup table or dynamically deriving the address.
func (s *SolanaChainWriterService) handleAccountLookup(decoded any, lookup AccountLookup, readableTableMap map[string][]*solana.AccountMeta, debugID string) ([]*solana.AccountMeta, error) {
	if derivedAddresses, ok := readableTableMap[lookup.Name]; ok {
		return derivedAddresses, nil
	}

	derivedAddresses, err := GetAddressAtLocation(decoded, lookup.Location, debugID)
	if err != nil {
		return nil, errorWithDebugID(fmt.Errorf("error getting account from lookup: %w", err), debugID)
	}

	var metas []*solana.AccountMeta
	for _, address := range derivedAddresses {
		metas = append(metas, &solana.AccountMeta{
			PublicKey:  address,
			IsSigner:   lookup.IsSigner,
			IsWritable: lookup.IsWritable,
		})
	}
	return metas, nil
}

// handlePDALookup processes a `PDALookup` by resolving seeds and generating the PDA address.
func (s *SolanaChainWriterService) handlePDALookup(ctx context.Context, decoded any, lookup PDALookup, readableTableMap map[string][]*solana.AccountMeta, debugID string) ([]*solana.AccountMeta, error) {
	publicKeys, err := s.GetAddresses(ctx, decoded, []Lookup{lookup.PublicKey}, readableTableMap, debugID)
	if err != nil {
		return nil, errorWithDebugID(fmt.Errorf("error getting public key for PDALookup: %w", err), debugID)
	}

	seeds, err := s.getSeedBytes(ctx, lookup, decoded, readableTableMap, debugID)
	if err != nil {
		return nil, errorWithDebugID(fmt.Errorf("error getting seeds for PDALookup: %w", err), debugID)
	}

	return s.generatePDAs(publicKeys, seeds, lookup, debugID)
}

// generatePDAs generates program-derived addresses (PDAs) from public keys and seeds.
func (s *SolanaChainWriterService) generatePDAs(publicKeys []*solana.AccountMeta, seeds [][]byte, lookup PDALookup, debugID string) ([]*solana.AccountMeta, error) {
	if len(seeds) > 1 && len(publicKeys) > 1 {
		return nil, errorWithDebugID(fmt.Errorf("multiple public keys and multiple seeds are not allowed"), debugID)
	}

	var addresses []*solana.AccountMeta
	for _, publicKeyMeta := range publicKeys {
		address, _, err := solana.FindProgramAddress(seeds, publicKeyMeta.PublicKey)
		if err != nil {
			return nil, errorWithDebugID(fmt.Errorf("error finding program address: %w", err), debugID)
		}
		addresses = append(addresses, &solana.AccountMeta{
			PublicKey:  address,
			IsSigner:   lookup.IsSigner,
			IsWritable: lookup.IsWritable,
		})
	}
	return addresses, nil
}

func (s *SolanaChainWriterService) getReadableTableMap(ctx context.Context, decoded any, lookupTables []ReadableLookupTable, debugID string) ([]*solana.AccountMeta, map[string][]*solana.AccountMeta, error) {
	var addresses []*solana.AccountMeta
	var addressMap = make(map[string][]*solana.AccountMeta)
	for _, lookup := range lookupTables {
		lookupTableAddresses, err := s.LoadTable(lookup, ctx, s.reader, addressMap, debugID)
		if err != nil {
			return nil, nil, errorWithDebugID(fmt.Errorf("error loading lookup table: %w", err), debugID)
		}
		for name, addressList := range lookupTableAddresses {
			for _, address := range addressList {
				addresses = append(addresses, address)
			}
			addressMap[name] = addressList
		}
	}
	return addresses, addressMap, nil
}

// getSeedBytes extracts the seeds for the PDALookup.
// It handles both AddressSeeds (which are public keys) and ValueSeeds (which are byte arrays from decoded data).
func (s *SolanaChainWriterService) getSeedBytes(ctx context.Context, lookup PDALookup, decoded any, readableTableMap map[string][]*solana.AccountMeta, debugID string) ([][]byte, error) {
	var seedBytes [][]byte

	// Process AddressSeeds first (e.g., public keys)
	for _, seed := range lookup.AddressSeeds {
		// Get the address(es) at the seed location
		seedAddresses, err := s.GetAddresses(ctx, decoded, []Lookup{seed}, readableTableMap, debugID)
		if err != nil {
			return nil, errorWithDebugID(fmt.Errorf("error getting address seed: %w", err), debugID)
		}

		// Add each address seed as bytes
		for _, address := range seedAddresses {
			seedBytes = append(seedBytes, address.PublicKey.Bytes())
		}
	}

	// Process ValueSeeds (e.g., raw byte values found in decoded data)
	for _, valueSeed := range lookup.ValueSeeds {
		// Get the byte array value at the seed location
		values, err := GetValueAtLocation(decoded, valueSeed.Location)
		if err != nil {
			return nil, errorWithDebugID(fmt.Errorf("error getting value seed: %w", err), debugID)
		}

		// Add each value seed (which is a byte array)
		seedBytes = append(seedBytes, values...)
	}

	return seedBytes, nil
}

// LoadTable reads the lookup table from the Solana chain, decodes it into the specified type, and returns a slice of addresses.
func (s *SolanaChainWriterService) LoadTable(rlt ReadableLookupTable, ctx context.Context, reader client.Reader, readableTableMap map[string][]*solana.AccountMeta, debugID string) (map[string][]*solana.AccountMeta, error) {
	// Fetch the account data using client.Reader.GetAccountInfoWithOpts
	accountInfo, err := reader.GetAccountInfoWithOpts(ctx, rlt.Address, &rpc.GetAccountInfoOpts{
		Encoding:   "base64", // or "jsonParsed" if needed
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		return nil, errorWithDebugID(fmt.Errorf("failed to get account info: %w", err), debugID)
	}
	if accountInfo == nil || accountInfo.Value == nil {
		return nil, errorWithDebugID(fmt.Errorf("no data found for account: %s", rlt.Address.String()), debugID)
	}

	// Decode the table data using the codec/EncodedTypeIDL
	decodedData, err := rlt.DecodeTableData(accountInfo.Value.Data.GetBinary(), debugID)
	if err != nil {
		return nil, errorWithDebugID(fmt.Errorf("failed to decode table data: %w", err), debugID)
	}

	// Convert the decoded entries into solana.PublicKey and return them
	var addresses map[string][]*solana.AccountMeta
	for _, location := range rlt.Locations {
		derivedAddresses, err := s.GetAddresses(ctx, decodedData, []Lookup{location}, readableTableMap, debugID)
		if err != nil {
			return nil, errorWithDebugID(fmt.Errorf("error getting addresses from decoded data: %w", err), debugID)
		}
		addresses[fmt.Sprintf("%s.%s", rlt.Name, location.Name)] = derivedAddresses
	}

	return addresses, nil
}

// DecodeTableData decodes the raw table data using the EncodedTypeIDL and the specified DecodedType.
func (rlt *ReadableLookupTable) DecodeTableData(data []byte, debugID string) (any, error) {
	var idl codec.IDL
	err := json.Unmarshal([]byte(rlt.EncodedTypeIDL), &idl)
	if err != nil {
		return nil, errorWithDebugID(fmt.Errorf("error unmarshalling IDL: %w", err), debugID)
	}

	cwCodec, err := codec.NewIDLAccountCodec(idl, binary.LittleEndian())
	if err != nil {
		return nil, errorWithDebugID(fmt.Errorf("error creating new IDLAccountCodec: %w", err), debugID)
	}

	decoded := reflect.New(rlt.DecodedType).Interface()

	err = cwCodec.Decode(nil, data, decoded, "")
	if err != nil {
		return nil, errorWithDebugID(fmt.Errorf("error decoding table data: %w", err), debugID)
	}

	return decoded, nil
}

func (s *SolanaChainWriterService) GetLookupTables(ctx context.Context, decoded any, lookupTables []LookupTable, readableTableMap map[string][]*solana.AccountMeta, debugID string) (map[solana.PublicKey]solana.PublicKeySlice, error) {
	tables := make(map[solana.PublicKey]solana.PublicKeySlice)
	for _, lookupTable := range lookupTables {
		// Prevent nested lookup tables.
		if reflect.TypeOf(lookupTable.Identifier) == reflect.TypeOf(LookupTable{}) {
			return nil, errorWithDebugID(fmt.Errorf("nested lookup tables are not supported"), debugID)
		}

		// Get the public keys for the lookup table's identifier (can be one or more).
		ids, err := s.GetAddresses(ctx, decoded, []Lookup{lookupTable.Identifier}, readableTableMap, debugID)
		if err != nil {
			return nil, errorWithDebugID(fmt.Errorf("error getting accounts from lookup table: %w", err), debugID)
		}

		// Convert the ids to a solana.PublicKeySlice and add to the lookup table map.
		addresses := make(solana.PublicKeySlice, len(ids))
		for i, accountMeta := range ids {
			addresses[i] = accountMeta.PublicKey
		}
		tables[lookupTable.Address] = addresses
	}
	return tables, nil
}

func (s *SolanaChainWriterService) SubmitTransaction(ctx context.Context, contractName, method string, args any, transactionID string, toAddress string, meta *types.TxMeta, value *big.Int) error {
	programConfig := s.config.Programs[contractName]
	methodConfig := programConfig.Methods[method]

	data, ok := args.([]byte)
	if !ok {
		return fmt.Errorf("Unable to convert args to []byte")
	}

	// decode data
	var idl codec.IDL
	err := json.Unmarshal([]byte(methodConfig.EncodedTypeIDL), &idl)
	if err != nil {
		return fmt.Errorf("error unmarshalling IDL: %w", err)
	}
	cwCodec, err := codec.NewIDLAccountCodec(idl, binary.LittleEndian())
	if err != nil {
		return fmt.Errorf("error creating new IDLAccountCodec: %w", err)
	}

	// Create an instance of the type defined by methodConfig.DataType
	decoded := reflect.New(methodConfig.DataType).Interface()
	err = cwCodec.Decode(ctx, data, decoded, methodConfig.DecodedTypeName)

	debugID := ""
	if methodConfig.DebugIDLocation != "" {
		debugID, err = GetDebugIDAtLocation(decoded, methodConfig.DebugIDLocation)
		if err != nil {
			return errorWithDebugID(fmt.Errorf("error getting debug ID from decoded data: %w", err), debugID)
		}
	}
	readableTableAccounts, readableTableMap, err := s.getReadableTableMap(ctx, decoded, methodConfig.ReadableLookupTables, debugID)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error getting readable table map: %w", err), debugID)
	}
	accounts, err := s.GetAddresses(ctx, decoded, methodConfig.Accounts, readableTableMap, debugID)
	accounts = append(accounts, readableTableAccounts...)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error getting addresses from decoded data: %w", err), debugID)
	}
	lookupTables, err := s.GetLookupTables(ctx, decoded, methodConfig.LookupTables, readableTableMap, debugID)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error getting lookup tables from decoded data: %w", err), debugID)
	}

	blockhash, err := s.reader.LatestBlockhash(ctx)

	programId, err := solana.PublicKeyFromBase58(contractName)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("Error getting programId: %w", err), debugID)
	}

	// This isn't a real method, TBD how we will get this
	feePayer := accounts[0]

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			solana.NewInstruction(programId, accounts, data),
		},
		blockhash.Value.Blockhash,
		solana.TransactionPayer(feePayer.PublicKey),
		solana.TransactionAddressTables(lookupTables),
	)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error creating new transaction: %w", err), debugID)
	}

	if err = s.txm.Enqueue(ctx, accounts[0].PublicKey.String(), tx); err != nil {
		return errorWithDebugID(fmt.Errorf("error on sending trasnaction to TXM: %w", err), debugID)
	}
	return nil
}

var (
	_ services.Service  = &SolanaChainWriterService{}
	_ types.ChainWriter = &SolanaChainWriterService{}
)

// GetTransactionStatus returns the current status of a transaction in the underlying chain's TXM.
func (s *SolanaChainWriterService) GetTransactionStatus(ctx context.Context, transactionID string) (types.TransactionStatus, error) {
	return types.Unknown, nil
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

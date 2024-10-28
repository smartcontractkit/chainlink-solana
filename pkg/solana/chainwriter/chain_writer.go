package chainwriter

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"

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
	InputModifications commoncodec.ModifiersConfig
	EncodedTypeIDL     string
	DataType           any
	DecodedTypeName    string
	ChainSpecificName  string
	Accounts           []Account
	LookupTables       []LookupTable
}

type Account interface {
}

type AccountConstant struct {
	Address    string
	IsSigner   bool
	IsWritable bool
}

type AccountLookup struct {
	Location   string
	IsSigner   bool
	IsWritable bool
}

type PDALookup struct {
	PublicKey  solana.PublicKey
	Seeds      [][]byte
	IsSigner   bool
	IsWritable bool
}

type LookupTable struct {
	Address        solana.PublicKey
	Identifier     Account
	AccountIndices []int
}

func (s *SolanaChainWriterService) GetAddresses(decoded any, accounts []Account) ([]*solana.AccountMeta, error) {
	var addresses []*solana.AccountMeta
	for _, accountConfig := range accounts {
		switch lookupType := accountConfig.(type) {
		case AccountConstant:
			address, err := solana.PublicKeyFromBase58(lookupType.Address)
			if err != nil {
				return nil, fmt.Errorf("error getting account from constant: %w", err)
			}
			addresses = append(addresses, &solana.AccountMeta{
				PublicKey:  address,
				IsSigner:   lookupType.IsSigner,
				IsWritable: lookupType.IsWritable,
			})
		case AccountLookup:
			derivedAddresses, err := GetAddressAtLocation(decoded, lookupType.Location)
			if err != nil {
				return nil, fmt.Errorf("error getting account from lookup: %w", err)
			}
			for _, address := range derivedAddresses {
				addresses = append(addresses, &solana.AccountMeta{
					PublicKey:  address,
					IsSigner:   lookupType.IsSigner,
					IsWritable: lookupType.IsWritable,
				})
			}
		case PDALookup:
			pda, _, err := solana.FindProgramAddress(lookupType.Seeds, lookupType.PublicKey)
			if err != nil {
				return nil, fmt.Errorf("error finding program address: %w", err)
			}
			addresses = append(addresses, &solana.AccountMeta{
				PublicKey:  pda,
				IsSigner:   lookupType.IsSigner,
				IsWritable: lookupType.IsWritable,
			})
		default:
			return nil, fmt.Errorf("unsupported account type: %T", lookupType)
		}
	}
	return addresses, nil
}

func (s *SolanaChainWriterService) GetLookupTables(decoded any, accounts []*solana.AccountMeta, lookupTables []LookupTable) (map[solana.PublicKey]solana.PublicKeySlice, error) {
	tables := make(map[solana.PublicKey]solana.PublicKeySlice)
	for _, lookupTable := range lookupTables {
		if reflect.TypeOf(lookupTable.Identifier) == reflect.TypeOf(LookupTable{}) {
			return nil, fmt.Errorf("nested lookup tables are not supported")
		}
		// ids, err := s.GetAddresses(decoded, []Account{lookupTable.Identifier})
		// if err != nil {
		// 	return nil, fmt.Errorf("error getting account from lookup table: %w", err)
		// }
		addresses := make(solana.PublicKeySlice, len(lookupTable.AccountIndices))
		for i, index := range lookupTable.AccountIndices {
			addresses[i] = accounts[index].PublicKey
		}
		tables[lookupTable.Address] = addresses
	}
	return tables, nil
}

func NewSolanaChainWriterService(reader client.Reader, txm txm.Txm, ge fees.Estimator, config ChainWriterConfig) *SolanaChainWriterService {
	return &SolanaChainWriterService{
		reader: reader,
		txm:    txm,
		ge:     ge,
		config: config,
	}
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
	decoded := reflect.New(reflect.TypeOf(methodConfig.DataType)).Interface()
	err = cwCodec.Decode(ctx, data, decoded, methodConfig.DecodedTypeName)

	accounts, err := s.GetAddresses(decoded, methodConfig.Accounts)
	if err != nil {
		return fmt.Errorf("error getting addresses from decoded data: %w", err)
	}
	lookupTables, err := s.GetLookupTables(decoded, accounts, methodConfig.LookupTables)
	if err != nil {
		return fmt.Errorf("error getting lookup tables from decoded data: %w", err)
	}

	blockhash, err := s.reader.LatestBlockhash(ctx)

	programId, err := solana.PublicKeyFromBase58(contractName)
	if err != nil {
		return fmt.Errorf("Error getting programId: %w", err)
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
		return fmt.Errorf("error creating new transaction: %w", err)
	}

	if err = s.txm.Enqueue(ctx, accounts[0].PublicKey.String(), tx); err != nil {
		return fmt.Errorf("error on sending trasnaction to TXM: %w", err)
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

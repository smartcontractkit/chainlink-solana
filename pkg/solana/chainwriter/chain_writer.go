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
	AddressLocations   []string
	Signers            []solana.AccountMeta
	Writables          []solana.AccountMeta
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

	accounts, err := GetAddressesFromDecodedData(decoded, methodConfig.AddressLocations)
	if err != nil {
		return fmt.Errorf("error getting addresses from decoded data: %w", err)
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

func getAccounts(contractName string, method string, args any) (accounts []*solana.AccountMeta, feePayer *solana.AccountMeta, err error) {
	// TO DO: Use on-chain team's helper functions to get the accounts from CCIP related metadata.
	return nil, nil, nil
}

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

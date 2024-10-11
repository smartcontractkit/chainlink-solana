package chainwriter

import (
	"context"
	"fmt"
	"math/big"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
)

type SolanaChainWriterService struct {
	reader client.Reader
	txm    txm.Txm
	ge     fees.Estimator
}

type ChainWriterConfig struct {
	Programs map[string]ProgramConfig `json:"contracts" toml:"contracts"`
}

type ProgramConfig struct {
	Methods map[string]MethodConfig `json:"methods" toml:"methods"`
}

type MethodConfig struct {
	InputModifications codec.ModifiersConfig `json:"inputModifications,omitempty"`
	ChainSpecificName  string                `json:"chainSpecificName"`
}

func NewSolanaChainWriterService(reader client.Reader, txm txm.Txm, ge fees.Estimator) *SolanaChainWriterService {
	return &SolanaChainWriterService{
		reader: reader,
		txm:    txm,
		ge:     ge,
	}
}

var (
	_ services.Service  = &SolanaChainWriterService{}
	_ types.ChainWriter = &SolanaChainWriterService{}
)

func (s *SolanaChainWriterService) SubmitTransaction(ctx context.Context, contractName, method string, args any, transactionID string, toAddress string, meta *types.TxMeta, value *big.Int) error {
	data, ok := args.([]byte)
	if !ok {
		return fmt.Errorf("Unable to convert args to []byte")
	}

	blockhash, err := s.reader.LatestBlockhash()

	programId, err := solana.PublicKeyFromBase58(contractName)
	if err != nil {
		return fmt.Errorf("Error getting programId: %w", err)
	}

	// placeholder method to get accounts
	accounts, feePayer, err := getAccounts(contractName, method, args)
	if err != nil || len(accounts) == 0 {
		return fmt.Errorf("Error getting accounts: %w", err)
	}

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

	if err = s.txm.Enqueue(accounts[0].PublicKey.String(), tx); err != nil {
		return fmt.Errorf("error on sending trasnaction to TXM: %w", err)
	}
	return nil
}

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

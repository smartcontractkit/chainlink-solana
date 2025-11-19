package chainwriter

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	chainwriterutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/chain_writer_utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
)

const (
	ServiceName = "SolanaChainWriter"
)

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
	Programs map[string]chainwriterutils.ProgramConfig `json:"programs"`
}

func NewSolanaChainWriterService(logger logger.Logger, client client.MultiClient, txm txm.TxManager, ge fees.Estimator, config ChainWriterConfig) (*SolanaChainWriterService, error) {
	w := SolanaChainWriterService{
		lggr:   logger,
		client: client,
		txm:    txm,
		ge:     ge,
		config: config,
	}

	// Parse programs and create codec
	parsed, encoder, err := chainwriterutils.ParseProgramsToCodec(config.Programs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse programs: %w", err)
	}

	w.parsed = parsed
	w.encoder = encoder

	w.lggr.Info("SolanaChainWriterService initialized")
	return &w, nil
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

	return chainwriterutils.SubmitTransactionImpl(ctx, chainwriterutils.SubmitTransactionParams{
		ProgramConfig: programConfig,
		ContractName:  contractName,
		Method:        method,
		Args:          args,
		TransactionID: transactionID,
		ToAddress:     toAddress,
		Client:        s.client,
		TxManager:     s.txm,
		Encoder:       s.encoder,
		Lggr:          s.lggr,
	})
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

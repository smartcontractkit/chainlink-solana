package api

import (
	"context"
	"time"

	"github.com/gagliardetto/solana-go"
	solanago "github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types/core"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

// TxState represents the different states that a submitted Solana transaction can be in.
type TxState int

// tx not found
// < tx errored
// < tx broadcasted
// < tx processed
// < tx confirmed
// < tx finalized
// < tx fatallyErrored
const (
	NotFound TxState = iota
	Errored
	AwaitingBroadcast
	Broadcasted
	Processed
	Confirmed
	Finalized
	FatallyErrored
)

type TransactionRequest struct {
	Instructions []solanago.Instruction
	Accounts     []solanago.AccountMeta
	Payer        solana.PublicKey
	LookupTables *map[solanago.PublicKey]solanago.PublicKeySlice
}

// WaitUntilStateRequest request to wait the TX to reach a certain TxState
type WaitUntilStateRequest struct {
	// Id of the transaction return by SubmitTransaction
	TxID string
	// Expected TxState to reach
	TxState TxState
	// Default value will be 5 seconds
	Duration *time.Duration
}

// Creates an WaitUntilStateRequest to wait for TxState.Finalized with the default wait time.
func UntilFinalizedRequest(txID string) WaitUntilStateRequest {
	return WaitUntilStateRequest{
		TxID:    txID,
		TxState: Finalized,
	}
}

// SolanaRelayerAPI is the Solana relayer API that exposes access to Solana blockchain
type SolanaRelayerAPI interface {
	SubmitTransaction(ctx context.Context, request TransactionRequest) (string, error)
	GetTransactionStatus(ctx context.Context, txID string) (TxState, error)
	WaitUntilState(ctx context.Context, request WaitUntilStateRequest) error
}

// SolanaRelayerConfig is the global configuration for instantiating a SolanaRelayerAPI instance
type SolanaRelayerConfig struct {
	//TODO for now we expose the whole config. We should revisit required vs optional fields and that all the config makes sense outside the core node.
	config.TOMLConfig
	Logger *logger.Logger
}

// RequiredAPIs are the APIs that can be customized from the client side.
type RequiredAPIs struct {
	// KeystoreAPI API used for storing, retrieving and signing Solana transactions. Default implementation is empty which means that execution SolanaRelayerAPI.SubmitTransaction will fail since there's no way to sign a transaction.
	KeystoreAPI core.Keystore
}

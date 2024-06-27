package chainwriter

import (
	"context"
	"math/big"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"

	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type ChainWriterService interface {
	commontypes.ChainWriter
}

type chainWriter struct {
	txm txm.Txm
	ge  fees.Estimator
}

// Compile-time assertion that chainWriter implements the ChainWriterService interface.
var _ ChainWriterService = (*chainWriter)(nil)

func (w *chainWriter) GetFeeComponents(ctx context.Context) (*commontypes.ChainFeeComponents, error) {
	return nil, nil
}

func (w *chainWriter) GetTransactionStatus(ctx context.Context, transactionID string) (commontypes.TransactionStatus, error) {
	return 0, nil
}

func (w *chainWriter) SubmitTransaction(ctx context.Context, contractName, method string, args any, transactionID string, toAddress string, meta *commontypes.TxMeta, value *big.Int) error {
	return nil
}

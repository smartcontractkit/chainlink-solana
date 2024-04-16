package monitoring

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

//go:generate mockery --name ChainReader --output ./mocks/
type ChainReader interface {
	GetState(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (state pkgSolana.State, blockHeight uint64, err error)
	GetLatestTransmission(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (answer pkgSolana.Answer, blockHeight uint64, err error)

	GetTokenAccountBalance(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (out *rpc.GetTokenAccountBalanceResult, err error)
	GetBalance(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (out *rpc.GetBalanceResult, err error)
	GetSignaturesForAddressWithOpts(ctx context.Context, account solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) (out []*rpc.TransactionSignature, err error)
	GetTransaction(ctx context.Context, txSig solana.Signature, opts *rpc.GetTransactionOpts) (out *rpc.GetTransactionResult, err error)
}

func NewChainReader(client *rpc.Client) ChainReader {
	return &chainReader{client}
}

type chainReader struct {
	client *rpc.Client
}

func (c *chainReader) GetState(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (state pkgSolana.State, blockHeight uint64, err error) {
	return pkgSolana.GetState(ctx, c.client, account, commitment)
}

func (c *chainReader) GetLatestTransmission(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (answer pkgSolana.Answer, blockHeight uint64, err error) {
	return pkgSolana.GetLatestTransmission(ctx, c.client, account, commitment)
}

func (c *chainReader) GetTokenAccountBalance(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (out *rpc.GetTokenAccountBalanceResult, err error) {
	return c.client.GetTokenAccountBalance(ctx, account, commitment)
}

func (c *chainReader) GetBalance(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (out *rpc.GetBalanceResult, err error) {
	return c.client.GetBalance(ctx, account, commitment)
}

func (c *chainReader) GetSignaturesForAddressWithOpts(ctx context.Context, account solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) (out []*rpc.TransactionSignature, err error) {
	return c.client.GetSignaturesForAddressWithOpts(ctx, account, opts)
}

func (c *chainReader) GetTransaction(ctx context.Context, txSig solana.Signature, opts *rpc.GetTransactionOpts) (out *rpc.GetTransactionResult, err error) {
	return c.client.GetTransaction(ctx, txSig, opts)
}

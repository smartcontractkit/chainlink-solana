package monitoring

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	mn "github.com/smartcontractkit/chainlink-framework/multinode"

	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
)

type ChainReader interface {
	GetState(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (state pkgSolana.State, blockHeight uint64, err error)
	GetLatestTransmission(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (answer pkgSolana.Answer, blockHeight uint64, err error)

	GetTokenAccountBalance(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (out *rpc.GetTokenAccountBalanceResult, err error)
	GetBalance(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (out *rpc.GetBalanceResult, err error)
	GetSignaturesForAddressWithOpts(ctx context.Context, account solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) (out []*rpc.TransactionSignature, err error)
	GetTransaction(ctx context.Context, txSig solana.Signature, opts *rpc.GetTransactionOpts) (out *rpc.GetTransactionResult, err error)
	GetSlot(ctx context.Context) (slot uint64, err error)
	GetLatestBlock(ctx context.Context, commitment rpc.CommitmentType) (*rpc.GetBlockResult, error)
}

func NewChainReader(client *rpc.Client) ChainReader {
	return &chainReader{client}
}

type chainReader struct {
	client *rpc.Client
}

func (c *chainReader) GetState(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (state pkgSolana.State, blockHeight uint64, err error) {
	getReader := func() (client.AccountReader, error) { return c.client, nil }
	state, blockHeight, err = pkgSolana.GetState(ctx, getReader, account, commitment)
	return state, blockHeight, mn.SanitizeRPCError(err)
}

func (c *chainReader) GetLatestTransmission(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (answer pkgSolana.Answer, blockHeight uint64, err error) {
	getReader := func() (client.AccountReader, error) { return c.client, nil }
	answer, blockHeight, err = pkgSolana.GetLatestTransmission(ctx, getReader, account, commitment)
	return answer, blockHeight, mn.SanitizeRPCError(err)
}

func (c *chainReader) GetTokenAccountBalance(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (out *rpc.GetTokenAccountBalanceResult, err error) {
	out, err = c.client.GetTokenAccountBalance(ctx, account, commitment)
	return out, mn.SanitizeRPCError(err)
}

func (c *chainReader) GetBalance(ctx context.Context, account solana.PublicKey, commitment rpc.CommitmentType) (out *rpc.GetBalanceResult, err error) {
	out, err = c.client.GetBalance(ctx, account, commitment)
	return out, mn.SanitizeRPCError(err)
}

func (c *chainReader) GetSignaturesForAddressWithOpts(ctx context.Context, account solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) (out []*rpc.TransactionSignature, err error) {
	out, err = c.client.GetSignaturesForAddressWithOpts(ctx, account, opts)
	return out, mn.SanitizeRPCError(err)
}

func (c *chainReader) GetTransaction(ctx context.Context, txSig solana.Signature, opts *rpc.GetTransactionOpts) (out *rpc.GetTransactionResult, err error) {
	out, err = c.client.GetTransaction(ctx, txSig, opts)
	return out, mn.SanitizeRPCError(err)
}

func (c *chainReader) GetSlot(ctx context.Context) (uint64, error) {
	slot, err := c.client.GetSlot(ctx, rpc.CommitmentProcessed) // get latest height
	return slot, mn.SanitizeRPCError(err)
}

func (c *chainReader) GetLatestBlock(ctx context.Context, commitment rpc.CommitmentType) (*rpc.GetBlockResult, error) {
	// get slot based on confirmation
	slot, err := c.client.GetSlot(ctx, commitment)
	if err != nil {
		return nil, fmt.Errorf("GetSlot failed: %w", mn.SanitizeRPCError(err))
	}

	// get block based on slot
	version := client.MaxSupportTransactionVersion // pull all tx types (legacy + v0 + v1)
	block, err := c.client.GetBlockWithOpts(ctx, slot, &rpc.GetBlockOpts{
		Commitment:                     commitment,
		MaxSupportedTransactionVersion: &version,
	})
	return block, mn.SanitizeRPCError(err)
}

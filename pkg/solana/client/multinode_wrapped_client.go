package client

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	mn "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode"
)

var _ ReaderWriter = (*MultiNodeWrappedClient)(nil)

// MultiNodeWrappedClient - wrapper over MultiNode that reselect an RPC for each method call.
type MultiNodeWrappedClient struct {
	multiNode *mn.MultiNode[mn.StringID, *MultiNodeClient]
}

func NewMultiNodeWrappedClient(multiNode *mn.MultiNode[mn.StringID, *MultiNodeClient]) *MultiNodeWrappedClient {
	return &MultiNodeWrappedClient{multiNode}
}

func (m *MultiNodeWrappedClient) Start(ctx context.Context) error {
	return m.multiNode.Start(ctx)
}

func (m *MultiNodeWrappedClient) SendTx(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return solana.Signature{}, err
	}

	return r.SendTx(ctx, tx)
}

func (m *MultiNodeWrappedClient) SimulateTx(ctx context.Context, tx *solana.Transaction, opts *rpc.SimulateTransactionOpts) (*rpc.SimulateTransactionResult, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return nil, err
	}

	return r.SimulateTx(ctx, tx, opts)
}

func (m *MultiNodeWrappedClient) SignatureStatuses(ctx context.Context, sigs []solana.Signature) ([]*rpc.SignatureStatusesResult, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return nil, err
	}

	return r.SignatureStatuses(ctx, sigs)
}

func (m *MultiNodeWrappedClient) GetAccountInfoWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return nil, err
	}

	return r.GetAccountInfoWithOpts(ctx, addr, opts)
}

func (m *MultiNodeWrappedClient) Balance(ctx context.Context, addr solana.PublicKey) (uint64, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return 0, err
	}

	return r.Balance(ctx, addr)
}

func (m *MultiNodeWrappedClient) SlotHeight(ctx context.Context) (uint64, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return 0, err
	}

	return r.SlotHeight(ctx)
}

func (m *MultiNodeWrappedClient) LatestBlockhash(ctx context.Context) (*rpc.GetLatestBlockhashResult, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return nil, err
	}

	return r.LatestBlockhash(ctx)
}

func (m *MultiNodeWrappedClient) ChainID(ctx context.Context) (mn.StringID, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return "", err
	}

	return r.ChainID(ctx)
}

func (m *MultiNodeWrappedClient) GetFeeForMessage(ctx context.Context, msg string) (uint64, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return 0, err
	}

	return r.GetFeeForMessage(ctx, msg)
}

func (m *MultiNodeWrappedClient) GetLatestBlock(ctx context.Context) (*rpc.GetBlockResult, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return nil, err
	}

	return r.GetLatestBlock(ctx)
}

func (m *MultiNodeWrappedClient) GetTransaction(ctx context.Context, txHash solana.Signature, opts *rpc.GetTransactionOpts) (*rpc.GetTransactionResult, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return nil, err
	}

	return r.GetTransaction(ctx, txHash, opts)
}

func (m *MultiNodeWrappedClient) GetBlocks(ctx context.Context, startSlot uint64, endSlot *uint64) (rpc.BlocksResult, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return nil, err
	}

	return r.GetBlocks(ctx, startSlot, endSlot)
}

func (m *MultiNodeWrappedClient) GetBlocksWithLimit(ctx context.Context, startSlot uint64, limit uint64) (*rpc.BlocksResult, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return nil, err
	}

	return r.GetBlocksWithLimit(ctx, startSlot, limit)
}

func (m *MultiNodeWrappedClient) GetBlock(ctx context.Context, slot uint64) (*rpc.GetBlockResult, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return nil, err
	}

	return r.GetBlock(ctx, slot)
}

func (m *MultiNodeWrappedClient) GetSignaturesForAddressWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error) {
	r, err := m.multiNode.SelectRPC()
	if err != nil {
		return nil, err
	}

	return r.GetSignaturesForAddressWithOpts(ctx, addr, opts)
}

func (m *MultiNodeWrappedClient) Close() error {
	return m.multiNode.Close()
}

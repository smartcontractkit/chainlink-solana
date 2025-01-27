package client

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	mn "github.com/smartcontractkit/chainlink-framework/multinode"
)

var _ ReaderWriter = (*MultiClient)(nil)

// MultiClient - wrapper over multiple RPCs, underlying provider can be MultiNode or LazyLoader.
// Main purpose is to eliminate need for frequent error handling on selection of a client.
type MultiClient struct {
	getClient func() (ReaderWriter, error)
}

func NewMultiClient(getClient func() (ReaderWriter, error)) *MultiClient {
	return &MultiClient{
		getClient: getClient,
	}
}

func (m *MultiClient) GetLatestBlockHeight(ctx context.Context) (uint64, error) {
	r, err := m.getClient()
	if err != nil {
		return 0, err
	}

	return r.GetLatestBlockHeight(ctx)
}

func (m *MultiClient) SendTx(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	r, err := m.getClient()
	if err != nil {
		return solana.Signature{}, err
	}

	return r.SendTx(ctx, tx)
}

func (m *MultiClient) SimulateTx(ctx context.Context, tx *solana.Transaction, opts *rpc.SimulateTransactionOpts) (*rpc.SimulateTransactionResult, error) {
	r, err := m.getClient()
	if err != nil {
		return nil, err
	}

	return r.SimulateTx(ctx, tx, opts)
}

func (m *MultiClient) SignatureStatuses(ctx context.Context, sigs []solana.Signature) ([]*rpc.SignatureStatusesResult, error) {
	r, err := m.getClient()
	if err != nil {
		return nil, err
	}

	return r.SignatureStatuses(ctx, sigs)
}

func (m *MultiClient) GetAccountInfoWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error) {
	r, err := m.getClient()
	if err != nil {
		return nil, err
	}

	return r.GetAccountInfoWithOpts(ctx, addr, opts)
}

func (m *MultiClient) GetMultipleAccountsWithOpts(ctx context.Context, accounts []solana.PublicKey, opts *rpc.GetMultipleAccountsOpts) (out *rpc.GetMultipleAccountsResult, err error) {
	r, err := m.getClient()
	if err != nil {
		return nil, err
	}

	return r.GetMultipleAccountsWithOpts(ctx, accounts, opts)
}

func (m *MultiClient) Balance(ctx context.Context, addr solana.PublicKey) (uint64, error) {
	r, err := m.getClient()
	if err != nil {
		return 0, err
	}

	return r.Balance(ctx, addr)
}

func (m *MultiClient) SlotHeight(ctx context.Context) (uint64, error) {
	r, err := m.getClient()
	if err != nil {
		return 0, err
	}

	return r.SlotHeight(ctx)
}

func (m *MultiClient) LatestBlockhash(ctx context.Context) (*rpc.GetLatestBlockhashResult, error) {
	r, err := m.getClient()
	if err != nil {
		return nil, err
	}

	return r.LatestBlockhash(ctx)
}

func (m *MultiClient) ChainID(ctx context.Context) (mn.StringID, error) {
	r, err := m.getClient()
	if err != nil {
		return "", err
	}

	return r.ChainID(ctx)
}

func (m *MultiClient) GetFeeForMessage(ctx context.Context, msg string) (uint64, error) {
	r, err := m.getClient()
	if err != nil {
		return 0, err
	}

	return r.GetFeeForMessage(ctx, msg)
}

func (m *MultiClient) GetLatestBlock(ctx context.Context) (*rpc.GetBlockResult, error) {
	r, err := m.getClient()
	if err != nil {
		return nil, err
	}

	return r.GetLatestBlock(ctx)
}

func (m *MultiClient) GetTransaction(ctx context.Context, txHash solana.Signature) (*rpc.GetTransactionResult, error) {
	r, err := m.getClient()
	if err != nil {
		return nil, err
	}

	return r.GetTransaction(ctx, txHash)
}

func (m *MultiClient) GetBlocks(ctx context.Context, startSlot uint64, endSlot *uint64) (rpc.BlocksResult, error) {
	r, err := m.getClient()
	if err != nil {
		return nil, err
	}

	return r.GetBlocks(ctx, startSlot, endSlot)
}

func (m *MultiClient) GetBlocksWithLimit(ctx context.Context, startSlot uint64, limit uint64) (*rpc.BlocksResult, error) {
	r, err := m.getClient()
	if err != nil {
		return nil, err
	}

	return r.GetBlocksWithLimit(ctx, startSlot, limit)
}

func (m *MultiClient) GetBlock(ctx context.Context, slot uint64) (*rpc.GetBlockResult, error) {
	r, err := m.getClient()
	if err != nil {
		return nil, err
	}

	return r.GetBlock(ctx, slot)
}

func (m *MultiClient) GetSignaturesForAddressWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error) {
	r, err := m.getClient()
	if err != nil {
		return nil, err
	}

	return r.GetSignaturesForAddressWithOpts(ctx, addr, opts)
}

func (m *MultiClient) GetBlockWithOpts(ctx context.Context, slot uint64, opts *rpc.GetBlockOpts) (*rpc.GetBlockResult, error) {
	r, err := m.getClient()
	if err != nil {
		return nil, err
	}

	return r.GetBlockWithOpts(ctx, slot, opts)
}

func (m *MultiClient) SlotHeightWithCommitment(ctx context.Context, commitment rpc.CommitmentType) (uint64, error) {
	r, err := m.getClient()
	if err != nil {
		return 0, err
	}

	return r.SlotHeightWithCommitment(ctx, commitment)
}

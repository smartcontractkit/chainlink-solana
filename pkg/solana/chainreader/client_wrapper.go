package chainreader

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

type RPCClientWrapper struct {
	*rpc.Client
}

func (w *RPCClientWrapper) GetMultipleAccountData(ctx context.Context, keys ...solana.PublicKey) ([][]byte, error) {
	result, err := w.Client.GetMultipleAccountsWithOpts(ctx, keys, &rpc.GetMultipleAccountsOpts{
		Encoding:   solana.EncodingBase64,
		Commitment: rpc.CommitmentFinalized,
	})
	if err != nil {
		return nil, err
	}

	bts := make([][]byte, len(result.Value))

	for idx, result := range result.Value {
		if result == nil {
			return nil, rpc.ErrNotFound
		}

		if result.Data == nil {
			return nil, rpc.ErrNotFound
		}

		if result.Data.GetBinary() == nil {
			return nil, rpc.ErrNotFound
		}

		bts[idx] = result.Data.GetBinary()
	}

	return bts, nil
}

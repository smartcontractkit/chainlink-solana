package chainreader

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
)

// RPCClientWrapper is a wrapper for an RPC client. This was necessary due to the solana RPC interface not
// providing directly mockable components in the GetMultipleAccounts response.
type RPCClientWrapper struct {
	client.AccountReader
}

// GetMultipleAccountData is a helper function that extracts byte data from a GetMultipleAccounts rpc call.
func (w *RPCClientWrapper) GetMultipleAccountData(ctx context.Context, keys ...solana.PublicKey) ([]*rpc.Account, error) {
	result, err := w.GetMultipleAccountsWithOpts(ctx, keys, &rpc.GetMultipleAccountsOpts{
		Encoding:   solana.EncodingBase64,
		Commitment: rpc.CommitmentFinalized,
	})
	if err != nil {
		return nil, err
	}

	var accounts []*rpc.Account
	for _, res := range result.Value {
		if res == nil || res.Data == nil || res.Data.GetBinary() == nil {
			accounts = append(accounts, &rpc.Account{Data: nil})
			continue
		}
		accounts = append(accounts, res)
	}

	return accounts, nil
}

package chainreader

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

// accountReadBinding provides decoding and reading Solana Account data using a defined codec. The
// `idlAccount` refers to the account name in the IDL for which the codec has a type mapping.
type accountReadBinding struct {
	idlAccount string
	codec      types.RemoteCodec
	key        solana.PublicKey
	opts       *rpc.GetAccountInfoOpts
}

func newAccountReadBinding(acct string, codec types.RemoteCodec, opts *rpc.GetAccountInfoOpts) *accountReadBinding {
	return &accountReadBinding{
		idlAccount: acct,
		codec:      codec,
		opts:       opts,
	}
}

var _ readBinding = &accountReadBinding{}

func (b *accountReadBinding) SetAddress(key solana.PublicKey) {
	b.key = key
}

func (b *accountReadBinding) GetAddress() solana.PublicKey {
	return b.key
}

func (b *accountReadBinding) CreateType(_ bool) (any, error) {
	return b.codec.CreateType(b.idlAccount, false)
}

func (b *accountReadBinding) Decode(ctx context.Context, bts []byte, outVal any) error {
	return b.codec.Decode(ctx, bts, outVal, b.idlAccount)
}

package chainreader

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

// accountReadBinding provides decoding and reading Solana Account data using a defined codec. The
// `idlAccount` refers to the account onChainName in the IDL for which the codec has a type mapping.
type accountReadBinding struct {
	namespace, onChainName, chainAgnosticName string
	codec                                     types.RemoteCodec
	key                                       solana.PublicKey
	opts                                      *rpc.GetAccountInfoOpts
}

func newAccountReadBinding(namespace, onChainName, chainAgnosticName string, opts *rpc.GetAccountInfoOpts) *accountReadBinding {
	return &accountReadBinding{
		namespace:         namespace,
		onChainName:       onChainName,
		chainAgnosticName: chainAgnosticName,
		opts:              opts,
	}
}

var _ readBinding = &accountReadBinding{}

func (b *accountReadBinding) SetCodec(codec types.RemoteCodec) {
	b.codec = codec
}

func (b *accountReadBinding) SetAddress(key solana.PublicKey) {
	b.key = key
}

func (b *accountReadBinding) GetAddress() solana.PublicKey {
	return b.key
}

func (b *accountReadBinding) CreateType(forEncoding bool) (any, error) {
	return b.codec.CreateType(codec.WrapItemType(forEncoding, b.namespace, b.chainAgnosticName, codec.ChainConfigTypeAccountDef), forEncoding)
}

func (b *accountReadBinding) Decode(ctx context.Context, bts []byte, outVal any) error {
	return b.codec.Decode(ctx, bts, outVal, codec.WrapItemType(false, b.namespace, b.chainAgnosticName, codec.ChainConfigTypeAccountDef))
}

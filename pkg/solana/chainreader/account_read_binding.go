package chainreader

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

// BinaryDataReader provides an interface for reading bytes from a source. This is likely a wrapper
// for a solana client.
type BinaryDataReader interface {
	ReadAll(context.Context, solana.PublicKey) ([]byte, error)
}

// accountReadBinding provides decoding and reading Solana Account data using a defined codec. The
// `idlAccount` refers to the account name in the IDL for which the codec has a type mapping.
type accountReadBinding struct {
	idlAccount string
	account    solana.PublicKey
	codec      types.RemoteCodec
	reader     BinaryDataReader
}

var _ readBinding = &accountReadBinding{}

func (b *accountReadBinding) GetLatestValue(ctx context.Context, _ any, outVal any) error {
	bts, err := b.reader.ReadAll(ctx, b.account)
	if err != nil {
		return fmt.Errorf("%w: failed to get binary data", err)
	}

	return b.codec.Decode(ctx, bts, outVal, b.idlAccount)
}

func (b *accountReadBinding) Bind(contract types.BoundContract) error {
	account, err := solana.PublicKeyFromBase58(contract.Address)
	if err != nil {
		return err
	}

	b.account = account

	return nil
}

func (b *accountReadBinding) CreateType(_ bool) (any, error) {
	return b.codec.CreateType(b.idlAccount, false)
}

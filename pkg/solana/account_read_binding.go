package solana

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

var ErrUnimplemented = errors.New("function not implemented")

type BinaryDataReader interface {
	GetAccountInfoBinaryData(context.Context, solana.PublicKey) ([]byte, error)
}

type accountReadBinding struct {
	idlAccount string
	account    solana.PublicKey
	codec      commontypes.Codec
	client     BinaryDataReader
}

var _ readBinding = &accountReadBinding{}

func (b *accountReadBinding) SetCodec(codec commontypes.RemoteCodec) {
	b.codec = codec
}

func (b *accountReadBinding) GetLatestValue(ctx context.Context, _ any, outVal any) error {
	bts, err := b.client.GetAccountInfoBinaryData(ctx, b.account)
	if err != nil {
		return fmt.Errorf("%w: failed to get binary data", err)
	}

	// log.Printf("decoding for %s and len bytes %d", b.idlAccount, len(bts))

	return b.codec.Decode(ctx, bts, outVal, b.idlAccount)
}

func (b *accountReadBinding) Bind(contract commontypes.BoundContract) error {
	account, err := solana.PublicKeyFromBase58(contract.Address)
	if err != nil {
		return err
	}

	b.account = account

	return nil
}

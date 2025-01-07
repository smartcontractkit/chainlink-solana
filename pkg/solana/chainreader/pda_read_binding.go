package chainreader

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

// pdaReadBinding provides calculating PDA addresses with the provided seeds and reading decoded PDA Account data using a defined codec
type pdaReadBinding struct {
	namespace   string
	genericName string
	codec       types.RemoteCodec
	programID   solana.PublicKey
	seeds       []config.Seed
}

func newPdaReadBinding(namespace, genericName string, seeds []config.Seed) *pdaReadBinding {
	return &pdaReadBinding{
		namespace:   namespace,
		genericName: genericName,
		seeds:       seeds,
	}
}

var _ readBinding = &pdaReadBinding{}

func (b *pdaReadBinding) SetCodec(codec types.RemoteCodec) {
	b.codec = codec
}

func (b *pdaReadBinding) SetAddress(programID solana.PublicKey) {
	b.programID = programID
}

func (b *pdaReadBinding) GetAddress(params any) (solana.PublicKey, error) {
	seedBytes, err := b.buildSeedsSlice(params)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed build seeds list for PDA generation: %w", err)
	}
	key, _, err := solana.FindProgramAddress(seedBytes, b.programID)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed find program address for PDA: %w", err)
	}
	return key, nil
}

func (b *pdaReadBinding) CreateType(forEncoding bool) (any, error) {
	return b.codec.CreateType(codec.WrapItemType(forEncoding, b.namespace, b.genericName, codec.ChainConfigTypeAccountDef), forEncoding)
}

func (b *pdaReadBinding) Decode(ctx context.Context, bts []byte, outVal any) error {
	return b.codec.Decode(ctx, bts, outVal, codec.WrapItemType(false, b.namespace, b.genericName, codec.ChainConfigTypeAccountDef))
}

func (b *pdaReadBinding) buildSeedsSlice(params any) ([][]byte, error) {
	if b.seeds == nil {
		return [][]byte{}, nil
	}

	seedByteArray := make([][]byte, 0, len(b.seeds))
	for _, seed := range b.seeds {
		if seed.Value != nil && len(seed.Location) > 0 {
			return nil, fmt.Errorf("seed cannot have both Value (%v) and Location (%s) defined", seed.Value, seed.Location)
		}
		if seed.Value != nil {
			byteArray := utils.ConvertAnyToPDASeed(seed.Value)
			if byteArray == nil {
				return nil, fmt.Errorf("failed to convert seed %v to byte array", seed.Value)
			}
			if len(byteArray) > solana.MaxSeedLength {
				return nil, fmt.Errorf("seed length %d exceeds the max allowed length %d", len(byteArray), solana.MaxSeedLength)
			}
			seedByteArray = append(seedByteArray, utils.ConvertAnyToPDASeed(seed.Value))
			continue
		}
		if len(seed.Location) > 0 {
			byteArrays, err := utils.GetValuesAtLocation(params, seed.Location)
			if err != nil {
				return nil, fmt.Errorf("failed to find seed at location %s in params: %w", seed.Location, err)
			}
			if len(byteArrays) != 1 {
				return nil, fmt.Errorf("expected 1 seed. found %d seeds at location %s", len(byteArrays), seed.Location)
			}
			seedByteArray = append(seedByteArray, byteArrays[0])
			continue
		}
	}
	return seedByteArray, nil
}

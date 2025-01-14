package chainreader

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

// pdaReadBinding provides calculating PDA addresses with the provided seeds and reading decoded PDA Account data using a defined codec
type pdaReadBinding struct {
	namespace   string
	genericName string
	codec       types.RemoteCodec
	programID   solana.PublicKey
	prefix      string
}

func newPdaReadBinding(namespace, genericName string, prefix string) *pdaReadBinding {
	return &pdaReadBinding{
		namespace:   namespace,
		genericName: genericName,
		prefix:      prefix,
	}
}

var _ readBinding = &pdaReadBinding{}

func (b *pdaReadBinding) SetCodec(codec types.RemoteCodec) {
	b.codec = codec
}

func (b *pdaReadBinding) SetAddress(programID solana.PublicKey) {
	b.programID = programID
}

func (b *pdaReadBinding) GetAddress(ctx context.Context, params any) (solana.PublicKey, error) {
	seedBytes, err := b.buildSeedsSlice(ctx, params)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed build seeds list for PDA generation: %w", err)
	}
	key, _, err := solana.FindProgramAddress(seedBytes, b.programID)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed find program address for PDA: %w", err)
	}
	fmt.Println("calculated PDA", key)
	return key, nil
}

func (b *pdaReadBinding) CreateType(forEncoding bool) (any, error) {
	return b.codec.CreateType(codec.WrapItemType(forEncoding, b.namespace, b.genericName, codec.ChainConfigTypeAccountDef), forEncoding)
}

func (b *pdaReadBinding) Decode(ctx context.Context, bts []byte, outVal any) error {
	return b.codec.Decode(ctx, bts, outVal, codec.WrapItemType(false, b.namespace, b.genericName, codec.ChainConfigTypeAccountDef))
}

func (b *pdaReadBinding) buildSeedsSlice(ctx context.Context, params any) ([][]byte, error) {
	flattenedSeeds := make([]byte, 0, solana.MaxSeeds*solana.MaxSeedLength)
	// Append the static prefix string first
	flattenedSeeds = append(flattenedSeeds, []byte(b.prefix)...)
	// Encode the seeds provided in the params
	genericSeedName := fmt.Sprintf("%s.%s", b.genericName, "seed")
	encodedParamSeeds, err := b.codec.Encode(ctx, params, codec.WrapItemType(true, b.namespace, genericSeedName, ""))
	if err != nil {
		return nil, fmt.Errorf("failed to encode params into bytes for PDA seeds: %w", err)
	}
	// Append the encoded seeds
	flattenedSeeds = append(flattenedSeeds, encodedParamSeeds...)
	fmt.Println("params encoded", flattenedSeeds)

	if len(flattenedSeeds) > solana.MaxSeeds*solana.MaxSeedLength {
		return nil, fmt.Errorf("seeds exceed the maximum allowed length")
	}

	// Splitting the seeds since they are expected to be provided separately to FindProgramAddress
	// Arbitrarily separating the seeds at max seed length would still yield the same PDA since
	// FindProgramAddress appends the seed bytes together under the hood
	numSeeds := len(flattenedSeeds) / solana.MaxSeedLength
	if len(flattenedSeeds)%solana.MaxSeedLength != 0 {
		numSeeds++
	}
	seedByteArray := make([][]byte, 0, numSeeds)
	for i := 0; i < numSeeds; i++ {
		startIdx := i * solana.MaxSeedLength
		endIdx := startIdx + solana.MaxSeedLength
		if endIdx > len(flattenedSeeds) {
			endIdx = len(flattenedSeeds)
		}
		seedByteArray = append(seedByteArray, flattenedSeeds[startIdx:endIdx])
	}
	return seedByteArray, nil
}

package chainreader

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

// accountReadBinding provides decoding and reading Solana Account data using a defined codec.
type accountReadBinding struct {
	namespace, genericName   string
	codec                    types.RemoteCodec
	key                      solana.PublicKey
	isPda                    bool   // flag to signify whether or not the account read is for a PDA
	prefix                   []byte // only used for PDA public key calculation
	responseAddressHardCoder *commoncodec.HardCodeModifierConfig
	readDefinition           config.ReadDefinition
	idl                      codec.IDL
	inputIDLType             interface{}
	outputIDLTypeDef         codec.IdlTypeDef
}

func newAccountReadBinding(namespace, genericName string, isPda bool, idl codec.IDL, inputIDLType interface{}, outputIDLTypeDef codec.IdlTypeDef, readDefinition config.ReadDefinition) *accountReadBinding {
	rb := &accountReadBinding{
		namespace:                namespace,
		genericName:              genericName,
		prefix:                   readDefinition.PDADefinition.Prefix,
		isPda:                    isPda,
		readDefinition:           readDefinition,
		idl:                      idl,
		inputIDLType:             inputIDLType,
		outputIDLTypeDef:         outputIDLTypeDef,
		responseAddressHardCoder: nil,
	}

	if readDefinition.ResponseAddressHardCoder != nil {
		rb.responseAddressHardCoder = readDefinition.ResponseAddressHardCoder
	}

	return rb
}

var _ readBinding = &accountReadBinding{}

func (b *accountReadBinding) GetAddress(ctx context.Context, params any) (solana.PublicKey, error) {
	// Return the bound key if normal account read
	if !b.isPda {
		return b.key, nil
	}
	// Calculate the public key if PDA account read
	seedBytes, err := b.buildSeedsSlice(ctx, params)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed build seeds list for PDA calculation: %w", err)
	}
	key, _, err := solana.FindProgramAddress(seedBytes, b.key)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed find program address for PDA: %w", err)
	}
	return key, nil
}

func (b *accountReadBinding) GetGenericName() string {
	return b.genericName
}

func (b *accountReadBinding) GetReadDefinition() config.ReadDefinition {
	return b.readDefinition
}

func (b *accountReadBinding) GetIDLInfo() (idl codec.IDL, inputIDLTypeDef interface{}, outputIDLTypeDef codec.IdlTypeDef) {
	return b.idl, b.inputIDLType, b.outputIDLTypeDef
}

func (b *accountReadBinding) GetAddressResponseHardCoder() *commoncodec.HardCodeModifierConfig {
	return b.responseAddressHardCoder
}

func (b *accountReadBinding) SetAddress(key solana.PublicKey) {
	b.key = key
}

func (b *accountReadBinding) SetCodec(codec types.RemoteCodec) {
	b.codec = codec
}

func (b *accountReadBinding) SetModifier(commoncodec.Modifier) {}

func (b *accountReadBinding) CreateType(forEncoding bool) (any, error) {
	return b.codec.CreateType(codec.WrapItemType(forEncoding, b.namespace, b.genericName), forEncoding)
}

func (b *accountReadBinding) Decode(ctx context.Context, bts []byte, outVal any) error {
	return b.codec.Decode(ctx, bts, outVal, codec.WrapItemType(false, b.namespace, b.genericName))
}

func (b *accountReadBinding) QueryKey(_ context.Context, _ query.KeyFilter, _ query.LimitAndSort, _ any) ([]types.Sequence, error) {
	return nil, errors.New("unimplemented")
}

// buildSeedsSlice encodes and builds the seedslist to calculate the PDA public key
func (b *accountReadBinding) buildSeedsSlice(ctx context.Context, params any) ([][]byte, error) {
	flattenedSeeds := make([]byte, 0, solana.MaxSeeds*solana.MaxSeedLength)
	// Append the static prefix string first
	flattenedSeeds = append(flattenedSeeds, b.prefix...)
	// Encode the seeds provided in the params
	encodedParamSeeds, err := b.codec.Encode(ctx, params, codec.WrapItemType(true, b.namespace, b.genericName))
	if err != nil {
		return nil, fmt.Errorf("failed to encode params into bytes for PDA seeds: %w", err)
	}
	// Append the encoded seeds
	flattenedSeeds = append(flattenedSeeds, encodedParamSeeds...)

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

package chainreader

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
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
	idl                      codecv1.IDL
	inputIDLType             interface{}
	outputIDLTypeDef         codecv1.IdlTypeDef
}

func newAccountReadBinding(
	namespace, genericName string,
	isPda bool,
	pdaPrefix []byte,
	idl codecv1.IDL,
	inputIDLType interface{},
	outputIDLTypeDef codecv1.IdlTypeDef,
	readDefinition config.ReadDefinition,
) *accountReadBinding {
	rb := &accountReadBinding{
		namespace:                namespace,
		genericName:              genericName,
		prefix:                   pdaPrefix,
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

func (b *accountReadBinding) SetCodec(codec types.RemoteCodec) {
	b.codec = codec
}

func (b *accountReadBinding) SetModifier(commoncodec.Modifier) {}

func (b *accountReadBinding) Register(context.Context) error { return nil }

func (b *accountReadBinding) Unregister(context.Context) error { return nil }

func (b *accountReadBinding) Bind(_ context.Context, address solana.PublicKey) error {
	b.key = address

	return nil
}

func (b *accountReadBinding) Unbind(_ context.Context) error {
	b.key = solana.PublicKey{}

	return nil
}

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

func (b *accountReadBinding) GetIDLInfo() (idl codecv1.IDL, inputIDLTypeDef interface{}, outputIDLTypeDef codecv1.IdlTypeDef) {
	return b.idl, b.inputIDLType, b.outputIDLTypeDef
}

func (b *accountReadBinding) GetAddressResponseHardCoder() *commoncodec.HardCodeModifierConfig {
	return b.responseAddressHardCoder
}

func (b *accountReadBinding) CreateType(forEncoding bool) (any, error) {
	return b.codec.CreateType(solcommoncodec.WrapItemType(forEncoding, b.namespace, b.genericName), forEncoding)
}

func (b *accountReadBinding) Decode(ctx context.Context, bts []byte, outVal any) error {
	return b.codec.Decode(ctx, bts, outVal, solcommoncodec.WrapItemType(false, b.namespace, b.genericName))
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
	encodedParamSeeds, err := b.codec.Encode(ctx, params, solcommoncodec.WrapItemType(true, b.namespace, b.genericName))
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

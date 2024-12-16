package solanacodec

import (
	"fmt"
	"reflect"

	"github.com/smartcontractkit/chainlink-common/pkg/codec"
	commonencodings "github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type Entry interface {
	Encode(value any, into []byte) ([]byte, error)
	Decode(encoded []byte) (any, []byte, error)
	GetCodecType() commonencodings.TypeCodec
	GetType() reflect.Type
	Modifier() codec.Modifier
}

func NewAccountEntry(offchainName string, idlAccount IdlTypeDef, idlTypes IdlTypeDefSlice, includeDiscriminator bool, mod codec.Modifier, builder commonencodings.Builder) (Entry, error) {
	refs := &codecRefs{
		builder:      builder,
		codecs:       make(map[string]commonencodings.TypeCodec),
		typeDefs:     idlTypes,
		dependencies: make(map[string][]string),
	}

	_, accCodec, err := createCodecType(idlAccount, refs, false)
	if err != nil {
		return nil, err
	}

	entry := &CodecEntry{
		offchainName:         offchainName,
		onchainName:          idlAccount.Name,
		includeDiscriminator: includeDiscriminator,
		typeCodec:            accCodec,
		reflectType:          accCodec.GetType(),
		mod:                  ensureModifier(mod),
	}
	return entry, nil
}

func NewInstructionArgsEntry(offChainName string, instructions IdlInstruction, idlTypes IdlTypeDefSlice, mod codec.Modifier, builder commonencodings.Builder) (Entry, error) {
	refs := &codecRefs{
		builder:      builder,
		codecs:       make(map[string]commonencodings.TypeCodec),
		typeDefs:     idlTypes,
		dependencies: make(map[string][]string),
	}

	_, instructionCodecArgs, err := asStruct(instructions.Args, refs, instructions.Name, false, true)
	if err != nil {
		return nil, err
	}

	return &CodecEntry{
		offchainName: offChainName,
		onchainName:  instructions.Name,
		typeCodec:    instructionCodecArgs,
		reflectType:  instructionCodecArgs.GetType(),
		mod:          ensureModifier(mod),
	}, nil
}

type CodecEntry struct {
	// TODO this might not be needed in the end, it was handy to make tests simpler
	offchainName         string
	onchainName          string
	reflectType          reflect.Type
	typeCodec            commonencodings.TypeCodec
	mod                  codec.Modifier
	includeDiscriminator bool
}

func (entry *CodecEntry) GetType() reflect.Type {
	return entry.reflectType
}

func (entry *CodecEntry) GetCodecType() commonencodings.TypeCodec {
	return entry.typeCodec
}

func (entry *CodecEntry) Encode(value any, into []byte) ([]byte, error) {
	// Special handling for encoding a nil pointer to an empty struct.
	t := entry.reflectType
	if value == nil {
		if t.Kind() == reflect.Pointer {
			elem := t.Elem()
			if elem.Kind() == reflect.Struct && elem.NumField() == 0 {
				return []byte{}, nil
			}
		}
		return nil, fmt.Errorf("%w: cannot encode nil value for %s", commontypes.ErrInvalidType, entry.offchainName)
	}

	encodedVal, err := entry.typeCodec.Encode(value, into)
	if err != nil {
		return nil, err
	}

	if entry.includeDiscriminator {
		var byt []byte
		disc := NewDiscriminator(entry.onchainName)
		encodedDisc, err := disc.Encode(&disc.hashPrefix, byt)
		if err != nil {
			return nil, err
		}
		return append(encodedDisc, encodedVal...), nil
	}

	return encodedVal, nil
}

func (entry *CodecEntry) Decode(encoded []byte) (any, []byte, error) {
	if entry.includeDiscriminator {
		if len(encoded) < discriminatorLength {
			return nil, nil, fmt.Errorf("%w: encoded data too short to contain discriminator for %s", commontypes.ErrInvalidType, entry.offchainName)
		}
		encoded = encoded[discriminatorLength:]
	}
	return entry.typeCodec.Decode(encoded)
}

func (entry *CodecEntry) Modifier() codec.Modifier {
	return entry.mod
}

func ensureModifier(mod codec.Modifier) codec.Modifier {
	if mod == nil {
		return codec.MultiModifier{}
	}
	return mod
}

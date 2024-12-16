package solanacodec

import (
	"fmt"
	"reflect"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"

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

	if mod == nil {
		mod = codec.MultiModifier{}
	}

	entry := &CodecEntry{offchainName: offchainName, onchainName: idlAccount.Name, includeDiscriminator: includeDiscriminator, typeCodec: accCodec, reflectType: accCodec.GetType(), mod: mod}
	return entry, nil
}

func NewInstructionArgsEntry(offChainName string, instructions IdlInstruction, idlTypes IdlTypeDefSlice, mod codec.Modifier, builder commonencodings.Builder) (Entry, error) {
	refs := &codecRefs{
		builder:      binary.LittleEndian(),
		codecs:       make(map[string]commonencodings.TypeCodec),
		typeDefs:     idlTypes,
		dependencies: make(map[string][]string),
	}

	_, instructionCodecArgs, err := asStruct(instructions.Args, refs, instructions.Name, false, true)
	if err != nil {
		return nil, err
	}

	if mod == nil {
		mod = codec.MultiModifier{}
	}

	return &CodecEntry{offchainName: offChainName, onchainName: instructions.Name, typeCodec: instructionCodecArgs, reflectType: instructionCodecArgs.GetType(), mod: mod}, nil
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
	// handle nil encoding for empty struct as an empty byte slice
	t := entry.reflectType
	if value == nil && t.Kind() == reflect.Pointer {
		elem := t.Elem()
		if elem.Kind() == reflect.Struct && elem.NumField() == 0 {
			return []byte{}, nil
		}
	}

	if value == nil {
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
		encoded = encoded[discriminatorLength:]
	}
	return entry.typeCodec.Decode(encoded)
}

func (entry *CodecEntry) Modifier() codec.Modifier {
	return entry.mod
}

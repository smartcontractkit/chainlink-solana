package codec

import (
	"bytes"
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
	Size(numItems int) (int, error)
	FixedSize() (int, error)
}

type entry struct {
	// TODO this might not be needed in the end, it was handy to make tests simpler
	offchainName string
	onchainName  string
	reflectType  reflect.Type
	typeCodec    commonencodings.TypeCodec
	mod          codec.Modifier
	// includeDiscriminator during Encode adds a discriminator to the encoded bytes under an assumption that the provided value didn't have a discriminator.
	// During Decode includeDiscriminator removes discriminator from bytes under an assumption that the provided struct doesn't need a discriminator.
	includeDiscriminator bool
	discriminator        Discriminator
}

func NewAccountEntry(offchainName string, idlAccount IdlTypeDef, idlTypes IdlTypeDefSlice, includeDiscriminator bool, mod codec.Modifier, builder commonencodings.Builder) (Entry, error) {
	_, accCodec, err := createCodecType(idlAccount, createRefs(idlTypes, builder), false)
	if err != nil {
		return nil, err
	}

	return newEntry(
		offchainName,
		idlAccount.Name,
		accCodec,
		includeDiscriminator,
		mod,
	), nil
}

func NewInstructionArgsEntry(offChainName string, instructions IdlInstruction, idlTypes IdlTypeDefSlice, mod codec.Modifier, builder commonencodings.Builder) (Entry, error) {
	_, instructionCodecArgs, err := asStruct(instructions.Args, createRefs(idlTypes, builder), instructions.Name, false, true)
	if err != nil {
		return nil, err
	}

	return newEntry(
		offChainName,
		instructions.Name,
		instructionCodecArgs,
		// Instruction arguments don't need a discriminator by default
		false,
		mod,
	), nil
}

func NewEventArgsEntry(offChainName string, event IdlEvent, idlTypes IdlTypeDefSlice, includeDiscriminator bool, mod codec.Modifier, builder commonencodings.Builder) (Entry, error) {
	_, eventCodec, err := asStruct(eventFieldsToFields(event.Fields), createRefs(idlTypes, builder), event.Name, false, false)
	if err != nil {
		return nil, err
	}

	return newEntry(
		offChainName,
		event.Name,
		eventCodec,
		includeDiscriminator,
		mod,
	), nil
}

func newEntry(
	offchainName, onchainName string,
	typeCodec commonencodings.TypeCodec,
	includeDiscriminator bool,
	mod codec.Modifier,
) Entry {
	return &entry{
		offchainName:         offchainName,
		onchainName:          onchainName,
		reflectType:          typeCodec.GetType(),
		typeCodec:            typeCodec,
		mod:                  ensureModifier(mod),
		includeDiscriminator: includeDiscriminator,
		discriminator:        *NewDiscriminator(onchainName),
	}
}

func createRefs(idlTypes IdlTypeDefSlice, builder commonencodings.Builder) *codecRefs {
	return &codecRefs{
		builder:      builder,
		codecs:       make(map[string]commonencodings.TypeCodec),
		typeDefs:     idlTypes,
		dependencies: make(map[string][]string),
	}
}

func (e *entry) Encode(value any, into []byte) ([]byte, error) {
	// Special handling for encoding a nil pointer to an empty struct.
	t := e.reflectType
	if value == nil {
		if t.Kind() == reflect.Pointer {
			elem := t.Elem()
			if elem.Kind() == reflect.Struct && elem.NumField() == 0 {
				return []byte{}, nil
			}
		}
		return nil, fmt.Errorf("%w: cannot encode nil value for offchainName: %q, onchainName: %q",
			commontypes.ErrInvalidType, e.offchainName, e.onchainName)
	}

	encodedVal, err := e.typeCodec.Encode(value, into)
	if err != nil {
		return nil, err
	}

	if e.includeDiscriminator {
		var byt []byte
		encodedDisc, err := e.discriminator.Encode(&e.discriminator.hashPrefix, byt)
		if err != nil {
			return nil, err
		}
		return append(encodedDisc, encodedVal...), nil
	}

	return encodedVal, nil
}

func (e *entry) Decode(encoded []byte) (any, []byte, error) {
	if e.includeDiscriminator {
		if len(encoded) < discriminatorLength {
			return nil, nil, fmt.Errorf("%w: encoded data too short to contain discriminator for offchainName: %q, onchainName: %q",
				commontypes.ErrInvalidType, e.offchainName, e.onchainName)
		}

		if !bytes.Equal(e.discriminator.hashPrefix, encoded[:discriminatorLength]) {
			return nil, nil, fmt.Errorf("%w: encoded data has a bad discriminator %v for offchainName: %q, onchainName: %q",
				commontypes.ErrInvalidType, encoded[:discriminatorLength], e.offchainName, e.onchainName)
		}

		encoded = encoded[discriminatorLength:]
	}
	return e.typeCodec.Decode(encoded)
}

func (e *entry) GetCodecType() commonencodings.TypeCodec {
	return e.typeCodec
}

func (e *entry) GetType() reflect.Type {
	return e.reflectType
}

func (e *entry) Modifier() codec.Modifier {
	return e.mod
}

func (e *entry) Size(numItems int) (int, error) {
	return e.typeCodec.Size(numItems)
}

func (e *entry) FixedSize() (int, error) {
	return e.typeCodec.FixedSize()
}

func ensureModifier(mod codec.Modifier) codec.Modifier {
	if mod == nil {
		return codec.MultiModifier{}
	}
	return mod
}

func eventFieldsToFields(evFields []IdlEventField) []IdlField {
	var idlFields []IdlField
	for _, evField := range evFields {
		idlFields = append(idlFields, IdlField{
			Name: evField.Name,
			Type: evField.Type,
		})
	}
	return idlFields
}

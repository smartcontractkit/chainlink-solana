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
	genericName       string
	chainSpecificName string
	reflectType       reflect.Type
	typeCodec         commonencodings.TypeCodec
	mod               codec.Modifier
	// includeDiscriminator during Encode adds a discriminator to the encoded bytes under an assumption that the provided value didn't have a discriminator.
	// During Decode includeDiscriminator removes discriminator from bytes under an assumption that the provided struct doesn't need a discriminator.
	includeDiscriminator bool
	discriminator        Discriminator
}

type AccountIDLTypes struct {
	Account IdlTypeDef
	Types   IdlTypeDefSlice
}

func NewAccountEntry(offchainName string, idlTypes AccountIDLTypes, includeDiscriminator bool, mod codec.Modifier, builder commonencodings.Builder) (Entry, error) {
	_, accCodec, err := createCodecType(idlTypes.Account, createRefs(idlTypes.Types, builder), false)
	if err != nil {
		return nil, err
	}

	return newEntry(
		offchainName,
		idlTypes.Account.Name,
		accCodec,
		includeDiscriminator,
		mod,
	), nil
}

func NewPDAEntry(offchainName string, pdaTypeDef PDATypeDef, mod codec.Modifier, builder commonencodings.Builder) (Entry, error) {
	// PDA seeds do not have any dependecies in the IDL so the type def slice can be left empty for refs
	_, accCodec, err := asStruct(pdaSeedsToIdlField(pdaTypeDef.Seeds), createRefs(IdlTypeDefSlice{}, builder), offchainName, false, false)
	if err != nil {
		return nil, err
	}

	return newEntry(
		offchainName,
		offchainName, // PDA seeds do not correlate to anything on-chain so reusing offchain name
		accCodec,
		false,
		mod,
	), nil
}

type InstructionArgsIDLTypes struct {
	Instruction IdlInstruction
	Types       IdlTypeDefSlice
}

func NewInstructionArgsEntry(offChainName string, idlTypes InstructionArgsIDLTypes, mod codec.Modifier, builder commonencodings.Builder) (Entry, error) {
	_, instructionCodecArgs, err := asStruct(idlTypes.Instruction.Args, createRefs(idlTypes.Types, builder), idlTypes.Instruction.Name, false, true)
	if err != nil {
		return nil, err
	}

	return newEntry(
		offChainName,
		idlTypes.Instruction.Name,
		instructionCodecArgs,
		// Instruction arguments don't need a discriminator by default
		false,
		mod,
	), nil
}

type EventIDLTypes struct {
	Event IdlEvent
	Types IdlTypeDefSlice
}

func NewEventArgsEntry(offChainName string, idlTypes EventIDLTypes, includeDiscriminator bool, mod codec.Modifier, builder commonencodings.Builder) (Entry, error) {
	_, eventCodec, err := asStruct(eventFieldsToFields(idlTypes.Event.Fields), createRefs(idlTypes.Types, builder), idlTypes.Event.Name, false, false)
	if err != nil {
		return nil, err
	}

	return newEntry(
		offChainName,
		idlTypes.Event.Name,
		eventCodec,
		includeDiscriminator,
		mod,
	), nil
}

func newEntry(
	genericName, chainSpecificName string,
	typeCodec commonencodings.TypeCodec,
	includeDiscriminator bool,
	mod codec.Modifier,
) Entry {
	return &entry{
		genericName:          genericName,
		chainSpecificName:    chainSpecificName,
		reflectType:          typeCodec.GetType(),
		typeCodec:            typeCodec,
		mod:                  ensureModifier(mod),
		includeDiscriminator: includeDiscriminator,
		discriminator:        *NewDiscriminator(chainSpecificName),
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
		return nil, fmt.Errorf("%w: cannot encode nil value for genericName: %q, chainSpecificName: %q",
			commontypes.ErrInvalidType, e.genericName, e.chainSpecificName)
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
			return nil, nil, fmt.Errorf("%w: encoded data too short to contain discriminator for genericName: %q, chainSpecificName: %q",
				commontypes.ErrInvalidType, e.genericName, e.chainSpecificName)
		}

		if !bytes.Equal(e.discriminator.hashPrefix, encoded[:discriminatorLength]) {
			return nil, nil, fmt.Errorf("%w: encoded data has a bad discriminator %v for genericName: %q, chainSpecificName: %q",
				commontypes.ErrInvalidType, encoded[:discriminatorLength], e.genericName, e.chainSpecificName)
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

func pdaSeedsToIdlField(seeds []PDASeed) []IdlField {
	idlFields := make([]IdlField, 0, len(seeds))
	for _, seed := range seeds {
		idlFields = append(idlFields, IdlField{
			Name: seed.Name,
			Type: NewIdlStringType(seed.Type),
		})
	}
	return idlFields
}

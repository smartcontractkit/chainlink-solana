package codecv1

import (
	"github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
)

type AccountIDLTypes struct {
	Account IdlTypeDef
	Types   IdlTypeDefSlice
}

func NewAccountEntry(offchainName string, idlTypes AccountIDLTypes, includeDiscriminator bool, mod codec.Modifier, builder encodings.Builder) (solcommoncodec.Entry, error) {
	_, accCodec, err := createCodecType(idlTypes.Account, createRefs(idlTypes.Types, builder), false)
	if err != nil {
		return nil, err
	}

	var discriminator *solcommoncodec.Discriminator
	if includeDiscriminator {
		discriminator = solcommoncodec.NewDiscriminator(idlTypes.Account.Name, true)
	}

	return solcommoncodec.NewEntry(
		offchainName,
		idlTypes.Account.Name,
		accCodec,
		discriminator,
		mod,
	), nil
}

func NewPDAEntry(offchainName string, pdaTypeDef PDATypeDef, mod codec.Modifier, builder encodings.Builder) (solcommoncodec.Entry, error) {
	// PDA seeds do not have any dependencies in the IDL so the type def slice can be left empty for refs
	_, accCodec, err := asStruct(pdaSeedsToIdlField(pdaTypeDef.Seeds), createRefs(IdlTypeDefSlice{}, builder), offchainName, false, false)
	if err != nil {
		return nil, err
	}

	return solcommoncodec.NewEntry(
		offchainName,
		offchainName, // PDA seeds do not correlate to anything on-chain so reusing offchain name
		accCodec,
		nil,
		mod,
	), nil
}

type InstructionArgsIDLTypes struct {
	Instruction IdlInstruction
	Types       IdlTypeDefSlice
}

func NewInstructionArgsEntry(offChainName string, idlTypes InstructionArgsIDLTypes, mod codec.Modifier, builder encodings.Builder) (solcommoncodec.Entry, error) {
	_, instructionCodecArgs, err := asStruct(idlTypes.Instruction.Args, createRefs(idlTypes.Types, builder), idlTypes.Instruction.Name, false, true)
	if err != nil {
		return nil, err
	}

	return solcommoncodec.NewEntry(
		offChainName,
		idlTypes.Instruction.Name,
		instructionCodecArgs,
		// Instruction arguments don't need a discriminator by default
		nil,
		mod,
	), nil
}

type EventIDLTypes struct {
	Event IdlEvent
	Types IdlTypeDefSlice
}

func NewEventArgsEntry(offChainName string, idlTypes EventIDLTypes, includeDiscriminator bool, mod codec.Modifier, builder encodings.Builder) (solcommoncodec.Entry, error) {
	_, eventCodec, err := asStruct(eventFieldsToFields(idlTypes.Event.Fields), createRefs(idlTypes.Types, builder), idlTypes.Event.Name, false, false)
	if err != nil {
		return nil, err
	}

	var discriminator *solcommoncodec.Discriminator
	if includeDiscriminator {
		discriminator = solcommoncodec.NewDiscriminator(idlTypes.Event.Name, false)
	}

	return solcommoncodec.NewEntry(
		offChainName,
		idlTypes.Event.Name,
		eventCodec,
		discriminator,
		mod,
	), nil
}

func createRefs(idlTypes IdlTypeDefSlice, builder encodings.Builder) *codecRefs {
	return &codecRefs{
		builder:      builder,
		codecs:       make(map[string]encodings.TypeCodec),
		typeDefs:     idlTypes,
		dependencies: make(map[string][]string),
	}
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
			Type: seed.Type,
		})
	}
	return idlFields
}

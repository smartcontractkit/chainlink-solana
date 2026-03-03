package codecv2

import (
	anchoridl "github.com/gagliardetto/anchor-go/idl"

	"github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
)

// No AccountIDLTypes, NewAccountEntry, NewPDAEntry required as chain_reader (codec based) has moved to chain_accessor (binding based)

type InstructionArgsIDLTypes struct {
	Instruction anchoridl.IdlInstruction
	Types       anchoridl.IdTypeDef_slice
}

func NewInstructionArgsEntry(offChainName string, idlTypes InstructionArgsIDLTypes, mod codec.Modifier, builder encodings.Builder) (solcommoncodec.Entry, error) {
	instructionCodecArgs, err := asStructForInstructionArgs(idlTypes.Instruction.Args, createRefs(idlTypes.Types, builder), idlTypes.Instruction.Name)
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
	Event anchoridl.IdlEvent
	Types anchoridl.IdTypeDef_slice
}

func NewEventArgsEntry(offChainName string, idlTypes EventIDLTypes, includeDiscriminator bool, mod codec.Modifier, builder encodings.Builder) (solcommoncodec.Entry, error) {
	_, eventCodec, err := asStruct(createRefs(idlTypes.Types, builder), idlTypes.Event.Name)
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

func createRefs(idlTypes anchoridl.IdTypeDef_slice, builder encodings.Builder) *codecRefs {
	return &codecRefs{
		builder:      builder,
		codecs:       make(map[string]encodings.TypeCodec),
		typeDefs:     idlTypes,
		dependencies: make(map[string][]string),
	}
}

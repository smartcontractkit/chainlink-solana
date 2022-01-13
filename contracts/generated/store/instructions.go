// Code generated by https://github.com/gagliardetto/anchor-go. DO NOT EDIT.

package store

import (
	"bytes"
	"fmt"
	ag_spew "github.com/davecgh/go-spew/spew"
	ag_binary "github.com/gagliardetto/binary"
	ag_solanago "github.com/gagliardetto/solana-go"
	ag_text "github.com/gagliardetto/solana-go/text"
	ag_treeout "github.com/gagliardetto/treeout"
)

var ProgramID ag_solanago.PublicKey

func SetProgramID(pubkey ag_solanago.PublicKey) {
	ProgramID = pubkey
	ag_solanago.RegisterInstructionDecoder(ProgramID, registryDecodeInstruction)
}

const ProgramName = "Store"

func init() {
	if !ProgramID.IsZero() {
		ag_solanago.RegisterInstructionDecoder(ProgramID, registryDecodeInstruction)
	}
}

var (
	Instruction_Initialize = ag_binary.TypeID([8]byte{175, 175, 109, 31, 13, 152, 155, 237})

	Instruction_CreateFeed = ag_binary.TypeID([8]byte{173, 86, 95, 94, 13, 193, 67, 180})

	Instruction_CloseFeed = ag_binary.TypeID([8]byte{153, 14, 92, 89, 19, 78, 211, 46})

	Instruction_SetValidatorConfig = ag_binary.TypeID([8]byte{87, 248, 224, 193, 17, 41, 80, 250})

	Instruction_SetWriter = ag_binary.TypeID([8]byte{174, 36, 177, 122, 86, 142, 32, 109})

	Instruction_TransferOwnership = ag_binary.TypeID([8]byte{65, 177, 215, 73, 53, 45, 99, 47})

	Instruction_AcceptOwnership = ag_binary.TypeID([8]byte{172, 23, 43, 13, 238, 213, 85, 150})

	Instruction_Submit = ag_binary.TypeID([8]byte{88, 166, 102, 181, 162, 127, 170, 48})

	Instruction_LowerFlags = ag_binary.TypeID([8]byte{254, 119, 153, 185, 135, 158, 115, 23})

	Instruction_SetLoweringAccessController = ag_binary.TypeID([8]byte{207, 68, 147, 34, 164, 94, 189, 113})

	Instruction_Query = ag_binary.TypeID([8]byte{39, 251, 130, 159, 46, 136, 164, 169})
)

// InstructionIDToName returns the name of the instruction given its ID.
func InstructionIDToName(id ag_binary.TypeID) string {
	switch id {
	case Instruction_Initialize:
		return "Initialize"
	case Instruction_CreateFeed:
		return "CreateFeed"
	case Instruction_CloseFeed:
		return "CloseFeed"
	case Instruction_SetValidatorConfig:
		return "SetValidatorConfig"
	case Instruction_SetWriter:
		return "SetWriter"
	case Instruction_TransferOwnership:
		return "TransferOwnership"
	case Instruction_AcceptOwnership:
		return "AcceptOwnership"
	case Instruction_Submit:
		return "Submit"
	case Instruction_LowerFlags:
		return "LowerFlags"
	case Instruction_SetLoweringAccessController:
		return "SetLoweringAccessController"
	case Instruction_Query:
		return "Query"
	default:
		return ""
	}
}

type Instruction struct {
	ag_binary.BaseVariant
}

func (inst *Instruction) EncodeToTree(parent ag_treeout.Branches) {
	if enToTree, ok := inst.Impl.(ag_text.EncodableToTree); ok {
		enToTree.EncodeToTree(parent)
	} else {
		parent.Child(ag_spew.Sdump(inst))
	}
}

var InstructionImplDef = ag_binary.NewVariantDefinition(
	ag_binary.AnchorTypeIDEncoding,
	[]ag_binary.VariantType{
		{
			"initialize", (*Initialize)(nil),
		},
		{
			"create_feed", (*CreateFeed)(nil),
		},
		{
			"close_feed", (*CloseFeed)(nil),
		},
		{
			"set_validator_config", (*SetValidatorConfig)(nil),
		},
		{
			"set_writer", (*SetWriter)(nil),
		},
		{
			"transfer_ownership", (*TransferOwnership)(nil),
		},
		{
			"accept_ownership", (*AcceptOwnership)(nil),
		},
		{
			"submit", (*Submit)(nil),
		},
		{
			"lower_flags", (*LowerFlags)(nil),
		},
		{
			"set_lowering_access_controller", (*SetLoweringAccessController)(nil),
		},
		{
			"query", (*Query)(nil),
		},
	},
)

func (inst *Instruction) ProgramID() ag_solanago.PublicKey {
	return ProgramID
}

func (inst *Instruction) Accounts() (out []*ag_solanago.AccountMeta) {
	return inst.Impl.(ag_solanago.AccountsGettable).GetAccounts()
}

func (inst *Instruction) Data() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := ag_binary.NewBorshEncoder(buf).Encode(inst); err != nil {
		return nil, fmt.Errorf("unable to encode instruction: %w", err)
	}
	return buf.Bytes(), nil
}

func (inst *Instruction) TextEncode(encoder *ag_text.Encoder, option *ag_text.Option) error {
	return encoder.Encode(inst.Impl, option)
}

func (inst *Instruction) UnmarshalWithDecoder(decoder *ag_binary.Decoder) error {
	return inst.BaseVariant.UnmarshalBinaryVariant(decoder, InstructionImplDef)
}

func (inst *Instruction) MarshalWithEncoder(encoder *ag_binary.Encoder) error {
	err := encoder.WriteBytes(inst.TypeID.Bytes(), false)
	if err != nil {
		return fmt.Errorf("unable to write variant type: %w", err)
	}
	return encoder.Encode(inst.Impl)
}

func registryDecodeInstruction(accounts []*ag_solanago.AccountMeta, data []byte) (interface{}, error) {
	inst, err := DecodeInstruction(accounts, data)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

func DecodeInstruction(accounts []*ag_solanago.AccountMeta, data []byte) (*Instruction, error) {
	inst := new(Instruction)
	if err := ag_binary.NewBorshDecoder(data).Decode(inst); err != nil {
		return nil, fmt.Errorf("unable to decode instruction: %w", err)
	}
	if v, ok := inst.Impl.(ag_solanago.AccountsSettable); ok {
		err := v.SetAccounts(accounts)
		if err != nil {
			return nil, fmt.Errorf("unable to set accounts for instruction: %w", err)
		}
	}
	return inst, nil
}

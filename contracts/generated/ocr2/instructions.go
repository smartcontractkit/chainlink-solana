// Code generated by https://github.com/gagliardetto/anchor-go. DO NOT EDIT.

package ocr_2

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

const ProgramName = "Ocr2"

func init() {
	if !ProgramID.IsZero() {
		ag_solanago.RegisterInstructionDecoder(ProgramID, registryDecodeInstruction)
	}
}

var (
	Instruction_Initialize = ag_binary.TypeID([8]byte{175, 175, 109, 31, 13, 152, 155, 237})

	Instruction_Close = ag_binary.TypeID([8]byte{98, 165, 201, 177, 108, 65, 206, 96})

	Instruction_TransferOwnership = ag_binary.TypeID([8]byte{65, 177, 215, 73, 53, 45, 99, 47})

	Instruction_AcceptOwnership = ag_binary.TypeID([8]byte{172, 23, 43, 13, 238, 213, 85, 150})

	Instruction_BeginOffchainConfig = ag_binary.TypeID([8]byte{124, 77, 17, 185, 6, 147, 219, 60})

	Instruction_WriteOffchainConfig = ag_binary.TypeID([8]byte{171, 64, 173, 138, 151, 188, 68, 168})

	Instruction_CommitOffchainConfig = ag_binary.TypeID([8]byte{56, 171, 18, 191, 137, 247, 109, 33})

	Instruction_SetConfig = ag_binary.TypeID([8]byte{108, 158, 154, 175, 212, 98, 52, 66})

	Instruction_SetRequesterAccessController = ag_binary.TypeID([8]byte{182, 229, 210, 202, 190, 116, 92, 236})

	Instruction_RequestNewRound = ag_binary.TypeID([8]byte{79, 230, 6, 173, 193, 109, 226, 61})

	Instruction_SetBillingAccessController = ag_binary.TypeID([8]byte{176, 167, 195, 39, 175, 182, 51, 23})

	Instruction_SetBilling = ag_binary.TypeID([8]byte{58, 131, 213, 166, 230, 120, 88, 95})

	Instruction_WithdrawFunds = ag_binary.TypeID([8]byte{241, 36, 29, 111, 208, 31, 104, 217})

	Instruction_WithdrawPayment = ag_binary.TypeID([8]byte{118, 231, 133, 187, 151, 154, 111, 95})

	Instruction_PayRemaining = ag_binary.TypeID([8]byte{183, 66, 188, 183, 187, 154, 20, 99})

	Instruction_PayOracles = ag_binary.TypeID([8]byte{150, 220, 13, 20, 104, 214, 61, 89})

	Instruction_SetPayees = ag_binary.TypeID([8]byte{92, 10, 255, 107, 111, 30, 22, 33})

	Instruction_TransferPayeeship = ag_binary.TypeID([8]byte{116, 68, 213, 225, 193, 225, 171, 206})

	Instruction_AcceptPayeeship = ag_binary.TypeID([8]byte{142, 208, 219, 62, 82, 13, 189, 70})
)

// InstructionIDToName returns the name of the instruction given its ID.
func InstructionIDToName(id ag_binary.TypeID) string {
	switch id {
	case Instruction_Initialize:
		return "Initialize"
	case Instruction_Close:
		return "Close"
	case Instruction_TransferOwnership:
		return "TransferOwnership"
	case Instruction_AcceptOwnership:
		return "AcceptOwnership"
	case Instruction_BeginOffchainConfig:
		return "BeginOffchainConfig"
	case Instruction_WriteOffchainConfig:
		return "WriteOffchainConfig"
	case Instruction_CommitOffchainConfig:
		return "CommitOffchainConfig"
	case Instruction_SetConfig:
		return "SetConfig"
	case Instruction_SetRequesterAccessController:
		return "SetRequesterAccessController"
	case Instruction_RequestNewRound:
		return "RequestNewRound"
	case Instruction_SetBillingAccessController:
		return "SetBillingAccessController"
	case Instruction_SetBilling:
		return "SetBilling"
	case Instruction_WithdrawFunds:
		return "WithdrawFunds"
	case Instruction_WithdrawPayment:
		return "WithdrawPayment"
	case Instruction_PayRemaining:
		return "PayRemaining"
	case Instruction_PayOracles:
		return "PayOracles"
	case Instruction_SetPayees:
		return "SetPayees"
	case Instruction_TransferPayeeship:
		return "TransferPayeeship"
	case Instruction_AcceptPayeeship:
		return "AcceptPayeeship"
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
			"close", (*Close)(nil),
		},
		{
			"transfer_ownership", (*TransferOwnership)(nil),
		},
		{
			"accept_ownership", (*AcceptOwnership)(nil),
		},
		{
			"begin_offchain_config", (*BeginOffchainConfig)(nil),
		},
		{
			"write_offchain_config", (*WriteOffchainConfig)(nil),
		},
		{
			"commit_offchain_config", (*CommitOffchainConfig)(nil),
		},
		{
			"set_config", (*SetConfig)(nil),
		},
		{
			"set_requester_access_controller", (*SetRequesterAccessController)(nil),
		},
		{
			"request_new_round", (*RequestNewRound)(nil),
		},
		{
			"set_billing_access_controller", (*SetBillingAccessController)(nil),
		},
		{
			"set_billing", (*SetBilling)(nil),
		},
		{
			"withdraw_funds", (*WithdrawFunds)(nil),
		},
		{
			"withdraw_payment", (*WithdrawPayment)(nil),
		},
		{
			"pay_remaining", (*PayRemaining)(nil),
		},
		{
			"pay_oracles", (*PayOracles)(nil),
		},
		{
			"set_payees", (*SetPayees)(nil),
		},
		{
			"transfer_payeeship", (*TransferPayeeship)(nil),
		},
		{
			"accept_payeeship", (*AcceptPayeeship)(nil),
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

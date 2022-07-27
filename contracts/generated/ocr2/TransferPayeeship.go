// Code generated by https://github.com/gagliardetto/anchor-go. DO NOT EDIT.

package ocr2

import (
	"errors"
	ag_binary "github.com/gagliardetto/binary"
	ag_solanago "github.com/gagliardetto/solana-go"
	ag_format "github.com/gagliardetto/solana-go/text/format"
	ag_treeout "github.com/gagliardetto/treeout"
)

// TransferPayeeship is the `transferPayeeship` instruction.
type TransferPayeeship struct {

	// [0] = [WRITE] state
	//
	// [1] = [SIGNER] authority
	//
	// [2] = [] transmitter
	//
	// [3] = [] payee
	//
	// [4] = [] proposedPayee
	ag_solanago.AccountMetaSlice `bin:"-"`
}

// NewTransferPayeeshipInstructionBuilder creates a new `TransferPayeeship` instruction builder.
func NewTransferPayeeshipInstructionBuilder() *TransferPayeeship {
	nd := &TransferPayeeship{
		AccountMetaSlice: make(ag_solanago.AccountMetaSlice, 5),
	}
	return nd
}

// SetStateAccount sets the "state" account.
func (inst *TransferPayeeship) SetStateAccount(state ag_solanago.PublicKey) *TransferPayeeship {
	inst.AccountMetaSlice[0] = ag_solanago.Meta(state).WRITE()
	return inst
}

// GetStateAccount gets the "state" account.
func (inst *TransferPayeeship) GetStateAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice.Get(0)
}

// SetAuthorityAccount sets the "authority" account.
func (inst *TransferPayeeship) SetAuthorityAccount(authority ag_solanago.PublicKey) *TransferPayeeship {
	inst.AccountMetaSlice[1] = ag_solanago.Meta(authority).SIGNER()
	return inst
}

// GetAuthorityAccount gets the "authority" account.
func (inst *TransferPayeeship) GetAuthorityAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice.Get(1)
}

// SetTransmitterAccount sets the "transmitter" account.
func (inst *TransferPayeeship) SetTransmitterAccount(transmitter ag_solanago.PublicKey) *TransferPayeeship {
	inst.AccountMetaSlice[2] = ag_solanago.Meta(transmitter)
	return inst
}

// GetTransmitterAccount gets the "transmitter" account.
func (inst *TransferPayeeship) GetTransmitterAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice.Get(2)
}

// SetPayeeAccount sets the "payee" account.
func (inst *TransferPayeeship) SetPayeeAccount(payee ag_solanago.PublicKey) *TransferPayeeship {
	inst.AccountMetaSlice[3] = ag_solanago.Meta(payee)
	return inst
}

// GetPayeeAccount gets the "payee" account.
func (inst *TransferPayeeship) GetPayeeAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice.Get(3)
}

// SetProposedPayeeAccount sets the "proposedPayee" account.
func (inst *TransferPayeeship) SetProposedPayeeAccount(proposedPayee ag_solanago.PublicKey) *TransferPayeeship {
	inst.AccountMetaSlice[4] = ag_solanago.Meta(proposedPayee)
	return inst
}

// GetProposedPayeeAccount gets the "proposedPayee" account.
func (inst *TransferPayeeship) GetProposedPayeeAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice.Get(4)
}

func (inst TransferPayeeship) Build() *Instruction {
	return &Instruction{BaseVariant: ag_binary.BaseVariant{
		Impl:   inst,
		TypeID: Instruction_TransferPayeeship,
	}}
}

// ValidateAndBuild validates the instruction parameters and accounts;
// if there is a validation error, it returns the error.
// Otherwise, it builds and returns the instruction.
func (inst TransferPayeeship) ValidateAndBuild() (*Instruction, error) {
	if err := inst.Validate(); err != nil {
		return nil, err
	}
	return inst.Build(), nil
}

func (inst *TransferPayeeship) Validate() error {
	// Check whether all (required) accounts are set:
	{
		if inst.AccountMetaSlice[0] == nil {
			return errors.New("accounts.State is not set")
		}
		if inst.AccountMetaSlice[1] == nil {
			return errors.New("accounts.Authority is not set")
		}
		if inst.AccountMetaSlice[2] == nil {
			return errors.New("accounts.Transmitter is not set")
		}
		if inst.AccountMetaSlice[3] == nil {
			return errors.New("accounts.Payee is not set")
		}
		if inst.AccountMetaSlice[4] == nil {
			return errors.New("accounts.ProposedPayee is not set")
		}
	}
	return nil
}

func (inst *TransferPayeeship) EncodeToTree(parent ag_treeout.Branches) {
	parent.Child(ag_format.Program(ProgramName, ProgramID)).
		//
		ParentFunc(func(programBranch ag_treeout.Branches) {
			programBranch.Child(ag_format.Instruction("TransferPayeeship")).
				//
				ParentFunc(func(instructionBranch ag_treeout.Branches) {

					// Parameters of the instruction:
					instructionBranch.Child("Params[len=0]").ParentFunc(func(paramsBranch ag_treeout.Branches) {})

					// Accounts of the instruction:
					instructionBranch.Child("Accounts[len=5]").ParentFunc(func(accountsBranch ag_treeout.Branches) {
						accountsBranch.Child(ag_format.Meta("        state", inst.AccountMetaSlice.Get(0)))
						accountsBranch.Child(ag_format.Meta("    authority", inst.AccountMetaSlice.Get(1)))
						accountsBranch.Child(ag_format.Meta("  transmitter", inst.AccountMetaSlice.Get(2)))
						accountsBranch.Child(ag_format.Meta("        payee", inst.AccountMetaSlice.Get(3)))
						accountsBranch.Child(ag_format.Meta("proposedPayee", inst.AccountMetaSlice.Get(4)))
					})
				})
		})
}

func (obj TransferPayeeship) MarshalWithEncoder(encoder *ag_binary.Encoder) (err error) {
	return nil
}
func (obj *TransferPayeeship) UnmarshalWithDecoder(decoder *ag_binary.Decoder) (err error) {
	return nil
}

// NewTransferPayeeshipInstruction declares a new TransferPayeeship instruction with the provided parameters and accounts.
func NewTransferPayeeshipInstruction(
	// Accounts:
	state ag_solanago.PublicKey,
	authority ag_solanago.PublicKey,
	transmitter ag_solanago.PublicKey,
	payee ag_solanago.PublicKey,
	proposedPayee ag_solanago.PublicKey) *TransferPayeeship {
	return NewTransferPayeeshipInstructionBuilder().
		SetStateAccount(state).
		SetAuthorityAccount(authority).
		SetTransmitterAccount(transmitter).
		SetPayeeAccount(payee).
		SetProposedPayeeAccount(proposedPayee)
}

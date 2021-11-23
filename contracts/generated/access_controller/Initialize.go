// Code generated by https://github.com/gagliardetto/anchor-go. DO NOT EDIT.

package access_controller

import (
	"errors"
	ag_binary "github.com/gagliardetto/binary"
	ag_solanago "github.com/gagliardetto/solana-go"
	ag_format "github.com/gagliardetto/solana-go/text/format"
	ag_treeout "github.com/gagliardetto/treeout"
)

// Initialize is the `initialize` instruction.
type Initialize struct {

	// [0] = [WRITE] state
	//
	// [1] = [] payer
	//
	// [2] = [SIGNER] owner
	//
	// [3] = [] rent
	//
	// [4] = [] systemProgram
	ag_solanago.AccountMetaSlice `bin:"-" borsh_skip:"true"`
}

// NewInitializeInstructionBuilder creates a new `Initialize` instruction builder.
func NewInitializeInstructionBuilder() *Initialize {
	nd := &Initialize{
		AccountMetaSlice: make(ag_solanago.AccountMetaSlice, 5),
	}
	return nd
}

// SetStateAccount sets the "state" account.
func (inst *Initialize) SetStateAccount(state ag_solanago.PublicKey) *Initialize {
	inst.AccountMetaSlice[0] = ag_solanago.Meta(state).WRITE()
	return inst
}

// GetStateAccount gets the "state" account.
func (inst *Initialize) GetStateAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[0]
}

// SetPayerAccount sets the "payer" account.
func (inst *Initialize) SetPayerAccount(payer ag_solanago.PublicKey) *Initialize {
	inst.AccountMetaSlice[1] = ag_solanago.Meta(payer)
	return inst
}

// GetPayerAccount gets the "payer" account.
func (inst *Initialize) GetPayerAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[1]
}

// SetOwnerAccount sets the "owner" account.
func (inst *Initialize) SetOwnerAccount(owner ag_solanago.PublicKey) *Initialize {
	inst.AccountMetaSlice[2] = ag_solanago.Meta(owner).SIGNER()
	return inst
}

// GetOwnerAccount gets the "owner" account.
func (inst *Initialize) GetOwnerAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[2]
}

// SetRentAccount sets the "rent" account.
func (inst *Initialize) SetRentAccount(rent ag_solanago.PublicKey) *Initialize {
	inst.AccountMetaSlice[3] = ag_solanago.Meta(rent)
	return inst
}

// GetRentAccount gets the "rent" account.
func (inst *Initialize) GetRentAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[3]
}

// SetSystemProgramAccount sets the "systemProgram" account.
func (inst *Initialize) SetSystemProgramAccount(systemProgram ag_solanago.PublicKey) *Initialize {
	inst.AccountMetaSlice[4] = ag_solanago.Meta(systemProgram)
	return inst
}

// GetSystemProgramAccount gets the "systemProgram" account.
func (inst *Initialize) GetSystemProgramAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[4]
}

func (inst Initialize) Build() *Instruction {
	return &Instruction{BaseVariant: ag_binary.BaseVariant{
		Impl:   inst,
		TypeID: Instruction_Initialize,
	}}
}

// ValidateAndBuild validates the instruction parameters and accounts;
// if there is a validation error, it returns the error.
// Otherwise, it builds and returns the instruction.
func (inst Initialize) ValidateAndBuild() (*Instruction, error) {
	if err := inst.Validate(); err != nil {
		return nil, err
	}
	return inst.Build(), nil
}

func (inst *Initialize) Validate() error {
	// Check whether all (required) accounts are set:
	{
		if inst.AccountMetaSlice[0] == nil {
			return errors.New("accounts.State is not set")
		}
		if inst.AccountMetaSlice[1] == nil {
			return errors.New("accounts.Payer is not set")
		}
		if inst.AccountMetaSlice[2] == nil {
			return errors.New("accounts.Owner is not set")
		}
		if inst.AccountMetaSlice[3] == nil {
			return errors.New("accounts.Rent is not set")
		}
		if inst.AccountMetaSlice[4] == nil {
			return errors.New("accounts.SystemProgram is not set")
		}
	}
	return nil
}

func (inst *Initialize) EncodeToTree(parent ag_treeout.Branches) {
	parent.Child(ag_format.Program(ProgramName, ProgramID)).
		//
		ParentFunc(func(programBranch ag_treeout.Branches) {
			programBranch.Child(ag_format.Instruction("Initialize")).
				//
				ParentFunc(func(instructionBranch ag_treeout.Branches) {

					// Parameters of the instruction:
					instructionBranch.Child("Params[len=0]").ParentFunc(func(paramsBranch ag_treeout.Branches) {})

					// Accounts of the instruction:
					instructionBranch.Child("Accounts[len=5]").ParentFunc(func(accountsBranch ag_treeout.Branches) {
						accountsBranch.Child(ag_format.Meta("        state", inst.AccountMetaSlice[0]))
						accountsBranch.Child(ag_format.Meta("        payer", inst.AccountMetaSlice[1]))
						accountsBranch.Child(ag_format.Meta("        owner", inst.AccountMetaSlice[2]))
						accountsBranch.Child(ag_format.Meta("         rent", inst.AccountMetaSlice[3]))
						accountsBranch.Child(ag_format.Meta("systemProgram", inst.AccountMetaSlice[4]))
					})
				})
		})
}

func (obj Initialize) MarshalWithEncoder(encoder *ag_binary.Encoder) (err error) {
	return nil
}
func (obj *Initialize) UnmarshalWithDecoder(decoder *ag_binary.Decoder) (err error) {
	return nil
}

// NewInitializeInstruction declares a new Initialize instruction with the provided parameters and accounts.
func NewInitializeInstruction(
	// Accounts:
	state ag_solanago.PublicKey,
	payer ag_solanago.PublicKey,
	owner ag_solanago.PublicKey,
	rent ag_solanago.PublicKey,
	systemProgram ag_solanago.PublicKey) *Initialize {
	return NewInitializeInstructionBuilder().
		SetStateAccount(state).
		SetPayerAccount(payer).
		SetOwnerAccount(owner).
		SetRentAccount(rent).
		SetSystemProgramAccount(systemProgram)
}

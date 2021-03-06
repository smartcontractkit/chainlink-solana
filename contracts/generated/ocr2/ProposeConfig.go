// Code generated by https://github.com/gagliardetto/anchor-go. DO NOT EDIT.

package ocr_2

import (
	"errors"
	ag_binary "github.com/gagliardetto/binary"
	ag_solanago "github.com/gagliardetto/solana-go"
	ag_format "github.com/gagliardetto/solana-go/text/format"
	ag_treeout "github.com/gagliardetto/treeout"
)

// ProposeConfig is the `proposeConfig` instruction.
type ProposeConfig struct {
	NewOracles *[]NewOracle
	F          *uint8

	// [0] = [WRITE] proposal
	//
	// [1] = [SIGNER] authority
	ag_solanago.AccountMetaSlice `bin:"-" borsh_skip:"true"`
}

// NewProposeConfigInstructionBuilder creates a new `ProposeConfig` instruction builder.
func NewProposeConfigInstructionBuilder() *ProposeConfig {
	nd := &ProposeConfig{
		AccountMetaSlice: make(ag_solanago.AccountMetaSlice, 2),
	}
	return nd
}

// SetNewOracles sets the "newOracles" parameter.
func (inst *ProposeConfig) SetNewOracles(newOracles []NewOracle) *ProposeConfig {
	inst.NewOracles = &newOracles
	return inst
}

// SetF sets the "f" parameter.
func (inst *ProposeConfig) SetF(f uint8) *ProposeConfig {
	inst.F = &f
	return inst
}

// SetProposalAccount sets the "proposal" account.
func (inst *ProposeConfig) SetProposalAccount(proposal ag_solanago.PublicKey) *ProposeConfig {
	inst.AccountMetaSlice[0] = ag_solanago.Meta(proposal).WRITE()
	return inst
}

// GetProposalAccount gets the "proposal" account.
func (inst *ProposeConfig) GetProposalAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[0]
}

// SetAuthorityAccount sets the "authority" account.
func (inst *ProposeConfig) SetAuthorityAccount(authority ag_solanago.PublicKey) *ProposeConfig {
	inst.AccountMetaSlice[1] = ag_solanago.Meta(authority).SIGNER()
	return inst
}

// GetAuthorityAccount gets the "authority" account.
func (inst *ProposeConfig) GetAuthorityAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[1]
}

func (inst ProposeConfig) Build() *Instruction {
	return &Instruction{BaseVariant: ag_binary.BaseVariant{
		Impl:   inst,
		TypeID: Instruction_ProposeConfig,
	}}
}

// ValidateAndBuild validates the instruction parameters and accounts;
// if there is a validation error, it returns the error.
// Otherwise, it builds and returns the instruction.
func (inst ProposeConfig) ValidateAndBuild() (*Instruction, error) {
	if err := inst.Validate(); err != nil {
		return nil, err
	}
	return inst.Build(), nil
}

func (inst *ProposeConfig) Validate() error {
	// Check whether all (required) parameters are set:
	{
		if inst.NewOracles == nil {
			return errors.New("NewOracles parameter is not set")
		}
		if inst.F == nil {
			return errors.New("F parameter is not set")
		}
	}

	// Check whether all (required) accounts are set:
	{
		if inst.AccountMetaSlice[0] == nil {
			return errors.New("accounts.Proposal is not set")
		}
		if inst.AccountMetaSlice[1] == nil {
			return errors.New("accounts.Authority is not set")
		}
	}
	return nil
}

func (inst *ProposeConfig) EncodeToTree(parent ag_treeout.Branches) {
	parent.Child(ag_format.Program(ProgramName, ProgramID)).
		//
		ParentFunc(func(programBranch ag_treeout.Branches) {
			programBranch.Child(ag_format.Instruction("ProposeConfig")).
				//
				ParentFunc(func(instructionBranch ag_treeout.Branches) {

					// Parameters of the instruction:
					instructionBranch.Child("Params[len=2]").ParentFunc(func(paramsBranch ag_treeout.Branches) {
						paramsBranch.Child(ag_format.Param("NewOracles", *inst.NewOracles))
						paramsBranch.Child(ag_format.Param("         F", *inst.F))
					})

					// Accounts of the instruction:
					instructionBranch.Child("Accounts[len=2]").ParentFunc(func(accountsBranch ag_treeout.Branches) {
						accountsBranch.Child(ag_format.Meta(" proposal", inst.AccountMetaSlice[0]))
						accountsBranch.Child(ag_format.Meta("authority", inst.AccountMetaSlice[1]))
					})
				})
		})
}

func (obj ProposeConfig) MarshalWithEncoder(encoder *ag_binary.Encoder) (err error) {
	// Serialize `NewOracles` param:
	err = encoder.Encode(obj.NewOracles)
	if err != nil {
		return err
	}
	// Serialize `F` param:
	err = encoder.Encode(obj.F)
	if err != nil {
		return err
	}
	return nil
}
func (obj *ProposeConfig) UnmarshalWithDecoder(decoder *ag_binary.Decoder) (err error) {
	// Deserialize `NewOracles`:
	err = decoder.Decode(&obj.NewOracles)
	if err != nil {
		return err
	}
	// Deserialize `F`:
	err = decoder.Decode(&obj.F)
	if err != nil {
		return err
	}
	return nil
}

// NewProposeConfigInstruction declares a new ProposeConfig instruction with the provided parameters and accounts.
func NewProposeConfigInstruction(
	// Parameters:
	newOracles []NewOracle,
	f uint8,
	// Accounts:
	proposal ag_solanago.PublicKey,
	authority ag_solanago.PublicKey) *ProposeConfig {
	return NewProposeConfigInstructionBuilder().
		SetNewOracles(newOracles).
		SetF(f).
		SetProposalAccount(proposal).
		SetAuthorityAccount(authority)
}

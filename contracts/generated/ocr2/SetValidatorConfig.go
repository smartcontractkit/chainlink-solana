// Code generated by https://github.com/gagliardetto/anchor-go. DO NOT EDIT.

package ocr_2

import (
	"errors"
	ag_binary "github.com/gagliardetto/binary"
	ag_solanago "github.com/gagliardetto/solana-go"
	ag_format "github.com/gagliardetto/solana-go/text/format"
	ag_treeout "github.com/gagliardetto/treeout"
)

// SetValidatorConfig is the `setValidatorConfig` instruction.
type SetValidatorConfig struct {
	FlaggingThreshold *uint32

	// [0] = [WRITE] state
	//
	// [1] = [SIGNER] authority
	//
	// [2] = [] validator
	ag_solanago.AccountMetaSlice `bin:"-" borsh_skip:"true"`
}

// NewSetValidatorConfigInstructionBuilder creates a new `SetValidatorConfig` instruction builder.
func NewSetValidatorConfigInstructionBuilder() *SetValidatorConfig {
	nd := &SetValidatorConfig{
		AccountMetaSlice: make(ag_solanago.AccountMetaSlice, 3),
	}
	return nd
}

// SetFlaggingThreshold sets the "flaggingThreshold" parameter.
func (inst *SetValidatorConfig) SetFlaggingThreshold(flaggingThreshold uint32) *SetValidatorConfig {
	inst.FlaggingThreshold = &flaggingThreshold
	return inst
}

// SetStateAccount sets the "state" account.
func (inst *SetValidatorConfig) SetStateAccount(state ag_solanago.PublicKey) *SetValidatorConfig {
	inst.AccountMetaSlice[0] = ag_solanago.Meta(state).WRITE()
	return inst
}

// GetStateAccount gets the "state" account.
func (inst *SetValidatorConfig) GetStateAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[0]
}

// SetAuthorityAccount sets the "authority" account.
func (inst *SetValidatorConfig) SetAuthorityAccount(authority ag_solanago.PublicKey) *SetValidatorConfig {
	inst.AccountMetaSlice[1] = ag_solanago.Meta(authority).SIGNER()
	return inst
}

// GetAuthorityAccount gets the "authority" account.
func (inst *SetValidatorConfig) GetAuthorityAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[1]
}

// SetValidatorAccount sets the "validator" account.
func (inst *SetValidatorConfig) SetValidatorAccount(validator ag_solanago.PublicKey) *SetValidatorConfig {
	inst.AccountMetaSlice[2] = ag_solanago.Meta(validator)
	return inst
}

// GetValidatorAccount gets the "validator" account.
func (inst *SetValidatorConfig) GetValidatorAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[2]
}

func (inst SetValidatorConfig) Build() *Instruction {
	return &Instruction{BaseVariant: ag_binary.BaseVariant{
		Impl:   inst,
		TypeID: Instruction_SetValidatorConfig,
	}}
}

// ValidateAndBuild validates the instruction parameters and accounts;
// if there is a validation error, it returns the error.
// Otherwise, it builds and returns the instruction.
func (inst SetValidatorConfig) ValidateAndBuild() (*Instruction, error) {
	if err := inst.Validate(); err != nil {
		return nil, err
	}
	return inst.Build(), nil
}

func (inst *SetValidatorConfig) Validate() error {
	// Check whether all (required) parameters are set:
	{
		if inst.FlaggingThreshold == nil {
			return errors.New("FlaggingThreshold parameter is not set")
		}
	}

	// Check whether all (required) accounts are set:
	{
		if inst.AccountMetaSlice[0] == nil {
			return errors.New("accounts.State is not set")
		}
		if inst.AccountMetaSlice[1] == nil {
			return errors.New("accounts.Authority is not set")
		}
		if inst.AccountMetaSlice[2] == nil {
			return errors.New("accounts.Validator is not set")
		}
	}
	return nil
}

func (inst *SetValidatorConfig) EncodeToTree(parent ag_treeout.Branches) {
	parent.Child(ag_format.Program(ProgramName, ProgramID)).
		//
		ParentFunc(func(programBranch ag_treeout.Branches) {
			programBranch.Child(ag_format.Instruction("SetValidatorConfig")).
				//
				ParentFunc(func(instructionBranch ag_treeout.Branches) {

					// Parameters of the instruction:
					instructionBranch.Child("Params[len=1]").ParentFunc(func(paramsBranch ag_treeout.Branches) {
						paramsBranch.Child(ag_format.Param("FlaggingThreshold", *inst.FlaggingThreshold))
					})

					// Accounts of the instruction:
					instructionBranch.Child("Accounts[len=3]").ParentFunc(func(accountsBranch ag_treeout.Branches) {
						accountsBranch.Child(ag_format.Meta("    state", inst.AccountMetaSlice[0]))
						accountsBranch.Child(ag_format.Meta("authority", inst.AccountMetaSlice[1]))
						accountsBranch.Child(ag_format.Meta("validator", inst.AccountMetaSlice[2]))
					})
				})
		})
}

func (obj SetValidatorConfig) MarshalWithEncoder(encoder *ag_binary.Encoder) (err error) {
	// Serialize `FlaggingThreshold` param:
	err = encoder.Encode(obj.FlaggingThreshold)
	if err != nil {
		return err
	}
	return nil
}
func (obj *SetValidatorConfig) UnmarshalWithDecoder(decoder *ag_binary.Decoder) (err error) {
	// Deserialize `FlaggingThreshold`:
	err = decoder.Decode(&obj.FlaggingThreshold)
	if err != nil {
		return err
	}
	return nil
}

// NewSetValidatorConfigInstruction declares a new SetValidatorConfig instruction with the provided parameters and accounts.
func NewSetValidatorConfigInstruction(
	// Parameters:
	flaggingThreshold uint32,
	// Accounts:
	state ag_solanago.PublicKey,
	authority ag_solanago.PublicKey,
	validator ag_solanago.PublicKey) *SetValidatorConfig {
	return NewSetValidatorConfigInstructionBuilder().
		SetFlaggingThreshold(flaggingThreshold).
		SetStateAccount(state).
		SetAuthorityAccount(authority).
		SetValidatorAccount(validator)
}

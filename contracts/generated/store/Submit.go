// Code generated by https://github.com/gagliardetto/anchor-go. DO NOT EDIT.

package store

import (
	"errors"
	ag_binary "github.com/gagliardetto/binary"
	ag_solanago "github.com/gagliardetto/solana-go"
	ag_format "github.com/gagliardetto/solana-go/text/format"
	ag_treeout "github.com/gagliardetto/treeout"
)

// Submit is the `submit` instruction.
type Submit struct {
	Round *Transmission

	// [0] = [WRITE] store
	//
	// [1] = [SIGNER] authority
	//
	// [2] = [] accessController
	//
	// [3] = [WRITE] feed
	ag_solanago.AccountMetaSlice `bin:"-" borsh_skip:"true"`
}

// NewSubmitInstructionBuilder creates a new `Submit` instruction builder.
func NewSubmitInstructionBuilder() *Submit {
	nd := &Submit{
		AccountMetaSlice: make(ag_solanago.AccountMetaSlice, 4),
	}
	return nd
}

// SetRound sets the "round" parameter.
func (inst *Submit) SetRound(round Transmission) *Submit {
	inst.Round = &round
	return inst
}

// SetStoreAccount sets the "store" account.
func (inst *Submit) SetStoreAccount(store ag_solanago.PublicKey) *Submit {
	inst.AccountMetaSlice[0] = ag_solanago.Meta(store).WRITE()
	return inst
}

// GetStoreAccount gets the "store" account.
func (inst *Submit) GetStoreAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[0]
}

// SetAuthorityAccount sets the "authority" account.
func (inst *Submit) SetAuthorityAccount(authority ag_solanago.PublicKey) *Submit {
	inst.AccountMetaSlice[1] = ag_solanago.Meta(authority).SIGNER()
	return inst
}

// GetAuthorityAccount gets the "authority" account.
func (inst *Submit) GetAuthorityAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[1]
}

// SetAccessControllerAccount sets the "accessController" account.
func (inst *Submit) SetAccessControllerAccount(accessController ag_solanago.PublicKey) *Submit {
	inst.AccountMetaSlice[2] = ag_solanago.Meta(accessController)
	return inst
}

// GetAccessControllerAccount gets the "accessController" account.
func (inst *Submit) GetAccessControllerAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[2]
}

// SetFeedAccount sets the "feed" account.
func (inst *Submit) SetFeedAccount(feed ag_solanago.PublicKey) *Submit {
	inst.AccountMetaSlice[3] = ag_solanago.Meta(feed).WRITE()
	return inst
}

// GetFeedAccount gets the "feed" account.
func (inst *Submit) GetFeedAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[3]
}

func (inst Submit) Build() *Instruction {
	return &Instruction{BaseVariant: ag_binary.BaseVariant{
		Impl:   inst,
		TypeID: Instruction_Submit,
	}}
}

// ValidateAndBuild validates the instruction parameters and accounts;
// if there is a validation error, it returns the error.
// Otherwise, it builds and returns the instruction.
func (inst Submit) ValidateAndBuild() (*Instruction, error) {
	if err := inst.Validate(); err != nil {
		return nil, err
	}
	return inst.Build(), nil
}

func (inst *Submit) Validate() error {
	// Check whether all (required) parameters are set:
	{
		if inst.Round == nil {
			return errors.New("Round parameter is not set")
		}
	}

	// Check whether all (required) accounts are set:
	{
		if inst.AccountMetaSlice[0] == nil {
			return errors.New("accounts.Store is not set")
		}
		if inst.AccountMetaSlice[1] == nil {
			return errors.New("accounts.Authority is not set")
		}
		if inst.AccountMetaSlice[2] == nil {
			return errors.New("accounts.AccessController is not set")
		}
		if inst.AccountMetaSlice[3] == nil {
			return errors.New("accounts.Feed is not set")
		}
	}
	return nil
}

func (inst *Submit) EncodeToTree(parent ag_treeout.Branches) {
	parent.Child(ag_format.Program(ProgramName, ProgramID)).
		//
		ParentFunc(func(programBranch ag_treeout.Branches) {
			programBranch.Child(ag_format.Instruction("Submit")).
				//
				ParentFunc(func(instructionBranch ag_treeout.Branches) {

					// Parameters of the instruction:
					instructionBranch.Child("Params[len=1]").ParentFunc(func(paramsBranch ag_treeout.Branches) {
						paramsBranch.Child(ag_format.Param("Round", *inst.Round))
					})

					// Accounts of the instruction:
					instructionBranch.Child("Accounts[len=4]").ParentFunc(func(accountsBranch ag_treeout.Branches) {
						accountsBranch.Child(ag_format.Meta("           store", inst.AccountMetaSlice[0]))
						accountsBranch.Child(ag_format.Meta("       authority", inst.AccountMetaSlice[1]))
						accountsBranch.Child(ag_format.Meta("accessController", inst.AccountMetaSlice[2]))
						accountsBranch.Child(ag_format.Meta("            feed", inst.AccountMetaSlice[3]))
					})
				})
		})
}

func (obj Submit) MarshalWithEncoder(encoder *ag_binary.Encoder) (err error) {
	// Serialize `Round` param:
	err = encoder.Encode(obj.Round)
	if err != nil {
		return err
	}
	return nil
}
func (obj *Submit) UnmarshalWithDecoder(decoder *ag_binary.Decoder) (err error) {
	// Deserialize `Round`:
	err = decoder.Decode(&obj.Round)
	if err != nil {
		return err
	}
	return nil
}

// NewSubmitInstruction declares a new Submit instruction with the provided parameters and accounts.
func NewSubmitInstruction(
	// Parameters:
	round Transmission,
	// Accounts:
	store ag_solanago.PublicKey,
	authority ag_solanago.PublicKey,
	accessController ag_solanago.PublicKey,
	feed ag_solanago.PublicKey) *Submit {
	return NewSubmitInstructionBuilder().
		SetRound(round).
		SetStoreAccount(store).
		SetAuthorityAccount(authority).
		SetAccessControllerAccount(accessController).
		SetFeedAccount(feed)
}

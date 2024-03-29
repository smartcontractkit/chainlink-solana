// Code generated by https://github.com/gagliardetto/anchor-go. DO NOT EDIT.

package ocr_2

import (
	"errors"
	ag_binary "github.com/gagliardetto/binary"
	ag_solanago "github.com/gagliardetto/solana-go"
	ag_format "github.com/gagliardetto/solana-go/text/format"
	ag_treeout "github.com/gagliardetto/treeout"
)

// AcceptProposal is the `acceptProposal` instruction.
type AcceptProposal struct {
	Digest *[]byte

	// [0] = [WRITE] state
	//
	// [1] = [WRITE] proposal
	//
	// [2] = [WRITE] receiver
	//
	// [3] = [WRITE] tokenReceiver
	//
	// [4] = [SIGNER] authority
	//
	// [5] = [WRITE] tokenVault
	//
	// [6] = [] vaultAuthority
	//
	// [7] = [] tokenProgram
	ag_solanago.AccountMetaSlice `bin:"-" borsh_skip:"true"`
}

// NewAcceptProposalInstructionBuilder creates a new `AcceptProposal` instruction builder.
func NewAcceptProposalInstructionBuilder() *AcceptProposal {
	nd := &AcceptProposal{
		AccountMetaSlice: make(ag_solanago.AccountMetaSlice, 8),
	}
	return nd
}

// SetDigest sets the "digest" parameter.
func (inst *AcceptProposal) SetDigest(digest []byte) *AcceptProposal {
	inst.Digest = &digest
	return inst
}

// SetStateAccount sets the "state" account.
func (inst *AcceptProposal) SetStateAccount(state ag_solanago.PublicKey) *AcceptProposal {
	inst.AccountMetaSlice[0] = ag_solanago.Meta(state).WRITE()
	return inst
}

// GetStateAccount gets the "state" account.
func (inst *AcceptProposal) GetStateAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[0]
}

// SetProposalAccount sets the "proposal" account.
func (inst *AcceptProposal) SetProposalAccount(proposal ag_solanago.PublicKey) *AcceptProposal {
	inst.AccountMetaSlice[1] = ag_solanago.Meta(proposal).WRITE()
	return inst
}

// GetProposalAccount gets the "proposal" account.
func (inst *AcceptProposal) GetProposalAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[1]
}

// SetReceiverAccount sets the "receiver" account.
func (inst *AcceptProposal) SetReceiverAccount(receiver ag_solanago.PublicKey) *AcceptProposal {
	inst.AccountMetaSlice[2] = ag_solanago.Meta(receiver).WRITE()
	return inst
}

// GetReceiverAccount gets the "receiver" account.
func (inst *AcceptProposal) GetReceiverAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[2]
}

// SetTokenReceiverAccount sets the "tokenReceiver" account.
func (inst *AcceptProposal) SetTokenReceiverAccount(tokenReceiver ag_solanago.PublicKey) *AcceptProposal {
	inst.AccountMetaSlice[3] = ag_solanago.Meta(tokenReceiver).WRITE()
	return inst
}

// GetTokenReceiverAccount gets the "tokenReceiver" account.
func (inst *AcceptProposal) GetTokenReceiverAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[3]
}

// SetAuthorityAccount sets the "authority" account.
func (inst *AcceptProposal) SetAuthorityAccount(authority ag_solanago.PublicKey) *AcceptProposal {
	inst.AccountMetaSlice[4] = ag_solanago.Meta(authority).SIGNER()
	return inst
}

// GetAuthorityAccount gets the "authority" account.
func (inst *AcceptProposal) GetAuthorityAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[4]
}

// SetTokenVaultAccount sets the "tokenVault" account.
func (inst *AcceptProposal) SetTokenVaultAccount(tokenVault ag_solanago.PublicKey) *AcceptProposal {
	inst.AccountMetaSlice[5] = ag_solanago.Meta(tokenVault).WRITE()
	return inst
}

// GetTokenVaultAccount gets the "tokenVault" account.
func (inst *AcceptProposal) GetTokenVaultAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[5]
}

// SetVaultAuthorityAccount sets the "vaultAuthority" account.
func (inst *AcceptProposal) SetVaultAuthorityAccount(vaultAuthority ag_solanago.PublicKey) *AcceptProposal {
	inst.AccountMetaSlice[6] = ag_solanago.Meta(vaultAuthority)
	return inst
}

// GetVaultAuthorityAccount gets the "vaultAuthority" account.
func (inst *AcceptProposal) GetVaultAuthorityAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[6]
}

// SetTokenProgramAccount sets the "tokenProgram" account.
func (inst *AcceptProposal) SetTokenProgramAccount(tokenProgram ag_solanago.PublicKey) *AcceptProposal {
	inst.AccountMetaSlice[7] = ag_solanago.Meta(tokenProgram)
	return inst
}

// GetTokenProgramAccount gets the "tokenProgram" account.
func (inst *AcceptProposal) GetTokenProgramAccount() *ag_solanago.AccountMeta {
	return inst.AccountMetaSlice[7]
}

func (inst AcceptProposal) Build() *Instruction {
	return &Instruction{BaseVariant: ag_binary.BaseVariant{
		Impl:   inst,
		TypeID: Instruction_AcceptProposal,
	}}
}

// ValidateAndBuild validates the instruction parameters and accounts;
// if there is a validation error, it returns the error.
// Otherwise, it builds and returns the instruction.
func (inst AcceptProposal) ValidateAndBuild() (*Instruction, error) {
	if err := inst.Validate(); err != nil {
		return nil, err
	}
	return inst.Build(), nil
}

func (inst *AcceptProposal) Validate() error {
	// Check whether all (required) parameters are set:
	{
		if inst.Digest == nil {
			return errors.New("Digest parameter is not set")
		}
	}

	// Check whether all (required) accounts are set:
	{
		if inst.AccountMetaSlice[0] == nil {
			return errors.New("accounts.State is not set")
		}
		if inst.AccountMetaSlice[1] == nil {
			return errors.New("accounts.Proposal is not set")
		}
		if inst.AccountMetaSlice[2] == nil {
			return errors.New("accounts.Receiver is not set")
		}
		if inst.AccountMetaSlice[3] == nil {
			return errors.New("accounts.TokenReceiver is not set")
		}
		if inst.AccountMetaSlice[4] == nil {
			return errors.New("accounts.Authority is not set")
		}
		if inst.AccountMetaSlice[5] == nil {
			return errors.New("accounts.TokenVault is not set")
		}
		if inst.AccountMetaSlice[6] == nil {
			return errors.New("accounts.VaultAuthority is not set")
		}
		if inst.AccountMetaSlice[7] == nil {
			return errors.New("accounts.TokenProgram is not set")
		}
	}
	return nil
}

func (inst *AcceptProposal) EncodeToTree(parent ag_treeout.Branches) {
	parent.Child(ag_format.Program(ProgramName, ProgramID)).
		//
		ParentFunc(func(programBranch ag_treeout.Branches) {
			programBranch.Child(ag_format.Instruction("AcceptProposal")).
				//
				ParentFunc(func(instructionBranch ag_treeout.Branches) {

					// Parameters of the instruction:
					instructionBranch.Child("Params[len=1]").ParentFunc(func(paramsBranch ag_treeout.Branches) {
						paramsBranch.Child(ag_format.Param("Digest", *inst.Digest))
					})

					// Accounts of the instruction:
					instructionBranch.Child("Accounts[len=8]").ParentFunc(func(accountsBranch ag_treeout.Branches) {
						accountsBranch.Child(ag_format.Meta("         state", inst.AccountMetaSlice[0]))
						accountsBranch.Child(ag_format.Meta("      proposal", inst.AccountMetaSlice[1]))
						accountsBranch.Child(ag_format.Meta("      receiver", inst.AccountMetaSlice[2]))
						accountsBranch.Child(ag_format.Meta(" tokenReceiver", inst.AccountMetaSlice[3]))
						accountsBranch.Child(ag_format.Meta("     authority", inst.AccountMetaSlice[4]))
						accountsBranch.Child(ag_format.Meta("    tokenVault", inst.AccountMetaSlice[5]))
						accountsBranch.Child(ag_format.Meta("vaultAuthority", inst.AccountMetaSlice[6]))
						accountsBranch.Child(ag_format.Meta("  tokenProgram", inst.AccountMetaSlice[7]))
					})
				})
		})
}

func (obj AcceptProposal) MarshalWithEncoder(encoder *ag_binary.Encoder) (err error) {
	// Serialize `Digest` param:
	err = encoder.Encode(obj.Digest)
	if err != nil {
		return err
	}
	return nil
}
func (obj *AcceptProposal) UnmarshalWithDecoder(decoder *ag_binary.Decoder) (err error) {
	// Deserialize `Digest`:
	err = decoder.Decode(&obj.Digest)
	if err != nil {
		return err
	}
	return nil
}

// NewAcceptProposalInstruction declares a new AcceptProposal instruction with the provided parameters and accounts.
func NewAcceptProposalInstruction(
	// Parameters:
	digest []byte,
	// Accounts:
	state ag_solanago.PublicKey,
	proposal ag_solanago.PublicKey,
	receiver ag_solanago.PublicKey,
	tokenReceiver ag_solanago.PublicKey,
	authority ag_solanago.PublicKey,
	tokenVault ag_solanago.PublicKey,
	vaultAuthority ag_solanago.PublicKey,
	tokenProgram ag_solanago.PublicKey) *AcceptProposal {
	return NewAcceptProposalInstructionBuilder().
		SetDigest(digest).
		SetStateAccount(state).
		SetProposalAccount(proposal).
		SetReceiverAccount(receiver).
		SetTokenReceiverAccount(tokenReceiver).
		SetAuthorityAccount(authority).
		SetTokenVaultAccount(tokenVault).
		SetVaultAuthorityAccount(vaultAuthority).
		SetTokenProgramAccount(tokenProgram)
}

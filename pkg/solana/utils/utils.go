package utils

import (
	"path/filepath"
	"runtime"
	"slices"

	"github.com/gagliardetto/solana-go"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/internal"
)

var (
	_, b, _, _ = runtime.Caller(0)
	// ProjectRoot Root folder of this project
	ProjectRoot = filepath.Join(filepath.Dir(b), "/../../..")
	// ContractsDir path to our contracts
	ContractsDir = filepath.Join(ProjectRoot, "contracts", "target", "deploy")
)

func LamportsToSol(lamports uint64) float64 { return internal.LamportsToSol(lamports) }

// InjectAddressModifier injects AddressModifier into InputModifications and OutputModifications.
// This is necessary because AddressModifier cannot be serialized and must be applied at runtime.
func InjectAddressModifier(inputModifications, outputModifications commoncodec.ModifiersConfig) {
	for i, modConfig := range inputModifications {
		if addrModifierConfig, ok := modConfig.(*commoncodec.AddressBytesToStringModifierConfig); ok {
			addrModifierConfig.Modifier = solcommoncodec.SolanaAddressModifier{}
			inputModifications[i] = addrModifierConfig
		}
	}

	for i, modConfig := range outputModifications {
		if addrModifierConfig, ok := modConfig.(*commoncodec.AddressBytesToStringModifierConfig); ok {
			addrModifierConfig.Modifier = solcommoncodec.SolanaAddressModifier{}
			outputModifications[i] = addrModifierConfig
		}
	}
}

// DeepCopyTx clones tx without aliasing any slices; nil slices stay nil.
func DeepCopyTx(tx solana.Transaction) solana.Transaction {
	// Clone the signatures.
	sigs := slices.Clone(tx.Signatures)

	// Clone the message.
	msg := tx.Message

	// Deep-copy AccountKeys.
	accountKeys := slices.Clone(msg.AccountKeys)

	// Deep-copy Instructions.
	var instructions []solana.CompiledInstruction
	if msg.Instructions != nil {
		instructions = make([]solana.CompiledInstruction, len(msg.Instructions))
		for i, instr := range msg.Instructions {
			instructions[i] = solana.CompiledInstruction{
				ProgramIDIndex: instr.ProgramIDIndex,
				Accounts:       slices.Clone(instr.Accounts),
				Data:           slices.Clone(instr.Data),
			}
		}
	}

	// Deep-copy AddressTableLookups.
	var lookups []solana.MessageAddressTableLookup
	if msg.AddressTableLookups != nil {
		lookups = make([]solana.MessageAddressTableLookup, len(msg.AddressTableLookups))
		for i, lookup := range msg.AddressTableLookups {
			lookups[i] = solana.MessageAddressTableLookup{
				AccountKey:      lookup.AccountKey,
				WritableIndexes: slices.Clone(lookup.WritableIndexes),
				ReadonlyIndexes: slices.Clone(lookup.ReadonlyIndexes),
			}
		}
	}

	// Reassemble the cloned message.
	msg.AccountKeys = accountKeys
	msg.Instructions = instructions
	msg.AddressTableLookups = lookups

	return solana.Transaction{
		Signatures: sigs,
		Message:    msg,
	}
}

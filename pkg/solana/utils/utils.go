package utils

import (
	"path/filepath"
	"runtime"

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

func DeepCopyTx(tx solana.Transaction) solana.Transaction {
	// Clone the signatures.
	sigs := make([]solana.Signature, len(tx.Signatures))
	copy(sigs, tx.Signatures)

	// Clone the message.
	msg := tx.Message

	// Deep-copy AccountKeys.
	accountKeys := make([]solana.PublicKey, len(msg.AccountKeys))
	copy(accountKeys, msg.AccountKeys)

	// Deep-copy Instructions.
	instructions := make([]solana.CompiledInstruction, len(msg.Instructions))
	for i, instr := range msg.Instructions {
		newInstr := solana.CompiledInstruction{
			ProgramIDIndex: instr.ProgramIDIndex,
			Accounts:       make([]uint16, len(instr.Accounts)),
			Data:           make([]byte, len(instr.Data)),
		}
		copy(newInstr.Accounts, instr.Accounts)
		copy(newInstr.Data, instr.Data)
		instructions[i] = newInstr
	}

	// Deep-copy AddressTableLookups.
	lookups := make([]solana.MessageAddressTableLookup, len(msg.AddressTableLookups))
	for i, lookup := range msg.AddressTableLookups {
		newLookup := solana.MessageAddressTableLookup{
			AccountKey:      lookup.AccountKey,
			WritableIndexes: make(solana.Uint8SliceAsNum, len(lookup.WritableIndexes)),
			ReadonlyIndexes: make(solana.Uint8SliceAsNum, len(lookup.ReadonlyIndexes)),
		}
		copy(newLookup.WritableIndexes, lookup.WritableIndexes)
		copy(newLookup.ReadonlyIndexes, lookup.ReadonlyIndexes)
		lookups[i] = newLookup
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

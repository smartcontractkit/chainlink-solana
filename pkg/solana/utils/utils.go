package utils

import (
	"path/filepath"
	"runtime"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
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
			addrModifierConfig.Modifier = codec.SolanaAddressModifier{}
			inputModifications[i] = addrModifierConfig
		}
	}

	for i, modConfig := range outputModifications {
		if addrModifierConfig, ok := modConfig.(*commoncodec.AddressBytesToStringModifierConfig); ok {
			addrModifierConfig.Modifier = codec.SolanaAddressModifier{}
			outputModifications[i] = addrModifierConfig
		}
	}
}

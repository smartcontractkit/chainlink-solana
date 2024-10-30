package chainwriter_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
)

type ExecutionReportSingleChain struct {
	SourceChainSelector uint64                `json:"source_chain_selector"`
	Message             Any2SolanaRampMessage `json:"message"`
	Root                [32]byte              `json:"root"`
	Proofs              [][]byte              `json:"proofs"`
}

type Any2SolanaRampMessage struct {
	Header    RampMessageHeader `json:"header"`
	Sender    []byte            `json:"sender"`
	Data      []byte            `json:"data"`
	Receiver  [32]byte          `json:"receiver"`
	ExtraArgs SolanaExtraArgs   `json:"extra_args"`
}

type RampMessageHeader struct {
	MessageID           [32]byte `json:"message_id"`
	SourceChainSelector uint64   `json:"source_chain_selector"`
	DestChainSelector   uint64   `json:"dest_chain_selector"`
	SequenceNumber      uint64   `json:"sequence_number"`
	Nonce               uint64   `json:"nonce"`
}

type SolanaExtraArgs struct {
	ComputeUnits             uint32 `json:"compute_units"`
	AllowOutOfOrderExecution bool   `json:"allow_out_of_order_execution"`
}

type RegistryTokenState struct {
	PoolProgram                [32]byte `json:"pool_program"`
	PoolConfig                 [32]byte `json:"pool_config"`
	TokenProgram               [32]byte `json:"token_program"`
	TokenState                 [32]byte `json:"token_state"`
	PoolAssociatedTokenAccount [32]byte `json:"pool_associated_token_account"`
}

inputParams := []{
	generatedReports,
	rs,
	vs,
	..
}

inputParams := []{
	merketRoots,
	generatedReports,
	rs,
	vs,
	..
}

CW.SubmitTransaction(address, "router", "executeReport", inputParams, ...)

SubmitReport([]report, ...)

func TestGetAddresses(t *testing.T) {
	registryAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6A"
	routerProgramAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6B"
	routerAccountConfigAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6C"
	cpiSignerAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6D"
	systemProgramAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6E"
	computeBudgetProgramAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6F"
	sysvarProgramAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6G"
	commonAddressesLookupTable := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6H"
	routerLookupTable := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6I"

	executionReportSingleChainIDL := `{"name":"ExecutionReportSingleChain","type":{"kind":"struct","fields":[{"name":"source_chain_selector","type":"u64"},{"name":"message","type":{"defined":"Any2SolanaRampMessage"}},{"name":"root","type":{"array":["u8",32]}},{"name":"proofs","type":{"vec":{"array":["u8",32]}}}]}},{"name":"Any2SolanaRampMessage","type":{"kind":"struct","fields":[{"name":"header","type":{"defined":"RampMessageHeader"}},{"name":"sender","type":{"vec":"u8"}},{"name":"data","type":{"vec":"u8"}},{"name":"receiver","type":{"array":["u8",32]}},{"name":"extra_args","type":{"defined":"SolanaExtraArgs"}}]}},{"name":"RampMessageHeader","type":{"kind":"struct","fields":[{"name":"message_id","type":{"array":["u8",32]}},{"name":"source_chain_selector","type":"u64"},{"name":"dest_chain_selector","type":"u64"},{"name":"sequence_number","type":"u64"},{"name":"nonce","type":"u64"}]}},{"name":"SolanaExtraArgs","type":{"kind":"struct","fields":[{"name":"compute_units","type":"u32"},{"name":"allow_out_of_order_execution","type":"bool"}]}}`
	registryTokenStateIDL := `{"name":"RegistryTokenState","type":"struct","fields":[{"name":"pool_program","type":{"array":[{"type":"u8"},32]}},{"name":"pool_config","type":{"array":[{"type":"u8"},32]}},{"name":"token_program","type":{"array":[{"type":"u8"},32]}},{"name":"token_state","type":{"array":[{"type":"u8"},32]}},{"name":"pool_associated_token_account","type":{"array":[{"type":"u8"},32]}}]}`

	executeConfig := chainwriter.MethodConfig{
		InputModifications: nil,
		EncodedTypeIDL:     executionReportSingleChainIDL,
		DataType:           reflect.TypeOf(ExecutionReportSingleChain{}),
		DecodedTypeName:    "ExecutionReportSingleChain",
		ChainSpecificName:  "execute",
		LookupTables: []chainwriter.LookupTable{
			{
				Name: "RegistryTokenState",
				Accounts: chainwriter.PDALookups{
					Name: "RegistryTokenState",
					PublicKey: chainwriter.AccountConstant{
						Address:    registryAddress,
						IsSigner:   false,
						IsWritable: false,
					},
					AddressSeeds: nil,
					ValueSeeds: []chainwriter.ValueLookup{
						{Location: "Message.TokenAmounts.DestTokenAddress"},
					},
					IsSigner:   false,
					IsWritable: false,
				} --> ["a", "b", "c"] each being a PDA account for a token which are address lookup table accounts,
			},
		},
		Accounts: []chainwriter.Lookup{
			// Account constant
			// Account Lookup - Based on data from input parameters
			// Lookup Table content - Get all the accounts from a lookup table
			// PDA Account Lookup - Based on another account and a seed/s
			// Nested PDA Account with seeds from:
				// input paramters
				// constant
			chainwriter.PDALookup{
				Name: "PerChainRateLimit",
				PublicKey: chainwriter.AccountConstant{
					Address:    registryAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				AddressSeeds: nil,
				ValueSeeds: []chainwriter.ValueLookup{
					{Location: "Message.TokenAmounts.DestTokenAddress"},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			{
				Name: "RegistryTokenState",
					Accounts: chainwriter.PDALookups{
						Name: "RegistryTokenState",
						PublicKey: chainwriter.AccountConstant{
							Address:    registryAddress,
							IsSigner:   false,
							IsWritable: false,
						},
						AddressSeeds: nil,
						ValueSeeds: []chainwriter.ValueLookup{
							{Location: "Message.TokenAmounts.DestTokenAddress"},
						},
						IsSigner:   false,
						IsWritable: false,
					},
			} 
			// Lookup Table content - Get all the accounts from a lookup table
			chainWriter.AccountsFromLookupTable: { // Just include all the accounts within the RegistryTokenState lookup table.
				LookupTablesName: "RegistryTokenState",
				IncludeIndexes: [1,4] // WE DON"T NEED THIS RIGHT NOW
			},
			// Account Lookup - Based on data from input parameters
			chainwriter.AccountLookup{
				Name:       "TokenAccount",
				Location:   "Message.TokenAmounts.DestTokenAddress",
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA Account Lookup -
			chainwriter.PDALookups{
				Name: "ReceiverAssociatedTokenAccount",
				PublicKey: chainwriter.AccountLookup{
					Name:       "TokenAccount",
					Location:   "Message.TokenAmounts.DestTokenAddress",
					IsSigner:   false,
					IsWritable: false,
				},
				AddressSeeds: []chainwriter.Lookup{
					chainwriter.AccountLookup{
						Name:       "Receiver",
						Location:   "Message.Receiver",
						IsSigner:   false,
						IsWritable: false,
					},
				},
			},
			chainwriter.AccountConstant{
				Name:       "Registry",
				Address:    registryAddress,
				IsSigner:   false,
				IsWritable: false,
			},
			chainwriter.PDALookup{
				Name: "RegistryTokenConfig",
				PublicKey: chainwriter.AccountConstant{
					Address:    registryAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				AddressSeeds: nil,
				ValueSeeds: []chainwriter.ValueLookup{
					{Location: "Message.TokenAmounts.DestTokenAddress"},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// Account constant
			chainwriter.AccountConstant{
				Name:       "RouterProgram",
				Address:    routerProgramAddress,
				IsSigner:   false,
				IsWritable: false,
			},
			chainwriter.AccountConstant{
				Name:       "RouterAccountConfig",
				Address:    routerAccountConfigAddress,
				IsSigner:   false,
				IsWritable: false,
			},
			chainwriter.PDALookup{
				Name: "RouterReportAccount",
				PublicKey: chainwriter.AccountConstant{
					Address:    routerProgramAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				AddressSeeds: nil,
				ValueSeeds: []chainwriter.ValueLookup{
					// TBD - need to clarify how merkle roots are handled
					{Location: "Message.ExtraArgs.MerkleRoot"},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			chainwriter.PDALookup{
				Name: "UserNoncePerChain",
				PublicKey: chainwriter.AccountConstant{
					Address:    routerProgramAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				AddressSeeds: nil,
				ValueSeeds: []chainwriter.ValueLookup{
					{Location: "Message.Receiver"},
					{Location: "Message.DestChainSelector"},
				},
			},
			chainwriter.AccountConstant{
				Name:       "CPISigner",
				Address:    cpiSignerAddress,
				IsSigner:   true,
				IsWritable: false,
			},
			chainwriter.AccountConstant{
				Name:       "SystemProgram",
				Address:    systemProgramAddress,
				IsSigner:   true,
				IsWritable: false,
			},
			chainwriter.AccountConstant{
				Name:       "ComputeBudgetProgram",
				Address:    computeBudgetProgramAddress,
				IsSigner:   true,
				IsWritable: false,
			},
			chainwriter.AccountConstant{
				Name:       "SysvarProgram",
				Address:    sysvarProgramAddress,
				IsSigner:   true,
				IsWritable: false,
			},
		},
		LookupTables: []string{
			commonAddressesLookupTable,
			routerLookupTable,
		},
		// TBD where this will be in the report
		DebugIDLocation: "Message.ExtraArgs.DebugID",
	}

	chainWriterConfig := chainwriter.ChainWriterConfig{
		Programs: map[string]chainwriter.ProgramConfig{
			"ccip-router": {
				Methods: map[string]chainwriter.MethodConfig{
					"execute": executeConfig,
				},
			},
		},
	}
	fmt.Println(chainWriterConfig)
}

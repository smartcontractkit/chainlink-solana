package chainwriter_test

import (
	"fmt"
	"reflect"
	"testing"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
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

func TestGetAddresses(t *testing.T) {
	// Fake constant addresses for the purpose of this example.
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
	// registryTokenStateIDL := `{"name":"RegistryTokenState","type":"struct","fields":[{"name":"pool_program","type":{"array":[{"type":"u8"},32]}},{"name":"pool_config","type":{"array":[{"type":"u8"},32]}},{"name":"token_program","type":{"array":[{"type":"u8"},32]}},{"name":"token_state","type":{"array":[{"type":"u8"},32]}},{"name":"pool_associated_token_account","type":{"array":[{"type":"u8"},32]}}]}`

	executeConfig := chainwriter.MethodConfig{
		InputModifications: commoncodec.ModifiersConfig{
			// remove merkle root since it isn't a part of the on-chain type
			&commoncodec.DropModifierConfig{
				Fields: []string{"Message.ExtraArgs.MerkleRoot"},
			},
		},
		EncodedTypeIDL:    executionReportSingleChainIDL,
		DataType:          reflect.TypeOf(ExecutionReportSingleChain{}),
		DecodedTypeName:   "ExecutionReportSingleChain",
		ChainSpecificName: "execute",
		// LookupTables are on-chain stores of accounts. They can be used in two ways:
		// 1. As a way to store a list of accounts that are all associated together (i.e. Token State registry)
		// 2. To compress the transactions in a TX and reduce the size of the TX. (The traditional way)
		LookupTables: chainwriter.LookupTables{
			// DerivedLookupTables are useful in both the ways described above.
			// 	a. The user can configure any type of look up to get a list of lookupTables to read from.
			// 	b. The ChainWriter reads from this lookup table and store the internal addresses in memory
			//	c. Later, in the []Accounts the user can specify which accounts to include in the TX with an AccountsFromLookupTable lookup.
			// 	d. Lastly, the lookup table is used to compress the size of the transaction.
			DerivedLookupTables: []chainwriter.DerivedLookupTable{
				{
					Name: "RegistryTokenState",
					// In this case, the user configured the lookup table accounts to use a PDALookup, which
					// generates a list of one of more PDA accounts based on the input parameters. Specifically,
					// there will be multple PDA accounts if there are multiple addresses in the message, otherwise,
					// there will only be one PDA account to read from. The PDA account corresponds to the lookup table.
					Accounts: chainwriter.PDALookups{
						Name: "RegistryTokenState",
						PublicKey: chainwriter.AccountConstant{
							Address:    registryAddress,
							IsSigner:   false,
							IsWritable: false,
						},
						// Seeds would be used if the user needed to look up addresses to use as seeds, which isn't the case here.
						Seeds: []chainwriter.Lookup{
							chainwriter.ValueLookup{Location: "Message.TokenAmounts.DestTokenAddress"},
						},
						IsSigner:   false,
						IsWritable: false,
					},
				},
			},
			// Static lookup tables are the traditional use case (point 2 above) of Lookup tables. These are lookup
			// tables which contain commonly used addresses in all CCIP execute transactions. The ChainWriter reads
			// these lookup tables and appends them to the transaction to reduce the size of the transaction.
			StaticLookupTables: []string{
				commonAddressesLookupTable,
				routerLookupTable,
			},
		},
		// The Accounts field is where the user specifies which accounts to include in the transaction. Each Lookup
		// resolves to one or more on-chain addresses.
		Accounts: []chainwriter.Lookup{
			// The accounts can be of any of the following types:
			// 1. Account constant
			// 2. Account Lookup - Based on data from input parameters
			// 3. Lookup Table content - Get all the accounts from a lookup table
			// 4. PDA Account Lookup - Based on another account and a seed/s
			//	Nested PDA Account with seeds from:
			// 		-> input paramters
			// 		-> constant
			// 	PDALookups can resolve to multiple addresses if:
			// 		A) The PublicKey lookup resolves to multiple addresses (i.e. multiple token addresses)
			// 		B) The Seeds or ValueSeeds resolve to multiple values
			chainwriter.PDALookups{
				Name: "PerChainRateLimit",
				// PublicKey is a constant account in this case, not a lookup.
				PublicKey: chainwriter.AccountConstant{
					Address:    registryAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				// Similar to the RegistryTokenState above, the user is looking up PDA accounts based on the dest tokens.
				Seeds: []chainwriter.Lookup{
					chainwriter.ValueLookup{Location: "Message.TokenAmounts.DestTokenAddress"},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// Lookup Table content - Get the accounts from the derived lookup table above
			chainwriter.AccountsFromLookupTable{
				LookupTablesName: "RegistryTokenState",
				IncludeIndexes:   []int{1, 4}, // If left empty, all addresses will be included.
			},
			// Account Lookup - Based on data from input parameters
			// In this case, the user wants to add the destination token addresses to the transaction.
			// Once again, this can be one or multiple addresses.
			chainwriter.AccountLookup{
				Name:       "TokenAccount",
				Location:   "Message.TokenAmounts.DestTokenAddress",
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA Account Lookup - Based on an account lookup and an address lookup
			chainwriter.PDALookups{
				// In this case, the token address is the public key, and the receiver is the seed.
				// Again, there could be multiple token addresses, in which case this would resolve to
				// multiple PDA accounts.
				Name: "ReceiverAssociatedTokenAccount",
				PublicKey: chainwriter.AccountLookup{
					Name:       "TokenAccount",
					Location:   "Message.TokenAmounts.DestTokenAddress",
					IsSigner:   false,
					IsWritable: false,
				},
				// The seed is the receiver address.
				Seeds: []chainwriter.Lookup{
					chainwriter.AccountLookup{
						Name:       "Receiver",
						Location:   "Message.Receiver",
						IsSigner:   false,
						IsWritable: false,
					},
				},
			},
			// Account constant
			chainwriter.AccountConstant{
				Name:       "Registry",
				Address:    registryAddress,
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA Lookup for the RegistryTokenConfig.
			chainwriter.PDALookups{
				Name: "RegistryTokenConfig",
				// constant public key
				PublicKey: chainwriter.AccountConstant{
					Address:    registryAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				// The seed, once again, is the destination token address.
				Seeds: []chainwriter.Lookup{
					chainwriter.ValueLookup{Location: "Message.TokenAmounts.DestTokenAddress"},
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
			// Account constant
			chainwriter.AccountConstant{
				Name:       "RouterAccountConfig",
				Address:    routerAccountConfigAddress,
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA lookup to get the Router Report Accounts.
			chainwriter.PDALookups{
				Name: "RouterReportAccount",
				// The public key is a constant Router address.
				PublicKey: chainwriter.AccountConstant{
					Address:    routerProgramAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				Seeds: []chainwriter.Lookup{
					chainwriter.ValueLookup{
						// The seed is the merkle root of the report, as passed into the input params.
						Location: "args.MerkleRoot",
					},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA lookup to get UserNoncePerChain
			chainwriter.PDALookups{
				Name: "UserNoncePerChain",
				// The public key is a constant Router address.
				PublicKey: chainwriter.AccountConstant{
					Address:    routerProgramAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				// In this case, the user configured multiple seeds. These will be used in conjunction
				// with the public key to generate one or multiple PDA accounts.
				Seeds: []chainwriter.Lookup{
					chainwriter.ValueLookup{Location: "Message.Receiver"},
					chainwriter.ValueLookup{Location: "Message.DestChainSelector"},
				},
			},
			// Account constant
			chainwriter.AccountConstant{
				Name:       "CPISigner",
				Address:    cpiSignerAddress,
				IsSigner:   true,
				IsWritable: false,
			},
			// Account constant
			chainwriter.AccountConstant{
				Name:       "SystemProgram",
				Address:    systemProgramAddress,
				IsSigner:   true,
				IsWritable: false,
			},
			// Account constant
			chainwriter.AccountConstant{
				Name:       "ComputeBudgetProgram",
				Address:    computeBudgetProgramAddress,
				IsSigner:   true,
				IsWritable: false,
			},
			// Account constant
			chainwriter.AccountConstant{
				Name:       "SysvarProgram",
				Address:    sysvarProgramAddress,
				IsSigner:   true,
				IsWritable: false,
			},
		},
		// TBD where this will be in the report
		// This will be appended to every error message (after args are decoded).
		DebugIDLocation: "Message.MessageID",
	}

	chainWriterConfig := chainwriter.ChainWriterConfig{
		Programs: map[string]chainwriter.ProgramConfig{
			"ccip-router": {
				Methods: map[string]chainwriter.MethodConfig{
					"execute": executeConfig,
				},
				IDL: "ccip-router",
			},
		},
	}
	fmt.Println(chainWriterConfig)
}

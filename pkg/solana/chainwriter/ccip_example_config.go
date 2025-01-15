package chainwriter

import (
	"github.com/gagliardetto/solana-go"
)

func TestConfig() {
	// Fake constant addresses for the purpose of this example.
	registryAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6A"
	routerProgramAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6B"
	routerAccountConfigAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6C"
	cpiSignerAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6D"
	systemProgramAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6E"
	computeBudgetProgramAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6F"
	sysvarProgramAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6G"
	commonAddressesLookupTable := solana.MustPublicKeyFromBase58("4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6H")
	routerLookupTable := solana.MustPublicKeyFromBase58("4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6I")
	userAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6J"

	executionReportSingleChainIDL := `{"name":"ExecutionReportSingleChain","type":{"kind":"struct","fields":[{"name":"source_chain_selector","type":"u64"},{"name":"message","type":{"defined":"Any2SolanaRampMessage"}},{"name":"root","type":{"array":["u8",32]}},{"name":"proofs","type":{"vec":{"array":["u8",32]}}}]}},{"name":"Any2SolanaRampMessage","type":{"kind":"struct","fields":[{"name":"header","type":{"defined":"RampMessageHeader"}},{"name":"sender","type":{"vec":"u8"}},{"name":"data","type":{"vec":"u8"}},{"name":"receiver","type":{"array":["u8",32]}},{"name":"extra_args","type":{"defined":"SolanaExtraArgs"}}]}},{"name":"RampMessageHeader","type":{"kind":"struct","fields":[{"name":"message_id","type":{"array":["u8",32]}},{"name":"source_chain_selector","type":"u64"},{"name":"dest_chain_selector","type":"u64"},{"name":"sequence_number","type":"u64"},{"name":"nonce","type":"u64"}]}},{"name":"SolanaExtraArgs","type":{"kind":"struct","fields":[{"name":"compute_units","type":"u32"},{"name":"allow_out_of_order_execution","type":"bool"}]}}`

	executeConfig := MethodConfig{
		FromAddress:        userAddress,
		InputModifications: nil,
		ChainSpecificName:  "execute",
		// LookupTables are on-chain stores of accounts. They can be used in two ways:
		// 1. As a way to store a list of accounts that are all associated together (i.e. Token State registry)
		// 2. To compress the transactions in a TX and reduce the size of the TX. (The traditional way)
		LookupTables: LookupTables{
			// DerivedLookupTables are useful in both the ways described above.
			// 	a. The user can configure any type of look up to get a list of lookupTables to read from.
			// 	b. The ChainWriter reads from this lookup table and store the internal addresses in memory
			//	c. Later, in the []Accounts the user can specify which accounts to include in the TX with an AccountsFromLookupTable lookup.
			// 	d. Lastly, the lookup table is used to compress the size of the transaction.
			DerivedLookupTables: []DerivedLookupTable{
				{
					Name: "RegistryTokenState",
					// In this case, the user configured the lookup table accounts to use a PDALookup, which
					// generates a list of one of more PDA accounts based on the input parameters. Specifically,
					// there will be multiple PDA accounts if there are multiple addresses in the message, otherwise,
					// there will only be one PDA account to read from. The PDA account corresponds to the lookup table.
					Accounts: PDALookups{
						Name: "RegistryTokenState",
						PublicKey: AccountConstant{
							Address:    registryAddress,
							IsSigner:   false,
							IsWritable: false,
						},
						// Seeds would be used if the user needed to look up addresses to use as seeds, which isn't the case here.
						Seeds: []Seed{
							{Dynamic: AccountLookup{Location: "Message.TokenAmounts.DestTokenAddress"}},
						},
						IsSigner:   false,
						IsWritable: false,
					},
				},
			},
			// Static lookup tables are the traditional use case (point 2 above) of Lookup tables. These are lookup
			// tables which contain commonly used addresses in all CCIP execute transactions. The ChainWriter reads
			// these lookup tables and appends them to the transaction to reduce the size of the transaction.
			StaticLookupTables: []solana.PublicKey{
				commonAddressesLookupTable,
				routerLookupTable,
			},
		},
		// The Accounts field is where the user specifies which accounts to include in the transaction. Each Lookup
		// resolves to one or more on-chain addresses.
		Accounts: []Lookup{
			// The accounts can be of any of the following types:
			// 1. Account constant
			// 2. Account Lookup - Based on data from input parameters
			// 3. Lookup Table content - Get all the accounts from a lookup table
			// 4. PDA Account Lookup - Based on another account and a seed/s
			//	Nested PDA Account with seeds from:
			// 		-> input parameters
			// 		-> constant
			// 	PDALookups can resolve to multiple addresses if:
			// 		A) The PublicKey lookup resolves to multiple addresses (i.e. multiple token addresses)
			// 		B) The Seeds or ValueSeeds resolve to multiple values
			PDALookups{
				Name: "PerChainConfig",
				// PublicKey is a constant account in this case, not a lookup.
				PublicKey: AccountConstant{
					Address:    registryAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				// Similar to the RegistryTokenState above, the user is looking up PDA accounts based on the dest tokens.
				Seeds: []Seed{
					{Dynamic: AccountLookup{Location: "Message.TokenAmounts.DestTokenAddress"}},
					{Dynamic: AccountLookup{Location: "Message.Header.DestChainSelector"}},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// Lookup Table content - Get the accounts from the derived lookup table above
			AccountsFromLookupTable{
				LookupTableName: "RegistryTokenState",
				IncludeIndexes:  []int{}, // If left empty, all addresses will be included. Otherwise, only the specified indexes will be included.
			},
			// Account Lookup - Based on data from input parameters
			// In this case, the user wants to add the destination token addresses to the transaction.
			// Once again, this can be one or multiple addresses.
			AccountLookup{
				Name:       "TokenAccount",
				Location:   "Message.TokenAmounts.DestTokenAddress",
				IsSigner:   MetaBool{Value: false},
				IsWritable: MetaBool{Value: false},
			},
			// PDA Account Lookup - Based on an account lookup and an address lookup
			PDALookups{
				// In this case, the token address is the public key, and the receiver is the seed.
				// Again, there could be multiple token addresses, in which case this would resolve to
				// multiple PDA accounts.
				Name: "ReceiverAssociatedTokenAccount",
				PublicKey: AccountLookup{
					Name:       "TokenAccount",
					Location:   "Message.TokenAmounts.DestTokenAddress",
					IsSigner:   MetaBool{Value: false},
					IsWritable: MetaBool{Value: false},
				},
				// The seed is the receiver address.
				Seeds: []Seed{
					{Dynamic: AccountLookup{
						Name:       "Receiver",
						Location:   "Message.Receiver",
						IsSigner:   MetaBool{Value: false},
						IsWritable: MetaBool{Value: false},
					}},
				},
			},
			// Account constant
			AccountConstant{
				Name:       "Registry",
				Address:    registryAddress,
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA Lookup for the RegistryTokenConfig.
			PDALookups{
				Name: "RegistryTokenConfig",
				// constant public key
				PublicKey: AccountConstant{
					Address:    registryAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				// The seed, once again, is the destination token address.
				Seeds: []Seed{
					{Dynamic: AccountLookup{Location: "Message.TokenAmounts.DestTokenAddress"}},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// Account constant
			AccountConstant{
				Name:       "RouterProgram",
				Address:    routerProgramAddress,
				IsSigner:   false,
				IsWritable: false,
			},
			// Account constant
			AccountConstant{
				Name:       "RouterAccountConfig",
				Address:    routerAccountConfigAddress,
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA lookup to get the Router Chain Config
			PDALookups{
				Name: "RouterChainConfig",
				// The public key is a constant Router address.
				PublicKey: AccountConstant{
					Address:    routerProgramAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				Seeds: []Seed{
					{Dynamic: AccountLookup{Location: "Message.Header.DestChainSelector"}},
					{Dynamic: AccountLookup{Location: "Message.Header.SourceChainSelector"}},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA lookup to get the Router Report Accounts.
			PDALookups{
				Name: "RouterReportAccount",
				// The public key is a constant Router address.
				PublicKey: AccountConstant{
					Address:    routerProgramAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				Seeds: []Seed{
					{Dynamic: AccountLookup{
						// The seed is the merkle root of the report, as passed into the input params.
						Location: "args.MerkleRoot",
					}},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA lookup to get UserNoncePerChain
			PDALookups{
				Name: "UserNoncePerChain",
				// The public key is a constant Router address.
				PublicKey: AccountConstant{
					Address:    routerProgramAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				// In this case, the user configured multiple seeds. These will be used in conjunction
				// with the public key to generate one or multiple PDA accounts.
				Seeds: []Seed{
					{Dynamic: AccountLookup{Location: "Message.Receiver"}},
					{Dynamic: AccountLookup{Location: "Message.Header.DestChainSelector"}},
				},
			},
			// Account constant
			AccountConstant{
				Name:       "CPISigner",
				Address:    cpiSignerAddress,
				IsSigner:   true,
				IsWritable: false,
			},
			// Account constant
			AccountConstant{
				Name:       "SystemProgram",
				Address:    systemProgramAddress,
				IsSigner:   true,
				IsWritable: false,
			},
			// Account constant
			AccountConstant{
				Name:       "ComputeBudgetProgram",
				Address:    computeBudgetProgramAddress,
				IsSigner:   true,
				IsWritable: false,
			},
			// Account constant
			AccountConstant{
				Name:       "SysvarProgram",
				Address:    sysvarProgramAddress,
				IsSigner:   true,
				IsWritable: false,
			},
		},
		// TBD where this will be in the report
		// This will be appended to every error message
		DebugIDLocation: "Message.MessageID",
	}

	commitConfig := MethodConfig{
		FromAddress:        userAddress,
		InputModifications: nil,
		ChainSpecificName:  "commit",
		LookupTables: LookupTables{
			StaticLookupTables: []solana.PublicKey{
				commonAddressesLookupTable,
				routerLookupTable,
			},
		},
		Accounts: []Lookup{
			// Account constant
			AccountConstant{
				Name:       "RouterProgram",
				Address:    routerProgramAddress,
				IsSigner:   false,
				IsWritable: false,
			},
			// Account constant
			AccountConstant{
				Name:       "RouterAccountConfig",
				Address:    routerAccountConfigAddress,
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA lookup to get the Router Report Accounts.
			PDALookups{
				Name: "RouterReportAccount",
				// The public key is a constant Router address.
				PublicKey: AccountConstant{
					Address:    routerProgramAddress,
					IsSigner:   false,
					IsWritable: false,
				},
				Seeds: []Seed{
					{Dynamic: AccountLookup{
						// The seed is the merkle root of the report, as passed into the input params.
						Location: "args.MerkleRoots",
					}},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// Account constant
			AccountConstant{
				Name:       "SystemProgram",
				Address:    systemProgramAddress,
				IsSigner:   true,
				IsWritable: false,
			},
			// Account constant
			AccountConstant{
				Name:       "ComputeBudgetProgram",
				Address:    computeBudgetProgramAddress,
				IsSigner:   true,
				IsWritable: false,
			},
			// Account constant
			AccountConstant{
				Name:       "SysvarProgram",
				Address:    sysvarProgramAddress,
				IsSigner:   true,
				IsWritable: false,
			},
		},
		DebugIDLocation: "",
	}

	chainWriterConfig := ChainWriterConfig{
		Programs: map[string]ProgramConfig{
			"ccip-router": {
				Methods: map[string]MethodConfig{
					"execute": executeConfig,
					"commit":  commitConfig,
				},
				IDL: executionReportSingleChainIDL,
			},
		},
	}
	_ = chainWriterConfig
}

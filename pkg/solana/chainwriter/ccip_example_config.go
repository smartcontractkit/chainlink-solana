package chainwriter

import (
	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-common/pkg/codec"
)

func TestConfig() {
	// Fake constant addresses for the purpose of this example.
	routerProgramAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6B"
	commonAddressesLookupTable := solana.MustPublicKeyFromBase58("4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6H")

	sysvarInstructionsAddress := solana.SysVarInstructionsPubkey.String()

	fromAddress := "4Nn9dsYBcSTzRbK9hg9kzCUdrCSkMZq1UR6Vw1Tkaf6J"

	// NOTE: This is not the real IDL, since the real one is some 3000+ lines long. In the plugin, the IDL will be imported.
	executionReportSingleChainIDL := `{"name":"ExecutionReportSingleChain","type":{"kind":"struct","fields":[{"name":"source_chain_selector","type":"u64"},{"name":"message","type":{"defined":"Any2SolanaRampMessage"}},{"name":"root","type":{"array":["u8",32]}},{"name":"proofs","type":{"vec":{"array":["u8",32]}}}]}},{"name":"Any2SolanaRampMessage","type":{"kind":"struct","fields":[{"name":"header","type":{"defined":"RampMessageHeader"}},{"name":"sender","type":{"vec":"u8"}},{"name":"data","type":{"vec":"u8"}},{"name":"receiver","type":{"array":["u8",32]}},{"name":"extra_args","type":{"defined":"SolanaExtraArgs"}}]}},{"name":"RampMessageHeader","type":{"kind":"struct","fields":[{"name":"message_id","type":{"array":["u8",32]}},{"name":"source_chain_selector","type":"u64"},{"name":"dest_chain_selector","type":"u64"},{"name":"sequence_number","type":"u64"},{"name":"nonce","type":"u64"}]}},{"name":"SolanaExtraArgs","type":{"kind":"struct","fields":[{"name":"compute_units","type":"u32"},{"name":"allow_out_of_order_execution","type":"bool"}]}}`

	executeConfig := MethodConfig{
		FromAddress: fromAddress,
		InputModifications: []codec.ModifierConfig{
			&codec.RenameModifierConfig{
				Fields: map[string]string{"ReportContextByteWords": "ReportContext"},
			},
			&codec.RenameModifierConfig{
				Fields: map[string]string{"RawExecutionReport": "Report"},
			},
		},
		ChainSpecificName: "execute",
		ArgsTransform:     "CCIP",
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
					Name: "PoolLookupTable",
					// In this case, the user configured the lookup table accounts to use a PDALookup, which
					// generates a list of one of more PDA accounts based on the input parameters. Specifically,
					// there will be multiple PDA accounts if there are multiple addresses in the message, otherwise,
					// there will only be one PDA account to read from. The internal field LookupTable
					// of the PDA account corresponds to the pool lookup table(s).
					Accounts: PDALookups{
						Name: "TokenAdminRegistry",
						PublicKey: AccountConstant{
							Address: routerProgramAddress,
						},
						// Seeds would be used if the user needed to look up addresses to use as seeds, which isn't the case here.
						Seeds: []Seed{
							{Static: []byte("token_admin_registry")},
							{Dynamic: AccountLookup{Location: "Info.AbstractReports.Messages.TokenAmounts.DestTokenAddress"}},
						},
						IsSigner:   false,
						IsWritable: false,
						InternalField: InternalField{
							TypeName: "TokenAdminRegistry",
							Location: "LookupTable",
						},
					},
				},
			},
			// Static lookup tables are the traditional use case (point 2 above) of Lookup tables. These are lookup
			// tables which contain commonly used addresses in all CCIP execute transactions. The ChainWriter reads
			// these lookup tables and appends them to the transaction to reduce the size of the transaction.
			StaticLookupTables: []solana.PublicKey{
				commonAddressesLookupTable,
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
			// PDA lookup with constant seed
			PDALookups{
				Name: "RouterAccountConfig",
				PublicKey: AccountConstant{
					Address: routerProgramAddress,
				},
				Seeds: []Seed{
					{Static: []byte("config")},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			PDALookups{
				Name: "SourceChainState",
				// PublicKey is a constant account in this case, not a lookup.
				PublicKey: AccountConstant{
					Address: routerProgramAddress,
				},
				// Similar to the TokenAdminRegistry above, the user is looking up PDA accounts based on the dest tokens.
				Seeds: []Seed{
					{Static: []byte("source_chain_state")},
					{Dynamic: AccountLookup{Location: "Info.AbstractReports.Messages.Header.DestChainSelector"}},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA lookup to get the Router Report Accounts.
			PDALookups{
				Name: "CommitReport",
				// The public key is a constant Router address.
				PublicKey: AccountConstant{
					Address: routerProgramAddress,
				},
				Seeds: []Seed{
					{Static: []byte("commit_report")},
					{Dynamic: AccountLookup{Location: "Info.AbstractReports.Messages.Header.DestChainSelector"}},
					{Dynamic: AccountLookup{
						// The seed is the merkle root of the report, as passed into the input params.
						Location: "Info.MerkleRoots.MerkleRoot",
					}},
				},
				IsSigner:   false,
				IsWritable: true,
			},
			// Static PDA lookup
			PDALookups{
				Name: "ExternalExecutionConfig",
				PublicKey: AccountConstant{
					Address: routerProgramAddress,
				},
				Seeds: []Seed{
					{Static: []byte("external_execution_config")},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// feePayer/authority address
			AccountConstant{
				Name:       "Authority",
				Address:    fromAddress,
				IsSigner:   true,
				IsWritable: true,
			},
			// Account constant
			AccountConstant{
				Name:       "SystemProgram",
				Address:    solana.SystemProgramID.String(),
				IsSigner:   false,
				IsWritable: false,
			},
			// Account constant
			AccountConstant{
				Name:       "SysvarInstructions",
				Address:    sysvarInstructionsAddress,
				IsSigner:   false,
				IsWritable: false,
			},
			// Static PDA lookup
			PDALookups{
				Name: "ExternalTokenPoolsSigner",
				PublicKey: AccountConstant{
					Address: routerProgramAddress,
				},
				Seeds: []Seed{
					{Static: []byte("external_token_pools_signer")},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// User specified accounts - formatted as AccountMeta
			AccountLookup{
				Name:       "UserAccounts",
				Location:   "Info.AbstractReports.Message.ExtraArgsDecoded.Accounts",
				IsWritable: MetaBool{BitmapLocation: "Info.AbstractReports.Message.ExtraArgsDecoded.IsWritableBitmap"},
				IsSigner:   MetaBool{Value: false},
			},
			// PDA Account Lookup - Based on an account lookup and an address lookup
			PDALookups{
				Name: "ReceiverAssociatedTokenAccount",
				PublicKey: AccountConstant{
					Address: solana.SPLAssociatedTokenAccountProgramID.String(),
				},
				Seeds: []Seed{
					// receiver address
					{Dynamic: AccountLookup{Location: "Info.AbstractReports.Messages.Receiver"}},
					// token programs
					{Dynamic: AccountsFromLookupTable{
						LookupTableName: "PoolLookupTable",
						IncludeIndexes:  []int{6},
					}},
					// mint
					{Dynamic: AccountLookup{Location: "Info.AbstractReports.Messages.TokenAmounts.DestTokenAddress"}},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA Account Lookup - Based on an account lookup and an address lookup
			PDALookups{
				Name: "SenderAssociatedTokenAccount",
				PublicKey: AccountConstant{
					Address: solana.SPLAssociatedTokenAccountProgramID.String(),
				},
				Seeds: []Seed{
					// sender address
					{Static: []byte(fromAddress)},
					// token program
					{Dynamic: AccountsFromLookupTable{
						LookupTableName: "PoolLookupTable",
						IncludeIndexes:  []int{6},
					}},
					// mint
					{Dynamic: AccountLookup{Location: "Info.AbstractReports.Messages.TokenAmounts.DestTokenAddress"}},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			PDALookups{
				Name: "PerChainTokenConfig",
				// PublicKey is a constant account in this case, not a lookup.
				PublicKey: AccountConstant{
					Address: routerProgramAddress,
				},
				// Similar to the TokenAdminRegistry above, the user is looking up PDA accounts based on the dest tokens.
				Seeds: []Seed{
					{Static: []byte("ccip_tokenpool_billing")},
					{Dynamic: AccountLookup{Location: "Info.AbstractReports.Messages.Header.DestChainSelector"}},
					{Dynamic: AccountLookup{Location: "Info.AbstractReports.Messages.TokenAmounts.DestTokenAddress"}},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			PDALookups{
				Name: "PoolChainConfig",
				// constant public key
				PublicKey: AccountsFromLookupTable{
					LookupTableName: "PoolLookupTable",
					// PoolProgram
					IncludeIndexes: []int{2},
				},
				Seeds: []Seed{
					{Static: []byte("ccip_tokenpool_chainconfig")},
					{Dynamic: AccountLookup{Location: "Info.AbstractReports.Messages.Header.DestChainSelector"}},
					{Dynamic: AccountLookup{Location: "Info.AbstractReports.Messages.TokenAmounts.DestTokenAddress"}},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// Lookup Table content - Get the accounts from the derived lookup table(s)
			AccountsFromLookupTable{
				LookupTableName: "PoolLookupTable",
				IncludeIndexes:  []int{}, // If left empty, all addresses will be included. Otherwise, only the specified indexes will be included.
			},
		},
		// TBD where this will be in the report
		// This will be appended to every error message
		DebugIDLocation: "AbstractReport.Message.MessageID",
	}

	commitConfig := MethodConfig{
		FromAddress: fromAddress,
		InputModifications: []codec.ModifierConfig{
			&codec.RenameModifierConfig{
				Fields: map[string]string{"ReportContextByteWords": "ReportContext"},
			},
			&codec.RenameModifierConfig{
				Fields: map[string]string{"RawReport": "Report"},
			},
		},
		ChainSpecificName: "commit",
		LookupTables: LookupTables{
			StaticLookupTables: []solana.PublicKey{
				commonAddressesLookupTable,
			},
		},
		Accounts: []Lookup{
			// Static PDA lookup
			PDALookups{
				Name: "RouterAccountConfig",
				PublicKey: AccountConstant{
					Address: routerProgramAddress,
				},
				Seeds: []Seed{
					{Static: []byte("config")},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			PDALookups{
				Name: "SourceChainState",
				// PublicKey is a constant account in this case, not a lookup.
				PublicKey: AccountConstant{
					Address: routerProgramAddress,
				},
				// Similar to the TokenAdminRegistry above, the user is looking up PDA accounts based on the dest tokens.
				Seeds: []Seed{
					{Static: []byte("source_chain_state")},
					{Dynamic: AccountLookup{Location: "Info.MerkleRoots.ChainSel"}},
				},
				IsSigner:   false,
				IsWritable: true,
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
					{Static: []byte("commit_report")},
					{Dynamic: AccountLookup{Location: "Info.MerkleRoots.ChainSel"}},
					{Dynamic: AccountLookup{
						// The seed is the merkle root of the report, as passed into the input params.
						Location: "Info.MerkleRoots.MerkleRoot",
					}},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// feePayer/authority address
			AccountConstant{
				Name:       "Authority",
				Address:    fromAddress,
				IsSigner:   true,
				IsWritable: true,
			},
			// Account constant
			AccountConstant{
				Name:       "SystemProgram",
				Address:    solana.SystemProgramID.String(),
				IsSigner:   false,
				IsWritable: false,
			},
			// Account constant
			AccountConstant{
				Name:       "SysvarInstructions",
				Address:    sysvarInstructionsAddress,
				IsSigner:   false,
				IsWritable: false,
			},
			// Static PDA lookup
			PDALookups{
				Name: "GlobalState",
				PublicKey: AccountConstant{
					Address: routerProgramAddress,
				},
				Seeds: []Seed{
					{Static: []byte("state")},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA lookup
			PDALookups{
				Name: "BillingTokenConfig",
				PublicKey: AccountConstant{
					Address: routerProgramAddress,
				},
				Seeds: []Seed{
					{Static: []byte("fee_billing_token_config")},
					{Dynamic: AccountLookup{Location: "Info.TokenPrices.TokenID"}},
				},
				IsSigner:   false,
				IsWritable: false,
			},
			// PDA lookup
			PDALookups{
				Name: "ChainConfigGasPrice",
				PublicKey: AccountConstant{
					Address: routerProgramAddress,
				},
				Seeds: []Seed{
					{Static: []byte("dest_chain_state")},
					{Dynamic: AccountLookup{Location: "Info.MerkleRoots.ChainSel"}},
				},
				IsSigner:   false,
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

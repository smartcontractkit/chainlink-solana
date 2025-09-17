package chainwriter

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/codec"

	idl "github.com/smartcontractkit/chainlink-ccip/chains/solana"
	ccipconsts "github.com/smartcontractkit/chainlink-ccip/pkg/consts"

	solanacodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

var ccipOfframpIDL = idl.FetchCCIPOfframpIDL()
var ccipRouterIDL = idl.FetchCCIPRouterIDL()

const (
	destTokenAddress              = "Info.AbstractReports.Messages.TokenAmounts.DestTokenAddress"
	tokenReceiverAddress          = "ExtraData.ExtraArgsDecoded.tokenReceiver"
	merkleRootSourceChainSelector = "Info.MerkleRoots.ChainSel"
	merkleRoot                    = "Info.MerkleRoots.MerkleRoot"
)

type ExecuteMethodConfigFunc func(string, string) MethodConfig

func getCommitMethodConfig(fromAddress string, offrampProgramAddress string, priceOnly bool) MethodConfig {
	chainSpecificName := "commit"
	if priceOnly {
		chainSpecificName = "commitPriceOnly"
	}
	return MethodConfig{
		FromAddress: fromAddress,
		InputModifications: []codec.ModifierConfig{
			&codec.RenameModifierConfig{
				Fields: map[string]string{"ReportContextByteWords": "ReportContext"},
			},
			&codec.RenameModifierConfig{
				Fields: map[string]string{"RawReport": "Report"},
			},
		},
		ChainSpecificName: chainSpecificName,
		ArgsTransform:     "CCIPCommit",
		LookupTables: LookupTables{
			DerivedLookupTables: []DerivedLookupTable{
				getCommonAddressLookupTableConfig(offrampProgramAddress),
			},
		},
		Accounts:        buildCommitAccountsList(fromAddress, offrampProgramAddress, priceOnly),
		DebugIDLocation: "",
	}
}

func buildCommitAccountsList(fromAddress, offrampProgramAddress string, priceOnly bool) []Lookup {
	accounts := []Lookup{}
	accounts = append(accounts,
		getOfframpAccountConfig(offrampProgramAddress),
		getReferenceAddressesConfig(offrampProgramAddress),
	)
	if !priceOnly {
		accounts = append(accounts,
			Lookup{
				PDALookups: &PDALookups{
					Name:      "SourceChainState",
					PublicKey: getAddressConstant(offrampProgramAddress),
					Seeds: []Seed{
						{Static: []byte("source_chain_state")},
						{Dynamic: Lookup{AccountLookup: &AccountLookup{Location: merkleRootSourceChainSelector}}},
					},
					IsSigner:   false,
					IsWritable: true,
				},
			},
			Lookup{
				PDALookups: &PDALookups{
					Name:      "CommitReport",
					PublicKey: getAddressConstant(offrampProgramAddress),
					Seeds: []Seed{
						{Static: []byte("commit_report")},
						{Dynamic: Lookup{AccountLookup: &AccountLookup{Location: merkleRootSourceChainSelector}}},
						{Dynamic: Lookup{AccountLookup: &AccountLookup{Location: merkleRoot}}},
					},
					IsSigner:   false,
					IsWritable: true,
				},
			},
		)
	}
	accounts = append(accounts,
		getAuthorityAccountConstant(fromAddress),
		getSystemProgramConstant(),
		getSysVarInstructionConstant(),
		getFeeBillingSignerConfig(offrampProgramAddress),
		getFeeQuoterProgramAccount(offrampProgramAddress),
		getFeeQuoterAllowedPriceUpdater(offrampProgramAddress),
		getFeeQuoterConfigLookup(offrampProgramAddress),
		getRMNRemoteProgramAccount(offrampProgramAddress),
		getRMNRemoteCursesLookup(offrampProgramAddress),
		getRMNRemoteConfigLookup(offrampProgramAddress),
		getGlobalStateConfig(offrampProgramAddress),
		getBillingTokenConfig(offrampProgramAddress),
		getChainConfigGasPriceConfig(offrampProgramAddress),
	)
	return accounts
}

func getExecuteMethodConfig(fromAddress string, _ string) MethodConfig {
	return MethodConfig{
		FromAddress: fromAddress,
		InputModifications: []codec.ModifierConfig{
			&codec.RenameModifierConfig{
				Fields: map[string]string{"ReportContextByteWords": "ReportContext"},
			},
			&codec.RenameModifierConfig{
				Fields: map[string]string{"RawExecutionReport": "Report"},
			},
		},
		ChainSpecificName:        "execute",
		ArgsTransform:            "CCIPExecuteV2",
		ComputeUnitLimitOverhead: 150_000,
		BufferPayloadMethod:      "CCIPExecutionReportBuffer",
		ATAs: []ATALookup{
			{
				Location:      destTokenAddress,
				WalletAddress: Lookup{AccountLookup: &AccountLookup{Location: tokenReceiverAddress}},
				MintAddress:   Lookup{AccountLookup: &AccountLookup{Location: destTokenAddress}},
				Optional:      true, // ATA lookup is optional if DestTokenAddress is not present in report
			},
		},
		// All accounts and lookup tables including the ones for messaging and token transfers are derived using an on-chain method
		// https://github.com/smartcontractkit/chainlink-ccip/blob/main/chains/solana/contracts/programs/ccip-offramp/src/instructions/v1/execute/derive.rs
		Accounts:        nil,
		DebugIDLocation: "Info.AbstractReports.Messages.Header.MessageID",
	}
}

func GetSolanaChainWriterConfig(offrampProgramAddress string, fromAddress string) (ChainWriterConfig, error) {
	// check fromAddress
	pk, err := solana.PublicKeyFromBase58(fromAddress)
	if err != nil {
		return ChainWriterConfig{}, fmt.Errorf("invalid from address %s: %w", fromAddress, err)
	}

	if pk.IsZero() {
		return ChainWriterConfig{}, errors.New("from address cannot be empty")
	}

	// validate CCIP Offramp IDL, errors not expected
	var offrampIDL solanacodec.IDL
	if err = json.Unmarshal([]byte(ccipOfframpIDL), &offrampIDL); err != nil {
		return ChainWriterConfig{}, fmt.Errorf("unexpected error: invalid CCIP Offramp IDL, error: %w", err)
	}
	// validate CCIP Router IDL, errors not expected
	var routerIDL solanacodec.IDL
	if err = json.Unmarshal([]byte(ccipRouterIDL), &routerIDL); err != nil {
		return ChainWriterConfig{}, fmt.Errorf("unexpected error: invalid CCIP Router IDL, error: %w", err)
	}
	solConfig := ChainWriterConfig{
		Programs: map[string]ProgramConfig{
			ccipconsts.ContractNameOffRamp: {
				Methods: map[string]MethodConfig{
					ccipconsts.MethodExecute:         getExecuteMethodConfig(fromAddress, offrampProgramAddress),
					ccipconsts.MethodCommit:          getCommitMethodConfig(fromAddress, offrampProgramAddress, false),
					ccipconsts.MethodCommitPriceOnly: getCommitMethodConfig(fromAddress, offrampProgramAddress, true),
				},
				IDL: ccipOfframpIDL,
			},
		},
	}

	return solConfig, nil
}

func getOfframpAccountConfig(offrampProgramAddress string) Lookup {
	return Lookup{
		PDALookups: &PDALookups{
			Name: "OfframpAccountConfig",
			PublicKey: Lookup{
				AccountConstant: &AccountConstant{
					Address: offrampProgramAddress,
				},
			},
			Seeds: []Seed{
				{Static: []byte("config")},
			},
			IsSigner:   false,
			IsWritable: false,
		},
	}
}

func getAddressConstant(address string) Lookup {
	return Lookup{
		AccountConstant: &AccountConstant{
			Address:    address,
			IsSigner:   false,
			IsWritable: false,
		},
	}
}

func getFeeQuoterProgramAccount(offrampProgramAddress string) Lookup {
	return Lookup{
		PDALookups: &PDALookups{
			Name:      ccipconsts.ContractNameFeeQuoter,
			PublicKey: getAddressConstant(offrampProgramAddress),
			Seeds: []Seed{
				{Static: []byte("reference_addresses")},
			},
			IsSigner:   false,
			IsWritable: false,
			// Reads the address from the reference addresses account
			InternalField: InternalField{
				TypeName: "ReferenceAddresses",
				Location: "FeeQuoter",
				IDL:      ccipOfframpIDL,
			},
		},
	}
}

func getReferenceAddressesConfig(offrampProgramAddress string) Lookup {
	return Lookup{
		PDALookups: &PDALookups{
			Name:      "ReferenceAddresses",
			PublicKey: getAddressConstant(offrampProgramAddress),
			Seeds: []Seed{
				{Static: []byte("reference_addresses")},
			},
			IsSigner:   false,
			IsWritable: false,
		},
	}
}

func getFeeBillingSignerConfig(offrampProgramAddress string) Lookup {
	return Lookup{
		PDALookups: &PDALookups{
			Name:      "FeeBillingSigner",
			PublicKey: getAddressConstant(offrampProgramAddress),
			Seeds: []Seed{
				{Static: []byte("fee_billing_signer")},
			},
			IsSigner:   false,
			IsWritable: false,
		},
	}
}

func getFeeQuoterAllowedPriceUpdater(offrampProgramAddress string) Lookup {
	return Lookup{
		PDALookups: &PDALookups{
			Name: "FeeQuoterAllowedPriceUpdater",
			// Fetch fee quoter public key to use as program ID for PDA
			PublicKey: getFeeQuoterProgramAccount(offrampProgramAddress),
			Seeds: []Seed{
				{Static: []byte("allowed_price_updater")},
				{Dynamic: getFeeBillingSignerConfig(offrampProgramAddress)},
			},
			IsSigner:   false,
			IsWritable: false,
		},
	}
}

func getFeeQuoterConfigLookup(offrampProgramAddress string) Lookup {
	return Lookup{
		PDALookups: &PDALookups{
			Name: "FeeQuoterConfig",
			// Fetch fee quoter public key to use as program ID for PDA
			PublicKey: getFeeQuoterProgramAccount(offrampProgramAddress),
			Seeds: []Seed{
				{Static: []byte("config")},
			},
			IsSigner:   false,
			IsWritable: false,
		},
	}
}

func getRMNRemoteProgramAccount(offrampProgramAddress string) Lookup {
	return Lookup{
		PDALookups: &PDALookups{
			Name:      ccipconsts.ContractNameRMNRemote,
			PublicKey: getAddressConstant(offrampProgramAddress),
			Seeds: []Seed{
				{Static: []byte("reference_addresses")},
			},
			IsSigner:   false,
			IsWritable: false,
			// Reads the address from the reference addresses account
			InternalField: InternalField{
				TypeName: "ReferenceAddresses",
				Location: "RmnRemote",
				IDL:      ccipOfframpIDL,
			},
		},
	}
}

func getRMNRemoteCursesLookup(offrampProgramAddress string) Lookup {
	return Lookup{
		PDALookups: &PDALookups{
			Name:      "RMNRemoteCurses",
			PublicKey: getRMNRemoteProgramAccount(offrampProgramAddress),
			Seeds: []Seed{
				{Static: []byte("curses")},
			},
			IsSigner:   false,
			IsWritable: false,
		},
	}
}

func getRMNRemoteConfigLookup(offrampProgramAddress string) Lookup {
	return Lookup{
		PDALookups: &PDALookups{
			Name:      "RMNRemoteConfig",
			PublicKey: getRMNRemoteProgramAccount(offrampProgramAddress),
			Seeds: []Seed{
				{Static: []byte("config")},
			},
			IsSigner:   false,
			IsWritable: false,
		},
	}
}

func getGlobalStateConfig(offrampProgramAddress string) Lookup {
	return Lookup{
		PDALookups: &PDALookups{
			Name:      "GlobalState",
			PublicKey: getAddressConstant(offrampProgramAddress),
			Seeds: []Seed{
				{Static: []byte("state")},
			},
			IsSigner:   false,
			IsWritable: true,
		},
		Optional: true,
	}
}

func getBillingTokenConfig(offrampProgramAddress string) Lookup {
	return Lookup{
		PDALookups: &PDALookups{
			Name:      "BillingTokenConfig",
			PublicKey: getFeeQuoterProgramAccount(offrampProgramAddress),
			Seeds: []Seed{
				{Static: []byte("fee_billing_token_config")},
				{Dynamic: Lookup{AccountLookup: &AccountLookup{Location: "Info.TokenPriceUpdates.TokenID"}}},
			},
			IsSigner:   false,
			IsWritable: true,
		},
		Optional: true,
	}
}

func getChainConfigGasPriceConfig(offrampProgramAddress string) Lookup {
	return Lookup{
		PDALookups: &PDALookups{
			Name:      "ChainConfigGasPrice",
			PublicKey: getFeeQuoterProgramAccount(offrampProgramAddress),
			Seeds: []Seed{
				{Static: []byte("dest_chain")},
				{Dynamic: Lookup{AccountLookup: &AccountLookup{Location: "Info.GasPriceUpdates.ChainSel"}}},
			},
			IsSigner:   false,
			IsWritable: true,
		},
		Optional: true,
	}
}

// getCommonAddressLookupTableConfig returns the lookup table config that fetches the lookup table address from a PDA on-chain
// The offramp contract contains a PDA with a ReferenceAddresses struct that stores the lookup table address in the OfframpLookupTable field
func getCommonAddressLookupTableConfig(offrampProgramAddress string) DerivedLookupTable {
	return DerivedLookupTable{
		Name: "CommonAddressLookupTable",
		Accounts: Lookup{
			PDALookups: &PDALookups{
				Name:      "OfframpLookupTable",
				PublicKey: getAddressConstant(offrampProgramAddress),
				Seeds: []Seed{
					{Static: []byte("reference_addresses")},
				},
				InternalField: InternalField{
					TypeName: "ReferenceAddresses",
					Location: "OfframpLookupTable",
					IDL:      ccipOfframpIDL,
				},
			},
		},
	}
}

func getAuthorityAccountConstant(fromAddress string) Lookup {
	return Lookup{
		AccountConstant: &AccountConstant{
			Name:       "Authority",
			Address:    fromAddress,
			IsSigner:   true,
			IsWritable: true,
		},
	}
}

func getSystemProgramConstant() Lookup {
	return Lookup{
		AccountConstant: &AccountConstant{
			Name:       "SystemProgram",
			Address:    solana.SystemProgramID.String(),
			IsSigner:   false,
			IsWritable: false,
		},
	}
}

func getSysVarInstructionConstant() Lookup {
	return Lookup{
		AccountConstant: &AccountConstant{
			Name:       "SysvarInstructions",
			Address:    solana.SysVarInstructionsPubkey.String(),
			IsSigner:   false,
			IsWritable: false,
		},
	}
}

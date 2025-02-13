package chainwriter

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_offramp"
	ccipconsts "github.com/smartcontractkit/chainlink-ccip/pkg/consts"
	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"
)

// TODO: make this type in the chainlink-common CW package
type ReportPreTransform struct {
	ReportContext  [2][32]byte
	Report         []byte
	Info           ccipocr3.ExecuteReportInfo
	AbstractReport ccip_offramp.ExecutionReportSingleChain
}

type ReportPostTransform struct {
	ReportContext  [2][32]byte
	Report         []byte
	Info           ccipocr3.ExecuteReportInfo
	AbstractReport ccip_offramp.ExecutionReportSingleChain
	TokenIndexes   []byte
}

func FindTransform(id string) (func(context.Context, *SolanaChainWriterService, any, solana.AccountMetaSlice, string) (any, error), error) {
	switch id {
	case "CCIP":
		return CCIPArgsTransform, nil
	default:
		return nil, fmt.Errorf("transform not found")
	}
}

// This Transform function looks up the token pool addresses in the accounts slice and augments the args
// with the indexes of the token pool addresses in the accounts slice.
func CCIPArgsTransform(ctx context.Context, cw *SolanaChainWriterService, args any, accounts solana.AccountMetaSlice, toAddress string) (any, error) {
	// Fetch offramp config to use to fetch the router address
	offrampProgramConfig, ok := cw.config.Programs[ccipconsts.ContractNameOffRamp]
	if !ok {
		return nil, fmt.Errorf("%s program not found in config", ccipconsts.ContractNameOffRamp)
	}
	// PDA lookup to fetch router address
	routerAddrLookup := PDALookups{
		Name: "ReferenceAddresses",
		PublicKey: Lookup{AccountConstant: &AccountConstant{
			Address: toAddress,
		}},
		Seeds: []Seed{
			{Static: []byte("reference_addresses")},
		},
		// Reads the router address from the reference addresses PDA
		InternalField: InternalField{
			TypeName: "ReferenceAddresses",
			Location: "Router",
			IDL:      offrampProgramConfig.IDL,
		},
	}
	accountMetas, err := routerAddrLookup.Resolve(ctx, nil, nil, cw.client)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the router program address from the reference addresses account: %w", err)
	}
	if len(accountMetas) != 1 {
		return nil, fmt.Errorf("expect 1 address to be returned for router address, received %d: %w", len(accountMetas), err)
	}

	// Fetch router config to use to fetch TokenAdminRegistry
	routerProgramConfig, ok := cw.config.Programs[ccipconsts.ContractNameRouter]
	if !ok {
		return nil, fmt.Errorf("%s program not found in config", ccipconsts.ContractNameRouter)
	}

	routerAddress := accountMetas[0].PublicKey
	TokenPoolLookupTable := LookupTables{
		DerivedLookupTables: []DerivedLookupTable{
			{
				Name: "PoolLookupTable",
				Accounts: Lookup{PDALookups: &PDALookups{
					Name: "TokenAdminRegistry",
					PublicKey: Lookup{AccountConstant: &AccountConstant{
						Address: routerAddress.String(),
					}},
					Seeds: []Seed{
						{Static: []byte("token_admin_registry")},
						{Dynamic: Lookup{AccountLookup: &AccountLookup{Location: "Info.AbstractReports.Messages.TokenAmounts.DestTokenAddress"}}},
					},
					IsSigner:   false,
					IsWritable: false,
					InternalField: InternalField{
						TypeName: "TokenAdminRegistry",
						Location: "LookupTable",
						IDL:      routerProgramConfig.IDL,
					},
				}},
			},
		},
	}

	tableMap, _, err := cw.ResolveLookupTables(ctx, args, TokenPoolLookupTable)
	if err != nil {
		return nil, err
	}
	registryTables := tableMap["PoolLookupTable"]
	tokenPoolAddresses := []solana.PublicKey{}
	for _, table := range registryTables {
		tokenPoolAddresses = append(tokenPoolAddresses, table[0].PublicKey)
	}

	tokenIndexes := []uint8{}
	for i, account := range accounts {
		for _, address := range tokenPoolAddresses {
			if account.PublicKey == address {
				if i > 255 {
					return nil, fmt.Errorf("index %d out of range for uint8", i)
				}
				tokenIndexes = append(tokenIndexes, uint8(i)) //nolint:gosec
			}
		}
	}

	if len(tokenIndexes) != len(tokenPoolAddresses) {
		return nil, fmt.Errorf("missing token pools in accounts")
	}

	argsTyped, ok := args.(ReportPreTransform)
	if !ok {
		return nil, fmt.Errorf("args is not of type ReportPreTransform")
	}

	argsTransformed := ReportPostTransform{
		ReportContext:  argsTyped.ReportContext,
		Report:         argsTyped.Report,
		AbstractReport: argsTyped.AbstractReport,
		Info:           argsTyped.Info,
		TokenIndexes:   tokenIndexes,
	}

	return argsTransformed, nil
}

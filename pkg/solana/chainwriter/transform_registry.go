package chainwriter

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"
)

// TODO: make this type in the chainlink-common CW package
type ReportPreTransform struct {
	ReportContext  [2][32]byte
	Report         []byte
	Info           ccipocr3.ExecuteReportInfo
	AbstractReport ccip_router.ExecutionReportSingleChain
}

type ReportPostTransform struct {
	ReportContext  [2][32]byte
	Report         []byte
	Info           ccipocr3.ExecuteReportInfo
	AbstractReport ccip_router.ExecutionReportSingleChain
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
	TokenPoolLookupTable := LookupTables{
		DerivedLookupTables: []DerivedLookupTable{
			{
				Name: "PoolLookupTable",
				Accounts: PDALookups{
					Name: "TokenAdminRegistry",
					PublicKey: AccountConstant{
						Address: toAddress,
					},
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
	}

	routerProgramConfig, ok := cw.config.Programs["ccip_router"]
	if !ok {
		return nil, fmt.Errorf("ccip_router program not found in config")
	}

	tableMap, _, err := cw.ResolveLookupTables(ctx, args, TokenPoolLookupTable, routerProgramConfig.IDL)
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

package chainwriter

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/mitchellh/mapstructure"

	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"

	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"
)

type ReportPostTransform struct {
	ReportContext [2][32]byte
	Report        []byte
	Info          ccipocr3.ExecuteReportInfo
	TokenIndexes  []byte
}

type ExtraDataDecoded struct {
	// ExtraArgsDecoded contain message specific extra args.
	ExtraArgsDecoded map[string]any
	// DestExecDataDecoded contain token transfer specific extra args.
	DestExecDataDecoded []map[string]any
}

type SVMExecCallArgs struct {
	ReportContext [2][32]byte                `mapstructure:"ReportContext"`
	Report        []byte                     `mapstructure:"Report"`
	Info          ccipocr3.ExecuteReportInfo `mapstructure:"Info"`
	ExtraData     ExtraDataDecoded           `mapstructure:"ExtraData"`
}

type SVMCommitCallArgs struct {
	ReportContext [2][32]byte               `mapstructure:"ReportContext"`
	Report        []byte                    `mapstructure:"Report"`
	Rs            [][32]byte                `mapstructure:"Rs"`
	Ss            [][32]byte                `mapstructure:"Ss"`
	RawVs         [32]byte                  `mapstructure:"RawVs"`
	Info          ccipocr3.CommitReportInfo `mapstructure:"Info"`
}

// TODO: replace with actual value from CCIP on-chain
const StaticCuOverhead uint32 = 120000

func FindTransform(id string) (func(context.Context, any, solana.AccountMetaSlice, map[string]map[string][]*solana.AccountMeta) (any, solana.AccountMetaSlice, []txmutils.SetTxConfig, error), error) {
	switch id {
	case "CCIPExecute":
		return CCIPExecuteTransform, nil
	case "CCIPCommit":
		return CCIPCommitTransform, nil
	default:
		return nil, fmt.Errorf("transform not found")
	}
}

// This Transform function looks up the token pool addresses in the accounts slice and augments the args
// with the indexes of the token pool addresses in the accounts slice.
func CCIPExecuteTransform(ctx context.Context, args any, accounts solana.AccountMetaSlice, tableMap map[string]map[string][]*solana.AccountMeta) (any, solana.AccountMetaSlice, []txmutils.SetTxConfig, error) {
	var argsDecoded SVMExecCallArgs
	err := mapstructure.Decode(args, &argsDecoded)
	if err != nil {
		return nil, nil, []txmutils.SetTxConfig{}, err
	}

	argsTransformed := ReportPostTransform{
		ReportContext: argsDecoded.ReportContext,
		Report:        argsDecoded.Report,
		Info:          argsDecoded.Info,
	}

	if len(argsTransformed.Info.AbstractReports) != 1 || len(argsTransformed.Info.AbstractReports[0].Messages) != 1 {
		return nil, nil, []txmutils.SetTxConfig{}, fmt.Errorf("Expected 1 report with 1 message")
	}

	cu, ok := argsDecoded.ExtraData.ExtraArgsDecoded["computeUnits"].(uint32)
	if !ok {
		return nil, nil, []txmutils.SetTxConfig{}, fmt.Errorf("computeUnits not found in ExtraData")
	}

	computeUnits := StaticCuOverhead + cu

	for _, execData := range argsDecoded.ExtraData.DestExecDataDecoded {
		destGasAmount, ok := execData["destGasAmount"].(uint32)
		if !ok {
			return nil, nil, []txmutils.SetTxConfig{}, fmt.Errorf("DestGasAmount not found in ExtraData")
		}
		computeUnits += destGasAmount
	}

	options := []txmutils.SetTxConfig{
		txmutils.SetEstimateComputeUnitLimit(false),
		txmutils.SetComputeUnitLimit(computeUnits),
	}

	registryTables, exists := tableMap["PoolLookupTable"]
	// If PoolLookupTable does not exist in the table map, token indexes are not needed
	// Return with empty TokenIndexes
	if !exists {
		argsTransformed.TokenIndexes = []byte{}
		return argsTransformed, accounts, options, nil
	}

	tokenPoolAddresses := []solana.PublicKey{}
	for _, table := range registryTables {
		tokenPoolAddresses = append(tokenPoolAddresses, table[0].PublicKey)
	}

	tokenIndexes := []uint8{}
	for i, account := range accounts {
		for _, address := range tokenPoolAddresses {
			if account.PublicKey == address {
				if i > 255 {
					return nil, nil, []txmutils.SetTxConfig{}, fmt.Errorf("index %d out of range for uint8", i)
				}
				tokenIndexes = append(tokenIndexes, uint8(i)) //nolint:gosec
			}
		}
	}

	if len(tokenIndexes) != len(tokenPoolAddresses) {
		return nil, nil, []txmutils.SetTxConfig{}, fmt.Errorf("missing token pools in accounts")
	}

	argsTransformed.TokenIndexes = tokenIndexes
	return argsTransformed, accounts, options, nil
}

// This Transform function trims off the GlobalState account from commit transactions if there are no token or gas price updates
func CCIPCommitTransform(ctx context.Context, args any, accounts solana.AccountMetaSlice, _ map[string]map[string][]*solana.AccountMeta) (any, solana.AccountMetaSlice, []txmutils.SetTxConfig, error) {
	var argsDecoded SVMCommitCallArgs
	err := mapstructure.Decode(args, &argsDecoded)
	if err != nil {
		return nil, nil, []txmutils.SetTxConfig{}, err
	}

	tokenPriceVals := argsDecoded.Info.TokenPriceUpdates
	gasPriceVals := argsDecoded.Info.GasPriceUpdates

	transformedAccounts := accounts
	if len(tokenPriceVals) == 0 && len(gasPriceVals) == 0 {
		transformedAccounts = accounts[:len(accounts)-1]
	}
	return args, transformedAccounts, []txmutils.SetTxConfig{txmutils.SetEstimateComputeUnitLimit(true)}, nil
}

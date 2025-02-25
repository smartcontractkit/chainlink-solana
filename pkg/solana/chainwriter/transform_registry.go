package chainwriter

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/mitchellh/mapstructure"
	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"
)

type ReportPostTransform struct {
	ReportContext [2][32]byte
	Report        []byte
	Info          ccipocr3.ExecuteReportInfo
	TokenIndexes  []byte
}

func FindTransform(id string) (func(context.Context, any, solana.AccountMetaSlice, map[string]map[string][]*solana.AccountMeta) (any, solana.AccountMetaSlice, uint32, error), error) {
	switch id {
	case "CCIPExecute":
		return CCIPExecuteArgsTransform, nil
	case "CCIPCommit":
		return CCIPCommitAccountTransform, nil
	default:
		return nil, fmt.Errorf("transform not found")
	}
}

// This Transform function looks up the token pool addresses in the accounts slice and augments the args
// with the indexes of the token pool addresses in the accounts slice.
func CCIPExecuteArgsTransform(ctx context.Context, args any, accounts solana.AccountMetaSlice, tableMap map[string]map[string][]*solana.AccountMeta) (any, solana.AccountMetaSlice, uint32, error) {
	var argsTransformed ReportPostTransform
	err := mapstructure.Decode(args, &argsTransformed)
	if err != nil {
		return nil, nil, 0, err
	}

	if len(argsTransformed.Info.AbstractReports) != 1 || len(argsTransformed.Info.AbstractReports[0].Messages) != 1 {
		return nil, nil, 0, fmt.Errorf("Expected 1 report with 1 message")
	}

	computeUnits := argsTransformed.Info.AbstractReports[0].Messages[0].ExtraArgsDecoded["ComputeUnits"]
	if computeUnits == nil {
		return nil, nil, 0, errors.New("missing ComputeUnits in ExtraArgs")
	}

	registryTables, exists := tableMap["PoolLookupTable"]
	// If PoolLookupTable does not exist in the table map, token indexes are not needed
	// Return with empty TokenIndexes
	if !exists {
		argsTransformed.TokenIndexes = []byte{}
		return argsTransformed, accounts, computeUnits.(uint32), nil
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
					return nil, nil, 0, fmt.Errorf("index %d out of range for uint8", i)
				}
				tokenIndexes = append(tokenIndexes, uint8(i)) //nolint:gosec
			}
		}
	}

	if len(tokenIndexes) != len(tokenPoolAddresses) {
		return nil, nil, 0, fmt.Errorf("missing token pools in accounts")
	}

	argsTransformed.TokenIndexes = tokenIndexes
	return argsTransformed, accounts, computeUnits.(uint32), nil
}

// This Transform function trims off the GlobalState account from commit transactions if there are no token or gas price updates
func CCIPCommitAccountTransform(ctx context.Context, args any, accounts solana.AccountMetaSlice, _ map[string]map[string][]*solana.AccountMeta) (any, solana.AccountMetaSlice, uint32, error) {
	var tokenPriceVals, gasPriceVals [][]byte
	var err error
	tokenPriceVals, err = GetValuesAtLocation(args, "Info.TokenPrices.TokenID")
	if err != nil && !errors.Is(err, errFieldNotFound) {
		return nil, nil, 0, fmt.Errorf("error getting values at location: %w", err)
	}
	gasPriceVals, err = GetValuesAtLocation(args, "Info.GasPrices.ChainSel")
	if err != nil && !errors.Is(err, errFieldNotFound) {
		return nil, nil, 0, fmt.Errorf("error getting values at location: %w", err)
	}
	transformedAccounts := accounts
	if len(tokenPriceVals) == 0 && len(gasPriceVals) == 0 {
		transformedAccounts = accounts[:len(accounts)-1]
	}
	return args, transformedAccounts, 0, nil
}

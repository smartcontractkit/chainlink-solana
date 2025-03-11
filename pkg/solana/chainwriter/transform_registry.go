package chainwriter

import (
	"context"
	"errors"
	"fmt"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mitchellh/mapstructure"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
)

type ReportPostTransform struct {
	ReportContext  [2][32]byte
	Report         []byte
	Info           ccipocr3.ExecuteReportInfo
	AbstractReport ccip_offramp.ExecutionReportSingleChain
	TokenIndexes   []byte
}

func FindTransform(id string) (func(context.Context, client.MultiClient, any, solana.AccountMetaSlice, map[string]map[string][]*solana.AccountMeta) (any, solana.AccountMetaSlice, error), error) {
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
func CCIPExecuteArgsTransform(ctx context.Context, client client.MultiClient, args any, accounts solana.AccountMetaSlice, tableMap map[string]map[string][]*solana.AccountMeta) (any, solana.AccountMetaSlice, error) {
	var argsTransformed ReportPostTransform
	err := mapstructure.Decode(args, &argsTransformed)
	if err != nil {
		return nil, nil, err
	}

	poolTables, exists := tableMap["PoolLookupTable"]
	// If PoolLookupTable does not exist in the table map, token indexes are not needed
	// Return with empty TokenIndexes
	if !exists {
		argsTransformed.TokenIndexes = []byte{}
		return argsTransformed, accounts, nil
	}

	// TODO: needs offsetting for receiver_program, account, etc. at the start of remaining accounts
	offset := 0
	tokenIndexes := make([]uint8, 0, len(poolTables))
	for _, table := range poolTables {
		tokenAdminRegistryPDA := table[1].PublicKey

		// load token admin registry
		resp, err := client.GetAccountInfoWithOpts(ctx, tokenAdminRegistryPDA, &rpc.GetAccountInfoOpts{
			Encoding:   "base64",
			Commitment: rpc.CommitmentFinalized,
		})
		if err != nil {
			return nil, nil, err
		}
		tokenAdminRegistry := ccip_router.TokenAdminRegistry{}
		bin.NewBorshDecoder(resp.GetBinary()).Decode(&tokenAdminRegistry)

		// copied from utils/tokens/tokenpool.go
		writableBytes := append(tokenAdminRegistry.WritableIndexes[0].Bytes(), tokenAdminRegistry.WritableIndexes[1].Bytes()...)
		writableBits := ""
		for _, b := range writableBytes {
			writableBits += fmt.Sprintf("%08b", b)
		}

		// set is_writable according to token admin registry
		for i := range table {
			writable := string(writableBits[i]) == "1"
			// skip first 14 accounts, since they are mandatory/not remaining_accounts
			// skip 3 accounts since they indicate other accounts than lookup field
			accounts[14+offset+3+i].IsWritable = writable
		}

		// calculate the token index
		tokenIndexes = append(tokenIndexes, uint8(offset)) //nolint:gosec
		offset += len(table) + 3
	}

	if len(tokenIndexes) != len(poolTables) {
		return nil, nil, fmt.Errorf("missing token pools in accounts")
	}

	argsTransformed.TokenIndexes = tokenIndexes
	return argsTransformed, accounts, nil
}

// This Transform function trims off the GlobalState account from commit transactions if there are no token or gas price updates
func CCIPCommitAccountTransform(ctx context.Context, client client.MultiClient, args any, accounts solana.AccountMetaSlice, _ map[string]map[string][]*solana.AccountMeta) (any, solana.AccountMetaSlice, error) {
	var tokenPriceVals, gasPriceVals [][]byte
	var err error
	tokenPriceVals, err = GetValuesAtLocation(args, "Info.TokenPriceUpdates.TokenID")
	if err != nil && !errors.Is(err, errFieldNotFound) {
		return nil, nil, fmt.Errorf("error getting values at location: %w", err)
	}
	gasPriceVals, err = GetValuesAtLocation(args, "Info.GasPriceUpdates.ChainSel")
	if err != nil && !errors.Is(err, errFieldNotFound) {
		return nil, nil, fmt.Errorf("error getting values at location: %w", err)
	}
	transformedAccounts := accounts
	if len(tokenPriceVals) == 0 && len(gasPriceVals) == 0 {
		transformedAccounts = accounts[:len(accounts)-1]
	}
	return args, transformedAccounts, nil
}

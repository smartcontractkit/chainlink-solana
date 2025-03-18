package chainwriter

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mitchellh/mapstructure"

	idl "github.com/smartcontractkit/chainlink-ccip/chains/solana"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_router"
	ccipconsts "github.com/smartcontractkit/chainlink-ccip/pkg/consts"
	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
)

const MandatoryExecuteAccounts = 14

type ReportPostTransform struct {
	ReportContext  [2][32]byte
	Report         []byte
	Info           ccipocr3.ExecuteReportInfo
	AbstractReport ccip_offramp.ExecutionReportSingleChain
	TokenIndexes   []byte
}

func FindTransform(id string) (func(context.Context, client.MultiClient, any, solana.AccountMetaSlice, map[string]map[string][]*solana.AccountMeta, string) (any, solana.AccountMetaSlice, error), error) {
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
func CCIPExecuteArgsTransform(ctx context.Context, client client.MultiClient, args any, accounts solana.AccountMetaSlice, tableMap map[string]map[string][]*solana.AccountMeta, toAddress string) (any, solana.AccountMetaSlice, error) {
	var argsTransformed ReportPostTransform
	err := mapstructure.Decode(args, &argsTransformed)
	if err != nil {
		return nil, nil, err
	}

	registryTables, exists := tableMap["PoolLookupTable"]
	// If PoolLookupTable does not exist in the table map, token indexes are not needed
	// Return with empty TokenIndexes
	if !exists {
		argsTransformed.TokenIndexes = []byte{}
		return argsTransformed, accounts, nil
	}

	// Fetch all of the accounts in the pool lookup table with the proper IsWritable flag set
	poolLookupAccounts, err := fetchPoolLookupAccounts(ctx, client, registryTables)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch pool lookup accounts and set wrtiable flags: %w", err)
	}

	var aggregatedMessages []ccipocr3.Message
	for _, report := range argsTransformed.Info.AbstractReports {
		aggregatedMessages = append(aggregatedMessages, report.Messages...)
	}

	feeQuoterAddress, err := getFeeQuoterAddress(ctx, toAddress, args, tableMap, client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch fee quoter address: %w", err)
	}

	var tokenIndexes []uint8
	// Accounts below are maintained to be in particular indexes in the Token Admin registry lookup table
	poolProgram := poolLookupAccounts[2]
	tokenProgram := poolLookupAccounts[6]
	// Append token accounts to the account list and track at which index accounts for each token transfer starts
	for _, message := range aggregatedMessages {
		receiver := message.Receiver
		sourceChainSelector := make([]byte, 8)
		binary.LittleEndian.PutUint64(sourceChainSelector, uint64(message.Header.SourceChainSelector))
		for _, tokenAmount := range message.TokenAmounts {
			destTokenAddress := tokenAmount.DestTokenAddress
			userTokenAccount, err := getUserTokenAccount(receiver, tokenProgram.PublicKey, destTokenAddress)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to calculate user token account PDA: %w", err)
			}
			perChainTokenConfig, err := getPerChainTokenConfig(sourceChainSelector, destTokenAddress, feeQuoterAddress)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to calculate per chain per token config PDA: %w", err)
			}
			poolChainConfig, err := getPoolChainConfig(sourceChainSelector, destTokenAddress, poolProgram.PublicKey)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to calculate pool chain config PDA: %w", err)
			}
			if len(accounts) < MandatoryExecuteAccounts {
				return nil, nil, fmt.Errorf("encountered unexpected number of accounts, expected at least %d, got %d", MandatoryExecuteAccounts, len(accounts))
			}
			// Token indexes are relative to the remaining accounts which exclude mandatory accounts
			tokenIndexes = append(tokenIndexes, uint8(len(accounts)-MandatoryExecuteAccounts)) //nolint:gosec
			// Append all token accounts for transfer
			accounts = append(accounts,
				&solana.AccountMeta{PublicKey: userTokenAccount, IsWritable: true},
				&solana.AccountMeta{PublicKey: perChainTokenConfig},
				&solana.AccountMeta{PublicKey: poolChainConfig, IsWritable: true},
			)
			// Append all pool lookup accounts needed for pool interaction
			accounts = append(accounts, poolLookupAccounts...)
		}
	}

	argsTransformed.TokenIndexes = tokenIndexes
	return argsTransformed, accounts, nil
}

// This Transform function trims off the GlobalState account from commit transactions if there are no token or gas price updates
func CCIPCommitAccountTransform(ctx context.Context, client client.MultiClient, args any, accounts solana.AccountMetaSlice, _ map[string]map[string][]*solana.AccountMeta, _ string) (any, solana.AccountMetaSlice, error) {
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

func fetchPoolLookupAccounts(ctx context.Context, client client.MultiClient, poolTables map[string][]*solana.AccountMeta) ([]*solana.AccountMeta, error) {
	var poolAccounts []*solana.AccountMeta
	// poolTables only contains a single lookup table for token admin registry
	for _, table := range poolTables {
		tokenAdminRegistryPDA := table[1].PublicKey

		// load token admin registry
		resp, err := client.GetAccountInfoWithOpts(ctx, tokenAdminRegistryPDA, &rpc.GetAccountInfoOpts{
			Encoding:   "base64",
			Commitment: rpc.CommitmentFinalized,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch token admin registry account: %w", err)
		}
		tokenAdminRegistry := ccip_router.TokenAdminRegistry{}
		err = bin.NewBorshDecoder(resp.GetBinary()).Decode(&tokenAdminRegistry)
		if err != nil {
			return nil, fmt.Errorf("failed to borsh decode token admin registry account: %w", err)
		}

		// lookup tables can store 256 addresses
		// token admin registry's WritableIndexes field is the binary representation of indexes that are writable stored in 2 separate uint128 on-chain
		writableBytes := append(tokenAdminRegistry.WritableIndexes[0].Bytes(), tokenAdminRegistry.WritableIndexes[1].Bytes()...)
		writableBits := ""
		for _, b := range writableBytes {
			writableBits += fmt.Sprintf("%08b", b)
		}
		// set IsWritable according to token admin registry's WritableIndexes
		for i, meta := range table {
			writable := string(writableBits[i]) == "1"
			meta.IsWritable = writable
			poolAccounts = append(poolAccounts, meta)
		}
	}
	return poolAccounts, nil
}

func getFeeQuoterAddress(ctx context.Context, toAddress string, args any, tableMap map[string]map[string][]*solana.AccountMeta, client client.MultiClient) (solana.PublicKey, error) {
	lookup := Lookup{
		PDALookups: &PDALookups{
			Name:      ccipconsts.ContractNameFeeQuoter,
			PublicKey: Lookup{AccountConstant: &AccountConstant{Address: toAddress}},
			Seeds: []Seed{
				{Static: []byte("reference_addresses")},
			},
			// Reads the address from the reference addresses account
			InternalField: InternalField{
				TypeName: "ReferenceAddresses",
				Location: "FeeQuoter",
				IDL:      idl.FetchCCIPOfframpIDL(),
			},
		},
	}
	feeQuoters, err := lookup.Resolve(ctx, args, tableMap, client)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to fetch the fee quoter address: %w", err)
	}
	if len(feeQuoters) != 1 {
		return solana.PublicKey{}, fmt.Errorf("expected 1 address for fee quoter, fetched %d", len(feeQuoters))
	}
	return feeQuoters[0].PublicKey, nil
}

func getUserTokenAccount(receiver []byte, tokenProgram solana.PublicKey, destTokenAddress []byte) (solana.PublicKey, error) {
	userTokenAccount, _, err := solana.FindProgramAddress([][]byte{receiver, tokenProgram.Bytes(), destTokenAddress}, solana.SPLAssociatedTokenAccountProgramID)
	return userTokenAccount, err
}

func getPerChainTokenConfig(sourceChainSelector, destTokenAddress []byte, feeQuoterAddress solana.PublicKey) (solana.PublicKey, error) {
	perChainTokenConfig, _, err := solana.FindProgramAddress([][]byte{[]byte("per_chain_per_token_config"), sourceChainSelector, destTokenAddress}, feeQuoterAddress)
	return perChainTokenConfig, err
}

func getPoolChainConfig(sourceChainSelector, destTokenAddress []byte, poolProgram solana.PublicKey) (solana.PublicKey, error) {
	poolChainConfig, _, err := solana.FindProgramAddress([][]byte{[]byte("ccip_tokenpool_chainconfig"), sourceChainSelector, destTokenAddress}, poolProgram)
	return poolChainConfig, err
}

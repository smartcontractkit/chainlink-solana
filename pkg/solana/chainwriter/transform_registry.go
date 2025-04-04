package chainwriter

import (
	"context"
	"encoding/binary"
	"fmt"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mitchellh/mapstructure"

	ccipsolana "github.com/smartcontractkit/chainlink-ccip/chains/solana"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_common"
	ccipconsts "github.com/smartcontractkit/chainlink-ccip/pkg/consts"

	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

// TODO: replace with exact value after CCIP testing is completed.
const StaticCuOverhead uint32 = 100000
const MandatoryExecuteAccounts = 14

func FindTransform(id string) (func(context.Context, client.MultiClient, any, solana.AccountMetaSlice, map[string]map[string][]*solana.AccountMeta, string) (any, solana.AccountMetaSlice, []txmutils.SetTxConfig, error), error) {
	switch id {
	case "CCIPExecute":
		return CCIPExecuteArgsTransform, nil
	case "CCIPCommit":
		return CCIPCommitAccountTransform, nil
	default:
		return nil, fmt.Errorf("transform not found")
	}
}

// CCIPExecuteArgsTransform calculates required compute units, and appends any needed accounts by fetching pool lookup table entries.
// It then updates token indexes based on appended PDAs and returns the transformed arguments, extended accounts slice, and cu tx configs.
func CCIPExecuteArgsTransform(ctx context.Context, client client.MultiClient, args any, accounts solana.AccountMetaSlice, tableMap map[string]map[string][]*solana.AccountMeta, toAddress string) (any, solana.AccountMetaSlice, []txmutils.SetTxConfig, error) {
	var argsTransformed ccipsolana.SVMExecCallArgs
	err := mapstructure.Decode(args, &argsTransformed)
	if err != nil {
		return nil, nil, []txmutils.SetTxConfig{}, err
	}

	cu, ok := argsTransformed.ExtraData.ExtraArgsDecoded["computeUnits"].(uint32)
	if !ok {
		return nil, nil, []txmutils.SetTxConfig{}, fmt.Errorf("computeUnits not found in ExtraData")
	}

	computeUnits := StaticCuOverhead + cu

	for _, execData := range argsTransformed.ExtraData.DestExecDataDecoded {
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

	// Fetch all of the accounts in the pool lookup table with the proper IsWritable flag set
	poolLookupAccounts, err := fetchPoolLookupAccounts(ctx, client, registryTables)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch pool lookup accounts and set wrtiable flags: %w", err)
	}

	var aggregatedMessages []ccipocr3.Message
	for _, report := range argsTransformed.Info.AbstractReports {
		aggregatedMessages = append(aggregatedMessages, report.Messages...)
	}

	feeQuoterAddress, err := getFeeQuoterAddress(ctx, toAddress, args, tableMap, client)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch fee quoter address: %w", err)
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
				return nil, nil, nil, fmt.Errorf("failed to calculate user token account PDA: %w", err)
			}
			perChainTokenConfig, err := getPerChainTokenConfig(sourceChainSelector, destTokenAddress, feeQuoterAddress)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("failed to calculate per chain per token config PDA: %w", err)
			}
			poolChainConfig, err := getPoolChainConfig(sourceChainSelector, destTokenAddress, poolProgram.PublicKey)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("failed to calculate pool chain config PDA: %w", err)
			}
			if len(accounts) < MandatoryExecuteAccounts {
				return nil, nil, nil, fmt.Errorf("encountered unexpected number of accounts, expected at least %d, got %d", MandatoryExecuteAccounts, len(accounts))
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
	return argsTransformed, accounts, options, nil
}

// This Transform function trims off the GlobalState account from commit transactions if there are no token or gas price updates
func CCIPCommitAccountTransform(ctx context.Context, client client.MultiClient, args any, accounts solana.AccountMetaSlice, _ map[string]map[string][]*solana.AccountMeta, _ string) (any, solana.AccountMetaSlice, []txmutils.SetTxConfig, error) {
	var argsDecoded ccipsolana.SVMCommitCallArgs
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

	options := []txmutils.SetTxConfig{
		txmutils.SetEstimateComputeUnitLimit(true),
	}

	return args, transformedAccounts, options, nil
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
		tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
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
				IDL:      ccipsolana.FetchCCIPOfframpIDL(),
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

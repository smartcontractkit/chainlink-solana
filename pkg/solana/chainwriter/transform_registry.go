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

	ccipsolana "github.com/smartcontractkit/chainlink-ccip/chains/solana"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_common"
	ccipconsts "github.com/smartcontractkit/chainlink-ccip/pkg/consts"

	"github.com/smartcontractkit/chainlink-ccip/pkg/types/ccipocr3"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

const MandatoryExecuteAccounts = 12

func FindTransform(id string) (func(context.Context, client.MultiClient, any, solana.AccountMetaSlice, map[string]map[string][]*solana.AccountMeta, string, uint32) (any, solana.AccountMetaSlice, []txmutils.SetTxConfig, error), error) {
	switch id {
	case "CCIPExecute":
		return CCIPExecuteArgsTransform, nil
	case "CCIPCommit":
		return CCIPCommitAccountTransform, nil
	default:
		return nil, fmt.Errorf("transform not found")
	}
}

type commonTokenTransferAccounts struct {
	poolLookupAccounts []*solana.AccountMeta
	poolProgram        *solana.AccountMeta
	tokenProgram       *solana.AccountMeta
	tokenReceiver      solana.PublicKey
	feeQuoterAddress   solana.PublicKey
	offrampPoolsSigner solana.PublicKey
}

// CCIPExecuteArgsTransform calculates required compute units, and appends any needed accounts by fetching pool lookup table entries.
// It then updates token indexes based on appended PDAs and returns the transformed arguments, extended accounts slice, and cu tx configs.
func CCIPExecuteArgsTransform(ctx context.Context, client client.MultiClient, args any, accounts solana.AccountMetaSlice, tableMap map[string]map[string][]*solana.AccountMeta, toAddress string, computeUnitLimitOverhead uint32) (any, solana.AccountMetaSlice, []txmutils.SetTxConfig, error) {
	var argsTransformed ccipsolana.SVMExecCallArgs
	err := mapstructure.Decode(args, &argsTransformed)
	if err != nil {
		return nil, nil, nil, err
	}

	computeUnits, err := calculateComputeUnitLimit(argsTransformed, computeUnitLimitOverhead)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to calculate compute unit limit: %w", err)
	}

	options := []txmutils.SetTxConfig{
		txmutils.SetEstimateComputeUnitLimit(false),
		txmutils.SetComputeUnitLimit(computeUnits),
	}

	if len(accounts) < MandatoryExecuteAccounts {
		return nil, nil, nil, fmt.Errorf("encountered unexpected number of accounts, expected at least %d, got %d", MandatoryExecuteAccounts, len(accounts))
	}

	var aggregatedMessages []ccipocr3.Message
	tokenAccountsRequired := false
	// Aggregate all report messages and track if token transfer accounts are required
	for _, report := range argsTransformed.Info.AbstractReports {
		aggregatedMessages = append(aggregatedMessages, report.Messages...)
		if tokenAccountsRequired {
			continue
		}
		// Token accounts are required if any message contains token amounts
		for _, message := range report.Messages {
			if len(message.TokenAmounts) > 0 {
				tokenAccountsRequired = true
			}
		}
	}

	tokenIndexes := []uint8{}
	commonTTAccounts, err := resolveCommonTokenTransferAccounts(ctx, tokenAccountsRequired, client, toAddress, args, tableMap)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to resolve accounts required for token transfer: %w", err)
	}

	// Append token accounts to the account list and track at which index accounts for each token transfer starts
	for _, message := range aggregatedMessages {
		// Append the logic receiver and the user defined messaging accounts to list
		accounts, err = appendMessagingAccounts(accounts, message.Receiver, args, toAddress)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to append user messaging accounts to list: %w", err)
		}
		// Append token transfer accounts for each TokenAmount if required
		accounts, tokenIndexes, err = appendTokenTransferAccounts(tokenAccountsRequired, accounts, message.Header.SourceChainSelector, message.TokenAmounts, commonTTAccounts, tokenIndexes)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to append token transfer accounts to list: %w", err)
		}
	}

	argsTransformed.TokenIndexes = tokenIndexes
	return argsTransformed, accounts, options, nil
}

// This Transform function trims off the GlobalState account from commit transactions if there are no token or gas price updates
func CCIPCommitAccountTransform(ctx context.Context, client client.MultiClient, args any, accounts solana.AccountMetaSlice, _ map[string]map[string][]*solana.AccountMeta, _ string, _ uint32) (any, solana.AccountMetaSlice, []txmutils.SetTxConfig, error) {
	var argsDecoded ccipsolana.SVMCommitCallArgs
	err := mapstructure.Decode(args, &argsDecoded)
	if err != nil {
		return nil, nil, []txmutils.SetTxConfig{}, err
	}

	tokenPriceVals := argsDecoded.Info.TokenPriceUpdates
	gasPriceVals := argsDecoded.Info.GasPriceUpdates

	transformedAccounts := accounts
	// Remove the global state config from the end of the account list if neither token nor gas price updates are included
	if len(accounts) > 0 && len(tokenPriceVals) == 0 && len(gasPriceVals) == 0 {
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

func calculateComputeUnitLimit(argsTransformed ccipsolana.SVMExecCallArgs, overhead uint32) (uint32, error) {
	cu, ok := argsTransformed.ExtraData.ExtraArgsDecoded["computeUnits"].(uint32)
	if !ok {
		return 0, fmt.Errorf("computeUnits not found in ExtraData")
	}

	computeUnits := overhead + cu

	for _, execData := range argsTransformed.ExtraData.DestExecDataDecoded {
		destGasAmount, ok := execData["destGasAmount"].(uint32)
		if !ok {
			return 0, fmt.Errorf("DestGasAmount not found in ExtraData")
		}
		computeUnits += destGasAmount
	}
	return computeUnits, nil
}

func resolveCommonTokenTransferAccounts(ctx context.Context, tokenAccountsRequired bool, client client.MultiClient, toAddress string, args any, tableMap map[string]map[string][]*solana.AccountMeta) (commonTokenTransferAccounts, error) {
	// Return empty struct if token accounts are not required
	if !tokenAccountsRequired {
		return commonTokenTransferAccounts{}, nil
	}
	registryTables, exists := tableMap["PoolLookupTable"]
	if !exists {
		return commonTokenTransferAccounts{}, fmt.Errorf("failed to find PoolLookupTable in table map, required for token transfer")
	}
	// Expect only one table for token admin registry
	if len(registryTables) != 1 {
		return commonTokenTransferAccounts{}, fmt.Errorf("unexpected number of registry tables %d, expected 1", len(registryTables))
	}
	// Fetch all of the accounts in the pool lookup table with the proper IsWritable flag set
	poolLookupAccounts, err := fetchPoolLookupAccounts(ctx, client, registryTables)
	if err != nil {
		return commonTokenTransferAccounts{}, fmt.Errorf("failed to fetch pool lookup accounts and set writable flags, required for token transfer: %w", err)
	}
	// Accounts below are maintained to be in particular indexes in the Token Admin registry lookup table
	if len(poolLookupAccounts) < 7 {
		return commonTokenTransferAccounts{}, fmt.Errorf("unexpected number of accounts in pool lookup table %d, expected at least 7", len(poolLookupAccounts))
	}
	poolProgram := poolLookupAccounts[2]
	tokenProgram := poolLookupAccounts[6]

	feeQuoterAddress, err := getFeeQuoterAddress(ctx, toAddress, args, tableMap, client)
	if err != nil {
		return commonTokenTransferAccounts{}, fmt.Errorf("failed to fetch fee quoter address, required for token transfer: %w", err)
	}

	tokenReceiverLookup := AccountLookup{Name: "TokenReceiver", Location: "ExtraData.ExtraArgsDecoded.tokenReceiver"}
	tokenReceivers, err := tokenReceiverLookup.Resolve(args)
	if err != nil {
		return commonTokenTransferAccounts{}, fmt.Errorf("failed to find token receiver, required for token transfers: %w", err)
	}
	if len(tokenReceivers) != 1 {
		return commonTokenTransferAccounts{}, fmt.Errorf("unexpected number of token receivers found %d, expected 1", len(tokenReceivers))
	}
	tokenReceiver := tokenReceivers[0].PublicKey

	offrampAddr, err := solana.PublicKeyFromBase58(toAddress)
	if err != nil {
		return commonTokenTransferAccounts{}, fmt.Errorf("failed to parse offramp address: %w", err)
	}
	offrampPoolsSigner, _, err := solana.FindProgramAddress([][]byte{[]byte("external_token_pools_signer"), poolProgram.PublicKey.Bytes()}, offrampAddr)
	if err != nil {
		return commonTokenTransferAccounts{}, fmt.Errorf("failed to calculate offramp pools signer PDA: %w", err)
	}

	return commonTokenTransferAccounts{
		poolLookupAccounts: poolLookupAccounts,
		poolProgram:        poolProgram,
		tokenProgram:       tokenProgram,
		feeQuoterAddress:   feeQuoterAddress,
		tokenReceiver:      tokenReceiver,
		offrampPoolsSigner: offrampPoolsSigner,
	}, nil
}

func appendMessagingAccounts(accounts solana.AccountMetaSlice, logicReceiver ccipocr3.UnknownAddress, args any, toAddress string) (solana.AccountMetaSlice, error) {
	// Messaging accounts do not need to be appended if logic receiver is zero or empty. Return accounts as is
	if !logicReceiver.IsZeroOrEmpty() {
		logicReceiverAddr := solana.PublicKeyFromBytes(logicReceiver)
		accounts = append(accounts, &solana.AccountMeta{
			PublicKey:  logicReceiverAddr,
			IsWritable: false,
			IsSigner:   false,
		})
		offrampAddr, err := solana.PublicKeyFromBase58(toAddress)
		if err != nil {
			return nil, fmt.Errorf("failed to parse offramp address: %w", err)
		}
		externalExecutionSigner, _, err := solana.FindProgramAddress([][]byte{[]byte("external_execution_config"), logicReceiverAddr.Bytes()}, offrampAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate external execution signer: %w", err)
		}
		accounts = append(accounts, &solana.AccountMeta{
			PublicKey:  externalExecutionSigner,
			IsWritable: false,
			IsSigner:   false,
		})
		userAccountsLookup := AccountLookup{
			Name:       "UserAccounts",
			Location:   "ExtraData.ExtraArgsDecoded.accounts",
			IsWritable: MetaBool{BitmapLocation: "ExtraData.ExtraArgsDecoded.accountIsWritableBitmap"},
			IsSigner:   MetaBool{Value: false},
		}
		userAccounts, err := userAccountsLookup.Resolve(args)
		// If err is ErrLookupNotFoundAtLocation, allow process to continue in case only logic receiver is needed for messaging
		if err != nil && !errors.Is(err, ErrLookupNotFoundAtLocation) {
			return nil, fmt.Errorf("failed to resolve user accounts: %w", err)
		}
		accounts = append(accounts, userAccounts...)
	}
	return accounts, nil
}

func appendTokenTransferAccounts(tokenAccountsRequired bool, accounts solana.AccountMetaSlice, sourceChainSel ccipocr3.ChainSelector, tokenAmounts []ccipocr3.RampTokenAmount, commonTTAccounts commonTokenTransferAccounts, tokenIndexes []uint8) (solana.AccountMetaSlice, []uint8, error) {
	// Return accounts and token indexes as is if token accounts are not required
	if !tokenAccountsRequired {
		return accounts, tokenIndexes, nil
	}
	sourceChainSelector := make([]byte, 8)
	binary.LittleEndian.PutUint64(sourceChainSelector, uint64(sourceChainSel))
	for _, tokenAmount := range tokenAmounts {
		destTokenAddress := tokenAmount.DestTokenAddress
		userTokenAccount, err := getUserTokenAccount(commonTTAccounts.tokenReceiver.Bytes(), commonTTAccounts.tokenProgram.PublicKey, destTokenAddress)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to calculate user token account PDA: %w", err)
		}
		perChainTokenConfig, err := getPerChainTokenConfig(sourceChainSelector, destTokenAddress, commonTTAccounts.feeQuoterAddress)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to calculate per chain per token config PDA: %w", err)
		}
		poolChainConfig, err := getPoolChainConfig(sourceChainSelector, destTokenAddress, commonTTAccounts.poolProgram.PublicKey)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to calculate pool chain config PDA: %w", err)
		}
		// Token indexes are relative to the remaining accounts which exclude mandatory accounts
		tokenIndexes = append(tokenIndexes, uint8(len(accounts)-MandatoryExecuteAccounts)) //nolint:gosec
		// Append all token accounts for transfer
		accounts = append(accounts,
			&solana.AccountMeta{PublicKey: commonTTAccounts.offrampPoolsSigner},
			&solana.AccountMeta{PublicKey: userTokenAccount, IsWritable: true},
			&solana.AccountMeta{PublicKey: perChainTokenConfig},
			&solana.AccountMeta{PublicKey: poolChainConfig, IsWritable: true},
		)
		// Append all pool lookup accounts needed for pool interaction
		accounts = append(accounts, commonTTAccounts.poolLookupAccounts...)
	}
	return accounts, tokenIndexes, nil
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

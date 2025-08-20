package chainwriter

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"math/big"
	"regexp"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/go-viper/mapstructure/v2"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"

	ccipsolana "github.com/smartcontractkit/chainlink-ccip/chains/solana"
	ccip_common_v0_1_0 "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/v0_1_0/ccip_common"
	ccip_offramp_v0_1_0 "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/v0_1_0/ccip_offramp"
	ccip_offramp_v0_1_1 "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/v0_1_1/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/state"
	ccipconsts "github.com/smartcontractkit/chainlink-ccip/pkg/consts"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

type ArgsTransformHandler func(context.Context, client.MultiClient, logger.Logger, any, solana.AccountMetaSlice, map[solana.PublicKey]solana.PublicKeySlice, map[string]map[string][]*solana.AccountMeta, solana.PublicKey, string, uint32, []txmutils.SetTxConfig, string) (any, solana.AccountMetaSlice, map[solana.PublicKey]solana.PublicKeySlice, []txmutils.SetTxConfig, error)

func FindTransform(id string) (ArgsTransformHandler, error) {
	switch id {
	case "CCIPExecute":
		return CCIPExecuteArgsTransform, nil
	case "CCIPExecuteV2":
		return CCIPExecuteArgsTransformV2, nil
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
// It then updates token indexes based on appended PDAs and returns the transformed arguments, extended accounts slice, unchanged static lookup tables map, and cu tx configs.
func CCIPExecuteArgsTransform(ctx context.Context, client client.MultiClient, lggr logger.Logger, args any, accounts solana.AccountMetaSlice, staticLUTs map[solana.PublicKey]solana.PublicKeySlice, derivedLUTs map[string]map[string][]*solana.AccountMeta, transmitter solana.PublicKey, toAddress string, computeUnitLimitOverhead uint32, options []txmutils.SetTxConfig, debugID string) (any, solana.AccountMetaSlice, map[solana.PublicKey]solana.PublicKeySlice, []txmutils.SetTxConfig, error) {
	var argsTransformed ccipsolana.SVMExecCallArgs
	err := mapstructure.Decode(args, &argsTransformed)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	computeUnits, err := calculateComputeUnitLimit(argsTransformed, computeUnitLimitOverhead)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to calculate compute unit limit: %w", err)
	}

	options = append(options, txmutils.SetEstimateComputeUnitLimit(false), txmutils.SetComputeUnitLimit(computeUnits))
	mandatoryAccountsLen := cap(ccip_offramp_v0_1_0.NewExecuteInstructionBuilder().AccountMetaSlice)

	if len(accounts) < mandatoryAccountsLen {
		return nil, nil, nil, nil, fmt.Errorf("encountered unexpected number of accounts, expected at least %d, got %d", mandatoryAccountsLen, len(accounts))
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
	commonTTAccounts, err := resolveCommonTokenTransferAccounts(ctx, tokenAccountsRequired, client, toAddress, args, derivedLUTs)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to resolve accounts required for token transfer: %w", err)
	}

	// Append token accounts to the account list and track at which index accounts for each token transfer starts
	for _, message := range aggregatedMessages {
		// Append the logic receiver and the user defined messaging accounts to list
		accounts, err = appendMessagingAccounts(accounts, message.Receiver, args, toAddress)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to append user messaging accounts to list: %w", err)
		}
		// Append token transfer accounts for each TokenAmount if required
		accounts, tokenIndexes, err = appendTokenTransferAccounts(tokenAccountsRequired, accounts, message.Header.SourceChainSelector, message.TokenAmounts, commonTTAccounts, tokenIndexes, mandatoryAccountsLen)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to append token transfer accounts to list: %w", err)
		}
	}

	argsTransformed.TokenIndexes = tokenIndexes
	return argsTransformed, accounts, staticLUTs, options, nil
}

// CCIPExecuteArgsTransformV2 calculates required compute units and uses on-chain account derivation to determine the accounts required for the execute transaction
// It tracks the token indexes for each token transfer and returns the transformed arguments, extended accounts slice, extended static lookup tables map, and cu tx configs.
func CCIPExecuteArgsTransformV2(ctx context.Context, client client.MultiClient, lggr logger.Logger, args any, accounts solana.AccountMetaSlice, staticLUTs map[solana.PublicKey]solana.PublicKeySlice, _ map[string]map[string][]*solana.AccountMeta, transmitter solana.PublicKey, toAddress string, computeUnitLimitOverhead uint32, options []txmutils.SetTxConfig, debugID string) (any, solana.AccountMetaSlice, map[solana.PublicKey]solana.PublicKeySlice, []txmutils.SetTxConfig, error) {
	if len(accounts) != 0 {
		return nil, nil, nil, nil, fmt.Errorf("expect accounts to be empty at start of CCIPExecuteArgsTransformV2, got %d", len(accounts))
	}

	var argsTransformed ccipsolana.SVMExecCallArgs
	err := mapstructure.Decode(args, &argsTransformed)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	computeUnits, err := calculateComputeUnitLimit(argsTransformed, computeUnitLimitOverhead)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to calculate compute unit limit: %w", err)
	}

	options = append(options, txmutils.SetEstimateComputeUnitLimit(false), txmutils.SetComputeUnitLimit(computeUnits))

	if len(argsTransformed.Info.AbstractReports) != 1 {
		return nil, nil, nil, nil, fmt.Errorf("encountered unexpected number of reports, got %d, expect 1", len(argsTransformed.Info.AbstractReports))
	}
	report := argsTransformed.Info.AbstractReports[0]
	if len(report.Messages) != 1 {
		return nil, nil, nil, nil, fmt.Errorf("encountered unexpected number of messages, got %d, expect 1", len(report.Messages))
	}

	message := report.Messages[0]
	sourceChainSel := message.Header.SourceChainSelector
	if len(argsTransformed.Info.MerkleRoots) != 1 {
		return nil, nil, nil, nil, fmt.Errorf("encountered unexpected number of merkle roots, got %d, expect 1", len(argsTransformed.Info.MerkleRoots))
	}
	merkleRoot := argsTransformed.Info.MerkleRoots[0].MerkleRoot

	var messageAccounts []ccip_offramp_v0_1_1.CcipAccountMeta
	if !message.Receiver.IsZeroOrEmpty() {
		logicReceiver := solana.PublicKeyFromBytes(message.Receiver)
		// Append logic receiver as the first messaging account for derivation
		messageAccounts = append(messageAccounts, ccip_offramp_v0_1_1.CcipAccountMeta{
			Pubkey:     logicReceiver,
			IsSigner:   false,
			IsWritable: false,
		})
		// Extract the user defined accounts
		userAccountsLookup := AccountLookup{
			Name:       "UserAccounts",
			Location:   "ExtraData.ExtraArgsDecoded.accounts",
			IsWritable: MetaBool{BitmapLocation: "ExtraData.ExtraArgsDecoded.accountIsWritableBitmap"},
			IsSigner:   MetaBool{Value: false},
		}
		userAccounts, resolveErr := userAccountsLookup.Resolve(args)
		// If err is ErrLookupNotFoundAtLocation, allow process to continue in case accounts are not needed
		if resolveErr != nil && !errors.Is(resolveErr, ErrLookupNotFoundAtLocation) {
			return nil, nil, nil, nil, fmt.Errorf("failed to resolve user accounts: %w", resolveErr)
		}
		messageAccounts = append(messageAccounts, ConvertToCCIPAccountMetas(userAccounts)...)
	}

	// Extract token transfers
	var tokenTransfers []ccip_offramp_v0_1_1.TokenTransferAndOffchainData
	var tokenReceiver solana.PublicKey
	var messageTokenData [][]byte
	if len(message.TokenAmounts) > 0 {
		tokenTransfers = make([]ccip_offramp_v0_1_1.TokenTransferAndOffchainData, 0, len(message.TokenAmounts))
		if len(argsTransformed.ExtraData.DestExecDataDecoded) != len(message.TokenAmounts) {
			return nil, nil, nil, nil, fmt.Errorf("unexpected number of DestExecData encountered. expect the same number as token transfers %d, got %d", len(message.TokenAmounts), len(argsTransformed.ExtraData.DestExecDataDecoded))
		}
		// If message contains token transfers, extract offchain token data for the message
		// Note: OffchainTokenData length equals the Messages length. If multiple messages are supported in the future, this restriction needs to be lifted as well.
		if len(report.OffchainTokenData) != 1 {
			return nil, nil, nil, nil, fmt.Errorf("unexpected number of OffchainTokenData encountered. expect the same number as messages %d, got %d", len(report.Messages), len(report.OffchainTokenData))
		}
		messageTokenData = report.OffchainTokenData[0]
		for i, tokenAmount := range message.TokenAmounts {
			destTokenAddress := solana.PublicKeyFromBytes(tokenAmount.DestTokenAddress)
			destGasAmount, ok := argsTransformed.ExtraData.DestExecDataDecoded[i]["destGasAmount"].(uint32)
			if !ok {
				return nil, nil, nil, nil, fmt.Errorf("dest gas amount not found in ExtraData for token transfer: %s", destTokenAddress.String())
			}
			if tokenAmount.Amount.IsEmpty() {
				return nil, nil, nil, nil, fmt.Errorf("token amount is empty for token transfer: %s", destTokenAddress.String())
			}
			if tokenAmount.Amount.Int.Sign() < 0 {
				return nil, nil, nil, nil, fmt.Errorf("negative amount for token: %s", destTokenAddress.String())
			}
			tokenTransfers = append(tokenTransfers, ccip_offramp_v0_1_1.TokenTransferAndOffchainData{
				Transfer: ccip_offramp_v0_1_1.Any2SVMTokenTransfer{
					SourcePoolAddress: tokenAmount.SourcePoolAddress,
					DestTokenAddress:  destTokenAddress,
					Amount:            ccip_offramp_v0_1_1.CrossChainAmount{LeBytes: [32]uint8(encodeBigIntToFixedLengthLE(tokenAmount.Amount.Int, 32))},
					ExtraData:         tokenAmount.ExtraData,
					DestGasAmount:     destGasAmount,
				},
				Data: nil, // Set to nil to optimize tx size during user messaging account derivation. Field set after user message account derivation is complete.
			})
		}
		tokenReceiverLookup := AccountLookup{Name: "TokenReceiver", Location: "ExtraData.ExtraArgsDecoded.tokenReceiver"}
		tokenReceivers, resolveErr := tokenReceiverLookup.Resolve(args)
		if resolveErr != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to find token receiver, required for token transfers: %w", resolveErr)
		}
		if len(tokenReceivers) != 1 {
			return nil, nil, nil, nil, fmt.Errorf("unexpected number of token receivers found %d, expected 1", len(tokenReceivers))
		}
		tokenReceiver = tokenReceivers[0].PublicKey
	}

	params := ccip_offramp_v0_1_1.DeriveAccountsExecuteParams{
		ExecuteCaller:       transmitter,
		MessageAccounts:     messageAccounts,
		SourceChainSelector: uint64(sourceChainSel),
		TokenTransfers:      tokenTransfers,
		MerkleRoot:          merkleRoot,
		TokenReceiver:       tokenReceiver,
		OriginalSender:      message.Sender,
	}

	lggr.Debugw("Deriving accounts", "params", params, "debugID", debugID)
	derivedAccounts, derivedLookupTables, tokenIndexes, err := deriveExecuteAccounts(ctx, client, params, messageTokenData, transmitter, toAddress, lggr)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to derive execute accounts: %w", err)
	}
	lggr.Debugw("Completed account derivation", "derivedAccounts", derivedAccounts, "lookupTables", derivedLookupTables, "tokenIndexes", tokenIndexes, "debugID", debugID)

	// Merge the derived lookup tables with the existing lookup table map
	maps.Copy(staticLUTs, derivedLookupTables)

	// Append derived accounts to the accounts list
	accounts = append(accounts, derivedAccounts...)

	argsTransformed.TokenIndexes = tokenIndexes
	return argsTransformed, accounts, staticLUTs, options, nil
}

// This Transform function trims off the GlobalState account from commit transactions if there are no token or gas price updates
func CCIPCommitAccountTransform(ctx context.Context, _ client.MultiClient, _ logger.Logger, args any, accounts solana.AccountMetaSlice, staticLUTs map[solana.PublicKey]solana.PublicKeySlice, _ map[string]map[string][]*solana.AccountMeta, _ solana.PublicKey, _ string, _ uint32, options []txmutils.SetTxConfig, _ string) (any, solana.AccountMetaSlice, map[solana.PublicKey]solana.PublicKeySlice, []txmutils.SetTxConfig, error) {
	var argsDecoded ccipsolana.SVMCommitCallArgs
	err := mapstructure.Decode(args, &argsDecoded)
	if err != nil {
		return nil, nil, nil, []txmutils.SetTxConfig{}, err
	}

	tokenPriceVals := argsDecoded.Info.TokenPriceUpdates
	gasPriceVals := argsDecoded.Info.GasPriceUpdates

	transformedAccounts := accounts
	// Remove the global state config from the end of the account list if neither token nor gas price updates are included
	if len(accounts) > 0 && len(tokenPriceVals) == 0 && len(gasPriceVals) == 0 {
		transformedAccounts = accounts[:len(accounts)-1]
	}

	options = append(options, txmutils.SetEstimateComputeUnitLimit(true))

	return args, transformedAccounts, staticLUTs, options, nil
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

	feeQuoterAddress, err := getFeeQuoterAddress(ctx, toAddress, args, client)
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

func appendTokenTransferAccounts(tokenAccountsRequired bool, accounts solana.AccountMetaSlice, sourceChainSel ccipocr3.ChainSelector, tokenAmounts []ccipocr3.RampTokenAmount, commonTTAccounts commonTokenTransferAccounts, tokenIndexes []uint8, mandatoryAccountsLen int) (solana.AccountMetaSlice, []uint8, error) {
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
		tokenIndexes = append(tokenIndexes, uint8(len(accounts)-mandatoryAccountsLen)) //nolint:gosec // safe: limitations on solana transaction sizes and account limits ensures the token index will always fit within uint8
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

func deriveExecuteAccounts(ctx context.Context, client client.MultiClient, params ccip_offramp_v0_1_1.DeriveAccountsExecuteParams, messageTokenData [][]byte, transmitter solana.PublicKey, offrampStr string, lggr logger.Logger) (solana.AccountMetaSlice, map[solana.PublicKey]solana.PublicKeySlice, []uint8, error) {
	blockhash, err := client.LatestBlockhash(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error fetching latest blockhash: %w", err)
	}
	offramp, err := solana.PublicKeyFromBase58(offrampStr)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse offramp address: %w", err)
	}
	config, _, err := state.FindOfframpConfigPDA(offramp)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to calculate offramp config address: %w", err)
	}
	var derivedAccounts, accountsToAskWith solana.AccountMetaSlice
	lookupTableMap := make(map[solana.PublicKey]solana.PublicKeySlice)
	tokenIndexes := []uint8{}
	mandatoryAccountsLen := cap(ccip_offramp_v0_1_1.NewExecuteInstructionBuilder().AccountMetaSlice)
	stage := "Start"
	ttAccountsMatcher, err := regexp.Compile(`^TokenTransferStaticAccounts/\d+/0$`)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to compile token transfer stage matcher: %w", err)
	}
	for {
		deriveAccountsIxRaw := ccip_offramp_v0_1_1.NewDeriveAccountsExecuteInstruction(params, stage, config)
		deriveAccountsIxRaw.AccountMetaSlice = append(deriveAccountsIxRaw.AccountMetaSlice, accountsToAskWith...)
		deriveAccountsIx, err := deriveAccountsIxRaw.ValidateAndBuild()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to build derive execute accounts instruction: %w", err)
		}
		deriveAccountsIxData, err := deriveAccountsIx.Data()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to encode account derivation instruction data: %w", err)
		}
		deriveAccountsSolIx := solana.NewInstruction(offramp, deriveAccountsIx.Accounts(), deriveAccountsIxData)
		tx, err := solana.NewTransaction([]solana.Instruction{deriveAccountsSolIx}, blockhash.Value.Blockhash, solana.TransactionPayer(transmitter), solana.TransactionAddressTables(lookupTableMap))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to build derive execute accounts transaction: %w", err)
		}

		tx.Signatures = append(tx.Signatures, solana.Signature{}) // Append empty signature since tx fails without any sigs even if SigVerify is false
		res, err := client.SimulateTx(ctx, tx, &rpc.SimulateTransactionOpts{SigVerify: false, ReplaceRecentBlockhash: true})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to simulate derive execute accounts transaction at stage %s: %w", stage, err)
		}
		if res.Err != nil {
			return nil, nil, nil, fmt.Errorf("failed to simulate derive execute accounts transaction at stage %s. Err: %v, Logs: %v", stage, res.Err, res.Logs)
		}
		derivation, err := common.ExtractAnchorTypedReturnValue[ccip_offramp_v0_1_1.DeriveAccountsResponse](ctx, res.Logs, offrampStr)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to exract accounts from simulated transaction log: %w", err)
		}

		if ttAccountsMatcher.MatchString(derivation.NextStage) {
			// Remove messaging accounts from the params to optimize token transfer account derivation tx sizes
			params.MessageAccounts = nil
			// Set the relevant offchain token data for each token transfer
			// This data is intentionally omitted from earlier stages to optimize user messaging account derivation tx sizes
			for i := range params.TokenTransfers {
				var tokenTransferTokenData []byte
				if len(messageTokenData) > i {
					tokenTransferTokenData = messageTokenData[i]
				}
				params.TokenTransfers[i].Data = tokenTransferTokenData
			}
		}

		// TokenTransferStaticAccounts stages derive the accounts needed for each token transfer
		// Track the index at which the first set of accounts for a token transfer are appended relative to the remaining accounts
		if ttAccountsMatcher.MatchString(derivation.CurrentStage) {
			tokenIndexes = append(tokenIndexes, uint8(len(derivedAccounts)-mandatoryAccountsLen)) //nolint:gosec // Limit on the number of token transfers prevents token index from exceeding uint8 max
		}

		// Convert CCIP metas to Solana metas and append to list
		derivedAccounts = append(derivedAccounts, ConvertToSolanaAccountMetas(derivation.AccountsToSave)...)
		// Convert CCIP metas to Solana metas and override previous list. Past ask again accounts are irrelevant.
		accountsToAskWith = ConvertToSolanaAccountMetas(derivation.AskAgainWith)

		lggr.Debugw("account derivation result", "stage", stage, "nextStage", derivation.NextStage, "save", derivation.AccountsToSave, "askAgain", derivation.AskAgainWith)

		// Fetch lookup tables on the fly so they can be used to lower future derivation tx sizes
		if len(derivation.LookUpTablesToSave) > 0 {
			currentStageLUTs, err := fetchLookupTables(ctx, client, derivation.LookUpTablesToSave)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("failed to fetch lookup tables: %w", err)
			}
			maps.Copy(lookupTableMap, currentStageLUTs)
		}

		stage = derivation.NextStage
		if stage == "" {
			return derivedAccounts, lookupTableMap, tokenIndexes, nil
		}
	}
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
		tokenAdminRegistry := ccip_common_v0_1_0.TokenAdminRegistry{}
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
			if meta == nil {
				continue
			}
			writable := string(writableBits[i]) == "1"
			meta.IsWritable = writable
			poolAccounts = append(poolAccounts, meta)
		}
	}
	return poolAccounts, nil
}

func fetchLookupTables(ctx context.Context, client client.MultiClient, lookupTablesAddrs []solana.PublicKey) (map[solana.PublicKey]solana.PublicKeySlice, error) {
	lookupTableMap := make(map[solana.PublicKey]solana.PublicKeySlice)
	for _, addr := range lookupTablesAddrs {
		lookupTableContents, err := getLookupTableAddresses(ctx, client, addr)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch lookup table contents for address %s: %w", addr.String(), err)
		}
		lookupTableMap[addr] = lookupTableContents
	}
	return lookupTableMap, nil
}

func getFeeQuoterAddress(ctx context.Context, toAddress string, args any, client client.MultiClient) (solana.PublicKey, error) {
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
	feeQuoters, err := lookup.Resolve(ctx, args, nil, client)
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

func ConvertToCCIPAccountMetas(metas solana.AccountMetaSlice) []ccip_offramp_v0_1_1.CcipAccountMeta {
	if len(metas) == 0 {
		return nil
	}
	ccipMetas := make([]ccip_offramp_v0_1_1.CcipAccountMeta, 0, len(metas))
	for _, account := range metas {
		ccipMetas = append(ccipMetas, ccip_offramp_v0_1_1.CcipAccountMeta{
			Pubkey:     account.PublicKey,
			IsSigner:   account.IsSigner,
			IsWritable: account.IsWritable,
		})
	}
	return ccipMetas
}

func ConvertToSolanaAccountMetas(metas []ccip_offramp_v0_1_1.CcipAccountMeta) solana.AccountMetaSlice {
	if len(metas) == 0 {
		return nil
	}
	solanaMetas := make([]*solana.AccountMeta, 0, len(metas))
	for _, account := range metas {
		solanaMetas = append(solanaMetas, &solana.AccountMeta{
			PublicKey:  account.Pubkey,
			IsSigner:   account.IsSigner,
			IsWritable: account.IsWritable,
		})
	}
	return solanaMetas
}

func encodeBigIntToFixedLengthLE(bi *big.Int, length int) []byte {
	// Create a fixed-length byte array
	paddedBytes := make([]byte, length)

	// Use FillBytes to fill the array with big-endian data, zero-padded
	bi.FillBytes(paddedBytes)

	// Reverse the array for little-endian encoding
	for i, j := 0, len(paddedBytes)-1; i < j; i, j = i+1, j-1 {
		paddedBytes[i], paddedBytes[j] = paddedBytes[j], paddedBytes[i]
	}

	return paddedBytes
}

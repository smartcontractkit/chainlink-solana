package chainwriter

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"

	"github.com/gagliardetto/solana-go"
	"github.com/go-viper/mapstructure/v2"

	ccipsolana "github.com/smartcontractkit/chainlink-ccip/chains/solana"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/state"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

func FindTransform(id string) (func(context.Context, client.MultiClient, any, solana.AccountMetaSlice, map[solana.PublicKey]solana.PublicKeySlice, solana.PublicKey, string, uint32, []txmutils.SetTxConfig) (any, solana.AccountMetaSlice, map[solana.PublicKey]solana.PublicKeySlice, []txmutils.SetTxConfig, error), error) {
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
func CCIPExecuteArgsTransform(ctx context.Context, client client.MultiClient, args any, accounts solana.AccountMetaSlice, lookupTables map[solana.PublicKey]solana.PublicKeySlice, transmitter solana.PublicKey, toAddress string, computeUnitLimitOverhead uint32, options []txmutils.SetTxConfig) (any, solana.AccountMetaSlice, map[solana.PublicKey]solana.PublicKeySlice, []txmutils.SetTxConfig, error) {
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

	var messageAccounts []ccip_offramp.CcipAccountMeta
	if !message.Receiver.IsZeroOrEmpty() {
		logicReceiver := solana.PublicKeyFromBytes(message.Receiver)
		// Append logic receiver as the first messaging account for derivation
		messageAccounts = append(messageAccounts, ccip_offramp.CcipAccountMeta{
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

	// Extract token transfer mints
	var transferredMints []solana.PublicKey
	var tokenReceiver solana.PublicKey
	if len(message.TokenAmounts) > 0 {
		transferredMints = make([]solana.PublicKey, 0, len(message.TokenAmounts))
		for _, tokenAmount := range message.TokenAmounts {
			transferredMints = append(transferredMints, solana.PublicKeyFromBytes(tokenAmount.DestTokenAddress))
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

	params := ccip_offramp.DeriveAccountsExecuteParams{
		ExecuteCaller:            transmitter,
		MessageAccounts:          messageAccounts,
		SourceChainSelector:      uint64(sourceChainSel),
		MintsOfTransferredTokens: transferredMints,
		MerkleRoot:               merkleRoot,
		TokenReceiver:            tokenReceiver,
	}
	derivedAccounts, derivedLookupTables, tokenIndexes, err := deriveExecuteAccounts(ctx, client, params, transmitter, toAddress)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to derived execute accounts: %w", err)
	}

	// Merge the derived lookup tables with the existing lookup table map
	maps.Copy(lookupTables, derivedLookupTables)

	// Append derived accounts to the accounts list
	accounts = append(accounts, derivedAccounts...)

	argsTransformed.TokenIndexes = tokenIndexes
	return argsTransformed, accounts, lookupTables, options, nil
}

// This Transform function trims off the GlobalState account from commit transactions if there are no token or gas price updates
func CCIPCommitAccountTransform(ctx context.Context, _ client.MultiClient, args any, accounts solana.AccountMetaSlice, _ map[solana.PublicKey]solana.PublicKeySlice, _ solana.PublicKey, _ string, _ uint32, options []txmutils.SetTxConfig) (any, solana.AccountMetaSlice, map[solana.PublicKey]solana.PublicKeySlice, []txmutils.SetTxConfig, error) {
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

	return args, transformedAccounts, nil, options, nil
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

func deriveExecuteAccounts(ctx context.Context, client client.MultiClient, params ccip_offramp.DeriveAccountsExecuteParams, transmitter solana.PublicKey, offrampStr string) (solana.AccountMetaSlice, map[solana.PublicKey]solana.PublicKeySlice, []uint8, error) {
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
	lookupTablesAddrs := []solana.PublicKey{}
	tokenIndexes := []uint8{}
	mandatoryAccountsLen := cap(ccip_offramp.NewExecuteInstructionBuilder().AccountMetaSlice)
	stage := "start"
	matcher, err := regexp.Compile(`^TokenTransferStaticAccounts/\d+/0$`)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to compile token transfer stage matcher: %w", err)
	}
	for {
		deriveAccountsIxRaw := ccip_offramp.NewDeriveAccountsExecuteInstruction(params, stage, config)
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
		tx, err := solana.NewTransaction([]solana.Instruction{deriveAccountsSolIx}, blockhash.Value.Blockhash, solana.TransactionPayer(transmitter))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to build derive execute accounts transaction: %w", err)
		}

		res, err := client.SimulateTx(ctx, tx, nil)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to simulate derive execute accounts transaction: %w", err)
		}
		derivation, err := common.ExtractAnchorTypedReturnValue[ccip_offramp.DeriveAccountsResponse](ctx, res.Logs, offrampStr)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to exract accounts from simulated transaction log: %w", err)
		}

		// TokenTransferStaticAccounts stages derive the accounts needed for each token transfer
		// Track the index at which the first set of accounts for a token transfer are appended relative to the remaining accounts
		if matcher.MatchString(derivation.CurrentStage) {
			tokenIndexes = append(tokenIndexes, uint8(len(derivedAccounts)-mandatoryAccountsLen)) //nolint:gosec // Limit on the number of token transfers prevents token index from exceeding uint8 max
		}

		// Convert CCIP metas to Solana metas and append to list
		derivedAccounts = append(derivedAccounts, ConvertToSolanaAccountMetas(derivation.AccountsToSave)...)
		// Convert CCIP metas to Solana metas and override previous list. Past ask again accounts are irrelevant.
		accountsToAskWith = ConvertToSolanaAccountMetas(derivation.AskAgainWith)

		lookupTablesAddrs = append(lookupTablesAddrs, derivation.LookUpTablesToSave...)

		stage = derivation.NextStage
		if stage == "" && len(accountsToAskWith) > 0 {
			return nil, nil, nil, fmt.Errorf("account derivation returned %d accounts for next stage but next stage string is empty", len(accountsToAskWith))
		}

		if len(accountsToAskWith) == 0 {
			if stage != "" {
				return nil, nil, nil, fmt.Errorf("account derivation returned 0 accounts for next stage but next stage string is %s", stage)
			}
			lookupTableMap, err := fetchLookupTables(ctx, client, lookupTablesAddrs)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("failed to fetch lookup tables: %w", err)
			}
			return derivedAccounts, lookupTableMap, tokenIndexes, nil
		}
	}
}

func fetchLookupTables(ctx context.Context, client client.MultiClient, lookupTablesAddrs []solana.PublicKey) (map[solana.PublicKey]solana.PublicKeySlice, error) {
	lookupTableMap := make(map[solana.PublicKey]solana.PublicKeySlice)
	for _, addr := range lookupTablesAddrs {
		lookupTableContent, err := getLookupTableAddresses(ctx, client, addr)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch lookup table contents for address %s: %w", addr.String(), err)
		}
		lookupTableMap[addr] = lookupTableContent
	}
	return lookupTableMap, nil
}

func ConvertToCCIPAccountMetas(metas solana.AccountMetaSlice) []ccip_offramp.CcipAccountMeta {
	if len(metas) == 0 {
		return nil
	}
	ccipMetas := make([]ccip_offramp.CcipAccountMeta, 0, len(metas))
	for _, account := range metas {
		ccipMetas = append(ccipMetas, ccip_offramp.CcipAccountMeta{
			Pubkey:     account.PublicKey,
			IsSigner:   account.IsSigner,
			IsWritable: account.IsWritable,
		})
	}
	return ccipMetas
}

func ConvertToSolanaAccountMetas(metas []ccip_offramp.CcipAccountMeta) solana.AccountMetaSlice {
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

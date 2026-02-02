package chainaccessor

import (
	"context"
	"encoding/binary"
	"fmt"
	"slices"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"golang.org/x/exp/maps"

	"github.com/smartcontractkit/chainlink-common/pkg/types/ccip/consts"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"

	offramp "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/ccip_offramp"
	router "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/ccip_router"
	feequoter "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/fee_quoter"
	rmnremote "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/rmn_remote"
)

// https://github.com/smartcontractkit/chainlink-ccip/blob/7cae1b8434dd376eb70f2ddaace43093982f3a57/chains/solana/contracts/programs/rmn-remote/src/state.rs#L20-L27
var globalCurseValue = []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

// getOffRampConfig retrieves static, dynamic, commitOCR3, and execOCR3 configurations for the off-ramp contract
func (a *SolanaAccessor) getOffRampConfig(ctx context.Context) (ccipocr3.OfframpConfig, error) {
	// Validate offramp binding exists
	_, err := a.pdaCache.getBinding(consts.ContractNameOffRamp)
	if err != nil {
		return ccipocr3.OfframpConfig{}, fmt.Errorf("failed to get binding for offramp: %w", err)
	}

	config, err := a.getOfframpConfig(ctx)
	if err != nil {
		return ccipocr3.OfframpConfig{}, err
	}

	offrampRefAddress, err := a.getOfframpReferenceAddresses(ctx)
	if err != nil {
		return ccipocr3.OfframpConfig{}, err
	}

	staticConfig := ccipocr3.OffRampStaticChainConfig{
		ChainSelector:        ccipocr3.ChainSelector(config.SvmChainSelector),
		GasForCallExactCheck: 0,
		RmnRemote:            offrampRefAddress.RmnRemote.Bytes(),
		TokenAdminRegistry:   []byte{},                         // PDA dependent on the token address
		NonceManager:         offrampRefAddress.Router.Bytes(), // Nonces are PDAs on the Router contract
	}

	dynamicConfig := ccipocr3.OffRampDynamicChainConfig{
		FeeQuoter:                               offrampRefAddress.FeeQuoter.Bytes(),
		PermissionLessExecutionThresholdSeconds: uint32(config.EnableManualExecutionAfter), // nolint:gosec // G115: value validated to be within uint32 max above
		IsRMNVerificationDisabled:               true,
		MessageInterceptor:                      []byte{}, // expected to be empty for solana
	}

	commitConfig := config.Ocr3[0]
	commitOCR3Config := ccipocr3.OCRConfigResponse{
		OCRConfig: ccipocr3.OCRConfig{
			ConfigInfo: ccipocr3.ConfigInfo{
				ConfigDigest:                   commitConfig.ConfigInfo.ConfigDigest,
				F:                              commitConfig.ConfigInfo.F,
				N:                              commitConfig.ConfigInfo.N,
				IsSignatureVerificationEnabled: commitConfig.ConfigInfo.IsSignatureVerificationEnabled == 1,
			},
			Signers:      convertSignersType(commitConfig.Signers, commitConfig.ConfigInfo.N),
			Transmitters: convertTransmittersType(commitConfig.Transmitters, commitConfig.ConfigInfo.N),
		},
	}

	executeConfig := config.Ocr3[1]
	executeOCR3Config := ccipocr3.OCRConfigResponse{
		OCRConfig: ccipocr3.OCRConfig{
			ConfigInfo: ccipocr3.ConfigInfo{
				ConfigDigest:                   executeConfig.ConfigInfo.ConfigDigest,
				F:                              executeConfig.ConfigInfo.F,
				N:                              executeConfig.ConfigInfo.N,
				IsSignatureVerificationEnabled: executeConfig.ConfigInfo.IsSignatureVerificationEnabled == 1,
			},
			Signers:      convertSignersType(executeConfig.Signers, executeConfig.ConfigInfo.N),
			Transmitters: convertTransmittersType(executeConfig.Transmitters, executeConfig.ConfigInfo.N),
		},
	}

	return ccipocr3.OfframpConfig{
		CommitLatestOCRConfig: commitOCR3Config,
		ExecLatestOCRConfig:   executeOCR3Config,
		StaticConfig:          staticConfig,
		DynamicConfig:         dynamicConfig,
	}, nil
}

func convertSignersType(signers [16][20]uint8, n uint8) [][]byte {
	newSigners := make([][]byte, 0, n)
	for i := range n {
		newSigners = append(newSigners, signers[i][:])
	}
	return newSigners
}

func convertTransmittersType(transmitters [16][32]uint8, n uint8) [][]byte {
	newTransmitters := make([][]byte, 0, n)
	for i := range n {
		newTransmitters = append(newTransmitters, transmitters[i][:])
	}
	return newTransmitters
}

// getOffRampSourceChainConfigs retrieves source chain configurations from the off-ramp contract
func (a *SolanaAccessor) getOffRampSourceChainConfigs(ctx context.Context, sourceChainSelectors []ccipocr3.ChainSelector) (map[ccipocr3.ChainSelector]ccipocr3.SourceChainConfig, error) {
	offrampAddr, err := a.pdaCache.getBinding(consts.ContractNameOffRamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get binding for offramp: %w", err)
	}

	offrampRefAddress, err := a.getOfframpReferenceAddresses(ctx)
	if err != nil {
		return nil, err
	}

	pdaSelectorMap := make(map[solana.PublicKey]ccipocr3.ChainSelector)
	for _, selector := range sourceChainSelectors {
		sourceChainPDA, pdaErr := a.pdaCache.offrampSourceChain(uint64(selector), offrampAddr)
		if pdaErr != nil {
			return nil, fmt.Errorf("failed to calculate offramp source chain config PDA: %w", pdaErr)
		}
		pdaSelectorMap[sourceChainPDA] = selector
	}

	batches := batchPDAs(maps.Keys(pdaSelectorMap))

	sourceChainConfigs := make(map[ccipocr3.ChainSelector]ccipocr3.SourceChainConfig, len(sourceChainSelectors))
	for _, batch := range batches {
		result, err := a.client.GetMultipleAccountsWithOpts(ctx, batch, &rpc.GetMultipleAccountsOpts{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch source chain configs: %w", err)
		}

		if len(batch) != len(result.Value) {
			return nil, fmt.Errorf("source chain configs results contain unexpected number of accounts: %d, expected %d", len(result.Value), len(batch))
		}

		for i, account := range result.Value {
			selector := pdaSelectorMap[batch[i]]
			// Account not found, return disabled source chain config for selector
			if account == nil {
				sourceChainConfigs[selector] = ccipocr3.SourceChainConfig{
					IsEnabled: false,
				}
				continue
			}
			var sourceChain offramp.SourceChain
			decodeErr := bin.NewBorshDecoder(account.Data.GetBinary()).Decode(&sourceChain)
			if decodeErr != nil {
				a.lggr.Errorw("failed to decode source chain config", "selector", selector, "offrampAddr", offrampAddr.String(), "error", decodeErr)
				continue
			}
			// Extra bytes are padded on the right. Trim extra bytes before setting source chain config
			onRampBytes := sourceChain.Config.OnRamp.Bytes[:sourceChain.Config.OnRamp.Len]
			sourceChainConfigs[selector] = ccipocr3.SourceChainConfig{
				Router:                    offrampRefAddress.Router.Bytes(),
				IsEnabled:                 sourceChain.Config.IsEnabled,
				IsRMNVerificationDisabled: true, // Always disabled for Solana
				MinSeqNr:                  sourceChain.State.MinSeqNr,
				OnRamp:                    ccipocr3.UnknownAddress(onRampBytes),
			}
		}
	}

	a.lggr.Debugw("getOffRampSourceChainConfigs results", "sourceChainConfigs", sourceChainConfigs)
	return sourceChainConfigs, nil
}

// getFeeQuoterStaticConfig retrieves static configuration from the fee quoter contract
func (a *SolanaAccessor) getFeeQuoterStaticConfig(ctx context.Context) (ccipocr3.FeeQuoterStaticConfig, error) {
	// Validate fee quoter binding exists
	_, err := a.pdaCache.getBinding(consts.ContractNameFeeQuoter)
	if err != nil {
		return ccipocr3.FeeQuoterStaticConfig{}, fmt.Errorf("failed to get binding for fee quoter: %w", err)
	}
	configPDA := a.pdaCache.feeQuoterConfig()

	var cfg feequoter.Config
	err = a.client.GetAccountDataBorshInto(ctx, configPDA, &cfg)
	if err != nil {
		return ccipocr3.FeeQuoterStaticConfig{}, fmt.Errorf("failed to get fee quoter config account: %w", err)
	}
	return ccipocr3.FeeQuoterStaticConfig{
		MaxFeeJuelsPerMsg: ccipocr3.NewBigInt(cfg.MaxFeeJuelsPerMsg.BigInt()),
		LinkToken:         cfg.LinkTokenMint.Bytes(),
		// Value not used for Solana, defaulting to 0
		StalenessThreshold: 0,
	}, nil
}

// getOnRampDynamicConfig retrieves dynamic configuration from the on-ramp contract
func (a *SolanaAccessor) getOnRampDynamicConfig(ctx context.Context) (ccipocr3.OnRampDynamicConfig, error) {
	// Validate router binding exists
	_, err := a.pdaCache.getBinding(consts.ContractNameOnRamp)
	if err != nil {
		return ccipocr3.OnRampDynamicConfig{}, fmt.Errorf("failed to get binding for onramp: %w", err)
	}
	configPDA := a.pdaCache.routerConfig()

	var cfg router.Config
	err = a.client.GetAccountDataBorshInto(ctx, configPDA, &cfg)
	if err != nil {
		return ccipocr3.OnRampDynamicConfig{}, fmt.Errorf("failed to get onramp config account: %w", err)
	}
	return ccipocr3.OnRampDynamicConfig{
		FeeQuoter:              cfg.FeeQuoter.Bytes(),
		ReentrancyGuardEntered: false,
		MessageInterceptor:     []byte{}, // expected to be empty for Solana
		FeeAggregator:          cfg.FeeAggregator.Bytes(),
		AllowListAdmin:         cfg.Owner.Bytes(),
	}, nil
}

// getOnRampDestChainConfig retrieves destination chain configuration from the on-ramp contract
func (a *SolanaAccessor) getOnRampDestChainConfig(ctx context.Context, dest ccipocr3.ChainSelector) (ccipocr3.OnRampDestChainConfig, error) {
	routerAddr, err := a.pdaCache.getBinding(consts.ContractNameOnRamp)
	if err != nil {
		return ccipocr3.OnRampDestChainConfig{}, fmt.Errorf("failed to get binding for onramp: %w", err)
	}

	destChainStatePDA, err := a.pdaCache.routerDestChain(uint64(dest), routerAddr)
	if err != nil {
		return ccipocr3.OnRampDestChainConfig{}, fmt.Errorf("failed to fetch dest chain state PDA from cache: %w", err)
	}

	var destChain router.DestChain
	err = a.client.GetAccountDataBorshInto(ctx, destChainStatePDA, &destChain)
	if err != nil {
		return ccipocr3.OnRampDestChainConfig{}, fmt.Errorf("failed to get onramp destination chain config account: %w", err)
	}

	return ccipocr3.OnRampDestChainConfig{
		SequenceNumber:   destChain.State.SequenceNumber,
		AllowListEnabled: destChain.Config.AllowListEnabled,
		Router:           routerAddr.Bytes(),
	}, nil
}

// getCurseInfo retrieves curse information for RMN verification
func (a *SolanaAccessor) getCurseInfo(ctx context.Context, destChainSelector ccipocr3.ChainSelector) (ccipocr3.CurseInfo, error) {
	// Validate the RMN Remote contract binding exists
	_, err := a.pdaCache.getBinding(consts.ContractNameRMNRemote)
	if err != nil {
		return ccipocr3.CurseInfo{}, fmt.Errorf("failed to get binding for router: %w", err)
	}
	cursePDA := a.pdaCache.rmnRemoteCurse()

	var curses rmnremote.Curses
	err = a.client.GetAccountDataBorshInto(ctx, cursePDA, &curses)
	if err != nil {
		return ccipocr3.CurseInfo{}, fmt.Errorf("failed to get rmn remote curses account: %w", err)
	}

	cursedChains := make(map[ccipocr3.ChainSelector]bool)
	globalCurse := false
	destinationCurse := false
	for _, curse := range curses.CursedSubjects {
		if slices.Equal(curse.Value[:], globalCurseValue) {
			globalCurse = true
			continue
		}
		chainSel := binary.LittleEndian.Uint64(curse.Value[:])
		if chainSel == uint64(destChainSelector) {
			destinationCurse = true
			continue
		}
		cursedChains[ccipocr3.ChainSelector(chainSel)] = true
	}

	return ccipocr3.CurseInfo{
		CursedSourceChains: cursedChains,
		CursedDestination:  destinationCurse,
		GlobalCurse:        globalCurse,
	}, nil
}

func (a *SolanaAccessor) getOfframpConfig(ctx context.Context) (offramp.Config, error) {
	configPDA := a.pdaCache.offampConfigPDA()

	var config offramp.Config
	err := a.client.GetAccountDataBorshInto(ctx, configPDA, &config)
	if err != nil {
		return offramp.Config{}, fmt.Errorf("failed to get offramp reference addresses account: %w", err)
	}
	return config, nil
}

func (a *SolanaAccessor) getOfframpReferenceAddresses(ctx context.Context) (offramp.ReferenceAddresses, error) {
	refAddrPDA := a.pdaCache.offrampRefAddresses()

	var refAddreses offramp.ReferenceAddresses
	err := a.client.GetAccountDataBorshInto(ctx, refAddrPDA, &refAddreses)
	if err != nil {
		return offramp.ReferenceAddresses{}, fmt.Errorf("failed to get offramp reference addresses account: %w", err)
	}

	return refAddreses, nil
}

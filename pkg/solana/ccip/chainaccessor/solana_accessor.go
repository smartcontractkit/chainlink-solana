package chainaccessor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"

	offramp "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/ccip_offramp"
	router "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/ccip_router"
	feequoter "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/state"
	ccipchainaccessor "github.com/smartcontractkit/chainlink-ccip/pkg/chainaccessor"
	"github.com/smartcontractkit/chainlink-ccip/pkg/consts"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

var ErrNoBindings = errors.New("no bindings found")

type AccessorLogPoller interface {
	Ready() error
	HasFilter(context.Context, string) bool
	RegisterFilter(context.Context, logpollertypes.Filter) error
	FilteredLogs(context.Context, []query.Expression, query.LimitAndSort, string) ([]logpollertypes.Log, error)
}

type SolanaAccessor struct {
	lggr          logger.Logger
	chainSelector ccipocr3.ChainSelector
	client        client.MultiClient
	logPoller     AccessorLogPoller
	// Note: we might need to update this in the future to map[string][]address.Address
	// to support multi-bind addresses for the price aggregator contract: smartcontractkit/chainlink-ccip@main/pkg/contractreader/extended.go#L77-L79
	bindings   map[string]solana.PublicKey
	bindingsMu sync.RWMutex
	addrCodec  ccipocr3.ChainSpecificAddressCodec
	fee        fees.Estimator
	// Track relevant PDAs in a cache to avoid having to recalculate them every method call
	// Only need to be recalculated on calls to Sync
	pdaCache pdaCache
}

var _ ccipocr3.ChainAccessor = (*SolanaAccessor)(nil)

func NewSolanaAccessor(
	l logger.Logger,
	chainSelector ccipocr3.ChainSelector,
	client client.MultiClient,
	logPoller AccessorLogPoller,
	fee fees.Estimator,
	addrCodec ccipocr3.ChainSpecificAddressCodec,
) (*SolanaAccessor, error) {
	lggr := logger.Named(l, "SolanaAccessor")

	if err := logPoller.Ready(); err != nil {
		return nil, fmt.Errorf("log poller not ready: %w", err)
	}

	return &SolanaAccessor{
		lggr:          lggr,
		chainSelector: chainSelector,
		client:        client,
		logPoller:     logPoller,
		bindings:      make(map[string]solana.PublicKey),
		bindingsMu:    sync.RWMutex{},
		fee:           fee,
		addrCodec:     addrCodec,
		pdaCache:      newPDACache(),
	}, nil
}

// Common Accessor methods
func (a *SolanaAccessor) GetContractAddress(contractName string) ([]byte, error) {
	addr, err := a.getBinding(contractName)
	if err != nil {
		return nil, err
	}
	return addr.Bytes(), nil
}

func (a *SolanaAccessor) GetAllConfigsLegacy(ctx context.Context, destChainSelector ccipocr3.ChainSelector, sourceChainSelectors []ccipocr3.ChainSelector) (ccipocr3.ChainConfigSnapshot, map[ccipocr3.ChainSelector]ccipocr3.SourceChainConfig, error) {
	// Match old behaviour: if a contract isn't bound, we return an empty value so the nodes can achieve consensus on partial config
	// https://github.com/smartcontractkit/chainlink-ccip/blob/a8dbbdbf14a07593de2f0dbe608f8b64d893a6bd/pkg/contractreader/extended.go#L226-L231

	// TODO: pass in addresses we fetched so subsequent fetches don't fail (offramp->feeQuoter etc)

	var config ccipocr3.ChainConfigSnapshot
	var sourceChainConfigs map[ccipocr3.ChainSelector]ccipocr3.SourceChainConfig

	if a.chainSelector == destChainSelector {
		// we're fetching config on the destination chain (offramp + fee quoter static config + RMN)

		// OffRamp
		offrampConfig, err := a.getOffRampConfig(ctx)
		if !errors.Is(err, ErrNoBindings) && err != nil {
			return ccipocr3.ChainConfigSnapshot{}, nil, fmt.Errorf("failed to get current offramp static config: %w", err)
		}
		config.Offramp = offrampConfig

		// FeeQuoter
		feeQuoterStaticConfig, err := a.getFeeQuoterStaticConfig(ctx)
		if !errors.Is(err, ErrNoBindings) && err != nil {
			return ccipocr3.ChainConfigSnapshot{}, nil, fmt.Errorf("failed to get current feequoter static config: %w", err)
		}
		config.FeeQuoter = ccipocr3.FeeQuoterConfig{
			StaticConfig: feeQuoterStaticConfig,
		}

		rmnRemoteProxyAddr, err := a.getBinding(consts.ContractNameRMNProxy)
		if !errors.Is(err, ErrNoBindings) && err != nil {
			return ccipocr3.ChainConfigSnapshot{}, nil, fmt.Errorf("failed to get binding for rmn remote proxy: %w", err)
		}

		// RMN
		// TODO: RMNProxy should be an implementation detail hidden behind chainAccessor
		config.RMNProxy = ccipocr3.RMNProxyConfig{
			// TODO: point at a rmnremote address/router/offramp to allow fetching curseinfo
			// There is no proxy for Solana so is it right to just set the "proxy" address as the remote address here?
			RemoteAddress: rmnRemoteProxyAddr.Bytes(),
		}
		config.RMNRemote = ccipocr3.RMNRemoteConfig{
			// We don't support RMN so return an empty config
		}

		// CurseInfo
		curseInfo, err := a.getCurseInfo(ctx)
		if !errors.Is(err, ErrNoBindings) && err != nil {
			return ccipocr3.ChainConfigSnapshot{}, nil, fmt.Errorf("failed to get curse info: %w", err)
		}
		config.CurseInfo = curseInfo

		sourceChainConfigs, err = a.getOffRampSourceChainConfigs(ctx, sourceChainSelectors)
		if !errors.Is(err, ErrNoBindings) && err != nil {
			return ccipocr3.ChainConfigSnapshot{}, nil, fmt.Errorf("failed to get source chain configs: %w", err)
		}
	} else {
		// we're fetching config on the source chain (onramp + router config)

		// OnRamp
		routerDynamicConfig, err := a.getOnRampDynamicConfig(ctx)
		if !errors.Is(err, ErrNoBindings) && err != nil {
			return ccipocr3.ChainConfigSnapshot{}, nil, fmt.Errorf("failed to get current onramp dynamic config: %w", err)
		}
		onRampDestChainConfig, err := a.getOnRampDestChainConfig(ctx, destChainSelector)
		if !errors.Is(err, ErrNoBindings) && err != nil {
			return ccipocr3.ChainConfigSnapshot{}, nil, fmt.Errorf("failed to get current onramp dest chain config: %w", err)
		}
		config.OnRamp = ccipocr3.OnRampConfig{
			DynamicConfig:   ccipocr3.GetOnRampDynamicConfigResponse{DynamicConfig: routerDynamicConfig},
			DestChainConfig: onRampDestChainConfig,
		}

		// Router
		config.Router = ccipocr3.RouterConfig{
			WrappedNativeAddress: solana.WrappedSol.Bytes(),
		}

		// sourceChainConfigs represents sources on the *destination chain* contract, since this is the source chain
		// we'll return an empty map
		sourceChainConfigs = make(map[ccipocr3.ChainSelector]ccipocr3.SourceChainConfig, 0)
	}
	a.lggr.Debugw("GetAllConfigsLegacy", "accessorChainSelector", a.chainSelector, "destChainSelector", destChainSelector, "sourceChainSelectors", sourceChainSelectors, "config", config, "sourceChainConfigs", sourceChainConfigs)
	return config, sourceChainConfigs, nil
}

func (a *SolanaAccessor) GetChainFeeComponents(ctx context.Context) (ccipocr3.ChainFeeComponents, error) {
	if a.fee == nil {
		return ccipocr3.ChainFeeComponents{}, fmt.Errorf("gas estimator not available")
	}

	fee := a.fee.BaseComputeUnitPrice()
	return ccipocr3.ChainFeeComponents{
		ExecutionFee:        new(big.Int).SetUint64(fee),
		DataAvailabilityFee: big.NewInt(0), // required field so return 0 instead of nil
	}, nil
}

// Matching CCIP Plugins - default accessor w/ CR behavior
// CCIP contract discovery follows the same two-phase approach for Solana:
// 1. Initial binding: Offramp address registered at startup (chainlink-ccip/pkg/reader/ccip.go:113-118)
// 2. Dynamic discovery: Onramp addresses discovered from offramp.SourceChainConfig (ccip.go:644-656)
//
// - Solana Accessor: Bypasses CR entirely - implements ChainAccessor interface directly
//   - Sync() directly calls bindContractEvent() to register event filters with Solana logPoller
//   - Both expose same Sync() interface to CCIPChainReader
func (a *SolanaAccessor) Sync(ctx context.Context, contractName string, contractAddress ccipocr3.UnknownAddress) error {
	// TODO: Add method to address codec to convert bytes to solana pub key and use here
	if len(contractAddress) != solana.PublicKeyLength {
		return fmt.Errorf("address is unexpected length to be solana public key %d, expect %d", len(contractAddress), solana.PublicKeyLength)
	}
	addr := solana.PublicKeyFromBytes(contractAddress)

	if err := a.bindContractEvent(ctx, contractName, addr); err != nil {
		return fmt.Errorf("failed to bind contract event: %w", err)
	}
	a.bindingsMu.Lock()
	defer a.bindingsMu.Unlock()
	a.bindings[contractName] = addr
	return a.pdaCache.updateCache(contractName, addr)
}

// Solana as source chain methods
func (a *SolanaAccessor) MsgsBetweenSeqNums(ctx context.Context, dest ccipocr3.ChainSelector, seqNumRange ccipocr3.SeqNumRange) ([]ccipocr3.Message, error) {
	onrampAddr, err := a.getBinding(consts.ContractNameOnRamp)
	if err != nil {
		return nil, fmt.Errorf("OnRamp not bound: %w", err)
	}

	expressions := []query.Expression{
		logpoller.NewAddressFilter(onrampAddr),
		logpoller.NewEventSigFilter(logpollertypes.NewEventSignatureFromName(consts.EventNameCCIPMessageSent)),
		query.Comparator(consts.EventAttributeDestChain, primitives.ValueComparator{
			Value:    dest,
			Operator: primitives.Eq,
		}),
		query.Comparator(consts.EventAttributeSequenceNumber, primitives.ValueComparator{
			Value:    seqNumRange.Start(),
			Operator: primitives.Gte,
		}, primitives.ValueComparator{
			Value:    seqNumRange.End(),
			Operator: primitives.Lte,
		}),
		query.Confidence(primitives.Finalized),
	}

	limitSort := query.LimitAndSort{
		SortBy: []query.SortBy{
			query.NewSortBySequence(query.Asc),
		},
		Limit: query.Limit{
			Count: uint64(seqNumRange.End() - seqNumRange.Start() + 1),
		},
	}

	// query Solana logs
	logs, err := a.logPoller.FilteredLogs(ctx, expressions, limitSort, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch filtered logs from log poller: %w", err)
	}

	a.lggr.Infow("queried MsgsBetweenSeqNums",
		"numMsgs", len(logs),
		"sourceChainSelector", a.chainSelector,
		"destinationChainSelector", dest,
		"seqNumRange", seqNumRange.String(),
	)

	events, err := a.convertCCIPMessageSent(logs, onrampAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert solana message sent event to generic CCIP type: %w", err)
	}

	msgs := make([]ccipocr3.Message, 0)
	for _, event := range events {
		// validate event
		if err := ccipchainaccessor.ValidateSendRequestedEvent(event, a.chainSelector, dest, seqNumRange); err != nil {
			a.lggr.Errorw("validate send requested event", "err", err, "message", event)
			continue
		}
		msgs = append(msgs, event.Message)
	}
	return msgs, nil
}

func (a *SolanaAccessor) LatestMessageTo(ctx context.Context, dest ccipocr3.ChainSelector) (ccipocr3.SeqNum, error) {
	onrampAddr, err := a.getBinding(consts.ContractNameOnRamp)
	if err != nil {
		return 0, fmt.Errorf("OnRamp not bound: %w", err)
	}

	expressions := []query.Expression{
		logpoller.NewAddressFilter(onrampAddr),
		logpoller.NewEventSigFilter(logpollertypes.NewEventSignatureFromName(consts.EventNameCCIPMessageSent)),
		query.Comparator(consts.EventAttributeDestChain, primitives.ValueComparator{
			Value:    dest,
			Operator: primitives.Eq,
		}),
		query.Confidence(primitives.Finalized),
	}

	limitSort := query.LimitAndSort{
		SortBy: []query.SortBy{
			query.NewSortBySequence(query.Desc),
		},
		Limit: query.Limit{Count: 1},
	}

	// query solana logs
	logs, err := a.logPoller.FilteredLogs(ctx, expressions, limitSort, "")
	if err != nil {
		return 0, fmt.Errorf("failed to fetch logs from log poller: %w", err)
	}

	a.lggr.Infow("queried LatestMessageTo",
		"numMsgs", len(logs),
		"sourceChainSelector", a.chainSelector,
		"destinationChainSelector", dest,
	)

	if len(logs) > 1 {
		return 0, fmt.Errorf("more than one message found for the latest message query, found: %d", len(logs))
	}
	if len(logs) == 0 {
		return 0, nil
	}

	// convert logs to generic CCIP events
	events, err := a.convertCCIPMessageSent(logs, onrampAddr)
	if err != nil {
		return 0, fmt.Errorf("failed to convert solana message sent event to generic CCIP type: %w", err)
	}

	if len(events) == 0 {
		return 0, errors.New("expected single event for log")
	}
	event := events[0]

	// validate event
	if err := ccipchainaccessor.ValidateSendRequestedEvent(event, a.chainSelector, dest, ccipocr3.NewSeqNumRange(event.Message.Header.SequenceNumber, event.Message.Header.SequenceNumber)); err != nil {
		a.lggr.Errorw("send requested event validation failed", "err", err, "message", event)
		return 0, fmt.Errorf("message invalid msg %v: %w", event, err)
	}

	return event.SequenceNumber, nil
}

func (a *SolanaAccessor) getBinding(contractName string) (solana.PublicKey, error) {
	a.bindingsMu.RLock()
	defer a.bindingsMu.RUnlock()
	addr, exists := a.bindings[contractName]
	if !exists {
		return solana.PublicKey{}, ErrNoBindings
	}
	return addr, nil
}

func (a *SolanaAccessor) GetExpectedNextSequenceNumber(ctx context.Context, dest ccipocr3.ChainSelector) (ccipocr3.SeqNum, error) {
	onRampConfig, err := a.getOnRampDestChainConfig(ctx, dest)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch on ramp dest chain config account: %w", err)
	}

	return ccipocr3.SeqNum(onRampConfig.SequenceNumber), nil
}

func (a *SolanaAccessor) GetTokenPriceUSD(ctx context.Context, rawTokenAddress ccipocr3.UnknownAddress) (ccipocr3.TimestampedUnixBig, error) {
	feeQuoterAddr, err := a.getBinding(consts.ContractNameFeeQuoter)
	if err != nil {
		return ccipocr3.TimestampedUnixBig{}, err
	}

	if len(rawTokenAddress) != solana.PublicKeyLength {
		return ccipocr3.TimestampedUnixBig{}, fmt.Errorf("raw token address is unexpected length to be solana public key %d, expect %d", len(rawTokenAddress), solana.PublicKeyLength)
	}
	tokenAddress := solana.PublicKeyFromBytes(rawTokenAddress)

	tokenConfigPDA, err := a.pdaCache.feeQuoterBillingTokenConfig(tokenAddress, feeQuoterAddr)
	if err != nil {
		return ccipocr3.TimestampedUnixBig{}, fmt.Errorf("failed to fetch fee quoter billing token config PDA from cache: %w", err)
	}

	var billingTokenConfig feequoter.BillingTokenConfig
	err = a.client.GetAccountDataBorshInto(ctx, tokenConfigPDA, &billingTokenConfig)
	if err != nil {
		return ccipocr3.TimestampedUnixBig{}, fmt.Errorf("failed to get fee quoter billing token config account: %w", err)
	}
	value := new(big.Int).SetBytes(billingTokenConfig.UsdPerToken.Value[:])
	if billingTokenConfig.UsdPerToken.Timestamp > math.MaxUint32 {
		return ccipocr3.TimestampedUnixBig{}, fmt.Errorf("billing token config timestamp exceeds uint32 max: %d", billingTokenConfig.UsdPerToken.Timestamp)
	}
	return ccipocr3.TimestampedUnixBig{
		Value: value,
		// TODO: u64 -> u32? should we fix the onchain type?
		Timestamp: uint32(billingTokenConfig.UsdPerToken.Timestamp), //nolint:gosec // G115: validated to be within uint32 max above
	}, nil
}

func (a *SolanaAccessor) GetFeeQuoterDestChainConfig(ctx context.Context, dest ccipocr3.ChainSelector) (ccipocr3.FeeQuoterDestChainConfig, error) {
	feeQuoterAddr, err := a.getBinding(consts.ContractNameFeeQuoter)
	if err != nil {
		return ccipocr3.FeeQuoterDestChainConfig{}, err
	}

	fqDestChainPDA, err := a.pdaCache.feeQuoterDestChain(uint64(dest), feeQuoterAddr)
	if err != nil {
		return ccipocr3.FeeQuoterDestChainConfig{}, fmt.Errorf("failed to fethc fee quoter dest chain PDA from cache: %w", err)
	}

	var destChain feequoter.DestChain
	err = a.client.GetAccountDataBorshInto(ctx, fqDestChainPDA, &destChain)
	if err != nil {
		return ccipocr3.FeeQuoterDestChainConfig{}, fmt.Errorf("failed to get fee quoter dest chain account: %w", err)
	}

	return ccipocr3.FeeQuoterDestChainConfig{
		IsEnabled:                         destChain.Config.IsEnabled,
		MaxNumberOfTokensPerMsg:           destChain.Config.MaxNumberOfTokensPerMsg,
		MaxDataBytes:                      destChain.Config.MaxDataBytes,
		MaxPerMsgGasLimit:                 destChain.Config.MaxPerMsgGasLimit,
		DestGasOverhead:                   destChain.Config.DestGasOverhead,
		DestGasPerPayloadByteBase:         destChain.Config.DestGasPerPayloadByteBase,
		DestGasPerPayloadByteHigh:         destChain.Config.DestGasPerPayloadByteHigh,
		DestGasPerPayloadByteThreshold:    destChain.Config.DestGasPerPayloadByteThreshold,
		DestDataAvailabilityOverheadGas:   destChain.Config.DestDataAvailabilityOverheadGas,
		DestGasPerDataAvailabilityByte:    destChain.Config.DestGasPerDataAvailabilityByte,
		DestDataAvailabilityMultiplierBps: destChain.Config.DestDataAvailabilityMultiplierBps,
		DefaultTokenFeeUSDCents:           destChain.Config.DefaultTokenFeeUsdcents,
		DefaultTokenDestGasOverhead:       destChain.Config.DefaultTokenDestGasOverhead,
		DefaultTxGasLimit:                 destChain.Config.DefaultTxGasLimit,
		GasMultiplierWeiPerEth:            destChain.Config.GasMultiplierWeiPerEth,
		NetworkFeeUSDCents:                destChain.Config.NetworkFeeUsdcents,
		GasPriceStalenessThreshold:        destChain.Config.GasPriceStalenessThreshold,
		EnforceOutOfOrder:                 destChain.Config.EnforceOutOfOrder,
		ChainFamilySelector:               destChain.Config.ChainFamilySelector,
	}, nil
}

// Solana as destination chain methods
func (a *SolanaAccessor) CommitReportsGTETimestamp(ctx context.Context, ts time.Time, _ primitives.ConfidenceLevel, limit int) ([]ccipocr3.CommitPluginReportWithMeta, error) {
	offrampAddr, err := a.getBinding(consts.ContractNameOffRamp)
	if err != nil {
		return nil, fmt.Errorf("offramp not bound: %w", err)
	}

	// TODO: Add a way to filter reports to only return ones with merkle roots present. Roots can be nil for price only updates.
	// Either add it to the LogPoller query itself or filter it in-memory within processCommitReports
	expressions := []query.Expression{
		logpoller.NewAddressFilter(offrampAddr),
		logpoller.NewEventSigFilter(logpollertypes.NewEventSignatureFromName(consts.EventNameCommitReportAccepted)),
		query.Timestamp(uint64(ts.Unix()), primitives.Gte), // nolint:gosec // G115: timestamp is always positive
		query.Confidence(primitives.Finalized),             // solana log poller only operates with finalized confidence
	}

	internalLimit := limit * 2
	limitSort := query.LimitAndSort{
		SortBy: []query.SortBy{query.NewSortBySequence(query.Asc)},
		Limit: query.Limit{
			Count: uint64(internalLimit), // nolint:gosec // G115: limit can never reasonably exceed uint64 max
		},
	}

	// query solana logs
	logs, err := a.logPoller.FilteredLogs(ctx, expressions, limitSort, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch commit report accepted logs from log poller: %w", err)
	}

	a.lggr.Debugw("queried CommitReportsGTETimestamp", "numReports", len(logs),
		"destinationChainSelector", a.chainSelector,
		"ts", ts,
		"limit", internalLimit)

	// convert event to generic CCIP reports
	return a.processCommitReports(logs, ts, limit)
}

func (a *SolanaAccessor) ExecutedMessages(ctx context.Context, ranges map[ccipocr3.ChainSelector][]ccipocr3.SeqNumRange, confidence primitives.ConfidenceLevel) (map[ccipocr3.ChainSelector][]ccipocr3.SeqNum, error) {
	offrampAddr, err := a.getBinding(consts.ContractNameOffRamp)
	if err != nil {
		return nil, fmt.Errorf("offramp not bound: %w", err)
	}

	// trim empty ranges from rangesPerChain
	// otherwise we may get SQL errors from the chainreader.
	nonEmptyRangesPerChain := make(map[ccipocr3.ChainSelector][]ccipocr3.SeqNumRange)
	for chain, ranges := range ranges {
		if len(ranges) > 0 {
			nonEmptyRangesPerChain[chain] = ranges
		}
	}

	keyFilter, countSqNrs := createExecutedMessagesKeyFilter(nonEmptyRangesPerChain)
	if countSqNrs == 0 {
		a.lggr.Debugw("no sequence numbers to query", "nonEmptyRangesPerChain", nonEmptyRangesPerChain)
		return nil, nil
	}
	limitSort := query.LimitAndSort{
		SortBy: []query.SortBy{query.NewSortBySequence(query.Asc)},
		Limit: query.Limit{
			Count: countSqNrs,
		},
	}

	expressions := []query.Expression{
		logpoller.NewAddressFilter(offrampAddr),
		logpoller.NewEventSigFilter(logpollertypes.NewEventSignatureFromName(consts.EventNameCommitReportAccepted)),
	}
	expressions = append(expressions, keyFilter.Expressions...)

	logs, err := a.logPoller.FilteredLogs(ctx, expressions, limitSort, "")
	if err != nil {
		return nil, fmt.Errorf("failed to query executed message logs from log poller: %w", err)
	}

	return a.processExecutionStateChangesEvents(logs, nonEmptyRangesPerChain)
}

func (a *SolanaAccessor) NextSeqNum(ctx context.Context, sources []ccipocr3.ChainSelector) (seqNum map[ccipocr3.ChainSelector]ccipocr3.SeqNum, err error) {
	// TODO: not needed yet. CCIP reader extracts this info from GetAllConfigsLegacy for now
	// https://github.com/smartcontractkit/chainlink-ccip/blob/7cae1b8434dd376eb70f2ddaace43093982f3a57/pkg/reader/ccip.go#L936
	return nil, errors.New("not implemented")
}

func (a *SolanaAccessor) Nonces(ctx context.Context, addressesMap map[ccipocr3.ChainSelector][]ccipocr3.UnknownEncodedAddress) (map[ccipocr3.ChainSelector]map[string]uint64, error) {
	routerAddr, err := a.getBinding(consts.ContractNameRouter)
	if err != nil {
		return nil, fmt.Errorf("failed to get binding for router: %w", err)
	}

	results := make(map[ccipocr3.ChainSelector]map[string]uint64)
	// TODO: Leverage multi-account read to minimize RPC calls
	for sel, addresses := range addressesMap {
		if _, ok := results[sel]; !ok {
			results[sel] = make(map[string]uint64)
		}
		for _, addrStr := range addresses {
			// Nonce already fetched for address
			if _, ok := results[sel][string(addrStr)]; ok {
				continue
			}

			user, err := solana.PublicKeyFromBase58(string(addrStr))
			if err != nil {
				return nil, fmt.Errorf("failed to parse sender address %s: %w", addrStr, err)
			}
			noncePDA, err := state.FindNoncePDA(uint64(sel), user, routerAddr)
			if err != nil {
				return nil, fmt.Errorf("failed to calculate nonce PDA for selector %d and address %s: %w", sel, addrStr, err)
			}

			var nonce router.Nonce
			err = a.client.GetAccountDataBorshInto(ctx, noncePDA, &nonce)
			if err != nil {
				return nil, fmt.Errorf("failed to get nonce account for selector %d and address %s: %w", sel, addrStr, err)
			}

			results[sel][string(addrStr)] = nonce.OrderedNonce // TODO: Is this supposed to be ordered nonce or total nonce?
		}
	}

	return results, nil
}

func (a *SolanaAccessor) GetChainFeePriceUpdate(ctx context.Context, selectors []ccipocr3.ChainSelector) (map[ccipocr3.ChainSelector]ccipocr3.TimestampedUnixBig, error) {
	feeQuoterAddr, err := a.getBinding(consts.ContractNameFeeQuoter)
	if err != nil {
		return nil, fmt.Errorf("failed to get fee quoter binding: %w", err)
	}

	feePriceUpdates := make(map[ccipocr3.ChainSelector]ccipocr3.TimestampedUnixBig)
	// TODO: Leverage multi-account read to minimize RPC calls
	for _, sel := range selectors {
		destChainPDA, err := a.pdaCache.feeQuoterDestChain(uint64(sel), feeQuoterAddr)
		if err != nil {
			continue
		}

		var destChain feequoter.DestChain
		err = a.client.GetAccountDataBorshInto(ctx, destChainPDA, &destChain)
		// The plugin is built with EVM behaviour in mind: if account is not found the zero value is returned
		if errors.Is(err, rpc.ErrNotFound) {
			feePriceUpdates[sel] = ccipocr3.TimestampedUnixBig{
				Value:     big.NewInt(0),
				Timestamp: 0,
			}
			continue
		}
		if err != nil {
			a.lggr.Errorw("failed to batch get chain fee price updates", "err", err)
			continue
		}

		if destChain.State.UsdPerUnitGas.Timestamp > math.MaxUint32 {
			a.lggr.Errorw("gas price update timestamp exceeeds uint32 max", "timestamp", destChain.State.UsdPerUnitGas.Timestamp)
			continue
		}

		value := new(big.Int).SetBytes(destChain.State.UsdPerUnitGas.Value[:])
		feePriceUpdates[sel] = ccipocr3.TimestampedUnixBig{
			Value:     value,
			Timestamp: uint32(destChain.State.UsdPerUnitGas.Timestamp), //nolint:gosec // timestamp validated to be within uint32 bounds above
		}
	}

	return feePriceUpdates, nil
}

func (a *SolanaAccessor) GetLatestPriceSeqNr(ctx context.Context) (ccipocr3.SeqNum, error) {
	// Validate offramp binding exists
	_, err := a.getBinding(consts.ContractNameOffRamp)
	if err != nil {
		return 0, err
	}
	statePDA := a.pdaCache.offampStatePDA()

	var state offramp.GlobalState
	err = a.client.GetAccountDataBorshInto(ctx, statePDA, &state)
	if err != nil {
		return 0, fmt.Errorf("failed to get offramp reference addresses account: %w", err)
	}

	return ccipocr3.SeqNum(state.LatestPriceSequenceNumber), nil
}

func (a *SolanaAccessor) GetFeeQuoterTokenUpdates(
	ctx context.Context,
	tokens []ccipocr3.UnknownEncodedAddress,
	chain ccipocr3.ChainSelector,
) (map[ccipocr3.UnknownEncodedAddress]ccipocr3.TimestampedUnixBig, error) {
	return nil, fmt.Errorf("not implemented")
}

func (a *SolanaAccessor) GetFeedPricesUSD(
	ctx context.Context,
	tokens []ccipocr3.UnknownEncodedAddress,
	tokenInfoMap map[ccipocr3.UnknownEncodedAddress]ccipocr3.TokenInfo,
) (ccipocr3.TokenPriceMap, error) {
	return nil, fmt.Errorf("not implemented")
}

func (a *SolanaAccessor) MessagesByTokenID(
	ctx context.Context,
	source, dest ccipocr3.ChainSelector,
	tokens map[ccipocr3.MessageTokenID]ccipocr3.RampTokenAmount,
) (map[ccipocr3.MessageTokenID]ccipocr3.Bytes, error) {
	return nil, fmt.Errorf("not implemented")
}
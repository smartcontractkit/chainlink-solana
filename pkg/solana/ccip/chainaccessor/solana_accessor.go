package chainaccessor

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strings"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"golang.org/x/exp/maps"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccip/consts"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"

	offramp "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/ccip_offramp"
	feequoter "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/fee_quoter"
	ccipchainaccessor "github.com/smartcontractkit/chainlink-ccip/pkg/chainaccessor"

	"github.com/smartcontractkit/chainlink-ccip/pkg/contractreader"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

// https://solana.com/docs/rpc/http/getmultipleaccounts
var getMultipleAccountsLimit = 100

type AccessorLogPoller interface {
	Start(ctx context.Context) error
	Ready() error
	HasFilter(context.Context, string) bool
	RegisterFilter(context.Context, logpollertypes.Filter) error
	FilteredLogs(context.Context, []query.Expression, query.LimitAndSort, string) ([]logpollertypes.Log, error)
	CPIEventsEnabled() bool
}

type SolanaAccessor struct {
	lggr          logger.Logger
	chainSelector ccipocr3.ChainSelector
	client        client.MultiClient
	logPoller     AccessorLogPoller
	addrCodec     ccipocr3.ChainSpecificAddressCodec
	fee           fees.Estimator
	// Track relevant PDAs in a cache to avoid having to recalculate them every method call
	// Only need to be recalculated on calls to Sync
	pdaCache pdaCache
}

var _ ccipocr3.ChainAccessor = (*SolanaAccessor)(nil)

func NewSolanaAccessor(
	ctx context.Context,
	l logger.Logger,
	chainSelector ccipocr3.ChainSelector,
	client client.MultiClient,
	logPoller AccessorLogPoller,
	fee fees.Estimator,
	addrCodec ccipocr3.ChainSpecificAddressCodec,
) (*SolanaAccessor, error) {
	lggr := logger.Named(l, "SolanaAccessor")

	if err := logPoller.Ready(); err != nil {
		// Start LogPoller if it hasn't already been
		// Lazily starting it here rather than earlier, since nodes running only ordinary DF jobs don't need it
		err := logPoller.Start(ctx)
		// in case another thread calls Start() after Ready() returns
		if err != nil && !strings.Contains(err.Error(), "has already been started") {
			return nil, fmt.Errorf("failed to start log poller: %w", err)
		}
	}

	return &SolanaAccessor{
		lggr:          lggr,
		chainSelector: chainSelector,
		client:        client,
		logPoller:     logPoller,
		fee:           fee,
		addrCodec:     addrCodec,
		pdaCache:      newPDACache(lggr),
	}, nil
}

// Common Accessor methods
func (a *SolanaAccessor) GetContractAddress(contractName string) ([]byte, error) {
	addr, err := a.pdaCache.getBinding(contractName)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s binding: %w", contractName, err)
	}
	return addr.Bytes(), nil
}

func (a *SolanaAccessor) GetAllConfigsLegacy(ctx context.Context, destChainSelector ccipocr3.ChainSelector, sourceChainSelectors []ccipocr3.ChainSelector) (ccipocr3.ChainConfigSnapshot, map[ccipocr3.ChainSelector]ccipocr3.SourceChainConfig, error) {
	// Match old behaviour: if a contract isn't bound, we return an empty value so the nodes can achieve consensus on partial config
	// https://github.com/smartcontractkit/chainlink-ccip/blob/a8dbbdbf14a07593de2f0dbe608f8b64d893a6bd/pkg/contractreader/extended.go#L226-L231
	var config ccipocr3.ChainConfigSnapshot
	var sourceChainConfigs map[ccipocr3.ChainSelector]ccipocr3.SourceChainConfig

	if a.chainSelector == destChainSelector {
		// we're fetching config on the destination chain (offramp + fee quoter static config + RMN)

		// OffRamp
		offrampConfig, err := a.getOffRampConfig(ctx)
		if !errors.Is(err, contractreader.ErrNoBindings) && err != nil {
			return ccipocr3.ChainConfigSnapshot{}, nil, fmt.Errorf("failed to get current offramp static config: %w", err)
		}
		config.Offramp = offrampConfig

		// FeeQuoter
		feeQuoterStaticConfig, err := a.getFeeQuoterStaticConfig(ctx)
		if !errors.Is(err, contractreader.ErrNoBindings) && err != nil {
			return ccipocr3.ChainConfigSnapshot{}, nil, fmt.Errorf("failed to get current feequoter static config: %w", err)
		}
		config.FeeQuoter = ccipocr3.FeeQuoterConfig{
			StaticConfig: feeQuoterStaticConfig,
		}

		rmnRemoteProxyAddr, err := a.pdaCache.getBinding(consts.ContractNameRMNProxy)
		if !errors.Is(err, contractreader.ErrNoBindings) && err != nil {
			return ccipocr3.ChainConfigSnapshot{}, nil, fmt.Errorf("failed to get binding for rmn remote proxy: %w", err)
		}

		// RMN
		config.RMNProxy = ccipocr3.RMNProxyConfig{
			// TODO: point at a rmnremote address/router/offramp to allow fetching curseinfo
			// There is no proxy for Solana so is it right to just set the "proxy" address as the remote address here?
			RemoteAddress: rmnRemoteProxyAddr.Bytes(),
		}
		config.RMNRemote = ccipocr3.RMNRemoteConfig{
			// We don't support RMN so return an empty config
		}

		// CurseInfo
		curseInfo, err := a.getCurseInfo(ctx, destChainSelector)
		if !errors.Is(err, contractreader.ErrNoBindings) && err != nil {
			return ccipocr3.ChainConfigSnapshot{}, nil, fmt.Errorf("failed to get curse info: %w", err)
		}
		config.CurseInfo = curseInfo

		sourceChainConfigs, err = a.getOffRampSourceChainConfigs(ctx, sourceChainSelectors)
		if !errors.Is(err, contractreader.ErrNoBindings) && err != nil {
			return ccipocr3.ChainConfigSnapshot{}, nil, fmt.Errorf("failed to get source chain configs: %w", err)
		}
	} else {
		// we're fetching config on the source chain (onramp + router config)

		// OnRamp
		routerDynamicConfig, err := a.getOnRampDynamicConfig(ctx)
		if !errors.Is(err, contractreader.ErrNoBindings) && err != nil {
			return ccipocr3.ChainConfigSnapshot{}, nil, fmt.Errorf("failed to get current onramp dynamic config: %w", err)
		}
		onRampDestChainConfig, err := a.getOnRampDestChainConfig(ctx, destChainSelector)
		if !errors.Is(err, contractreader.ErrNoBindings) && err != nil {
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
	a.lggr.Debugw("GetAllConfigsLegacy results", "accessorChainSelector", a.chainSelector, "destChainSelector", destChainSelector, "sourceChainSelectors", sourceChainSelectors, "config", config, "sourceChainConfigs", sourceChainConfigs)
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
	if len(contractAddress) != solana.PublicKeyLength {
		return fmt.Errorf("address is unexpected length to be solana public key %d, expect %d", len(contractAddress), solana.PublicKeyLength)
	}
	addr := solana.PublicKeyFromBytes(contractAddress)

	a.lggr.Debugw("Sync: binding contract", "contract", contractName, "address", addr.String())
	if err := a.pdaCache.updateCache(contractName, addr); err != nil {
		return fmt.Errorf("failed to update pda cache: %w", err)
	}

	if err := a.bindContractEvent(ctx, contractName, addr); err != nil {
		return fmt.Errorf("failed to bind contract event: %w", err)
	}
	return nil
}

// Solana as source chain methods
func (a *SolanaAccessor) MsgsBetweenSeqNums(ctx context.Context, dest ccipocr3.ChainSelector, seqNumRange ccipocr3.SeqNumRange) ([]ccipocr3.Message, error) {
	onrampAddr, err := a.pdaCache.getBinding(consts.ContractNameOnRamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s binding: %w", consts.ContractNameOnRamp, err)
	}

	attributeIndexes, ok := eventFilterSubkeyIndexMap[consts.EventNameCCIPMessageSent]
	if !ok {
		return nil, fmt.Errorf("failed to find attribute indexes for event %s", consts.EventNameCCIPMessageSent)
	}
	destChainAttributeIndex, ok := attributeIndexes[consts.EventAttributeDestChain]
	if !ok {
		return nil, fmt.Errorf("failed to find index for attribute %s for event %s", consts.EventAttributeDestChain, consts.EventNameCCIPMessageSent)
	}
	destChainSubKeyFilter, err := logpoller.NewEventBySubKeyFilter(destChainAttributeIndex, []primitives.ValueComparator{{Value: dest, Operator: primitives.Eq}})
	if err != nil {
		return nil, fmt.Errorf("failed to build event sub key filter for dest chain attribute: %w", err)
	}
	seqNumAttributeIndex, ok := attributeIndexes[consts.EventAttributeSequenceNumber]
	if !ok {
		return nil, fmt.Errorf("failed to find index for attribute %s for event %s", consts.EventAttributeSequenceNumber, consts.EventNameCCIPMessageSent)
	}
	seqNumSubkeyFilter, err := logpoller.NewEventBySubKeyFilter(seqNumAttributeIndex, []primitives.ValueComparator{{Value: seqNumRange.Start(), Operator: primitives.Gte}, {Value: seqNumRange.End(), Operator: primitives.Lte}})
	if err != nil {
		return nil, fmt.Errorf("failed to build event sub key filter for sequence number attribute: %w", err)
	}

	expressions := []query.Expression{
		logpoller.NewAddressFilter(onrampAddr),
		logpoller.NewEventSigFilter(logpollertypes.NewEventSignatureFromName(consts.EventNameCCIPMessageSent)),
		destChainSubKeyFilter,
		seqNumSubkeyFilter,
		query.Confidence(primitives.Finalized),
	}

	// Hack to handle duplicate filters: we multiply the count by 2 if CPI events are enabled
	// and prefer the CPI events over the normal events if multiple are returned per seq num range.
	count := seqNumRange.End() - seqNumRange.Start() + 1
	if a.logPoller.CPIEventsEnabled() {
		count *= 2
	}

	limitSort := query.LimitAndSort{
		SortBy: []query.SortBy{
			query.NewSortBySequence(query.Asc),
		},
		Limit: query.Limit{
			Count: uint64(count),
		},
	}

	// query Solana logs
	logs, err := a.logPoller.FilteredLogs(ctx, expressions, limitSort, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch filtered logs from log poller: %w", err)
	}

	a.lggr.Debugw("MsgsBetweenSeqNums pre deduplication", "numLogs", len(logs), "count", count)

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

	// Deduplicate by SequenceNumber, preferring CPI events (higher LogIndex)
	if a.logPoller.CPIEventsEnabled() {
		events, err = a.deduplicateEvents(events, logs)
		if err != nil {
			return nil, fmt.Errorf("failed to deduplicate events:%w", err)
		}
	}

	msgs := make([]ccipocr3.Message, 0)
	for _, event := range events {
		// validate event
		if err := ccipchainaccessor.ValidateSendRequestedEvent(event, a.chainSelector, dest, seqNumRange); err != nil {
			a.lggr.Errorw("send requested event validation failed", "err", err, "message", event)
			continue
		}
		msgs = append(msgs, event.Message)
	}
	return msgs, nil
}

func (a *SolanaAccessor) LatestMessageTo(ctx context.Context, dest ccipocr3.ChainSelector) (ccipocr3.SeqNum, error) {
	onrampAddr, err := a.pdaCache.getBinding(consts.ContractNameOnRamp)
	if err != nil {
		return 0, fmt.Errorf("failed to get %s binding: %w", consts.ContractNameOnRamp, err)
	}

	attributeIndexes, ok := eventFilterSubkeyIndexMap[consts.EventNameCCIPMessageSent]
	if !ok {
		return 0, fmt.Errorf("failed to find attribute indexes for event %s", consts.EventNameCCIPMessageSent)
	}
	destChainAttributeIndex, ok := attributeIndexes[consts.EventAttributeDestChain]
	if !ok {
		return 0, fmt.Errorf("failed to find index for attribute %s for event %s", consts.EventAttributeDestChain, consts.EventNameCCIPMessageSent)
	}
	subKeyFilter, err := logpoller.NewEventBySubKeyFilter(destChainAttributeIndex, []primitives.ValueComparator{{Value: dest, Operator: primitives.Eq}})
	if err != nil {
		return 0, fmt.Errorf("failed to build event sub key filter for dest chain attribute: %w", err)
	}

	expressions := []query.Expression{
		logpoller.NewAddressFilter(onrampAddr),
		logpoller.NewEventSigFilter(logpollertypes.NewEventSignatureFromName(consts.EventNameCCIPMessageSent)),
		subKeyFilter,
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

	if len(logs) > 1 {
		return 0, fmt.Errorf("more than one message found for the latest message query, found: %d", len(logs))
	}
	if len(logs) == 0 {
		return 0, nil
	}

	a.lggr.Infow("queried LatestMessageTo",
		"log", logs[0],
		"sourceChainSelector", a.chainSelector,
		"destinationChainSelector", dest,
	)

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

func (a *SolanaAccessor) GetExpectedNextSequenceNumber(ctx context.Context, dest ccipocr3.ChainSelector) (ccipocr3.SeqNum, error) {
	onRampConfig, err := a.getOnRampDestChainConfig(ctx, dest)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch on ramp dest chain config account: %w", err)
	}

	return ccipocr3.SeqNum(onRampConfig.SequenceNumber), nil
}

func (a *SolanaAccessor) GetTokenPriceUSD(ctx context.Context, rawTokenAddress ccipocr3.UnknownAddress) (ccipocr3.TimestampedUnixBig, error) {
	feeQuoterAddr, err := a.pdaCache.getBinding(consts.ContractNameFeeQuoter)
	if err != nil {
		return ccipocr3.TimestampedUnixBig{}, fmt.Errorf("failed to get %s binding: %w", consts.ContractNameFeeQuoter, err)
	}

	if len(rawTokenAddress) != solana.PublicKeyLength {
		return ccipocr3.TimestampedUnixBig{}, fmt.Errorf("raw token address is unexpected length to be solana public key %d, expect %d", len(rawTokenAddress), solana.PublicKeyLength)
	}
	tokenAddress := solana.PublicKeyFromBytes(rawTokenAddress)

	tokenConfigPDA, err := a.pdaCache.feeQuoterBillingTokenConfig(tokenAddress, feeQuoterAddr)
	if err != nil {
		return ccipocr3.TimestampedUnixBig{}, fmt.Errorf("failed to fetch fee quoter billing token config PDA from cache: %w", err)
	}

	var billingTokenConfig feequoter.BillingTokenConfigWrapper
	err = a.client.GetAccountDataBorshInto(ctx, tokenConfigPDA, &billingTokenConfig)
	if err != nil {
		return ccipocr3.TimestampedUnixBig{}, fmt.Errorf("failed to get fee quoter billing token config account: %w", err)
	}
	value := new(big.Int).SetBytes(billingTokenConfig.Config.UsdPerToken.Value[:])
	if billingTokenConfig.Config.UsdPerToken.Timestamp > math.MaxUint32 {
		return ccipocr3.TimestampedUnixBig{}, fmt.Errorf("billing token config timestamp exceeds uint32 max: %d", billingTokenConfig.Config.UsdPerToken.Timestamp)
	}
	return ccipocr3.TimestampedUnixBig{
		Value:     value,
		Timestamp: uint32(billingTokenConfig.Config.UsdPerToken.Timestamp), //nolint:gosec // G115: validated to be within uint32 max above
	}, nil
}

func (a *SolanaAccessor) GetFeeQuoterDestChainConfig(ctx context.Context, dest ccipocr3.ChainSelector) (ccipocr3.FeeQuoterDestChainConfig, error) {
	feeQuoterAddr, err := a.pdaCache.getBinding(consts.ContractNameFeeQuoter)
	if err != nil {
		return ccipocr3.FeeQuoterDestChainConfig{}, fmt.Errorf("failed to get %s binding: %w", consts.ContractNameFeeQuoter, err)
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
	offrampAddr, err := a.pdaCache.getBinding(consts.ContractNameOffRamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s binding: %w", consts.ContractNameOffRamp, err)
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
	offrampAddr, err := a.pdaCache.getBinding(consts.ContractNameOffRamp)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s binding: %w", consts.ContractNameOffRamp, err)
	}

	// trim empty ranges from rangesPerChain
	// otherwise we may get SQL errors from the chainreader.
	nonEmptyRangesPerChain := make(map[ccipocr3.ChainSelector][]ccipocr3.SeqNumRange)
	for chain, ranges := range ranges {
		if len(ranges) > 0 {
			nonEmptyRangesPerChain[chain] = ranges
		}
	}

	keyFilter, countSqNrs, err := createExecutedMessagesKeyFilter(nonEmptyRangesPerChain)
	if err != nil {
		return nil, fmt.Errorf("failed to build key filter for executed messages: %w", err)
	}
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
		logpoller.NewEventSigFilter(logpollertypes.NewEventSignatureFromName(consts.EventNameExecutionStateChanged)),
	}
	expressions = append(expressions, keyFilter.Expressions...)

	logs, err := a.logPoller.FilteredLogs(ctx, expressions, limitSort, "")
	if err != nil {
		return nil, fmt.Errorf("failed to query executed message logs from log poller: %w", err)
	}

	a.lggr.Debugw("queried ExecutedMessages",
		"numEvents", len(logs),
		"seqRangesPerChain", nonEmptyRangesPerChain,
	)

	return a.processExecutionStateChangesEvents(logs, nonEmptyRangesPerChain)
}

func (a *SolanaAccessor) NextSeqNum(ctx context.Context, sources []ccipocr3.ChainSelector) (seqNum map[ccipocr3.ChainSelector]ccipocr3.SeqNum, err error) {
	// Not needed yet. CCIP reader extracts this info from GetAllConfigsLegacy for now
	// https://github.com/smartcontractkit/chainlink-ccip/blob/7cae1b8434dd376eb70f2ddaace43093982f3a57/pkg/reader/ccip.go#L936
	return nil, errors.New("not implemented")
}

// Nonces is used to determine the inbound nonce of senders per lane when Solana is the destination
// Since out-of-order execution is always enabled when Solana is destination, the nonce would always be 0
func (a *SolanaAccessor) Nonces(ctx context.Context, addressesMap map[ccipocr3.ChainSelector][]ccipocr3.UnknownEncodedAddress) (map[ccipocr3.ChainSelector]map[string]uint64, error) {
	results := make(map[ccipocr3.ChainSelector]map[string]uint64)

	// Populate results with 0 nonce for all selectors and senders
	for chainSel, addresses := range addressesMap {
		if _, ok := results[chainSel]; !ok {
			results[chainSel] = make(map[string]uint64)
		}
		for _, addr := range addresses {
			results[chainSel][string(addr)] = 0
		}
	}

	return results, nil
}

func (a *SolanaAccessor) GetChainFeePriceUpdate(ctx context.Context, selectors []ccipocr3.ChainSelector) (map[ccipocr3.ChainSelector]ccipocr3.TimestampedUnixBig, error) {
	feeQuoterAddr, err := a.pdaCache.getBinding(consts.ContractNameFeeQuoter)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s binding: %w", consts.ContractNameFeeQuoter, err)
	}

	pdaSelectorMap := make(map[solana.PublicKey]ccipocr3.ChainSelector)
	for _, sel := range selectors {
		destChainPDA, err := a.pdaCache.feeQuoterDestChain(uint64(sel), feeQuoterAddr)
		if err != nil {
			continue
		}

		pdaSelectorMap[destChainPDA] = sel
	}

	batches := batchPDAs(maps.Keys(pdaSelectorMap))
	feePriceUpdates := make(map[ccipocr3.ChainSelector]ccipocr3.TimestampedUnixBig)

	for _, batch := range batches {
		result, err := a.client.GetMultipleAccountsWithOpts(ctx, batch, &rpc.GetMultipleAccountsOpts{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch fee quoter destination chain PDAs: %w", err)
		}

		if len(batch) != len(result.Value) {
			return nil, fmt.Errorf("fee quoter destination chain results contain unexpected number of accounts: %d, expected %d", len(result.Value), len(batch))
		}

		for i, account := range result.Value {
			selector := pdaSelectorMap[batch[i]]

			// Account not found, return 0 fee price
			if account == nil {
				feePriceUpdates[selector] = ccipocr3.TimestampedUnixBig{
					Value:     big.NewInt(0),
					Timestamp: 0,
				}
				continue
			}
			var destChain feequoter.DestChain
			decodeErr := bin.NewBorshDecoder(account.Data.GetBinary()).Decode(&destChain)
			if decodeErr != nil {
				a.lggr.Errorw("failed to decode fee quoter destination chain PDA", "selector", selector, "error", decodeErr)
				continue
			}

			if destChain.State.UsdPerUnitGas.Timestamp > math.MaxUint32 {
				a.lggr.Errorw("gas price update timestamp exceeeds uint32 max", "timestamp", destChain.State.UsdPerUnitGas.Timestamp)
				continue
			}

			value := new(big.Int).SetBytes(destChain.State.UsdPerUnitGas.Value[:])
			feePriceUpdates[selector] = ccipocr3.TimestampedUnixBig{
				Value:     value,
				Timestamp: uint32(destChain.State.UsdPerUnitGas.Timestamp), //nolint:gosec // timestamp validated to be within uint32 bounds above
			}
		}
	}

	a.lggr.Debugw("GetChainFeePriceUpdate updates", "updates", feePriceUpdates)
	return feePriceUpdates, nil
}

func (a *SolanaAccessor) GetLatestPriceSeqNr(ctx context.Context) (ccipocr3.SeqNum, error) {
	// Validate offramp binding exists
	_, err := a.pdaCache.getBinding(consts.ContractNameOffRamp)
	if err != nil {
		return 0, fmt.Errorf("failed to get %s binding: %w", consts.ContractNameOffRamp, err)
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
	tokenBytes []ccipocr3.UnknownAddress,
) (map[ccipocr3.UnknownEncodedAddress]ccipocr3.TimestampedUnixBig, error) {
	feeQuoterAddr, err := a.pdaCache.getBinding(consts.ContractNameFeeQuoter)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s binding: %w", consts.ContractNameFeeQuoter, err)
	}

	billinConfigPDAs := make([]solana.PublicKey, 0, len(tokenBytes))
	for _, token := range tokenBytes {
		if len(token) != solana.PublicKeyLength {
			return nil, fmt.Errorf("invalid token bytes length, got %d, expected %d", len(token), solana.PublicKeyLength)
		}
		tokenPubKey := solana.PublicKeyFromBytes(token)
		tokenConfigPDA, err := a.pdaCache.feeQuoterBillingTokenConfig(tokenPubKey, feeQuoterAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch fee quoter billing token config PDA from cache: %w", err)
		}
		billinConfigPDAs = append(billinConfigPDAs, tokenConfigPDA)
	}

	batches := batchPDAs(billinConfigPDAs)
	feePriceUpdates := make(map[ccipocr3.UnknownEncodedAddress]ccipocr3.TimestampedUnixBig)

	for _, batch := range batches {
		result, err := a.client.GetMultipleAccountsWithOpts(ctx, batch, &rpc.GetMultipleAccountsOpts{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch fee quoter destination chain PDAs: %w", err)
		}

		if len(batch) != len(result.Value) {
			return nil, fmt.Errorf("fee quoter destination chain results contain unexpected number of accounts: %d, expected %d", len(result.Value), len(batch))
		}

		for i, account := range result.Value {
			// Account not found, continue to other updates
			if account == nil {
				a.lggr.Errorw("token update PDA not found", "pda", batch[i].String())
				continue
			}

			var billingConfig feequoter.BillingTokenConfigWrapper
			decodeErr := bin.NewBorshDecoder(account.Data.GetBinary()).Decode(&billingConfig)
			if decodeErr != nil {
				a.lggr.Errorw("failed to decode fee billing token config PDA", "selector", a.chainSelector, "error", decodeErr)
				continue
			}

			if billingConfig.Config.UsdPerToken.Timestamp > math.MaxUint32 {
				a.lggr.Errorw("token update timestamp exceeeds uint32 max", "timestamp", billingConfig.Config.UsdPerToken.Timestamp)
				continue
			}

			token := ccipocr3.UnknownEncodedAddress(billingConfig.Config.Mint.String())
			value := new(big.Int).SetBytes(billingConfig.Config.UsdPerToken.Value[:])
			feePriceUpdates[token] = ccipocr3.TimestampedUnixBig{
				Value:     value,
				Timestamp: uint32(billingConfig.Config.UsdPerToken.Timestamp), //nolint:gosec // G115: validated to be within uint32 max above
			}
		}
	}

	return feePriceUpdates, nil
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
	usdcTokenPoolAddr, err := a.pdaCache.getBinding(consts.ContractNameUSDCTokenPool)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s binding: %w", consts.ContractNameUSDCTokenPool, err)
	}

	if len(tokens) == 0 {
		return map[ccipocr3.MessageTokenID]ccipocr3.Bytes{}, nil
	}
	a.lggr.Debugw("Searching for Solana CCTP USDC logs", "numExpected", len(tokens))

	// Parse the extra data field to get the CCTP nonces and source domains.
	cctpData, err := getMessageTokenData(tokens)
	if err != nil {
		return nil, err
	}

	cctpExpressions, err := createCCTPMessageSentQueryExpressions(cctpData)
	if err != nil {
		return nil, fmt.Errorf("failed to create CCTP message sent query expressions: %w", err)
	}

	// Parent expressions for the query.
	expressions := []query.Expression{
		logpoller.NewAddressFilter(usdcTokenPoolAddr),
		logpoller.NewEventSigFilter(logpollertypes.NewEventSignatureFromName(ccipCCTPMessageSentEventName)),
		query.Confidence(primitives.Finalized), // solana log poller only operates with finalized confidence
		query.Or(cctpExpressions...),
	}

	limitSort := query.NewLimitAndSort(
		query.Limit{Count: uint64(len(cctpExpressions))},
		query.NewSortBySequence(query.Asc),
	)
	// query solana logs
	logs, err := a.logPoller.FilteredLogs(ctx, expressions, limitSort, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cctp message sent logs from log poller: %w", err)
	}

	a.lggr.Debugw("queried MessagesByTokenID", "numLogs", len(logs),
		"destinationChainSelector", a.chainSelector,
		"limit", len(cctpExpressions))

	return a.processCCTPMessageSentEvents(logs, source, tokens, cctpData)
}

// batchPDAs batches list of PDAs into groups of getMultipleAccountsLimit to be compatible with the getMultipleAccounts RPC limits
func batchPDAs(addrs []solana.PublicKey) [][]solana.PublicKey {
	if len(addrs) <= getMultipleAccountsLimit {
		return [][]solana.PublicKey{addrs}
	}

	batches := len(addrs) / getMultipleAccountsLimit
	if len(addrs)%getMultipleAccountsLimit == 0 {
		batches++
	}

	batchAddrs := make([][]solana.PublicKey, 0, batches)
	for i := 0; i < len(addrs); i += getMultipleAccountsLimit {
		end := min(i+getMultipleAccountsLimit, len(addrs))
		batchAddrs = append(batchAddrs, addrs[i:end])
	}

	return batchAddrs
}

// deduplicateEvents removes duplicate events for the same SequenceNumber,
// keeping the one with the highest LogIndex (CPI events have higher LogIndex than log-based events).
// events and logs must be parallel arrays (events[i] came from logs[i]).
func (a *SolanaAccessor) deduplicateEvents(events []*ccipocr3.SendRequestedEvent, logs []logpollertypes.Log) ([]*ccipocr3.SendRequestedEvent, error) {
	if len(events) == 0 {
		return events, nil
	}

	if len(events) != len(logs) {
		return events, fmt.Errorf("deduplicateEvents: events and logs have different lengths; events:%d logs:%d", len(events), len(logs))
	}

	type eventWithLogIndex struct {
		event    *ccipocr3.SendRequestedEvent
		logIndex int64
	}

	bestBySeqNum := make(map[ccipocr3.SeqNum]eventWithLogIndex)
	for i, event := range events {
		seqNum := event.SequenceNumber
		if seqNum == 0 {
			a.lggr.Errorw("deduplicateEvents: sequence number is 0", "event", event, "log", logs[i])
			continue
		}
		existing, exists := bestBySeqNum[seqNum]
		if !exists || logs[i].LogIndex > existing.logIndex {
			if exists {
				a.lggr.Debugw("deduplicateEvents: replacing event with higher LogIndex",
					"seqNum", seqNum,
					"newLogIndex", logs[i].LogIndex,
					"existingLogIndex", existing.logIndex,
				)
			}
			bestBySeqNum[seqNum] = eventWithLogIndex{event: event, logIndex: logs[i].LogIndex}
		}
	}

	values := make([]*ccipocr3.SendRequestedEvent, 0, len(bestBySeqNum))
	for _, ewl := range bestBySeqNum {
		values = append(values, ewl.event)
	}
	slices.SortFunc(values, func(a, b *ccipocr3.SendRequestedEvent) int {
		return cmp.Compare(a.SequenceNumber, b.SequenceNumber)
	})

	a.lggr.Debugw("deduplicateEvents", "before count:", len(events), "after count:", len(values))

	return values, nil
}

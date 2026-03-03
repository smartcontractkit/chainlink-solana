package chainaccessor

import (
	"context"
	"crypto/sha3"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"golang.org/x/exp/maps"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccip/consts"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"

	idl "github.com/smartcontractkit/chainlink-ccip/chains/solana"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/ccip"

	"github.com/smartcontractkit/chainlink-ccip/pkg/reader"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

var (
	ccipOffRampIDL   = idl.FetchCCIPOfframpIDL()
	ccipRouterIDL    = idl.FetchCCIPRouterIDL()
	cctpTokenPoolIDL = idl.FetchCctpTokenPoolIDL()

	// defaultCCIPLogsRetention defines the duration for which logs critical for Commit/Exec plugins processing are retained.
	// Although Exec relies on permissionlessExecThreshold which is lower than 24hours for picking eligible CommitRoots,
	// Commit still can reach to older logs because it filters them by sequence numbers. For instance, in case of RMN curse on chain,
	// we might have logs waiting in OnRamp to be committed first. When outage takes days we still would
	// be able to bring back processing without replaying any logs from chain. You can read that param as
	// "how long CCIP can be down and still be able to process all the messages after getting back to life".
	// Breaching this threshold would require replaying chain using LogPoller from the beginning of the outage.
	// Using same default retention as v1.5 https://github.com/smartcontractkit/ccip/pull/530/files
	defaultCCIPLogsRetention = 30 * 24 * time.Hour // 30 days
)

type filterConfig struct {
	idl               string
	chainSpecificName string
	includeReverted   bool
	indexedField0     *string
	indexedField1     *string
	indexedField2     *string
	indexedField3     *string
	// cpi filter specific fields
	cpiBackup        bool // skip this filter when CPI is enabled
	destContractName string
	methodName       string
}

func (f filterConfig) isCPIFilter() bool {
	return f.destContractName != ""
}

var (
	// On-chain paths for the CCIPMessageSent event
	msgSentSrcChainPath  = "Message.Header.SourceChainSelector"
	msgSentDestChainPath = "Message.Header.DestChainSelector"
	msgSentSeqNumPath    = "Message.Header.SequenceNumber"

	// On-chain paths for the ExecutionStateChanged event
	execStateChangedSrcChainPath = "SourceChainSelector"
	execStateChangedSeqNumPath   = consts.EventAttributeSequenceNumber
	execStateChangedStatePath    = consts.EventAttributeState

	// On-chain paths for the CCTP Message sent event
	cctpMsgSentNoncePath     = "CctpNonce"
	cctpMsgSentSrcDomainPath = "SourceDomain"

	ccipCCTPMessageSentEventName = "CcipCctpMessageSentEvent"

	rmnRemoteCPIEventMethodName = "cpiEvent"
)

// Map of relevant events and their metadata required to build the codec and bind program addresses
var eventFilterConfigMap = map[string]map[string]filterConfig{
	consts.ContractNameOnRamp: {
		consts.EventNameCCIPMessageSent: {
			idl:               ccipRouterIDL,
			chainSpecificName: consts.EventNameCCIPMessageSent,
			includeReverted:   false,
			indexedField0:     &msgSentSrcChainPath,
			indexedField1:     &msgSentDestChainPath,
			indexedField2:     &msgSentSeqNumPath,
			cpiBackup:         true,
		},
	},
	consts.ContractNameOffRamp: {
		consts.EventNameCommitReportAccepted: {
			idl:               ccipOffRampIDL,
			chainSpecificName: consts.EventNameCommitReportAccepted,
			includeReverted:   false,
		},
		consts.EventNameExecutionStateChanged: {
			idl:               ccipOffRampIDL,
			chainSpecificName: consts.EventNameExecutionStateChanged,
			includeReverted:   true,
			indexedField0:     &execStateChangedSrcChainPath,
			indexedField1:     &execStateChangedSeqNumPath,
			indexedField2:     &execStateChangedStatePath,
		},
	},
	consts.ContractNameUSDCTokenPool: {
		consts.EventNameCCTPMessageSent: {
			idl:               cctpTokenPoolIDL,
			chainSpecificName: ccipCCTPMessageSentEventName,
			includeReverted:   false,
			indexedField0:     &cctpMsgSentNoncePath,
			indexedField1:     &cctpMsgSentSrcDomainPath,
		},
	},
}

var cpiFilterConfigMap = map[string]map[string]filterConfig{
	consts.ContractNameRouter: {
		consts.EventNameCCIPMessageSent: {
			destContractName:  consts.ContractNameRMNRemote,
			chainSpecificName: consts.EventNameCCIPMessageSent,
			methodName:        rmnRemoteCPIEventMethodName,
			idl:               ccipRouterIDL,
			includeReverted:   false,
			indexedField0:     &msgSentSrcChainPath,
			indexedField1:     &msgSentDestChainPath,
			indexedField2:     &msgSentSeqNumPath,
		},
	},
}

// Map of event name to offchain attribute to its subkey index for querying the LogPoller
// Corresponds to the indexed fields in the eventFilterConfigMap above
var eventFilterSubkeyIndexMap = map[string]map[string]uint64{
	consts.EventNameCCIPMessageSent: {
		consts.EventAttributeSourceChain:    0,
		consts.EventAttributeDestChain:      1,
		consts.EventAttributeSequenceNumber: 2,
	},
	consts.EventNameExecutionStateChanged: {
		consts.EventAttributeSourceChain:    0,
		consts.EventAttributeSequenceNumber: 1,
		consts.EventAttributeState:          2,
	},
	// Event for USDC CCTP
	consts.EventNameCCTPMessageSent: {
		consts.EventAttributeCCTPNonce:    0,
		consts.EventAttributeSourceDomain: 1,
	},
}

// bindContractEvent binds contract events to the logpoller for monitoring blockchain events.
// This operation is idempotent - if the same address exists, it performs no operation;
// if the address is changed, it updates to the new address, overwriting the existing one;
// if the contract is not bound, it binds to the new address.
// Supports OnRamp and OffRamp contract types with their respective event filters.
// Returns an error if filter registration fails.
func (a *SolanaAccessor) bindContractEvent(ctx context.Context, contractName string, address solana.PublicKey) error {
	cpiEnabled := a.logPoller.CPIEventsEnabled()

	// Register normal event filters if this contract has any
	if eventsMap, exists := eventFilterConfigMap[contractName]; exists {
		for eventName, config := range eventsMap {
			if cpiEnabled && config.cpiBackup {
				continue
			}
			if err := a.registerFilterIfNotExists(ctx, config, address, solana.PublicKey{}); err != nil {
				return fmt.Errorf("failed to register filter for event %s: %w", eventName, err)
			}
		}
	}

	// Try to bind CPI filters only if CPI events are enabled
	if cpiEnabled {
		if err := a.tryBindCPIFilters(ctx, contractName); err != nil {
			return fmt.Errorf("failed to bind CPI filters: %w", err)
		}
	}

	return nil
}

func (a *SolanaAccessor) tryBindCPIFilters(ctx context.Context, contractName string) error {
	for sourceContractName, eventConfigs := range cpiFilterConfigMap {
		for _, cfg := range eventConfigs {
			if contractName != sourceContractName && contractName != cfg.destContractName {
				continue
			}

			sourceAddr, sourceErr := a.pdaCache.getBinding(sourceContractName)
			destAddr, destErr := a.pdaCache.getBinding(cfg.destContractName)
			if sourceErr != nil || destErr != nil {
				continue
			}

			if sourceAddr.Equals(solana.PublicKey{}) || destAddr.Equals(solana.PublicKey{}) {
				return fmt.Errorf("source or dest is empty: sourceAddr: %s, destAddr: %s", sourceAddr.String(), destAddr.String())
			}

			if err := a.registerFilterIfNotExists(ctx, cfg, sourceAddr, destAddr); err != nil {
				return fmt.Errorf("failed to register CPI filter for event %s: %w", cfg.chainSpecificName, err)
			}
		}
	}
	return nil
}

func extractEventIDL(eventName string, codecIDL codecv1.IDL) (codecv1.IdlEvent, error) {
	idlDef, err := codecv1.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeEventDef, eventName, codecIDL)
	if err != nil {
		return codecv1.IdlEvent{}, err
	}
	eventIdl, isOk := idlDef.(codecv1.IdlEvent)
	if !isOk {
		return codecv1.IdlEvent{}, fmt.Errorf("unexpected type from IDL definition for event read: %q", eventName)
	}
	return eventIdl, nil
}

// registerFilterIfNotExists registers a filter for the given event if it doesn't already exist.
// For CPI filters, destAddr must be provided; for regular filters, it's ignored.
func (a *SolanaAccessor) registerFilterIfNotExists(
	ctx context.Context,
	filterConfig filterConfig,
	sourceAddr solana.PublicKey,
	destAddr solana.PublicKey,
) error {
	conf := config.PollingFilter{
		Retention: &defaultCCIPLogsRetention,
	}

	eventName := filterConfig.chainSpecificName

	var codecIDL codecv1.IDL
	if err := json.Unmarshal([]byte(filterConfig.idl), &codecIDL); err != nil {
		return fmt.Errorf("unexpected error: invalid CCIP OffRamp IDL, error: %w", err)
	}

	eventIdl, err := extractEventIDL(eventName, codecIDL)
	if err != nil {
		return fmt.Errorf("failed to extract event IDL: %w", err)
	}

	lpEventIDL := logpollertypes.EventIdl{Event: eventIdl, Types: codecIDL.Types}
	subKeyPaths := processSubKeyPaths(filterConfig)

	filter := logpollertypes.Filter{
		Address:         logpollertypes.PublicKey(sourceAddr),
		EventName:       eventName,
		EventSig:        logpollertypes.NewEventSignatureFromName(eventName),
		EventIdl:        lpEventIDL,
		SubkeyPaths:     subKeyPaths,
		StartingBlock:   conf.GetStartingBlock(),
		Retention:       conf.GetRetention(),
		MaxLogsKept:     conf.GetMaxLogsKept(),
		IncludeReverted: filterConfig.includeReverted,
	}

	if filterConfig.isCPIFilter() {
		filter.SetCPIFilterConfig(logpollertypes.ExtraFilterConfig{
			DestProgram:     logpollertypes.PublicKey(destAddr),
			MethodSignature: logpollertypes.NewMethodSignatureFromName(filterConfig.methodName),
		})
	}

	filterName, err := deriveName(filter)
	if err != nil {
		return fmt.Errorf("failed to derive filter name: %w", err)
	}

	// Filter already registered so return early
	if hasFilter := a.logPoller.HasFilter(ctx, filterName); hasFilter {
		return nil
	}

	filter.Name = filterName

	if filterConfig.isCPIFilter() {
		a.lggr.Debugw("registering CPI log poller filter",
			"name", filterName,
			"eventName", eventName,
			"eventSig", filter.EventSig.String(),
			"sourceAddr", sourceAddr,
			"destAddr", destAddr,
			"methodSig", filter.ExtraFilterConfig.MethodSignature)
	} else {
		a.lggr.Debugw("registering normal log poller filter",
			"name", filterName,
			"eventName", eventName,
			"eventSig", filter.EventSig.String(),
			"address", sourceAddr)
	}

	if err := a.logPoller.RegisterFilter(ctx, filter); err != nil {
		return fmt.Errorf("failed to register logpoller filter: %w", err)
	}

	return nil
}

// convertCCIPMessageSent converts a Solana-specific CCIPMessageSent event to a generic
// ccipocr3.SendRequestedEvent. This function is idempotent and performs a
// one-to-one mapping of event fields from the Solana format to the standard CCIP format.
func (a *SolanaAccessor) convertCCIPMessageSent(logs []logpollertypes.Log, onrampAddr solana.PublicKey) ([]*ccipocr3.SendRequestedEvent, error) {
	iter, err := a.decodeLogsIntoSequences(consts.EventNameCCIPMessageSent, logs)
	if err != nil {
		return nil, fmt.Errorf("failed to decode logs into sequences: %w", err)
	}

	if len(logs) != len(iter) {
		return nil, fmt.Errorf("failed to convert all logs into generic ccip event, logs %d, events %d", len(logs), len(iter))
	}

	genericEvents := make([]*ccipocr3.SendRequestedEvent, 0)
	for _, seq := range iter {
		event, ok := seq.Data.(*ccip.EventCCIPMessageSent)
		if !ok {
			return nil, fmt.Errorf("failed to cast %T to EventCCIPMessageSent", seq.Data)
		}
		msg := ccipocr3.Message{
			Header: ccipocr3.RampMessageHeader{
				MessageID:           ccipocr3.Bytes32(event.Message.Header.MessageId),
				SourceChainSelector: a.chainSelector,
				DestChainSelector:   ccipocr3.ChainSelector(event.Message.Header.DestChainSelector),
				SequenceNumber:      ccipocr3.SeqNum(event.Message.Header.SequenceNumber),
				Nonce:               event.Message.Header.Nonce,
				OnRamp:              ccipocr3.UnknownAddress(onrampAddr.Bytes()),
				// TxHash: logs[i].TxHash.ToSolana().String(), // Populating TxHash causes inconsistent state with non-LOOPP. Eventually required for CCTPv2.
			},
			Sender:         ccipocr3.UnknownAddress(event.Message.Sender.Bytes()),
			Data:           ccipocr3.Bytes(event.Message.Data),
			Receiver:       ccipocr3.UnknownAddress(event.Message.Receiver),
			ExtraArgs:      ccipocr3.Bytes(event.Message.ExtraArgs),
			FeeToken:       ccipocr3.UnknownAddress(event.Message.FeeToken.Bytes()),
			FeeTokenAmount: solcommoncodec.DecodeLEToBigInt(event.Message.FeeTokenAmount.LeBytes[:]),
			TokenAmounts:   convertTokenAmounts(event.Message.TokenAmounts),
			FeeValueJuels:  solcommoncodec.DecodeLEToBigInt(event.Message.FeeValueJuels.LeBytes[:]),
		}
		genericEvents = append(genericEvents, &ccipocr3.SendRequestedEvent{
			DestChainSelector: msg.Header.DestChainSelector,
			SequenceNumber:    msg.Header.SequenceNumber,
			Message:           msg,
		})
	}
	return genericEvents, nil
}

func convertTokenAmounts(transfers []ccip_router.SVM2AnyTokenTransfer) []ccipocr3.RampTokenAmount {
	genericTokenAmounts := make([]ccipocr3.RampTokenAmount, 0, len(transfers))
	for _, transfer := range transfers {
		genericTokenAmounts = append(genericTokenAmounts, ccipocr3.RampTokenAmount{
			SourcePoolAddress: transfer.SourcePoolAddress.Bytes(),
			DestTokenAddress:  transfer.DestTokenAddress,
			ExtraData:         transfer.ExtraData,
			Amount:            solcommoncodec.DecodeLEToBigInt(transfer.Amount.LeBytes[:]),
			DestExecData:      transfer.DestExecData,
		})
	}
	return genericTokenAmounts
}

func deriveName(filter logpollertypes.Filter) (string, error) {
	// include eventSig, readDef, address, subkeyPaths, indexedSubkeys
	data := filter.EventSig[:]
	data = append(data, filter.Address.ToSolana().Bytes()...)
	data = append(data, []byte(filter.EventName)...)

	if len(filter.SubkeyPaths) > 0 {
		b, err := json.Marshal(filter.SubkeyPaths)
		if err != nil {
			return "", fmt.Errorf("failed to marshal subkey path: %w", err)
		}
		data = append(data, b...)
	}

	if filter.IsCPIFilter() {
		data = append(data, filter.ExtraFilterConfig.DestProgram[:]...)
		data = append(data, filter.ExtraFilterConfig.MethodSignature[:]...)
	}

	hash := sha3.Sum256(data)

	if filter.IsCPIFilter() {
		return fmt.Sprintf("cpi.%s.%s.%x", filter.EventName, filter.Address.String(), hash[:]), nil
	}

	return fmt.Sprintf("%s.%s.%x", filter.EventName, filter.Address.String(), hash[:]), nil
}

func processSubKeyPaths(cfg filterConfig) [][]string {
	subKeyPaths := make([][]string, 0)

	if cfg.indexedField0 != nil {
		subKeyPaths = append(subKeyPaths, strings.Split(*cfg.indexedField0, "."))
	}
	if cfg.indexedField1 != nil {
		subKeyPaths = append(subKeyPaths, strings.Split(*cfg.indexedField1, "."))
	}
	if cfg.indexedField2 != nil {
		subKeyPaths = append(subKeyPaths, strings.Split(*cfg.indexedField2, "."))
	}
	if cfg.indexedField3 != nil {
		subKeyPaths = append(subKeyPaths, strings.Split(*cfg.indexedField3, "."))
	}

	return subKeyPaths
}

func (a *SolanaAccessor) processCommitReports(
	logs []logpollertypes.Log, ts time.Time, limit int,
) ([]ccipocr3.CommitPluginReportWithMeta, error) {
	iter, err := a.decodeLogsIntoSequences(consts.EventNameCommitReportAccepted, logs)
	if err != nil {
		return nil, fmt.Errorf("failed to decode logs into sequences: %w", err)
	}

	reports := make([]ccipocr3.CommitPluginReportWithMeta, 0)
	for _, item := range iter {
		ev, err := validateCommitReportAcceptedEvent(item, ts)
		if err != nil {
			a.lggr.Errorw("validate commit report accepted event", "err", err, "ev", item.Data)
			continue
		}

		a.lggr.Debugw("processing commit report", "report", ev, "item", item)

		unblessedMerkleRoots := a.processMerkleRoot(ev.Report)

		priceUpdates, err := a.processPriceUpdates(ev.PriceUpdates)
		if err != nil {
			a.lggr.Errorw("failed to process price updates", "err", err, "priceUpdates", ev.PriceUpdates)
			continue
		}

		blockNum, err := strconv.ParseUint(item.Head.Height, 10, 64)
		if err != nil {
			a.lggr.Errorw("failed to parse block number", "blockNum", item.Head.Height, "err", err)
			continue
		}

		reports = append(reports, ccipocr3.CommitPluginReportWithMeta{
			Report: ccipocr3.CommitPluginReport{
				BlessedMerkleRoots:   nil,
				UnblessedMerkleRoots: unblessedMerkleRoots, // All roots default to unblessed on solana
				PriceUpdates:         priceUpdates,
			},
			Timestamp: time.Unix(int64(item.Timestamp), 0), // nolint:gosec // G115: timestamp will always fit in int64 for unix
			BlockNum:  blockNum,
		})
	}

	a.lggr.Debugw("decoded commit reports", "reports", reports)

	if len(reports) < limit {
		return reports, nil
	}

	a.lggr.Errorw("too many commit reports received, commit report results are truncated",
		"numTruncatedReports", len(reports)-limit)
	for l := limit; l < len(reports); l++ {
		if !reports[l].Report.HasNoRoots() {
			a.lggr.Warnw("dropping merkle root commit report which doesn't fit in limit", "report", reports[l])
		}
	}
	return reports[:limit], nil
}

func validateCommitReportAcceptedEvent(
	seq types.Sequence, gteTimestamp time.Time,
) (*ccip.EventCommitReportAccepted, error) {
	ev, is := (seq.Data).(*ccip.EventCommitReportAccepted)
	if !is {
		return nil, fmt.Errorf("unexpected type %T while expecting EventCommitReportAccepted", seq.Data)
	}

	if ev == nil {
		return nil, fmt.Errorf("commit report accepted event is nil")
	}

	if seq.Timestamp < uint64(gteTimestamp.Unix()) { // nolint:gosec // G115: timestamp is always positive
		return nil, fmt.Errorf("commit report accepted event timestamp is less than the minimum timestamp %v<%v",
			seq.Timestamp, gteTimestamp.Unix())
	}

	if err := validateMerkleRoot(ev.Report); err != nil {
		return nil, fmt.Errorf("merkle roots: %w", err)
	}

	for _, tpus := range ev.PriceUpdates.TokenPriceUpdates {
		if tpus.SourceToken.IsZero() {
			return nil, fmt.Errorf("invalid source token address: %s", tpus.SourceToken.String())
		}
		price := new(big.Int)
		price.SetBytes(tpus.UsdPerToken[:])
		if price.Cmp(big.NewInt(0)) <= 0 {
			return nil, fmt.Errorf("non-positive usd per token")
		}
	}

	for _, gpus := range ev.PriceUpdates.GasPriceUpdates {
		price := new(big.Int)
		price.SetBytes(gpus.UsdPerUnitGas[:])
		if price.Cmp(big.NewInt(0)) < 0 {
			return nil, fmt.Errorf("negative usd per unit gas: %s", price.String())
		}
	}

	return ev, nil
}

func (a *SolanaAccessor) processMerkleRoot(
	merkleRoot *ccip_offramp.MerkleRoot,
) (blessedMerkleRoot []ccipocr3.MerkleRootChain) {
	// Return early if merkle root is nil
	// Valid scenario if commit report only contains price updates
	if merkleRoot == nil {
		return nil
	}
	return []ccipocr3.MerkleRootChain{
		{
			ChainSel:      ccipocr3.ChainSelector(merkleRoot.SourceChainSelector),
			OnRampAddress: merkleRoot.OnRampAddress,
			SeqNumsRange: ccipocr3.NewSeqNumRange(
				ccipocr3.SeqNum(merkleRoot.MinSeqNr),
				ccipocr3.SeqNum(merkleRoot.MaxSeqNr),
			),
			MerkleRoot: merkleRoot.MerkleRoot,
		},
	}
}

func validateMerkleRoot(merkleRoot *ccip_offramp.MerkleRoot) error {
	// Return early if merkle root is nil
	// Valid scenario if commit report only contains price updates
	if merkleRoot == nil {
		return nil
	}
	if merkleRoot.SourceChainSelector == 0 {
		return fmt.Errorf("source chain is zero")
	}
	if merkleRoot.MinSeqNr == 0 {
		return fmt.Errorf("minSeqNr is zero")
	}
	if merkleRoot.MaxSeqNr == 0 {
		return fmt.Errorf("maxSeqNr is zero")
	}
	if merkleRoot.MinSeqNr > merkleRoot.MaxSeqNr {
		return fmt.Errorf("minSeqNr is greater than maxSeqNr")
	}
	if merkleRoot.MerkleRoot == [32]byte{} {
		return fmt.Errorf("empty merkle root")
	}
	if merkleRoot.OnRampAddress == nil {
		return errors.New("nil onramp address")
	}

	return nil
}

func (a *SolanaAccessor) processPriceUpdates(priceUpdates ccip_offramp.PriceUpdates) (ccipocr3.PriceUpdates, error) {
	updates := ccipocr3.PriceUpdates{
		TokenPriceUpdates: make([]ccipocr3.TokenPrice, 0),
		GasPriceUpdates:   make([]ccipocr3.GasPriceChain, 0),
	}

	for _, tokenPriceUpdate := range priceUpdates.TokenPriceUpdates {
		// UsdPerToken expected to be big endian so SetBytes works here
		// https://github.com/smartcontractkit/chainlink/blob/9383bea5a7c05b4c7ae807799d6a8cb03c6d3476/core/capabilities/ccip/ccipsolana/commitcodec.go#L61
		price := new(big.Int).SetBytes(tokenPriceUpdate.UsdPerToken[:])
		updates.TokenPriceUpdates = append(updates.TokenPriceUpdates, ccipocr3.TokenPrice{
			TokenID: ccipocr3.UnknownEncodedAddress(tokenPriceUpdate.SourceToken.String()),
			Price:   ccipocr3.NewBigInt(price),
		})
	}

	for _, gasPriceUpdate := range priceUpdates.GasPriceUpdates {
		// UsdPerUnitGas expected to be big endian so SetBytes works here
		// https://github.com/smartcontractkit/chainlink/blob/9383bea5a7c05b4c7ae807799d6a8cb03c6d3476/core/capabilities/ccip/ccipsolana/commitcodec.go#L73
		price := new(big.Int).SetBytes(gasPriceUpdate.UsdPerUnitGas[:])
		updates.GasPriceUpdates = append(updates.GasPriceUpdates, ccipocr3.GasPriceChain{
			ChainSel: ccipocr3.ChainSelector(gasPriceUpdate.DestChainSelector),
			GasPrice: ccipocr3.NewBigInt(price),
		})
	}

	return updates, nil
}

func createExecutedMessagesKeyFilter(rangesPerChain map[ccipocr3.ChainSelector][]ccipocr3.SeqNumRange) (query.KeyFilter, uint64, error) {
	var chainExpressions []query.Expression
	var countSqNrs uint64
	// final query should look like
	// (chainA && (sqRange1 || sqRange2 || ...)) || (chainB && (sqRange1 || sqRange2 || ...))
	sortedChains := maps.Keys(rangesPerChain)
	slices.Sort(sortedChains)

	attributeIndexes, ok := eventFilterSubkeyIndexMap[consts.EventNameExecutionStateChanged]
	if !ok {
		return query.KeyFilter{}, 0, fmt.Errorf("failed to find attribute indexes for event %s", consts.EventNameExecutionStateChanged)
	}
	seqAttributeIndex, ok := attributeIndexes[consts.EventAttributeSequenceNumber]
	if !ok {
		return query.KeyFilter{}, 0, fmt.Errorf("failed to find index for attribute %s for event %s", consts.EventAttributeSequenceNumber, consts.EventNameExecutionStateChanged)
	}
	srcChainAttributeIndex, ok := attributeIndexes[consts.EventAttributeSourceChain]
	if !ok {
		return query.KeyFilter{}, 0, fmt.Errorf("failed to find index for attribute %s for event %s", consts.EventAttributeSourceChain, consts.EventNameExecutionStateChanged)
	}

	for _, srcChain := range sortedChains {
		seqNumRanges := rangesPerChain[srcChain]
		var seqRangeExpressions []query.Expression
		for _, seqNrRange := range seqNumRanges {
			expr, err := logpoller.NewEventBySubKeyFilter(seqAttributeIndex, []primitives.ValueComparator{{
				Value:    seqNrRange.Start(),
				Operator: primitives.Gte,
			}, {
				Value:    seqNrRange.End(),
				Operator: primitives.Lte,
			}})
			if err != nil {
				return query.KeyFilter{}, 0, fmt.Errorf("failed to build event sub key filter for sequence number attribute: %w", err)
			}
			seqRangeExpressions = append(seqRangeExpressions, expr)
			countSqNrs += uint64(seqNrRange.End() - seqNrRange.Start() + 1)
		}
		combinedSeqNrs := query.Or(seqRangeExpressions...)

		expr, err := logpoller.NewEventBySubKeyFilter(srcChainAttributeIndex, []primitives.ValueComparator{{Value: srcChain, Operator: primitives.Eq}})
		if err != nil {
			return query.KeyFilter{}, 0, fmt.Errorf("failed to build event sub key filter for source chain attribute: %w", err)
		}

		chainExpressions = append(chainExpressions, query.And(
			combinedSeqNrs,
			expr,
		))
	}
	extendedQuery := query.Or(chainExpressions...)

	stateAttributeIndex, ok := attributeIndexes[consts.EventAttributeState]
	if !ok {
		return query.KeyFilter{}, 0, fmt.Errorf("failed to find index for attribute %s for event %s", consts.EventAttributeState, consts.EventNameExecutionStateChanged)
	}
	// We don't need to wait for an execute state changed event to be finalized
	// before we optimistically mark a message as executed.
	subKeyFilter, err := logpoller.NewEventBySubKeyFilter(stateAttributeIndex, []primitives.ValueComparator{{Value: 0, Operator: primitives.Gt}})
	if err != nil {
		return query.KeyFilter{}, 0, fmt.Errorf("failed to build event sub key filter for state attribute: %w", err)
	}

	keyFilter := query.KeyFilter{
		Key: consts.EventNameExecutionStateChanged,
		Expressions: []query.Expression{
			extendedQuery,
			subKeyFilter,
			query.Confidence(primitives.Finalized),
		},
	}
	return keyFilter, countSqNrs, nil
}

func (a *SolanaAccessor) processExecutionStateChangesEvents(logs []logpollertypes.Log, nonEmptyRangesPerChain map[ccipocr3.ChainSelector][]ccipocr3.SeqNumRange) (map[ccipocr3.ChainSelector][]ccipocr3.SeqNum, error) {
	iter, err := a.decodeLogsIntoSequences(consts.EventNameExecutionStateChanged, logs)
	if err != nil {
		return nil, fmt.Errorf("failed to decode logs into sequences: %w", err)
	}

	executed := make(map[ccipocr3.ChainSelector][]ccipocr3.SeqNum)
	for _, item := range iter {
		stateChange, ok := item.Data.(*ccip.EventExecutionStateChanged)
		if !ok {
			return nil, fmt.Errorf("failed to cast %T to ExecutionStateChangedEvent", item.Data)
		}

		if err := validateExecutionStateChangedEvent(stateChange, nonEmptyRangesPerChain); err != nil {
			a.lggr.Errorw("execution state changed event validation failed",
				"err", err, "stateChange", stateChange)
			continue
		}

		a.lggr.Debugw("decoded executed event", "event", stateChange)

		executed[ccipocr3.ChainSelector(stateChange.SourceChainSelector)] = append(executed[ccipocr3.ChainSelector(stateChange.SourceChainSelector)], ccipocr3.SeqNum(stateChange.SequenceNumber))
	}

	a.lggr.Debugw("executed results", "map", executed)

	return executed, nil
}

func validateExecutionStateChangedEvent(
	ev *ccip.EventExecutionStateChanged, rangesByChain map[ccipocr3.ChainSelector][]ccipocr3.SeqNumRange,
) error {
	if ev == nil {
		return errors.New("execution state changed event is nil")
	}

	if _, ok := rangesByChain[ccipocr3.ChainSelector(ev.SourceChainSelector)]; !ok {
		return errors.New("source chain of messages was not queries")
	}

	if !ccipocr3.SeqNum(ev.SequenceNumber).IsWithinRanges(rangesByChain[ccipocr3.ChainSelector(ev.SourceChainSelector)]) {
		return errors.New("execution state changed event sequence number is not in the expected range")
	}

	if ev.MessageHash == [32]byte{} {
		return errors.New("empty message hash")
	}

	if ev.MessageID == [32]byte{} {
		return errors.New("message ID is empty")
	}

	if ev.State == 0 {
		return errors.New("state is zero")
	}

	return nil
}

func createCCTPMessageSentQueryExpressions(cctpData map[ccipocr3.MessageTokenID]reader.SourceTokenDataPayload) ([]query.Expression, error) {
	attributeIndexes, ok := eventFilterSubkeyIndexMap[consts.EventNameCCTPMessageSent]
	if !ok {
		return nil, fmt.Errorf("failed to find attribute indexes for event %s", consts.EventNameCCTPMessageSent)
	}
	nonceAttributeIndex, ok := attributeIndexes[consts.EventAttributeCCTPNonce]
	if !ok {
		return nil, fmt.Errorf("failed to find index for attribute %s for event %s", consts.EventAttributeCCTPNonce, consts.EventNameCCTPMessageSent)
	}
	srcDomainAttributeIndex, ok := attributeIndexes[consts.EventAttributeSourceDomain]
	if !ok {
		return nil, fmt.Errorf("failed to find index for attribute %s for event %s", consts.EventAttributeSourceDomain, consts.EventNameCCTPMessageSent)
	}

	// Query the token pool contract for the MessageSent event data.
	cctpExpressions := []query.Expression{}
	for _, data := range cctpData {
		// This is much more expensive than the EVM version. Rather than a
		// single ANY expression, we have separate expressions for each
		// nonce and source domain pair. This is because Solana doesn't have
		// a combined ID like EVM does.
		// TODO: optimize. CR modifier or a new field in our event.

		nonceSubKeyFilter, err := logpoller.NewEventBySubKeyFilter(nonceAttributeIndex, []primitives.ValueComparator{{Value: data.Nonce, Operator: primitives.Eq}})
		if err != nil {
			return nil, fmt.Errorf("failed to build event sub key filter for cctp nonce attribute: %w", err)
		}

		srcDomainSubKeyFilter, err := logpoller.NewEventBySubKeyFilter(srcDomainAttributeIndex, []primitives.ValueComparator{{Value: data.SourceDomain, Operator: primitives.Eq}})
		if err != nil {
			return nil, fmt.Errorf("failed to build event sub key filter for cctp source domain attribute: %w", err)
		}

		cctpExpressions = append(cctpExpressions, query.And(
			nonceSubKeyFilter,
			srcDomainSubKeyFilter,
		))
	}

	return cctpExpressions, nil
}

// getMessageTokenData extracts token data from the CCTP MessageSent event.
func getMessageTokenData(
	tokens map[ccipocr3.MessageTokenID]ccipocr3.RampTokenAmount,
) (map[ccipocr3.MessageTokenID]reader.SourceTokenDataPayload, error) {
	messageTransmitterEvents := make(map[ccipocr3.MessageTokenID]reader.SourceTokenDataPayload)

	for id, token := range tokens {
		sourceTokenPayload, err := extractABIPayload(token.ExtraData)
		if err != nil || sourceTokenPayload == nil {
			return nil, err
		}
		messageTransmitterEvents[id] = *sourceTokenPayload
	}
	return messageTransmitterEvents, nil
}

// extractABIPayload manually parses the nonce and sourceDomain out of the extra data field.
// The ABI format is used on EVM and Solana. There is no re-encoding between chains, so other new
// chains should use manual formatting as well. This is specific to CCTPv1.
func extractABIPayload(extraData ccipocr3.Bytes) (*reader.SourceTokenDataPayload, error) {
	if len(extraData) < 64 {
		return nil, fmt.Errorf("extraData is too short, expected at least 64 bytes, got %d", len(extraData))
	}

	// Extract the nonce (first 8 bytes), padded to 32 bytes
	nonce := binary.BigEndian.Uint64(extraData[24:32])
	// Extract the sourceDomain (next 4 bytes), padded to 32 bytes
	sourceDomain := binary.BigEndian.Uint32(extraData[60:64])

	return &reader.SourceTokenDataPayload{
		Nonce:        nonce,
		SourceDomain: sourceDomain,
	}, nil
}

func (a *SolanaAccessor) processCCTPMessageSentEvents(
	logs []logpollertypes.Log,
	source ccipocr3.ChainSelector,
	tokens map[ccipocr3.MessageTokenID]ccipocr3.RampTokenAmount,
	cctpData map[ccipocr3.MessageTokenID]reader.SourceTokenDataPayload,
) (map[ccipocr3.MessageTokenID]ccipocr3.Bytes, error) {
	iter, err := a.decodeLogsIntoSequences(consts.EventNameCCTPMessageSent, logs)
	if err != nil {
		return nil, fmt.Errorf("failed to decode logs into sequences: %w", err)
	}

	msgs := make(map[ccipocr3.MessageTokenID]ccipocr3.Bytes)
	for _, item := range iter {
		event, ok1 := item.Data.(*ccip.EventCcipCctpMessageSent)
		if !ok1 {
			return nil, fmt.Errorf("failed to cast %v to Message", item.Data)
		}

		if err := validateCCTPMessageSentEvent(event); err != nil {
			a.lggr.Errorw("cctp message event validation failed", "error", err, "event", event)
			continue
		}

		// This is O(n^2). We could optimize it by storing the cctpData in a map with a composite key.
		for tokenID, metadata := range cctpData {
			if metadata.Nonce == event.CctpNonce && metadata.SourceDomain == event.SourceDomain {
				msgs[tokenID] = event.MessageSentBytes
				a.lggr.Debugw("Found CCTP event", "tokenID", tokenID, "event", event)
				break
			}
		}

		a.lggr.Warnw("Found unexpected CCTP event", "event", event)
	}

	// Check if any were missed.
	for tokenID := range tokens {
		if _, ok := msgs[tokenID]; !ok {
			// Token is not available in the source chain, it should never happen at this stage
			a.lggr.Warnw("Message not found in the source chain",
				"seqNr", tokenID.SeqNr,
				"tokenIndex", tokenID.Index,
				"chainSelector", source,
				"data", cctpData[tokenID],
			)
		}
	}

	return msgs, nil
}

func validateCCTPMessageSentEvent(event *ccip.EventCcipCctpMessageSent) error {
	if event == nil {
		return errors.New("cctp message sent event is nil")
	}
	if event.MessageSentBytes == nil {
		return errors.New("message sent bytes is empty")
	}
	return nil
}

func (a *SolanaAccessor) decodeLogsIntoSequences(
	event string,
	logs []logpollertypes.Log,
) ([]types.Sequence, error) {
	sequences := make([]types.Sequence, len(logs))

	for idx := range logs {
		sequences[idx] = types.Sequence{
			Cursor: logpoller.FormatContractReaderCursor(logs[idx]),
			Head: types.Head{
				Height:    fmt.Sprint(logs[idx].BlockNumber),
				Hash:      solana.PublicKey(logs[idx].BlockHash).Bytes(),
				Timestamp: uint64(logs[idx].BlockTimestamp.Unix()), //nolint:gosec // BlockTimestamp can never be negative so it is safe to cast it to uint64
			},
		}

		switch event {
		case consts.EventNameCommitReportAccepted:
			e := &ccip.EventCommitReportAccepted{}
			// if the event is `EventCommitReportAccepted`, we need to handle it separately
			derr := decodeCommitReportAcceptedEvent(logs[idx].Data, e)
			if derr != nil {
				return nil, derr
			}
			sequences[idx].Data = e
		case consts.EventNameCCIPMessageSent:
			e := &ccip.EventCCIPMessageSent{}
			if err := bin.UnmarshalBorsh(e, logs[idx].Data); err != nil {
				return nil, err
			}
			sequences[idx].Data = e
			a.lggr.Infow("Decoded CCIPMessageSent event",
				"seqNum", e.Message.Header.SequenceNumber,
				"sourceChain", e.Message.Header.SourceChainSelector,
				"destChain", e.Message.Header.DestChainSelector,
				"filterID", logs[idx].FilterID,
				"address", logs[idx].Address.ToSolana().String(),
				"eventSig", logs[idx].EventSig.String(),
			)
		case consts.EventNameExecutionStateChanged:
			e := &ccip.EventExecutionStateChanged{}
			if err := bin.UnmarshalBorsh(e, logs[idx].Data); err != nil {
				return nil, err
			}
			sequences[idx].Data = e
		case consts.EventNameCCTPMessageSent:
			e := &ccip.EventCcipCctpMessageSent{}
			if err := bin.UnmarshalBorsh(e, logs[idx].Data); err != nil {
				return nil, err
			}
			sequences[idx].Data = e
		default:
			return nil, fmt.Errorf("unsupported event %s", event)
		}
	}

	return sequences, nil
}

func decodeCommitReportAcceptedEvent(data []byte, obj *ccip.EventCommitReportAccepted) error {
	decoder := bin.NewBorshDecoder(data)

	// Deserialize `Discriminator`:
	err := decoder.Decode(&obj.Discriminator)
	if err != nil {
		return err
	}

	// Deserialize `Report` (optional):
	{
		ok, dErr := decoder.ReadBool()
		if dErr != nil {
			return dErr
		}
		if ok {
			dErr = decoder.Decode(&obj.Report)
			if dErr != nil {
				return dErr
			}
		}
	}
	// Deserialize `PriceUpdates`:
	err = decoder.Decode(&obj.PriceUpdates)
	if err != nil {
		return err
	}
	return nil
}

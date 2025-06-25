package writetarget

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	chainselectors "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget"
	monitor "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/beholder"
	df "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/data-feeds/on-chain/registry"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/report/platform/processor"
	"github.com/smartcontractkit/chainlink-solana/contracts/target/idl"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

func New(ctx context.Context, relayer types.Relayer, chain solana.Chain, lggr logger.Logger) (capabilities.ExecutableCapability, error) {
	chainID := chain.ID()

	id := generateWriteTargetName(chainID)
	cfg := chain.Config().WT()

	contractWriterCfgEncoded, err := getContractWriterCfg(cfg.NodeAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal contract writer config %w", err)
	}

	cw, err := relayer.NewContractWriter(ctx, contractWriterCfgEncoded)
	if err != nil {
		return nil, err
	}

	contractReaderEncoded, err := getContractReaderCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal contract reader config %w", err)
	}

	cr, err := relayer.NewContractReader(ctx, contractReaderEncoded)
	if err != nil {
		return nil, err
	}

	chainInfo, err := getChainInfo(chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to get chain info: %w", err)
	}

	// TODO metrics used to initialize product processors
	_, err = df.NewMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to create new registry metrics: %w", err)
	}

	emitter := writetarget.NewMonitorEmitter(lggr)

	processors, err := processor.NewPlatformProcessors(emitter)
	if err != nil {
		return nil, fmt.Errorf("failed to create solana platform processors: %w", err)
	}

	// TODO implement products processors
	//processors["evm-data-feeds"] = dfProcessor
	//processors["evm-data-feeds-ccip"] = ccipDfProcessor
	//processors["evm-por-feeds"] = porProcessor

	beholder, err := writetarget.NewMonitor(writetarget.MonitorOpts{
		Lggr:              lggr,
		Processors:        processors,
		EnabledProcessors: processor.PlatformDefaultProcessors,
		Emitter:           emitter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create solana WT monitor client: %+w", err)
	}

	opts := writetarget.WriteTargetOpts{
		ID:     id,
		Logger: lggr,
		Config: writetarget.Config{
			PollPeriod:        cfg.PollPeriod,
			AcceptanceTimeout: cfg.AcceptanceTimeout,
		},
		ChainInfo:            chainInfo,
		Beholder:             beholder,
		ChainService:         chain,
		ConfigValidateFn:     evaluate,
		NodeAddress:          cfg.NodeAddress,
		ForwarderAddress:     cfg.ForwarderAddress,
		TargetStrategy:       newTargetStrategy(cw, cr, cfg.ForwarderAddress, lggr),
		WriteAcceptanceState: cfg.AcceptanceState,
	}

	return writetarget.NewWriteTarget(opts), nil
}

func evaluate(request capabilities.CapabilityRequest) (string, error) {
	// TODO evaluate request
	return "", nil
}

const (
	forwarderProgram = "forwarder"
	reportMethod     = "report"
)

// TODO
func getContractReaderCfg() ([]byte, error) {
	cfg := config.ContractReader{
		//TODO
	}

	return json.Marshal(cfg)
}

func getContractWriterCfg(fromAddress string) ([]byte, error) {
	idl := idl.FetchForwarderIDL()

	var forwarderIDL codec.IDL
	err := json.Unmarshal([]byte(idl), &forwarderIDL)
	if err != nil {
		return nil, err
	}

	cfg := chainwriter.ChainWriterConfig{
		Programs: map[string]chainwriter.ProgramConfig{
			forwarderProgram: chainwriter.ProgramConfig{
				IDL: idl,
				Methods: map[string]chainwriter.MethodConfig{
					reportMethod: getReportMethodConfig(fromAddress),
				},
			},
		},
	}

	return json.Marshal(cfg)
}

func getReportMethodConfig(fromAddress string) chainwriter.MethodConfig {
	return chainwriter.MethodConfig{
		FromAddress:       fromAddress,
		ChainSpecificName: reportMethod,
		//TODO
	}
}

func generateWriteTargetName(chainID string) string {
	id := fmt.Sprintf("write_%v@1.0.0", chainID)

	chainName, err := chainselectors.SolanaNameFromChainId(chainID)
	if err == nil {
		wtID, err := writetarget.NewWriteTargetID("", chainName, chainID, "1.0.0")
		if err == nil {
			id = wtID
		}
	}

	return id
}

func getChainInfo(chainID string) (monitor.ChainInfo, error) {
	chainSelector := chainselectors.SolanaChainIdToChainSelector()[chainID]
	chainFamily, err := chainselectors.GetSelectorFamily(chainSelector)
	if err != nil {
		return monitor.ChainInfo{}, fmt.Errorf("failed to get chain family for selector %d: %w", chainSelector, err)
	}
	chainDetails, err := chainselectors.GetChainDetailsByChainIDAndFamily(chainID, chainFamily)
	if err != nil {
		return monitor.ChainInfo{}, fmt.Errorf("failed to get chain details for chain %d and family %s: %w", chainID, chainFamily, err)
	}

	neworkName, err := extractNetwork(chainDetails.ChainName)
	if err != nil {
		return monitor.ChainInfo{}, fmt.Errorf("failed to get network name for chain %d: %w", chainID, err)
	}

	return monitor.ChainInfo{
		FamilyName:      chainFamily,
		ChainID:         chainID,
		NetworkName:     neworkName,
		NetworkNameFull: chainDetails.ChainName,
	}, nil
}

func extractNetwork(selector string) (string, error) {
	// Create a regexp pattern that matches any of the three.
	re := regexp.MustCompile(`(mainnet|testnet|devnet)`)
	name := re.FindString(selector)
	if name == "" {
		return "", fmt.Errorf("failed to extract network name from selector: %s", selector)
	}
	return name, nil
}

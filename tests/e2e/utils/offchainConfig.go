package utils

import (
	"github.com/smartcontractkit/integrations-framework/contracts"
	"github.com/smartcontractkit/libocr/offchainreporting2/confighelper"
)

func chunkSlice(items []byte, chunkSize int) (chunks [][]byte) {
	for chunkSize < len(items) {
		chunks = append(chunks, items[0:chunkSize])
		items = items[chunkSize:]
	}
	return append(chunks, items)
}

// NewOCR2ConfigChunks chunk offchain config to mitigate Solana tx limit (~1600 bytes encoded)
func NewOCR2ConfigChunks(cfg contracts.OffChainAggregatorV2Config) (
	version uint64,
	offchainConfigChunks [][]byte,
	err error,
) {
	_, _, _, _, version, cfgBytes, err := confighelper.ContractSetConfigArgsForTests(
		cfg.DeltaProgress,
		cfg.DeltaResend,
		cfg.DeltaRound,
		cfg.DeltaGrace,
		cfg.DeltaStage,
		cfg.RMax,
		cfg.S,
		cfg.Oracles,
		cfg.ReportingPluginConfig,
		cfg.MaxDurationQuery,
		cfg.MaxDurationObservation,
		cfg.MaxDurationReport,
		cfg.MaxDurationShouldAcceptFinalizedReport,
		cfg.MaxDurationShouldTransmitAcceptedReport,
		cfg.F,
		cfg.OnchainConfig,
	)
	if err != nil {
		return 0, nil, err
	}
	return version, chunkSlice(cfgBytes, 1000), nil
}

package solclient

import (
	"time"

	"github.com/smartcontractkit/libocr/offchainreporting2/confighelper"
)

// OffChainAggregatorV2Config replaces contracts.OffChainAggregatorV2Config from
// chainlink/integration-tests/contracts. Contains all parameters needed for
// confighelper.ContractSetConfigArgsForTests.
type OffChainAggregatorV2Config struct {
	DeltaProgress                           Duration
	DeltaResend                             Duration
	DeltaRound                              Duration
	DeltaGrace                              Duration
	DeltaStage                              Duration
	MaxDurationQuery                        Duration
	MaxDurationObservation                  Duration
	MaxDurationReport                       Duration
	MaxDurationShouldAcceptFinalizedReport  Duration
	MaxDurationShouldTransmitAcceptedReport Duration
	RMax                                    uint8
	S                                       []int
	Oracles                                 []confighelper.OracleIdentityExtra
	ReportingPluginConfig                   []byte
	F                                       int
	OnchainConfig                           []byte
}

// Duration wraps time.Duration to match the Duration() accessor pattern used
// by the contracts.OffChainAggregatorV2Config it replaces.
type Duration time.Duration

func (d Duration) Duration() time.Duration { return time.Duration(d) }

// OffchainAggregatorData represents on-chain data from an OCR2 aggregator.
type OffchainAggregatorData struct {
	LatestRoundData RoundData
}

type RoundData struct {
	RoundID         uint32
	Answer          int64
	StartedAt       uint64
	UpdatedAt       uint64
	AnsweredInRound uint32
}

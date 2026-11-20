package config

import (
	_ "embed"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/config/configtest"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-framework/multinode"
	mnCfg "github.com/smartcontractkit/chainlink-framework/multinode/config"
)

func TestDefaults_fieldsNotNil(t *testing.T) {
	configtest.AssertFieldsNotNil(t, Defaults())
}

func TestDocsTOMLComplete(t *testing.T) {
	configtest.AssertDocsTOMLComplete[TOMLConfig](t, docsTOML)
}

//go:embed testdata/config-full.toml
var fullTOML string

var fullConfig = TOMLConfig{
	ChainID: new("fake-chain"),
	Enabled: new(true),
	Chain: Chain{
		BlockTime:                 config.MustNewDuration(time.Hour),
		BalancePollPeriod:         config.MustNewDuration(time.Minute),
		ConfirmPollPeriod:         config.MustNewDuration(time.Second),
		OCR2CachePollPeriod:       config.MustNewDuration(time.Minute),
		OCR2CacheTTL:              config.MustNewDuration(time.Hour),
		TxTimeout:                 config.MustNewDuration(time.Hour),
		TxRetryTimeout:            config.MustNewDuration(time.Minute),
		TxConfirmTimeout:          config.MustNewDuration(time.Second),
		TxExpirationRebroadcast:   new(false),
		TxRetentionTimeout:        config.MustNewDuration(0 * time.Second),
		SkipPreflight:             new(true),
		Commitment:                new("banana"),
		MaxRetries:                new(int64(7)),
		FeeEstimatorMode:          new("fixed"),
		ComputeUnitPriceMax:       new(uint64(1000)),
		ComputeUnitPriceMin:       new(uint64(10)),
		ComputeUnitPriceDefault:   new(uint64(100)),
		FeeBumpPeriod:             config.MustNewDuration(time.Minute),
		BlockHistoryPollPeriod:    config.MustNewDuration(time.Minute),
		BlockHistorySize:          new(uint64(1)),
		BlockHistoryBatchLoadSize: new(uint64(10)),
		ComputeUnitLimitDefault:   new(uint32(100_000)),
		EstimateComputeUnitLimit:  new(false),
		LogPollerStartingLookback: config.MustNewDuration(24 * time.Hour),
		LogPollerCPIEventsEnabled: new(true),
		LogPollerSlotsBatchSize:   new(int64(1000)),
	},
	Workflow: WorkflowConfig{
		AcceptanceTimeout: config.MustNewDuration(42 * time.Second),
		ForwarderAddress:  new(solana.MustPublicKeyFromBase58("14grJpemFaf88c8tiVb77W7TYg2W3ir6pfkKz3YjhhZ5")),
		ForwarderState:    new(solana.MustPublicKeyFromBase58("14grJpemFaf88c8tiVb77W7TYg2W3ir6pfkKz3YjhhZ5")),
		FromAddress:       new(solana.MustPublicKeyFromBase58("4BJXYkfvg37zEmBbsacZjeQDpTNx91KppxFJxRqrz48e")),
		GasLimitDefault:   new(uint64(3_000_000)),
		Local:             new(true),
		PollPeriod:        config.MustNewDuration(9 * time.Second),
		TxAcceptanceState: new(types.Finalized),
		RequestSizeLimit:  new(uint32(1048576)),
	},
	MultiNode: mnCfg.MultiNodeConfig{
		MultiNode: mnCfg.MultiNode{
			Enabled:                      new(false),
			PollFailureThreshold:         new(uint32(5)),
			PollInterval:                 config.MustNewDuration(time.Second),
			SelectionMode:                new(multinode.NodeSelectionModeHighestHead),
			SyncThreshold:                new(uint32(5)),
			NodeIsSyncingEnabled:         new(false),
			LeaseDuration:                config.MustNewDuration(time.Minute),
			NewHeadsPollInterval:         config.MustNewDuration(2 * time.Second),
			FinalizedBlockPollInterval:   config.MustNewDuration(3 * time.Second),
			EnforceRepeatableRead:        new(true),
			DeathDeclarationDelay:        config.MustNewDuration(2 * time.Minute),
			VerifyChainID:                new(true),
			NodeNoNewHeadsThreshold:      config.MustNewDuration(3 * time.Minute),
			NoNewFinalizedHeadsThreshold: config.MustNewDuration(time.Hour),
			FinalityDepth:                new(uint32(0)),
			FinalityTagEnabled:           new(true),
			FinalizedBlockOffset:         new(uint32(0)),
		},
	},
	Nodes: Nodes{
		{
			Name:              new("primary"),
			URL:               config.MustParseURL("http://solana.web"),
			Order:             new(int32(1)),
			IsLoadBalancedRPC: new(false),
		},
		{
			Name:              new("foo"),
			URL:               config.MustParseURL("http://solana.foo"),
			SendOnly:          true,
			Order:             new(int32(2)),
			IsLoadBalancedRPC: new(true),
		},
		{
			Name:              new("bar"),
			URL:               config.MustParseURL("http://solana.bar"),
			SendOnly:          true,
			Order:             new(int32(2)),
			IsLoadBalancedRPC: new(true),
		},
	},
}

func TestTOMLConfig_FullMarshal(t *testing.T) {
	configtest.AssertFullMarshal(t, fullConfig, fullTOML)
}

func TestTOMLConfig_SetFrom(t *testing.T) {
	var config TOMLConfig
	config.SetFrom(&fullConfig)
	require.Equal(t, fullConfig, config)
}

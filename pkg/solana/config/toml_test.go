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
	ChainID: ptr("fake-chain"),
	Enabled: ptr(true),
	Chain: Chain{
		BlockTime:                 config.MustNewDuration(time.Hour),
		BalancePollPeriod:         config.MustNewDuration(time.Minute),
		ConfirmPollPeriod:         config.MustNewDuration(time.Second),
		OCR2CachePollPeriod:       config.MustNewDuration(time.Minute),
		OCR2CacheTTL:              config.MustNewDuration(time.Hour),
		TxTimeout:                 config.MustNewDuration(time.Hour),
		TxRetryTimeout:            config.MustNewDuration(time.Minute),
		TxConfirmTimeout:          config.MustNewDuration(time.Second),
		TxExpirationRebroadcast:   ptr(false),
		TxRetentionTimeout:        config.MustNewDuration(0 * time.Second),
		SkipPreflight:             ptr(true),
		Commitment:                ptr("banana"),
		MaxRetries:                ptr[int64](7),
		FeeEstimatorMode:          ptr("fixed"),
		ComputeUnitPriceMax:       ptr[uint64](1000),
		ComputeUnitPriceMin:       ptr[uint64](10),
		ComputeUnitPriceDefault:   ptr[uint64](100),
		FeeBumpPeriod:             config.MustNewDuration(time.Minute),
		BlockHistoryPollPeriod:    config.MustNewDuration(time.Minute),
		BlockHistorySize:          ptr[uint64](1),
		BlockHistoryBatchLoadSize: ptr[uint64](10),
		ComputeUnitLimitDefault:   ptr[uint32](100_000),
		EstimateComputeUnitLimit:  ptr(false),
		LogPollerStartingLookback: config.MustNewDuration(24 * time.Hour),
		LogPollerCPIEventsEnabled: ptr(true),
	},
	Workflow: WorkflowConfig{
		AcceptanceTimeout: config.MustNewDuration(42 * time.Second),
		GasLimitDefault:   ptr[uint64](3_000_000),
		ForwarderAddress:  ptr(solana.MustPublicKeyFromBase58("14grJpemFaf88c8tiVb77W7TYg2W3ir6pfkKz3YjhhZ5")),
		ForwarderState:    ptr(solana.MustPublicKeyFromBase58("4BJXYkfvg37zEmBbsacZjeQDpTNx91KppxFJxRqrz48e")),
		FromAddress:       ptr(solana.MustPublicKeyFromBase58("14grJpemFaf88c8tiVb77W7TYg2W3ir6pfkKz3YjhhZ5")),
		Local:             ptr(true),
		PollPeriod:        config.MustNewDuration(9 * time.Second),
		TxAcceptanceState: ptr(types.Finalized),
	},
	MultiNode: mnCfg.MultiNodeConfig{
		MultiNode: mnCfg.MultiNode{
			Enabled:                      ptr(false),
			PollFailureThreshold:         ptr[uint32](5),
			PollInterval:                 config.MustNewDuration(time.Second),
			SelectionMode:                ptr(multinode.NodeSelectionModeHighestHead),
			SyncThreshold:                ptr[uint32](5),
			NodeIsSyncingEnabled:         ptr(false),
			LeaseDuration:                config.MustNewDuration(time.Minute),
			NewHeadsPollInterval:         config.MustNewDuration(2 * time.Second),
			FinalizedBlockPollInterval:   config.MustNewDuration(3 * time.Second),
			EnforceRepeatableRead:        ptr(true),
			DeathDeclarationDelay:        config.MustNewDuration(2 * time.Minute),
			VerifyChainID:                ptr(true),
			NodeNoNewHeadsThreshold:      config.MustNewDuration(3 * time.Minute),
			NoNewFinalizedHeadsThreshold: config.MustNewDuration(time.Hour),
			FinalityDepth:                ptr[uint32](0),
			FinalityTagEnabled:           ptr(true),
			FinalizedBlockOffset:         ptr[uint32](0),
		},
	},
	Nodes: Nodes{
		{
			Name:              ptr("primary"),
			URL:               config.MustParseURL("http://solana.web"),
			Order:             ptr[int32](1),
			IsLoadBalancedRPC: ptr(false),
		},
		{
			Name:              ptr("foo"),
			URL:               config.MustParseURL("http://solana.foo"),
			SendOnly:          true,
			Order:             ptr[int32](2),
			IsLoadBalancedRPC: ptr(true),
		},
		{
			Name:              ptr("bar"),
			URL:               config.MustParseURL("http://solana.bar"),
			SendOnly:          true,
			Order:             ptr[int32](2),
			IsLoadBalancedRPC: ptr(true),
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

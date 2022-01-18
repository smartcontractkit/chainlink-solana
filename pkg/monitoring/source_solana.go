package monitoring

import (
	"context"
	"fmt"

	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"github.com/smartcontractkit/chainlink/core/logger"
)

func NewSolanaSourceFactory(log logger.Logger) SourceFactory {
	return &sourceFactory{log}
}

type sourceFactory struct {
	log logger.Logger
}

func (s *sourceFactory) NewSources(
	chainConfig SolanaConfig,
	feedConfig FeedConfig,
) (Sources, error) {
	solanaFeedConfig := feedConfig.(SolanaFeedConfig)
	spec := pkgSolana.OCR2Spec{
		ProgramID: solanaFeedConfig.ContractAddress,
		StateID:   solanaFeedConfig.StateAccount,
	}
	client := pkgSolana.NewClient(chainConfig.RPCEndpoint)
	tracker := pkgSolana.NewTracker(spec, client, nil, s.log)
	return &sources{
		&tracker,
		chainConfig,
		solanaFeedConfig,
	}, nil
}

type sources struct {
	tracker     *pkgSolana.ContractTracker
	chainConfig SolanaConfig
	feedConfig  SolanaFeedConfig
}

func (s *sources) NewTransmissionsSource() Source {
	return &transmissionsSource{s.tracker, s.chainConfig, s.feedConfig}
}

func (s *sources) NewConfigSource() Source {
	return &configSource{s.tracker, s.chainConfig, s.feedConfig}
}

type transmissionsSource struct {
	tracker     *pkgSolana.ContractTracker
	chainConfig SolanaConfig
	feedConfig  SolanaFeedConfig
}

func (t *transmissionsSource) Fetch(ctx context.Context) (interface{}, error) {
	configDigest, epoch, round, latestAnswer, latestTimestamp, err := t.tracker.LatestTransmissionDetails(ctx)
	if err != nil {
		return TransmissionEnvelope{}, fmt.Errorf("failed to read latest transmission from on-chain for feed '%s'", t.feedConfig.GetName())
	}
	return TransmissionEnvelope{configDigest, epoch, round, latestAnswer, latestTimestamp}, err
}

type configSource struct {
	tracker     *pkgSolana.ContractTracker
	chainConfig SolanaConfig
	feedConfig  SolanaFeedConfig
}

func (c *configSource) Fetch(ctx context.Context) (interface{}, error) {
	cfg, err := c.tracker.LatestConfig(ctx, 0)
	if err != nil {
		return ConfigEnvelope{}, fmt.Errorf("failed to read transmissions from on-chain for feed '%s'", c.feedConfig.GetName())
	}
	return ConfigEnvelope{cfg}, nil
}

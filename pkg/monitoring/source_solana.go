package monitoring

import (
	"context"
	"fmt"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
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
	chainConfig config.Solana,
	feedConfig Feed,
) (Sources, error) {
	spec := pkgSolana.OCR2Spec{
		ProgramID: feedConfig.ContractAddress,
		StateID:   feedConfig.StateAccount,
	}
	client := pkgSolana.NewClient(chainConfig.RPCEndpoint)
	tracker := pkgSolana.NewTracker(spec, client, nil, s.log)
	return &sources{
		&tracker,
		chainConfig,
		feedConfig,
	}, nil
}

type sources struct {
	tracker     *pkgSolana.ContractTracker
	chainConfig config.Solana
	feedConfig  Feed
}

func (s *sources) NewTransmissionsSource() Source {
	return &transmissionsSource{s.tracker, s.chainConfig, s.feedConfig}
}

func (s *sources) NewConfigSource() Source {
	return &configSource{s.tracker, s.chainConfig, s.feedConfig}
}

type transmissionsSource struct {
	tracker     *pkgSolana.ContractTracker
	chainConfig config.Solana
	feedConfig  Feed
}

func (t *transmissionsSource) Fetch(ctx context.Context) (interface{}, error) {
	configDigest, epoch, round, latestAnswer, latestTimestamp, err := t.tracker.LatestTransmissionDetails(ctx)
	if err != nil {
		return TransmissionEnvelope{}, fmt.Errorf("failed to read latest transmission from on-chain for feed '%s'", t.feedConfig.FeedName)
	}
	return TransmissionEnvelope{configDigest, epoch, round, latestAnswer, latestTimestamp}, err
}

type configSource struct {
	tracker     *pkgSolana.ContractTracker
	chainConfig config.Solana
	feedConfig  Feed
}

func (c *configSource) Fetch(ctx context.Context) (interface{}, error) {
	cfg, err := c.tracker.LatestConfig(ctx, 0)
	if err != nil {
		return ConfigEnvelope{}, fmt.Errorf("failed to read transmissions from on-chain for feed '%s'", c.feedConfig.FeedName)
	}
	return ConfigEnvelope{cfg}, nil
}

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

func (s *sourceFactory) NewSource(
	chainConfig ChainConfig,
	feedConfig FeedConfig,
) (Source, error) {
	solanaConfig, ok := chainConfig.(SolanaConfig)
	if !ok {
		return nil, fmt.Errorf("expected chainConfig to be of type SolanaConfig not %T", chainConfig)
	}
	solanaFeedConfig, ok := feedConfig.(SolanaFeedConfig)
	if !ok {
		return nil, fmt.Errorf("expected feedConfig to be of type SolanaFeedConfig not %T", feedConfig)
	}
	spec := pkgSolana.OCR2Spec{
		ProgramID: solanaFeedConfig.ContractAddress,
		StateID:   solanaFeedConfig.StateAccount,
	}
	client := pkgSolana.NewClient(solanaConfig.RPCEndpoint)
	tracker := pkgSolana.NewTracker(spec, client, nil, s.log)
	return &solanaSource{
		&tracker,
		solanaConfig,
		solanaFeedConfig,
	}, nil
}

type solanaSource struct {
	tracker      *pkgSolana.ContractTracker
	solanaConfig SolanaConfig
	feedConfig   SolanaFeedConfig
}

func (s *solanaSource) Fetch(ctx context.Context) (interface{}, error) {
	changedInBlock, _, err := s.tracker.LatestConfigDetails(ctx)
	if err != nil {
		return Envelope{}, fmt.Errorf("failed to fetch latest config details from on-chain: %w", err)
	}
	cfg, err := s.tracker.LatestConfig(ctx, changedInBlock)
	if err != nil {
		return Envelope{}, fmt.Errorf("failed to read latest config from on-chain: %w", err)
	}
	configDigest, epoch, round, latestAnswer, latestTimestamp, err := s.tracker.LatestTransmissionDetails(ctx)
	if err != nil {
		return Envelope{}, fmt.Errorf("failed to read latest transmission from on-chain: %w", err)
	}
	transmitter := s.tracker.FromAccount()

	return Envelope{
		configDigest,
		epoch,
		round,
		latestAnswer,
		latestTimestamp,

		cfg,

		changedInBlock,
		transmitter,
	}, nil
}

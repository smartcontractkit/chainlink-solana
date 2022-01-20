package monitoring

import (
	"context"
	"fmt"

	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

func NewSolanaSourceFactory(log relayMonitoring.Logger) relayMonitoring.SourceFactory {
	return &sourceFactory{log}
}

type sourceFactory struct {
	log relayMonitoring.Logger
}

func (s *sourceFactory) NewSource(
	chainConfig relayMonitoring.ChainConfig,
	feedConfig relayMonitoring.FeedConfig,
) (relayMonitoring.Source, error) {
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
	tracker := pkgSolana.NewTracker(spec, client, nil, &logAdapter{s.log})
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
		return relayMonitoring.Envelope{}, fmt.Errorf("failed to fetch latest config details from on-chain: %w", err)
	}
	cfg, err := s.tracker.LatestConfig(ctx, changedInBlock)
	if err != nil {
		return relayMonitoring.Envelope{}, fmt.Errorf("failed to read latest config from on-chain: %w", err)
	}
	configDigest, epoch, round, latestAnswer, latestTimestamp, err := s.tracker.LatestTransmissionDetails(ctx)
	if err != nil {
		return relayMonitoring.Envelope{}, fmt.Errorf("failed to read latest transmission from on-chain: %w", err)
	}
	transmitter := s.tracker.FromAccount()

	return relayMonitoring.Envelope{
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

// Helper

type logAdapter struct {
	log relayMonitoring.Logger
}

func (l *logAdapter) Tracef(format string, values ...interface{}) {
	l.log.Tracew(fmt.Sprintf(format, values...))
}

func (l *logAdapter) Debugf(format string, values ...interface{}) {
	l.log.Debugw(fmt.Sprintf(format, values...))
}
func (l *logAdapter) Infof(format string, values ...interface{}) {
	l.log.Infow(fmt.Sprintf(format, values...))
}
func (l *logAdapter) Warnf(format string, values ...interface{}) {
	l.log.Warnw(fmt.Sprintf(format, values...))
}
func (l *logAdapter) Errorf(format string, values ...interface{}) {
	l.log.Errorw(fmt.Sprintf(format, values...))
}
func (l *logAdapter) Criticalf(format string, values ...interface{}) {
	l.log.Criticalw(fmt.Sprintf(format, values...))
}
func (l *logAdapter) Panicf(format string, values ...interface{}) {
	l.log.Panicw(fmt.Sprintf(format, values...))
}
func (l *logAdapter) Fatalf(format string, values ...interface{}) {
	l.log.Fatalw(fmt.Sprintf(format, values...))
}

package monitoring

import (
	"context"
	"fmt"
	"math/big"

	"github.com/gagliardetto/solana-go/rpc"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

func NewSolanaSourceFactory(
	solanaConfig SolanaConfig,
	log relayMonitoring.Logger,
) relayMonitoring.SourceFactory {
	client := rpc.New(solanaConfig.RPCEndpoint)
	pkgClient := pkgSolana.NewClient(solanaConfig.RPCEndpoint)
	return &sourceFactory{
		client,
		pkgClient,
		log,
	}
}

type sourceFactory struct {
	client    *rpc.Client
	pkgClient *pkgSolana.Client
	log       relayMonitoring.Logger
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
	tracker := pkgSolana.NewTracker(spec, s.pkgClient, nil, &logAdapter{s.log})
	return &solanaSource{
		&tracker,
		s.client,
		solanaConfig,
		solanaFeedConfig,
	}, nil
}

type solanaSource struct {
	tracker      *pkgSolana.ContractTracker
	client       *rpc.Client
	solanaConfig SolanaConfig
	feedConfig   SolanaFeedConfig
}

func (s *solanaSource) Fetch(ctx context.Context) (interface{}, error) {
	changedInBlock, _, err := s.tracker.LatestConfigDetails(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest config details from on-chain: %w", err)
	}
	cfg, err := s.tracker.LatestConfig(ctx, changedInBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to read latest config from on-chain: %w", err)
	}
	configDigest, epoch, round, latestAnswer, latestTimestamp, err := s.tracker.LatestTransmissionDetails(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read latest transmission from on-chain: %w", err)
	}
	state, _, err := pkgSolana.GetState(ctx, s.client, s.feedConfig.StateAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to get contract state: %w", err)
	}
	transmitter := types.Account(state.Config.LatestTransmitter.String())

	solBalanceRes, err := s.client.GetBalance(ctx, s.feedConfig.ContractAddress, rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("failed to read the feed's sol balance: %w", err)
	}
	linkBalanceRes, err := s.client.GetTokenAccountBalance(ctx, s.feedConfig.SPLTokenAccount, rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("failed to read the feed's link balance: %w", err)
	}
	linkBalance := big.NewInt(0)
	_, success := linkBalance.SetString(linkBalanceRes.Value.Amount, 10)
	if !success {
		return nil, fmt.Errorf("unable to decode the feed's link balance '%s': %w", linkBalanceRes.Value.Amount, err)
	}

	return relayMonitoring.Envelope{
		configDigest,
		epoch,
		round,
		latestAnswer,
		latestTimestamp,

		cfg,

		changedInBlock,
		transmitter,

		solBalanceRes.Value,
		linkBalance.Uint64(),
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

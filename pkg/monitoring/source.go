package monitoring

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

const (
	commitment = rpc.CommitmentConfirmed
)

func NewSolanaSourceFactory(
	solanaConfig SolanaConfig,
	log relayMonitoring.Logger,
) relayMonitoring.SourceFactory {
	client := rpc.New(solanaConfig.RPCEndpoint)
	return &sourceFactory{
		client,
		log,
	}
}

type sourceFactory struct {
	client *rpc.Client
	log    relayMonitoring.Logger
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
	return &solanaSource{
		s.client,
		solanaConfig,
		solanaFeedConfig,
	}, nil
}

type solanaSource struct {
	client       *rpc.Client
	solanaConfig SolanaConfig
	feedConfig   SolanaFeedConfig
}

func (s *solanaSource) Fetch(ctx context.Context) (interface{}, error) {
	state, blockNum, err := pkgSolana.GetState(ctx, s.client, s.feedConfig.StateAccount, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to state from on-chain: %w", err)
	}
	contractConfig, err := pkgSolana.ConfigFromState(state)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ContractConfig from on-chain state: %w", err)
	}
	answer, _, err := pkgSolana.GetLatestTransmission(ctx, s.client, state.Transmissions, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest on-chain transmission: %w", err)
	}
	linkBalanceRes, err := s.client.GetTokenAccountBalance(ctx, state.Config.TokenVault, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to read the feed's link balance: %w", err)
	}
	if linkBalanceRes.Value == nil {
		return nil, fmt.Errorf("link balance not found for token vault")
	}
	linkBalance, success := big.NewInt(0).SetString(linkBalanceRes.Value.Amount, 10)
	if !success {
		return nil, fmt.Errorf("failed to parse link balance value: %s", linkBalanceRes.Value.Amount)
	}
	return relayMonitoring.Envelope{
		ConfigDigest: state.Config.LatestConfigDigest,
		Epoch:        state.Config.Epoch,
		Round:        state.Config.Round,

		LatestAnswer:    answer.Data,
		LatestTimestamp: time.Unix(int64(answer.Timestamp), 0),

		// latest contract config
		ContractConfig: contractConfig,

		// extra
		BlockNumber: blockNum,
		Transmitter: types.Account(state.Config.LatestTransmitter.String()),
		LinkBalance: linkBalance.Uint64(),
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

// Just to silence golangci-lint
var _ = logAdapter{}

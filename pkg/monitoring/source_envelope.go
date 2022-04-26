package monitoring

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	"go.uber.org/multierr"
)

func NewEnvelopeSourceFactory(
	client *rpc.Client,
	log relayMonitoring.Logger,
) relayMonitoring.SourceFactory {
	return &envelopeSourceFactory{
		client,
		log,
	}
}

type envelopeSourceFactory struct {
	client *rpc.Client
	log    relayMonitoring.Logger
}

func (s *envelopeSourceFactory) NewSource(
	_ relayMonitoring.ChainConfig,
	feedConfig relayMonitoring.FeedConfig,
) (relayMonitoring.Source, error) {
	solanaFeedConfig, ok := feedConfig.(SolanaFeedConfig)
	if !ok {
		return nil, fmt.Errorf("expected feedConfig to be of type SolanaFeedConfig not %T", feedConfig)
	}
	return &envelopeSource{
		s.client,
		solanaFeedConfig,
	}, nil
}

func (s *envelopeSourceFactory) GetType() string {
	return "envelope"
}

type envelopeSource struct {
	client     *rpc.Client
	feedConfig SolanaFeedConfig
}

func (s *envelopeSource) Fetch(ctx context.Context) (interface{}, error) {
	state, blockNum, err := pkgSolana.GetState(ctx, s.client, s.feedConfig.StateAccount, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch state from on-chain: %w", err)
	}
	contractConfig, err := pkgSolana.ConfigFromState(state)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ContractConfig from on-chain state: %w", err)
	}
	var (
		answer      pkgSolana.Answer
		linkBalance *big.Int
		envelopeErr error
	)
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		var transmissionErr error
		answer, _, transmissionErr = pkgSolana.GetLatestTransmission(ctx, s.client, state.Transmissions, rpc.CommitmentConfirmed)
		if err != nil {
			envelopeErr = multierr.Combine(envelopeErr, fmt.Errorf("failed to fetch latest on-chain transmission: %w", transmissionErr))
		}
	}()
	go func() {
		defer wg.Done()
		linkBalanceRes, balanceErr := s.client.GetTokenAccountBalance(ctx, state.Config.TokenVault, rpc.CommitmentConfirmed)
		if balanceErr != nil {
			envelopeErr = multierr.Combine(envelopeErr, fmt.Errorf("failed to read the feed's link balance: %w", balanceErr))
			return
		}
		if linkBalanceRes == nil || linkBalanceRes.Value == nil {
			envelopeErr = multierr.Combine(envelopeErr, fmt.Errorf("link balance not found for token vault"))
			return
		}
		var success bool
		linkBalance, success = big.NewInt(0).SetString(linkBalanceRes.Value.Amount, 10)
		if !success {
			envelopeErr = multierr.Combine(envelopeErr, fmt.Errorf("failed to parse link balance value: %s", linkBalanceRes.Value.Amount))
			return
		}
	}()
	wg.Wait()
	if envelopeErr != nil {
		return nil, envelopeErr
	}
	linkAvailableForPayments, err := getLinkAvailableForPayment(state, linkBalance)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate link_available_for_payments: %w", err)
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
		BlockNumber:             blockNum,
		Transmitter:             types.Account(state.Config.LatestTransmitter.String()),
		LinkBalance:             linkBalance,
		LinkAvailableForPayment: linkAvailableForPayments,

		JuelsPerFeeCoin:   big.NewInt(0), // TODO (dru)
		AggregatorRoundID: state.Config.LatestAggregatorRoundID,
	}, nil
}

// Helpers

func getLinkAvailableForPayment(state pkgSolana.State, linkBalance *big.Int) (*big.Int, error) {
	oracles, err := state.Oracles.Data()
	if err != nil {
		return nil, err
	}
	var countUnpaidRounds, reimbursements uint64 = 0, 0
	for _, oracle := range oracles {
		numRounds := int(state.Config.LatestAggregatorRoundID) - int(oracle.FromRoundID)
		if numRounds < 0 {
			numRounds = 0
		}
		countUnpaidRounds += uint64(numRounds)
		reimbursements += oracle.Payment
	}
	amountDue := uint64(state.Config.Billing.ObservationPayment)*countUnpaidRounds + reimbursements
	remaining := new(big.Int).Sub(linkBalance, new(big.Int).SetUint64(amountDue))
	return remaining, nil
}

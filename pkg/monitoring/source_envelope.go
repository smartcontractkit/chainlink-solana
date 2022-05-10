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
	client ChainReader,
	log relayMonitoring.Logger,
) relayMonitoring.SourceFactory {
	return &envelopeSourceFactory{
		client,
		log,
	}
}

type envelopeSourceFactory struct {
	client ChainReader
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
	client     ChainReader
	feedConfig SolanaFeedConfig
}

func (s *envelopeSource) Fetch(ctx context.Context) (interface{}, error) {
	state, blockNum, err := s.client.GetState(ctx, s.feedConfig.StateAccount, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch state from on-chain: %w", err)
	}
	contractConfig, err := pkgSolana.ConfigFromState(state)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ContractConfig from on-chain state: %w", err)
	}
	envelope := relayMonitoring.Envelope{
		ConfigDigest: state.Config.LatestConfigDigest,
		Epoch:        state.Config.Epoch,
		Round:        state.Config.Round,
		// latest contract config
		ContractConfig: contractConfig,
		// extra
		BlockNumber:       blockNum,
		Transmitter:       types.Account(state.Config.LatestTransmitter.String()),
		JuelsPerFeeCoin:   big.NewInt(0), // TODO (dru)
		AggregatorRoundID: state.Config.LatestAggregatorRoundID,
	}
	var envelopeErr error
	envelopeMu := &sync.Mutex{}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		answer, _, transmissionErr := s.client.GetLatestTransmission(ctx, state.Transmissions, rpc.CommitmentConfirmed)
		envelopeMu.Lock()
		defer envelopeMu.Unlock()
		if transmissionErr != nil {
			envelopeErr = multierr.Combine(envelopeErr, fmt.Errorf("failed to fetch latest on-chain transmission: %w", transmissionErr))
			return
		}
		envelope.LatestAnswer = answer.Data
		envelope.LatestTimestamp = time.Unix(int64(answer.Timestamp), 0)
	}()
	go func() {
		defer wg.Done()
		linkBalanceRes, balanceErr := s.client.GetTokenAccountBalance(ctx, state.Config.TokenVault, rpc.CommitmentConfirmed)
		envelopeMu.Lock()
		defer envelopeMu.Unlock()
		if balanceErr != nil {
			envelopeErr = multierr.Combine(envelopeErr, fmt.Errorf("failed to read the feed's link balance: %w", balanceErr))
			return
		}
		if linkBalanceRes == nil || linkBalanceRes.Value == nil {
			envelopeErr = multierr.Combine(envelopeErr, fmt.Errorf("link balance not found for token vault"))
			return
		}
		linkBalance, success := big.NewInt(0).SetString(linkBalanceRes.Value.Amount, 10)
		if !success {
			envelopeErr = multierr.Combine(envelopeErr, fmt.Errorf("failed to parse link balance value: %s", linkBalanceRes.Value.Amount))
			return
		}
		envelope.LinkBalance = linkBalance
	}()
	wg.Wait()
	if envelopeErr != nil {
		return nil, envelopeErr
	}
	linkAvailableForPayment, err := getLinkAvailableForPayment(state, envelope.LinkBalance)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate link_available_for_payments: %w", err)
	}
	envelope.LinkAvailableForPayment = linkAvailableForPayment
	return envelope, nil
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

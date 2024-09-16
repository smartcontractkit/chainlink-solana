package monitoring

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/event"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

func NewEnvelopeSourceFactory(
	client ChainReader,
	log commonMonitoring.Logger,
) commonMonitoring.SourceFactory {
	return &envelopeSourceFactory{
		client,
		log,
	}
}

type envelopeSourceFactory struct {
	client ChainReader
	log    commonMonitoring.Logger
}

func (s *envelopeSourceFactory) NewSource(
	_ commonMonitoring.ChainConfig,
	feedConfig commonMonitoring.FeedConfig,
) (commonMonitoring.Source, error) {
	solanaFeedConfig, ok := feedConfig.(config.SolanaFeedConfig)
	if !ok {
		return nil, fmt.Errorf("expected feedConfig to be of type config.SolanaFeedConfig not %T", feedConfig)
	}
	return &envelopeSource{
		s.client,
		solanaFeedConfig,
		s.log,
	}, nil
}

func (s *envelopeSourceFactory) GetType() string {
	return "envelope"
}

type envelopeSource struct {
	client     ChainReader
	feedConfig config.SolanaFeedConfig
	log        commonMonitoring.Logger
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
	envelope := commonMonitoring.Envelope{
		ConfigDigest: state.Config.LatestConfigDigest,
		Epoch:        state.Config.Epoch,
		Round:        state.Config.Round,
		// latest contract config
		ContractConfig: contractConfig,
		// extra
		BlockNumber:       blockNum,
		Transmitter:       types.Account(state.Config.LatestTransmitter.String()),
		AggregatorRoundID: state.Config.LatestAggregatorRoundID,
	}
	var envelopeErr error
	envelopeMu := &sync.Mutex{}
	wg := &sync.WaitGroup{}
	wg.Add(3)
	go func() {
		defer wg.Done()
		answer, _, transmissionErr := s.client.GetLatestTransmission(ctx, state.Transmissions, rpc.CommitmentConfirmed)
		envelopeMu.Lock()
		defer envelopeMu.Unlock()
		if transmissionErr != nil {
			envelopeErr = errors.Join(envelopeErr, fmt.Errorf("failed to fetch latest on-chain transmission: %w", transmissionErr))
			return
		}
		envelope.LatestAnswer = answer.Data
		envelope.LatestTimestamp = time.Unix(int64(answer.Timestamp), 0)
	}()
	go func() {
		defer wg.Done()
		linkBalance, linkBalanceErr := s.getLinkBalance(ctx, state.Config.TokenVault)
		envelopeMu.Lock()
		defer envelopeMu.Unlock()
		if linkBalanceErr != nil {
			envelopeErr = errors.Join(envelopeErr, fmt.Errorf("failed to get the feed's link balance: %w", linkBalanceErr))
			return
		}
		envelope.LinkBalance = linkBalance
	}()
	go func() {
		defer wg.Done()
		juelsPerLamport, juelsErr := s.getJuelsPerLamport(ctx)
		envelopeMu.Lock()
		defer envelopeMu.Unlock()
		if juelsErr != nil {
			envelopeErr = errors.Join(envelopeErr, fmt.Errorf("Failed to fetch Juels/FeeCoin: %w", juelsErr))
			return
		}
		envelope.JuelsPerFeeCoin = juelsPerLamport
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

var zeroBigInt = big.NewInt(0)

func (s *envelopeSource) getLinkBalance(ctx context.Context, tokenVault solana.PublicKey) (*big.Int, error) {
	linkBalanceRes, err := s.client.GetTokenAccountBalance(ctx, tokenVault, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, fmt.Errorf("failed to read the feed's link balance: %w", err)
	}
	if linkBalanceRes == nil || linkBalanceRes.Value == nil {
		return nil, fmt.Errorf("link balance not found for token vault")
	}
	linkBalance, success := big.NewInt(0).SetString(linkBalanceRes.Value.Amount, 10)
	if !success {
		return nil, fmt.Errorf("failed to parse link balance value: %s", linkBalanceRes.Value.Amount)
	}
	if linkBalance.Cmp(zeroBigInt) == 0 {
		s.log.Warnw("contract's LINK balance should not be zero", "token_vautlt", tokenVault)
	}
	return linkBalance, nil
}

func (s *envelopeSource) getJuelsPerLamport(ctx context.Context) (*big.Int, error) {
	txSigsPageSize := 100
	txSigs, err := s.client.GetSignaturesForAddressWithOpts(
		ctx,
		s.feedConfig.StateAccount,
		&rpc.GetSignaturesForAddressOpts{
			Commitment: rpc.CommitmentConfirmed,
			Limit:      &txSigsPageSize, // we only need the last tx
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tx signatures for state account '%s': %w", s.feedConfig.StateAccountBase58, err)
	}
	if len(txSigs) == 0 {
		return nil, fmt.Errorf("found no transactions from state account '%s'", s.feedConfig.StateAccountBase58)
	}
	for _, txSig := range txSigs {
		if txSig.Err != nil {
			// We're not interested in failed transactions.
			continue
		}
		txRes, err := s.client.GetTransaction(
			ctx,
			txSig.Signature,
			&rpc.GetTransactionOpts{
				Commitment: rpc.CommitmentConfirmed,
				Encoding:   solana.EncodingBase64,
			},
		)
		if err != nil {
			s.log.Infow("failed to fetch tx", "txSig", txSig.Signature, "err", err)
			continue
		}
		if txRes == nil {
			s.log.Infow("no transaction found for signature", "txSig", txSig.Signature)
			continue
		}
		events := event.ExtractEvents(txRes.Meta.LogMessages, s.feedConfig.ContractAddressBase58)
		for _, rawEvent := range events {
			decodedEvent, err := event.Decode(rawEvent)
			if err != nil {
				s.log.Infow("failed to decode event", "rawEvent", rawEvent, "txSig", txSigs[0].Signature, "err", err)
				continue
			}
			newTransmission, isNewTransmission := decodedEvent.(event.NewTransmission)
			if !isNewTransmission {
				continue
			}
			if newTransmission.JuelsPerLamport == 0 {
				s.log.Infow("zero value for juels/lamport feed is not supported")
				continue
			}
			return new(big.Int).SetUint64(newTransmission.JuelsPerLamport), nil
		}
	}
	return nil, fmt.Errorf("no correct NewTransmission event found in the last %d transactions on contract state '%s'", txSigsPageSize, s.feedConfig.StateAccountBase58)
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

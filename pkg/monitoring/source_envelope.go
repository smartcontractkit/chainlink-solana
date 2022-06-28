package monitoring

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/event"
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
	go func() {
		defer wg.Done()
		juelsPerLamport, juelsErr := s.getJuelsPerLamport(ctx)
		envelopeMu.Lock()
		defer envelopeMu.Unlock()
		if juelsErr != nil {
			envelopeErr = multierr.Combine(envelopeErr, fmt.Errorf("Failed to fetch Juels/FeeCoin: %w", juelsErr))
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
			return nil, fmt.Errorf("failed to fetch tx with signature %s: %w", txSig.Signature, err)
		}
		if txRes == nil {
			return nil, fmt.Errorf("no transaction returned for signature %s", txSig.Signature)
		}
		events := event.ExtractEvents(txRes.Meta.LogMessages, s.feedConfig.ContractAddressBase58)
		for _, rawEvent := range events {
			decodedEvent, err := event.Decode(rawEvent)
			if err != nil {
				return nil, fmt.Errorf("failed decode events '%s' from tx with signature '%s': %w", rawEvent, txSigs[0].Signature, err)
			}
			if newTransmission, isNewTransmission := decodedEvent.(event.NewTransmission); isNewTransmission {
				return new(big.Int).SetUint64(newTransmission.JuelsPerLamport), nil
			}
		}
	}
	return nil, fmt.Errorf("no NewTransmission event found in the last %d transactions on contract state '%s'", txSigsPageSize, s.feedConfig.StateAccountBase58)
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

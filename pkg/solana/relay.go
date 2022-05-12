package solana

import (
	"context"
	"encoding/json"

	"github.com/gagliardetto/solana-go"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/chainlink/core/services/keystore/keys/solkey"
	relaytypes "github.com/smartcontractkit/chainlink/core/services/relay/types"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logger"
)

type TransmissionSigner interface {
	Sign(msg []byte) ([]byte, error)
	PublicKey() solana.PublicKey
}

// TODO: Goes away with solana txm
type KeyStore interface {
	Get(id string) (solkey.Key, error)
}

type TxManager interface {
	Enqueue(accountID string, msg *solana.Transaction) error
}

type Relayer struct {
	lggr     logger.Logger
	chainSet ChainSet
	ks       KeyStore
	ctx      context.Context
	cancel   func()
}

// Note: constructed in core
func NewRelayer(lggr logger.Logger, chainSet ChainSet, ks KeyStore) *Relayer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Relayer{
		lggr:     lggr,
		chainSet: chainSet,
		ks:       ks,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start starts the relayer respecting the given context.
func (r *Relayer) Start(context.Context) error {
	// No subservices started on relay start, but when the first job is started
	if r.chainSet == nil {
		return errors.New("Solana unavailable")
	}
	return nil
}

// Close will close all open subservices
func (r *Relayer) Close() error {
	r.cancel()
	return nil
}

func (r *Relayer) Ready() error {
	return r.chainSet.Ready()
}

// Healthy only if all subservices are healthy
func (r *Relayer) Healthy() error {
	return r.chainSet.Healthy()
}

var _ relaytypes.RelayerCtx = &Relayer{}

func (r *Relayer) NewMedianProvider(args relaytypes.OCR2Args) (relaytypes.MedianProvider, error) {
	relayConfigBytes, err := json.Marshal(args.RelayConfig)
	if err != nil {
		return nil, err
	}
	var relayConfig RelayConfig
	err = json.Unmarshal(relayConfigBytes, &relayConfig)
	if err != nil {
		return nil, err
	}
	stateID, err := solana.PublicKeyFromBase58(args.ContractID)
	if err != nil {
		return nil, errors.Wrap(err, "error on 'solana.PublicKeyFromBase58' for 'spec.ContractID")
	}
	programID, err := solana.PublicKeyFromBase58(relayConfig.OCR2ProgramID)
	if err != nil {
		return nil, errors.Wrap(err, "error on 'solana.PublicKeyFromBase58' for 'spec.RelayConfig.OCR2ProgramID")
	}

	storeProgramID, err := solana.PublicKeyFromBase58(relayConfig.StoreProgramID)
	if err != nil {
		return nil, errors.Wrap(err, "error on 'solana.PublicKeyFromBase58' for 'spec.RelayConfig.StateID")
	}

	transmissionsID, err := solana.PublicKeyFromBase58(relayConfig.TransmissionsID)
	if err != nil {
		return nil, errors.Wrap(err, "error on 'solana.PublicKeyFromBase58' for 'spec.RelayConfig.TransmissionsID")
	}

	var transmissionSigner TransmissionSigner
	if !args.IsBootstrap {
		if !args.TransmitterID.Valid {
			return nil, errors.New("transmitterID is required for non-bootstrap jobs")
		}
		transmissionSigner, err = r.ks.Get(args.TransmitterID.String)
		if err != nil {
			return nil, err
		}
	}
	offchainConfigDigester := OffchainConfigDigester{
		ProgramID: programID,
		StateID:   stateID,
	}

	chain, err := r.chainSet.Chain(r.ctx, relayConfig.ChainID)
	if err != nil {
		return nil, errors.Wrap(err, "error in NewOCR2Provider.chainSet.Chain")
	}
	chainReader, err := chain.Reader()
	if err != nil {
		return nil, errors.Wrap(err, "error in NewOCR2Provider.chain.Reader")
	}
	msgEnqueuer := chain.TxManager()
	cfg := chain.Config()

	// provide contract config + tracker reader + tx manager + signer + logger
	contractTracker := NewTracker(programID, stateID, storeProgramID, transmissionsID, cfg, chainReader, msgEnqueuer, transmissionSigner, r.lggr)

	if args.IsBootstrap {
		// Return early if bootstrap node (doesn't require the full OCR2 provider)
		return &medianProvider{
			offchainConfigDigester: offchainConfigDigester,
			tracker:                &contractTracker,
		}, nil
	}

	reportCodec := ReportCodec{}

	return &medianProvider{
		offchainConfigDigester: offchainConfigDigester,
		reportCodec:            reportCodec,
		tracker:                &contractTracker,
	}, nil
}

type medianProvider struct {
	offchainConfigDigester OffchainConfigDigester
	reportCodec            ReportCodec
	tracker                *ContractTracker
}

// Start starts OCR2Provider respecting the given context.
func (p *medianProvider) Start(context.Context) error {
	return p.tracker.Start()
}

func (p *medianProvider) Close() error {
	return p.tracker.Close()
}

func (p medianProvider) Ready() error {
	// always ready
	return p.tracker.Ready()
}

func (p medianProvider) Healthy() error {
	return p.tracker.Healthy()
}

func (p medianProvider) ContractTransmitter() types.ContractTransmitter {
	return p.tracker
}

func (p medianProvider) ContractConfigTracker() types.ContractConfigTracker {
	return p.tracker
}

func (p medianProvider) OffchainConfigDigester() types.OffchainConfigDigester {
	return p.offchainConfigDigester
}

func (p medianProvider) ReportCodec() median.ReportCodec {
	return p.reportCodec
}

func (p medianProvider) MedianContract() median.MedianContract {
	return p.tracker
}

package solana

import (
	"context"
	"errors"

	"github.com/gagliardetto/solana-go"
	uuid "github.com/satori/go.uuid"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logger"
	relaytypes "github.com/smartcontractkit/chainlink/core/services/relay/types"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

type TransmissionSigner interface {
	Sign(msg []byte) ([]byte, error)
	PublicKey() solana.PublicKey
}

type TxManager interface {
	Enqueue(accountID string, msg *solana.Transaction) error
}

type OCR2Spec struct {
	ID          int32
	IsBootstrap bool

	// network data
	ChainID  string
	NodeName string

	// on-chain program + 2x state accounts (state + transmissions) + store program
	ProgramID       solana.PublicKey
	StateID         solana.PublicKey
	StoreProgramID  solana.PublicKey
	TransmissionsID solana.PublicKey

	TransmissionSigner TransmissionSigner
}

type Relayer struct {
	lggr     logger.Logger
	chainSet ChainSet
	ctx      context.Context
	cancel   func()
}

// Note: constructed in core
func NewRelayer(lggr logger.Logger, chainSet ChainSet) *Relayer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Relayer{
		lggr:     lggr,
		chainSet: chainSet,
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

// NewOCR2Provider creates a new OCR2ProviderCtx instance.
func (r *Relayer) NewOCR2Provider(externalJobID uuid.UUID, s interface{}) (relaytypes.OCR2ProviderCtx, error) {
	var provider ocr2Provider
	spec, ok := s.(OCR2Spec)
	if !ok {
		return &provider, errors.New("unsuccessful cast to 'solana.OCR2Spec'")
	}

	offchainConfigDigester := OffchainConfigDigester{
		ProgramID: spec.ProgramID,
		StateID:   spec.StateID,
	}

	chain, err := r.chainSet.Chain(r.ctx, spec.ChainID)
	if err != nil {
		return nil, err
	}
	chainReader, err := chain.Reader(spec.NodeName)
	if err != nil {
		return nil, err
	}
	msgEnqueuer := chain.TxManager()
	cfg := chain.Config()

	// provide contract config + tracker reader + tx manager + signer + logger
	contractTracker := NewTracker(spec, cfg, chainReader, msgEnqueuer, spec.TransmissionSigner, r.lggr)

	if spec.IsBootstrap {
		// Return early if bootstrap node (doesn't require the full OCR2 provider)
		return &ocr2Provider{
			offchainConfigDigester: offchainConfigDigester,
			tracker:                &contractTracker,
		}, nil
	}

	reportCodec := ReportCodec{}

	return &ocr2Provider{
		offchainConfigDigester: offchainConfigDigester,
		reportCodec:            reportCodec,
		tracker:                &contractTracker,
	}, nil
}

type ocr2Provider struct {
	offchainConfigDigester OffchainConfigDigester
	reportCodec            ReportCodec
	tracker                *ContractTracker
}

// Start starts OCR2Provider respecting the given context.
func (p *ocr2Provider) Start(context.Context) error {
	// TODO: start all needed subservices
	return p.tracker.Start()
}

func (p *ocr2Provider) Close() error {
	// TODO: close all subservices
	return p.tracker.Close()
}

func (p ocr2Provider) Ready() error {
	// always ready
	return p.tracker.Ready()
}

func (p ocr2Provider) Healthy() error {
	// TODO: only if all subservices are healthy
	return p.tracker.Healthy()
}

func (p ocr2Provider) ContractTransmitter() types.ContractTransmitter {
	return p.tracker
}

func (p ocr2Provider) ContractConfigTracker() types.ContractConfigTracker {
	return p.tracker
}

func (p ocr2Provider) OffchainConfigDigester() types.OffchainConfigDigester {
	return p.offchainConfigDigester
}

func (p ocr2Provider) ReportCodec() median.ReportCodec {
	return p.reportCodec
}

func (p ocr2Provider) MedianContract() median.MedianContract {
	return p.tracker
}

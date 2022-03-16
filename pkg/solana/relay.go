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
	NodeEndpointHTTP string

	// on-chain program + 2x state accounts (state + transmissions) + store program
	ProgramID       solana.PublicKey
	StateID         solana.PublicKey
	StoreProgramID  solana.PublicKey
	TransmissionsID solana.PublicKey

	// transaction + state parameters [optional]
	UsePreflight bool
	Commitment   string
	TxTimeout    string

	// polling configuration [optional]
	PollingInterval   string
	PollingCtxTimeout string
	StaleTimeout      string

	TransmissionSigner TransmissionSigner
}

type Relayer struct {
	lggr logger.Logger
}

// Note: constructed in core
func NewRelayer(lggr logger.Logger, chainSet ChainSet) *Relayer {
	return &Relayer{
		lggr: lggr,
	}
}

// Start starts the relayer respecting the given context.
func (r *Relayer) Start(context.Context) error {
	// No subservices started on relay start, but when the first job is started
	return nil
}

// Close will close all open subservices
func (r *Relayer) Close() error {
	return nil
}

func (r *Relayer) Ready() error {
	// always ready
	return nil
}

// Healthy only if all subservices are healthy
func (r *Relayer) Healthy() error {
	return nil
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

	// establish network connection RPC
	client := NewClient(spec, r.lggr)
	contractTracker := NewTracker(spec, client, spec.TransmissionSigner, r.lggr)

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

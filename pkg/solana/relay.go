package solana

import (
	"errors"

	"github.com/gagliardetto/solana-go"
	uuid "github.com/satori/go.uuid"

	relaytypes "github.com/smartcontractkit/chainlink/core/services/relay/types"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

type Logger interface {
	Tracef(format string, values ...interface{})
	Debugf(format string, values ...interface{})
	Infof(format string, values ...interface{})
	Warnf(format string, values ...interface{})
	Errorf(format string, values ...interface{})
	Criticalf(format string, values ...interface{})
	Panicf(format string, values ...interface{})
	Fatalf(format string, values ...interface{})
}

type TransmissionSigner interface {
	Sign(msg []byte) ([]byte, error)
	PublicKey() solana.PublicKey
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
	lggr Logger
}

// Note: constructed in core
func NewRelayer(lggr Logger) *Relayer {
	return &Relayer{
		lggr: lggr,
	}
}

func (r *Relayer) Start() error {
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

// TODO [relay]: import from smartcontractkit/solana-integration impl
func (r *Relayer) NewOCR2Provider(externalJobID uuid.UUID, s interface{}, contractReady chan struct{}) (relaytypes.OCR2Provider, error) {
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
	contractCache := NewContractCache(spec, client, r.lggr, contractReady)
	contractConfigTracker := ContractConfigTracker{
		cache:  contractCache,
		client: client,
	}

	if spec.IsBootstrap {
		// Return early if bootstrap node (doesn't require the full OCR2 provider)
		return &ocr2Provider{
			offchainConfigDigester: offchainConfigDigester,
			tracker:                &contractConfigTracker,
		}, nil
	}

	reportCodec := ReportCodec{}
	transmitter := ContractTransmitter{
		client: client,
		cache:  contractCache,
		signer: spec.TransmissionSigner,
		lggr:   r.lggr,
	}

	return &ocr2Provider{
		offchainConfigDigester: offchainConfigDigester,
		reportCodec:            reportCodec,
		tracker:                &contractConfigTracker,
		medianContract:         &MedianContract{cache: contractCache},
		transmitter:            &transmitter,
	}, nil
}

type ocr2Provider struct {
	offchainConfigDigester OffchainConfigDigester
	reportCodec            ReportCodec
	tracker                *ContractConfigTracker
	transmitter            *ContractTransmitter
	medianContract         *MedianContract
	cache                  ContractCache
}

func (p *ocr2Provider) Start() error {
	return p.cache.Start()
}

func (p *ocr2Provider) Close() error {
	return p.cache.Close()
}

func (p ocr2Provider) ContractTransmitter() types.ContractTransmitter {
	return p.transmitter
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
	return p.medianContract
}

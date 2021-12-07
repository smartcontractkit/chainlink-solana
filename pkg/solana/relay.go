package solana

import (
	"context"
	"errors"
	"time"

	"github.com/gagliardetto/solana-go"
	uuid "github.com/satori/go.uuid"
	"gopkg.in/guregu/null.v4"

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

type relayer struct {
	lggr        Logger
	connections Connections
}

func NewRelayer(lggr Logger) *relayer {
	return &relayer{
		lggr:        lggr,
		connections: Connections{},
	}
}

func (r relayer) Start() error {
	// No subservices started on relay start, but when the first job is started
	return nil
}

// Close will close all open subservices
func (r *relayer) Close() error {
	// close all open network client connections
	return r.connections.Close()
}

func (r relayer) Ready() error {
	// always ready
	return nil
}

// Healthy only if all subservices are healthy
func (r relayer) Healthy() error {
	// TODO: are all open WS connections healthy?
	return nil
}

type Transmitter interface {
	Sign(msg []byte) ([]byte, error)
	PublicKey() solana.PublicKey
}

type OCR2Spec struct {
	ID          int32
	IsBootstrap bool

	// network data
	NodeEndpointRPC string
	NodeEndpointWS  string

	// on-chain program + 2x state accounts (state + transmissions) + validator program
	ProgramID          solana.PublicKey
	StateID            solana.PublicKey
	ValidatorProgramID solana.PublicKey
	TransmissionsID    solana.PublicKey

	Transmitter Transmitter

	// OCR key bundle (off/on-chain keys) id
	KeyBundleID null.String
}

// TODO [relay]: import from smartcontractkit/solana-integration impl
func (r relayer) NewOCR2Provider(externalJobID uuid.UUID, s interface{}) (*ocr2Provider, error) {
	spec, ok := s.(OCR2Spec)
	if !ok {
		return nil, errors.New("unsuccessful cast to 'solana.OCR2Spec'")
	}

	offchainConfigDigester := OffchainConfigDigester{
		ProgramID: spec.ProgramID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// establish network connection RPC + WS (reuses existing WS client if available)
	client, err := r.connections.NewConnectedClient(ctx, spec.NodeEndpointRPC, spec.NodeEndpointWS)
	if err != nil {
		return &ocr2Provider{}, err
	}

	contractTracker := NewTracker(spec, client, spec.Transmitter, r.lggr)

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

func (p ocr2Provider) Start() error {
	// TODO: start all needed subservices
	return nil
}

func (p ocr2Provider) Close() error {
	// TODO: close all subservices
	// TODO: close client WS connection if not used/shared anymore
	return nil
}

func (p ocr2Provider) Ready() error {
	// always ready
	return nil
}

func (p ocr2Provider) Healthy() error {
	// TODO: only if all subservices are healthy
	return nil
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

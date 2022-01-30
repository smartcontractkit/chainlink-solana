package solana

import (
	uuid "github.com/satori/go.uuid"
	"github.com/smartcontractkit/chainlink-relay/pkg/plugin"

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
	r.lggr.Debugf("Starting...")
	// No subservices started on relay start, but when the first job is started
	return nil
}

// Close will close all open subservices
func (r *Relayer) Close() error {
	r.lggr.Debugf("Stopping...")
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

func (r *Relayer) NewOCR2Provider(externalJobID uuid.UUID, spec plugin.SolanaSpec) (plugin.OCR2Provider, error) {
	offchainConfigDigester := OffchainConfigDigester{
		ProgramID: spec.ProgramID,
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

func (p *ocr2Provider) Start() error {
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

package solana

import (
	"context"
	"errors"
	"time"

	"github.com/gagliardetto/solana-go"
	uuid "github.com/satori/go.uuid"
	"gopkg.in/guregu/null.v4"

	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/service"
	"github.com/smartcontractkit/chainlink/core/services/keystore"
	"github.com/smartcontractkit/chainlink/core/services/keystore/keys/ocr2key"
	"github.com/smartcontractkit/chainlink/core/services/relay"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

var _ service.Service = (*relayer)(nil)
var _ relay.Relayer = (*relayer)(nil)

type relayer struct {
	keystore    keystore.Master
	lggr        logger.Logger
	connections Connections
}

func NewRelayer(config relay.Config) *relayer {
	return &relayer{
		keystore:    config.Keystore,
		lggr:        config.Lggr,
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

type OCR2Spec struct {
	ID          int32
	IsBootstrap bool

	// network data
	NodeEndpointRPC string
	NodeEndpointWS  string

	// on-chain program + 2x state accounts (state + transmissions)
	ProgramID       solana.PublicKey
	StateID         solana.PublicKey
	TransmissionsID solana.PublicKey

	// private key for the transmission signing
	Transmitter solana.PrivateKey

	// OCR key bundle (off/on-chain keys) id
	KeyBundleID null.String
}

// TODO [relay]: import from smartcontractkit/solana-integration impl
func (r relayer) NewOCR2Provider(externalJobID uuid.UUID, s interface{}) (relay.OCR2Provider, error) {
	spec, ok := s.(OCR2Spec)
	if !ok {
		return nil, errors.New("unsuccessful cast to 'solana.OCR2Spec'")
	}

	// TODO [relay]: solana OCR2 keys ('ocr2key.KeyBundle' is Ethereum specific)
	kb, err := r.keystore.OCR2().Get(spec.KeyBundleID.ValueOrZero())
	if err != nil {
		return nil, err
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

	// TODO: @Blaz/@Ryan the solana-go requires a private key (?)
	transmitter, err := solana.NewRandomPrivateKey()
	if err != nil {
		return &ocr2Provider{}, err
	}

	contractTracker := NewTracker(spec, client, transmitter, r.lggr)

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
		keyBundle:              kb,
		tracker:                &contractTracker,
	}, nil
}

var _ service.Service = (*ocr2Provider)(nil)

type ocr2Provider struct {
	offchainConfigDigester OffchainConfigDigester
	reportCodec            ReportCodec
	keyBundle              ocr2key.KeyBundle
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

func (p ocr2Provider) OffchainKeyring() types.OffchainKeyring {
	return &p.keyBundle.OffchainKeyring
}

func (p ocr2Provider) OnchainKeyring() types.OnchainKeyring {
	return &p.keyBundle.OnchainKeyring
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

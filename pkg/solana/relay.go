package solana

import (
	"errors"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	uuid "github.com/satori/go.uuid"
	"gopkg.in/guregu/null.v4"

	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/service"
	"github.com/smartcontractkit/chainlink/core/services/keystore"
	"github.com/smartcontractkit/chainlink/core/services/keystore/keys/ocr2key"
	"github.com/smartcontractkit/chainlink/core/services/relay"
	"github.com/smartcontractkit/chainlink/core/utils"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

var _ service.Service = (*relayer)(nil)
var _ relay.Relayer = (*relayer)(nil)

type relayer struct {
	keystore keystore.Master
	lggr     logger.Logger
}

func NewRelayer(config relay.Config) *relayer {
	return &relayer{
		keystore: config.Keystore,
		lggr:     config.Lggr,
	}
}

func (r relayer) Start() error {
	// No subservices started on relay start, but when the first job is started
	return nil
}

func (r relayer) Close() error {
	// TODO: close all subservices
	return nil
}

func (r relayer) Ready() error {
	// always ready
	return nil
}

func (r relayer) Healthy() error {
	// TODO: only if all subservices are healthy
	return nil
}

type OCR2Spec struct {
	ID          int32
	IsBootstrap bool

	// network data
	ChainID         *utils.Big
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

// TODO [relay]: import from smartcontractkit/chainlink-solana impl
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
		// TODO: copy the ProgramID from the OCR2 spec
		ProgramID: solana.PublicKeyFromBytes([]byte("mock")),
	}

	if spec.IsBootstrap {
		// Return early if bootstrap node (doesn't require the full OCR2 provider)
		return &ocr2Provider{
			// TODO: tracker:                tracker,
			offchainConfigDigester: offchainConfigDigester,
		}, nil
	}

	reportCodec := ReportCodec{}

	return &ocr2Provider{
		client:                 rpc.New(spec.NodeEndpointRPC),
		offchainConfigDigester: offchainConfigDigester,
		reportCodec:            reportCodec,
		keyBundle:              kb,
	}, nil
}

var _ service.Service = (*ocr2Provider)(nil)

type ocr2Provider struct {
	client                 *rpc.Client
	offchainConfigDigester OffchainConfigDigester
	reportCodec            ReportCodec
	keyBundle              ocr2key.KeyBundle
}

func (p ocr2Provider) Start() error {
	// TODO: start all needed subservices
	return nil
}

func (p ocr2Provider) Close() error {
	// TODO: close all subservices
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
	return p.keyBundle.OnchainKeyring()
}

func (p ocr2Provider) ContractTransmitter() types.ContractTransmitter {
	return nil
}

func (p ocr2Provider) ContractConfigTracker() types.ContractConfigTracker {
	return nil
}

func (p ocr2Provider) OffchainConfigDigester() types.OffchainConfigDigester {
	return p.offchainConfigDigester
}

func (p ocr2Provider) ReportCodec() median.ReportCodec {
	return p.reportCodec
}

func (p ocr2Provider) MedianContract() median.MedianContract {
	return nil
}

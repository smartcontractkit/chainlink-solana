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

var _ relaytypes.Relayer = &Relayer{}

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

func (r *Relayer) NewConfigWatcher(args relaytypes.ConfigWatcherArgs) (relaytypes.ConfigWatcher, error) {
	return newConfigWatcher(r.ctx, r.lggr, r.chainSet, args)
}

type configWatcher struct {
	chainID                            string
	programID, storeProgramID, stateID solana.PublicKey
	stateCache                         StateCache
	offchainConfigDigester             types.OffchainConfigDigester
	configTracker                      types.ContractConfigTracker
}

func newConfigWatcher(ctx context.Context, lggr logger.Logger, chainSet ChainSet, args relaytypes.ConfigWatcherArgs) (*configWatcher, error) {
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
	offchainConfigDigester := OffchainConfigDigester{
		ProgramID: programID,
		StateID:   stateID,
	}
	chain, err := chainSet.Chain(ctx, relayConfig.ChainID)
	if err != nil {
		return nil, errors.Wrap(err, "error in NewOCR2Provider.chainSet.Chain")
	}
	chainReader, err := chain.Reader()
	if err != nil {
		return nil, errors.Wrap(err, "error in NewOCR2Provider.chain.Reader")
	}
	stateCache := NewStateCache(programID, stateID, storeProgramID, chain.Config(), chainReader, lggr)
	return &configWatcher{
		chainID:                relayConfig.ChainID,
		stateID:                stateID,
		programID:              programID,
		storeProgramID:         storeProgramID,
		stateCache:             stateCache,
		offchainConfigDigester: offchainConfigDigester,
		configTracker:          &ConfigTracker{stateCache: stateCache, reader: chainReader},
	}, nil

}

func (c configWatcher) Start(ctx context.Context) error {
	return c.stateCache.Start()
}

func (c configWatcher) Close() error {
	return c.stateCache.Close()
}

func (c configWatcher) Ready() error {
	return nil
}

func (c configWatcher) Healthy() error {
	return nil
}

func (c configWatcher) OffchainConfigDigester() types.OffchainConfigDigester {
	return c.offchainConfigDigester
}

func (c configWatcher) ContractConfigTracker() types.ContractConfigTracker {
	return c.configTracker
}

func (r *Relayer) NewMedianProvider(args relaytypes.PluginArgs) (relaytypes.MedianProvider, error) {
	configWatcher, err := newConfigWatcher(r.ctx, r.lggr, r.chainSet, args.ConfigWatcherArgs)
	if err != nil {
		return nil, err
	}
	transmissionsID, err := solana.PublicKeyFromBase58(args.TransmitterID)
	if err != nil {
		return nil, errors.Wrap(err, "error on 'solana.PublicKeyFromBase58' for 'spec.RelayConfig.TransmissionsID")
	}
	transmissionSigner, err := r.ks.Get(transmissionsID.String())
	if err != nil {
		return nil, err
	}
	chain, err := r.chainSet.Chain(r.ctx, configWatcher.chainID)
	if err != nil {
		return nil, errors.Wrap(err, "error in NewOCR2Provider.chainSet.Chain")
	}
	chainReader, err := chain.Reader()
	if err != nil {
		return nil, errors.Wrap(err, "error in NewOCR2Provider.chain.Reader")
	}
	cfg := chain.Config()
	stateCache := NewStateCache(configWatcher.programID, configWatcher.stateID, configWatcher.storeProgramID, cfg, chainReader, r.lggr)
	transmissionsCache := NewTransmissionsCache(configWatcher.programID, configWatcher.stateID, configWatcher.storeProgramID, transmissionsID, cfg, chainReader, chain.TxManager(), transmissionSigner, r.lggr)
	return &medianProvider{
		configWatcher: configWatcher,
		reportCodec:   ReportCodec{},
		contract: &MedianContract{
			stateCache:         stateCache,
			transmissionsCache: transmissionsCache,
		},
		transmitter: &Transmitter{
			stateID:            configWatcher.stateID,
			programID:          configWatcher.stateID,
			storeProgramID:     configWatcher.stateID,
			transmissionsID:    configWatcher.stateID,
			transmissionSigner: transmissionSigner,
			reader:             chainReader,
			stateCache:         stateCache,
			lggr:               r.lggr,
			txManager:          chain.TxManager(),
		},
	}, nil
}

type medianProvider struct {
	*configWatcher
	reportCodec median.ReportCodec
	contract    median.MedianContract
	transmitter types.ContractTransmitter
}

func (p medianProvider) ContractTransmitter() types.ContractTransmitter {
	return p.transmitter
}

func (p medianProvider) ReportCodec() median.ReportCodec {
	return p.reportCodec
}

func (p medianProvider) MedianContract() median.MedianContract {
	return p.contract
}

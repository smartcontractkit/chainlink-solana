package solana

import (
	"context"
	"encoding/json"

	"github.com/gagliardetto/solana-go"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/chainlink/core/services/keystore/keys/solkey"
	relaytypes "github.com/smartcontractkit/chainlink/core/services/relay/types"
	"github.com/smartcontractkit/chainlink/core/utils"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
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
	configWatcher, err := newConfigWatcher(r.ctx, r.lggr, r.chainSet, args)
	if err != nil {
		// Never return (*configWatcher)(nil)
		return nil, err
	}
	return configWatcher, err
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
	cfg := configWatcher.chain.Config()
	transmissionsCache := NewTransmissionsCache(configWatcher.programID, configWatcher.stateID, configWatcher.storeProgramID, transmissionsID, cfg, configWatcher.reader, configWatcher.chain.TxManager(), transmissionSigner, r.lggr)
	return &medianProvider{
		configWatcher: configWatcher,
		reportCodec:   ReportCodec{},
		contract: &MedianContract{
			stateCache:         configWatcher.stateCache,
			transmissionsCache: transmissionsCache,
		},
		transmitter: &Transmitter{
			stateID:            configWatcher.stateID,
			programID:          configWatcher.stateID,
			storeProgramID:     configWatcher.stateID,
			transmissionsID:    configWatcher.stateID,
			transmissionSigner: transmissionSigner,
			reader:             configWatcher.reader,
			stateCache:         configWatcher.stateCache,
			lggr:               r.lggr,
			txManager:          configWatcher.chain.TxManager(),
		},
	}, nil
}

var _ relaytypes.ConfigWatcher = &configWatcher{}

type configWatcher struct {
	utils.StartStopOnce
	chainID                            string
	programID, storeProgramID, stateID solana.PublicKey
	stateCache                         *StateCache
	offchainConfigDigester             types.OffchainConfigDigester
	configTracker                      types.ContractConfigTracker
	chain                              Chain
	reader                             client.Reader
}

func newConfigWatcher(ctx context.Context, lggr logger.Logger, chainSet ChainSet, args relaytypes.ConfigWatcherArgs) (*configWatcher, error) {
	var relayConfig RelayConfig
	err := json.Unmarshal(args.RelayConfig, &relayConfig)
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
		return nil, errors.Wrap(err, "error in NewMedianProvider.chainSet.Chain")
	}
	reader, err := chain.Reader()
	if err != nil {
		return nil, errors.Wrap(err, "error in NewMedianProvider.chain.Reader")
	}
	stateCache := NewStateCache(programID, stateID, storeProgramID, chain.Config(), reader, lggr)
	return &configWatcher{
		chainID:                relayConfig.ChainID,
		stateID:                stateID,
		programID:              programID,
		storeProgramID:         storeProgramID,
		stateCache:             stateCache,
		offchainConfigDigester: offchainConfigDigester,
		configTracker:          &ConfigTracker{stateCache: stateCache, reader: reader},
		chain:                  chain,
		reader:                 reader,
	}, nil
}

func (c *configWatcher) Start(ctx context.Context) error {
	return c.StartOnce("SolanaConfigWatcher", func() error {
		return c.stateCache.Start()
	})
}

func (c *configWatcher) Close() error {
	return c.StopOnce("SolanaConfigWatcher", func() error {
		return c.stateCache.Close()
	})
}

func (c *configWatcher) OffchainConfigDigester() types.OffchainConfigDigester {
	return c.offchainConfigDigester
}

func (c *configWatcher) ContractConfigTracker() types.ContractConfigTracker {
	return c.configTracker
}

var _ relaytypes.MedianProvider = &medianProvider{}

type medianProvider struct {
	*configWatcher
	reportCodec median.ReportCodec
	contract    median.MedianContract
	transmitter types.ContractTransmitter
}

func (p *medianProvider) ContractTransmitter() types.ContractTransmitter {
	return p.transmitter
}

func (p *medianProvider) ReportCodec() median.ReportCodec {
	return p.reportCodec
}

func (p *medianProvider) MedianContract() median.MedianContract {
	return p.contract
}

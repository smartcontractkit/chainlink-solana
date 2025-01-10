package solana

import (
	"context"

	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

type ConfigTracker struct {
	stateCache *StateCache
	getReader  GetReader
}

func (c *ConfigTracker) Notify() <-chan struct{} {
	return nil // not using websocket, config changes will be handled by polling in libocr
}

// LatestConfigDetails returns information about the latest configuration,
// but not the configuration itself.
func (c *ConfigTracker) LatestConfigDetails(ctx context.Context) (changedInBlock uint64, configDigest types.ConfigDigest, err error) {
	state, err := c.stateCache.Read()
	return state.Config.LatestConfigBlockNumber, state.Config.LatestConfigDigest, err
}

func ConfigFromState(ctx context.Context, state State) (types.ContractConfig, error) {
	pubKeys := []types.OnchainPublicKey{}
	accounts := []types.Account{}

	oracles, err := state.Oracles.Data()
	if err != nil {
		return types.ContractConfig{}, err
	}

	for idx := range oracles {
		pubKeys = append(pubKeys, oracles[idx].Signer.Key[:])
		accounts = append(accounts, types.Account(oracles[idx].Transmitter.String()))
	}

	onchainConfigStruct := median.OnchainConfig{
		Min: state.Config.MinAnswer.BigInt(),
		Max: state.Config.MaxAnswer.BigInt(),
	}

	onchainConfig, err := median.StandardOnchainConfigCodec{}.Encode(ctx, onchainConfigStruct)
	if err != nil {
		return types.ContractConfig{}, err
	}
	offchainConfig, err := state.OffchainConfig.Data()
	if err != nil {
		return types.ContractConfig{}, err
	}

	return types.ContractConfig{
		ConfigDigest:          state.Config.LatestConfigDigest,
		ConfigCount:           uint64(state.Config.ConfigCount),
		Signers:               pubKeys,
		Transmitters:          accounts,
		F:                     state.Config.F,
		OnchainConfig:         onchainConfig,
		OffchainConfigVersion: state.OffchainConfig.Version,
		OffchainConfig:        offchainConfig,
	}, nil
}

// LatestConfig returns the latest configuration.
func (c *ConfigTracker) LatestConfig(ctx context.Context, changedInBlock uint64) (types.ContractConfig, error) {
	state, err := c.stateCache.Read()
	if err != nil {
		return types.ContractConfig{}, err
	}
	return ConfigFromState(ctx, state)
}

// LatestBlockHeight returns the height of the most recent block in the chain.
func (c *ConfigTracker) LatestBlockHeight(ctx context.Context) (blockHeight uint64, err error) {
	reader, err := c.getReader()
	if err != nil {
		return 0, err
	}
	return reader.SlotHeight(ctx) // this returns the latest slot height through CommitmentProcessed
}

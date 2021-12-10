package solana

import (
	"context"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

func (c ContractTracker) Notify() <-chan struct{} {
	return nil // not using websocket, config changes will be handled by polling in libocr
}

// LatestConfigDetails returns information about the latest configuration,
// but not the configuration itself.
func (c ContractTracker) LatestConfigDetails(ctx context.Context) (changedInBlock uint64, configDigest types.ConfigDigest, err error) {
	err = c.fetchState(ctx)
	return c.state.Config.LatestConfigBlockNumber, c.state.Config.LatestConfigDigest, err
}

func configFromState(state State) types.ContractConfig {
	pubKeys := []types.OnchainPublicKey{}
	accounts := []types.Account{}
	for _, o := range state.Oracles.Data() {
		o := o //  https://github.com/golang/go/wiki/CommonMistakes#using-reference-to-loop-iterator-variable
		pubKeys = append(pubKeys, o.Signer.Key[:])
		accounts = append(accounts, types.Account(o.Transmitter.String()))
	}

	return types.ContractConfig{
		ConfigDigest:          state.Config.LatestConfigDigest,
		ConfigCount:           uint64(state.Config.ConfigCount),
		Signers:               pubKeys,
		Transmitters:          accounts,
		F:                     state.Config.F,
		OnchainConfig:         []byte{}, // TODO: where to fetch?
		OffchainConfigVersion: state.Config.OffchainConfig.Version,
		OffchainConfig:        state.Config.OffchainConfig.Data(),
	}
}

// LatestConfig returns the latest configuration.
func (c ContractTracker) LatestConfig(ctx context.Context, changedInBlock uint64) (types.ContractConfig, error) {
	err := c.fetchState(ctx)
	return configFromState(c.state), err
}

// LatestBlockHeight returns the height of the most recent block in the chain.
func (c ContractTracker) LatestBlockHeight(ctx context.Context) (blockHeight uint64, err error) {
	return c.client.GetBlockHeight(ctx, rpc.CommitmentProcessed)
}

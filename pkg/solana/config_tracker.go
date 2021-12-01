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

// LatestConfig returns the latest configuration.
func (c ContractTracker) LatestConfig(ctx context.Context, changedInBlock uint64) (types.ContractConfig, error) {
	err := c.fetchState(ctx)

	pubKeys := []types.OnchainPublicKey{}
	accounts := []types.Account{}
	for _, o := range c.state.Oracles.Data() {
		pubKeys = append(pubKeys, o.Signer.Key[:])
		accounts = append(accounts, types.Account(o.Transmitter.String()))
	}

	return types.ContractConfig{
		ConfigDigest:          c.state.Config.LatestConfigDigest,
		ConfigCount:           uint64(c.state.Config.ConfigCount),
		Signers:               pubKeys,
		Transmitters:          accounts,
		F:                     c.state.Config.F,
		OnchainConfig:         []byte{}, // TODO: where to fetch?
		OffchainConfigVersion: c.state.Config.OffchainConfig.Version,
		OffchainConfig:        c.state.Config.OffchainConfig.Data(),
	}, err
}

// LatestBlockHeight returns the height of the most recent block in the chain.
func (c ContractTracker) LatestBlockHeight(ctx context.Context) (blockHeight uint64, err error) {
	return c.client.GetBlockHeight(ctx, rpc.CommitmentProcessed)
}

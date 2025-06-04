package api

import (
	"github.com/smartcontractkit/chainlink-solana/pkg/solana"
	oldConfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

type SolanaService interface {
}

type solanaServiceImpl struct {
	chain solana.Chain
}

func NewSolanaService(config SolanaConfig) (SolanaService, error) {
	defaultConfig := oldConfig.NewDefault()
	defaultConfig.ChainID = (*string)(&config.ChainID)
	err := defaultConfig.ValidateConfig()
	if err != nil {
		return nil, err
	}
	return solana.NewChain(defaultConfig, solana.ChainOpts{})
}

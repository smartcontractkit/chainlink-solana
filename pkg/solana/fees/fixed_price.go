package fees

import (
	"context"
	"fmt"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

var _ Estimator = &fixedPriceEstimator{}

type fixedPriceEstimator struct {
	cfg config.Config
}

func NewFixedPriceEstimator(cfg config.Config) (Estimator, error) {
	defaultPrice, minPrice, maxPrice := cfg.ComputeUnitPriceDefault(), cfg.ComputeUnitPriceMin(), cfg.ComputeUnitPriceMax()

	if defaultPrice < minPrice || defaultPrice > maxPrice {
		return nil, fmt.Errorf("default price (%d) is not within the min (%d) and max (%d) price bounds", defaultPrice, minPrice, maxPrice)
	}

	return &fixedPriceEstimator{
		cfg: cfg,
	}, nil
}

func (est *fixedPriceEstimator) Start(ctx context.Context) error {
	return nil
}

func (est *fixedPriceEstimator) Close() error {
	return nil
}

func (est *fixedPriceEstimator) BaseComputeUnitPrice() uint64 {
	return est.cfg.ComputeUnitPriceDefault()
}

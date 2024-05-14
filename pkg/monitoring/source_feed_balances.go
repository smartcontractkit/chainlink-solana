package monitoring

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

const (
	ErrBalancesSource = "error while fetching balances"
	ErrGetBalance     = "GetBalance failed"
	ErrGetBalanceNil  = "GetBalance returned nil"
)

func NewFeedBalancesSourceFactory(
	client ChainReader,
	log commonMonitoring.Logger,
) commonMonitoring.SourceFactory {
	return &feedBalancesSourceFactory{
		client,
		log,
	}
}

type feedBalancesSourceFactory struct {
	client ChainReader
	log    commonMonitoring.Logger
}

func (s *feedBalancesSourceFactory) NewSource(
	_ commonMonitoring.ChainConfig,
	feedConfig commonMonitoring.FeedConfig,
) (commonMonitoring.Source, error) {
	solanaFeedConfig, ok := feedConfig.(config.SolanaFeedConfig)
	if !ok {
		return nil, fmt.Errorf("expected feedConfig to be of type config.SolanaFeedConfig not %T", feedConfig)
	}
	return &feedBalancesSource{
		bs: balancesSource{
			client: s.client,
			log:    s.log,
			// nodes are initialized during Fetch - accounts may change
		},
		feedConfig: solanaFeedConfig,
	}, nil
}

func (s *feedBalancesSourceFactory) GetType() string {
	return types.BalanceType
}

type feedBalancesSource struct {
	bs         balancesSource
	feedConfig config.SolanaFeedConfig
}

func (s *feedBalancesSource) Fetch(ctx context.Context) (interface{}, error) {
	state, _, err := s.bs.client.GetState(ctx, s.feedConfig.StateAccount, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, fmt.Errorf("failed to get contract state: %w", err)
	}

	s.bs.addresses = map[string]solana.PublicKey{
		"contract":                    s.feedConfig.ContractAddress,
		"state":                       s.feedConfig.StateAccount,
		"transmissions":               state.Transmissions,
		"token_vault":                 state.Config.TokenVault,
		"requester_access_controller": state.Config.RequesterAccessController,
		"billing_access_controller":   state.Config.BillingAccessController,
	}
	return s.bs.Fetch(ctx)
}

// balancesSource is a reusable component for reading balances of specified accounts
// this is used as a subcomponent of feedBalancesSource and nodeBalancesSourceFactory
type balancesSource struct {
	client    ChainReader
	log       commonMonitoring.Logger
	addresses map[string]solana.PublicKey
}

// Fetch is the shared logic for reading balances from the chain
func (s *balancesSource) Fetch(ctx context.Context) (interface{}, error) {
	var totalErr error
	balances := types.Balances{
		Values:    make(map[string]uint64),
		Addresses: make(map[string]solana.PublicKey),
	}
	rwlock := &sync.Mutex{}
	wg := &sync.WaitGroup{}
	wg.Add(len(s.addresses))

	// exit early if no addresses
	if s.addresses == nil {
		return types.Balances{}, fmt.Errorf("balancesSource.addresses is nil")
	}
	if len(s.addresses) == 0 {
		return types.Balances{}, nil
	}

	for key, address := range s.addresses {
		go func(key string, address solana.PublicKey) {
			defer wg.Done()
			res, err := s.client.GetBalance(ctx, address, rpc.CommitmentProcessed)
			rwlock.Lock()
			defer rwlock.Unlock()
			if err != nil {
				s.log.Errorw(ErrGetBalance, "key", key, "address", address.String(), "error", err)
				totalErr = errors.Join(totalErr, fmt.Errorf("%s (%s, %s): %w", ErrGetBalance, key, address.String(), err))
				return
			}
			if res == nil {
				s.log.Errorw(ErrGetBalanceNil, "key", key, "address", address.String())
				totalErr = errors.Join(totalErr, fmt.Errorf("%s (%s, %s)", ErrGetBalanceNil, key, address.String()))
				return
			}
			balances.Values[key] = res.Value
			balances.Addresses[key] = address
		}(key, address)
	}

	wg.Wait()
	if totalErr != nil {
		return types.Balances{}, fmt.Errorf("%s: %w", ErrBalancesSource, totalErr)
	}
	return balances, nil
}

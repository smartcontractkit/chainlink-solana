package monitoring

import (
	"context"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

const (
	balancesType = "balances"
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
		s.client,
		s.log,
		solanaFeedConfig,
	}, nil
}

func (s *feedBalancesSourceFactory) GetType() string {
	return balancesType
}

type feedBalancesSource struct {
	client     ChainReader
	log        commonMonitoring.Logger
	feedConfig config.SolanaFeedConfig
}

func (s *feedBalancesSource) Fetch(ctx context.Context) (interface{}, error) {
	state, _, err := s.client.GetState(ctx, s.feedConfig.StateAccount, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, fmt.Errorf("failed to get contract state: %w", err)
	}
	isErr := false
	balances := types.Balances{
		Values:    make(map[string]uint64),
		Addresses: make(map[string]solana.PublicKey),
	}
	balancesMu := &sync.Mutex{}
	wg := &sync.WaitGroup{}
	wg.Add(len(types.FeedBalanceAccountNames))
	for key, address := range map[string]solana.PublicKey{
		"contract":                    s.feedConfig.ContractAddress,
		"state":                       s.feedConfig.StateAccount,
		"transmissions":               state.Transmissions,
		"token_vault":                 state.Config.TokenVault,
		"requester_access_controller": state.Config.RequesterAccessController,
		"billing_access_controller":   state.Config.BillingAccessController,
	} {
		go func(key string, address solana.PublicKey) {
			defer wg.Done()
			res, err := s.client.GetBalance(ctx, address, rpc.CommitmentProcessed)
			balancesMu.Lock()
			defer balancesMu.Unlock()
			if err != nil {
				s.log.Errorw("GetBalance failed", "key", key, "address", address.String(), "error", err)
				isErr = true
				return
			}
			if res == nil {
				s.log.Errorw("GetBalance returned nil", "key", key, "address", address.String())
				isErr = true
				return
			}
			balances.Values[key] = res.Value
			balances.Addresses[key] = address
		}(key, address)
	}

	wg.Wait()
	if isErr {
		return types.Balances{}, fmt.Errorf("error while fetching balances")
	}
	return balances, nil
}

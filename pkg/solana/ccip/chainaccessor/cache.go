package chainaccessor

import (
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/state"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccip/consts"

	"github.com/smartcontractkit/chainlink-ccip/pkg/contractreader"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
)

type pdaCache struct {
	// Note: we might need to update this in the future to map[string][]address.Address
	// to support multi-bind addresses for the price aggregator contract: smartcontractkit/chainlink-ccip@main/pkg/contractreader/extended.go#L77-L79
	bindings       map[string]solana.PublicKey
	offrampCache   offrampPDACache
	feeQuoterCache feeQuoterPDACache
	routerCache    routerPDACache
	rmnRemoteCache rmnRemotePDACache
	cacheMu        sync.RWMutex
	lggr           logger.Logger
}

type offrampPDACache struct {
	state        solana.PublicKey
	config       solana.PublicKey
	refAddresses solana.PublicKey
	sourceChain  map[uint64]solana.PublicKey
}

type feeQuoterPDACache struct {
	config             solana.PublicKey
	billingTokenConfig map[solana.PublicKey]solana.PublicKey
	destChainConfig    map[uint64]solana.PublicKey
}

type routerPDACache struct {
	config         solana.PublicKey
	destChainState map[uint64]solana.PublicKey
	// Intentionally not caching nonce PDAs to avoid having to cache for every user on every selector
}

type rmnRemotePDACache struct {
	curse solana.PublicKey
}

func newPDACache(lggr logger.Logger) pdaCache {
	return pdaCache{
		bindings:       make(map[string]solana.PublicKey),
		offrampCache:   newOfframpPDACache(),
		feeQuoterCache: newFeeQuoterPDACache(),
		routerCache:    newRouterPDACache(),
		rmnRemoteCache: newRMNRemotePDACache(),
		cacheMu:        sync.RWMutex{},
		lggr:           lggr,
	}
}

func newOfframpPDACache() offrampPDACache {
	return offrampPDACache{
		sourceChain: make(map[uint64]solana.PublicKey),
	}
}

func newFeeQuoterPDACache() feeQuoterPDACache {
	return feeQuoterPDACache{
		billingTokenConfig: make(map[solana.PublicKey]solana.PublicKey),
		destChainConfig:    make(map[uint64]solana.PublicKey),
	}
}

func newRouterPDACache() routerPDACache {
	return routerPDACache{
		destChainState: make(map[uint64]solana.PublicKey),
	}
}

func newRMNRemotePDACache() rmnRemotePDACache {
	return rmnRemotePDACache{}
}

func (c *pdaCache) updateCache(contractName string, addr solana.PublicKey) error {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	c.bindings[contractName] = addr

	switch contractName {
	case consts.ContractNameOffRamp:
		return c.updateOfframpPDA(addr)
	case consts.ContractNameRouter, consts.ContractNameOnRamp: // Router and OnRamp are the same program in Solana
		return c.updateRouterPDA(addr)
	case consts.ContractNameFeeQuoter:
		return c.updateFeeQuoterPDA(addr)
	case consts.ContractNameRMNRemote:
		return c.updateRMNRemotePDA(addr)
	default:
		// Return nil if contract is not recognized. PDAs do not need to be tracked for it.
		return nil
	}
}

func (c *pdaCache) updateOfframpPDA(addr solana.PublicKey) error {
	offrampState, _, err := state.FindOfframpStatePDA(addr)
	if err != nil {
		return err
	}
	c.offrampCache.state = offrampState

	config, _, err := state.FindOfframpConfigPDA(addr)
	if err != nil {
		return err
	}
	c.offrampCache.config = config

	refAddresses, _, err := state.FindOfframpReferenceAddressesPDA(addr)
	if err != nil {
		return err
	}
	c.offrampCache.refAddresses = refAddresses

	for sel := range c.offrampCache.sourceChain {
		sourceChain, _, err := state.FindOfframpSourceChainPDA(sel, addr)
		if err != nil {
			return err
		}
		c.offrampCache.sourceChain[sel] = sourceChain
	}
	return nil
}

func (c *pdaCache) updateRouterPDA(addr solana.PublicKey) error {
	config, _, err := state.FindConfigPDA(addr)
	if err != nil {
		return err
	}
	c.routerCache.config = config

	for sel := range c.routerCache.destChainState {
		destChain, err := state.FindDestChainStatePDA(sel, addr)
		if err != nil {
			return err
		}
		c.routerCache.destChainState[sel] = destChain
	}
	return nil
}

func (c *pdaCache) updateFeeQuoterPDA(addr solana.PublicKey) error {
	config, _, err := state.FindFqConfigPDA(addr)
	if err != nil {
		return err
	}
	c.feeQuoterCache.config = config

	for token := range c.feeQuoterCache.billingTokenConfig {
		billingConfig, _, err := state.FindFqBillingTokenConfigPDA(token, addr)
		if err != nil {
			return err
		}
		c.feeQuoterCache.billingTokenConfig[token] = billingConfig
	}

	for sel := range c.feeQuoterCache.destChainConfig {
		destChain, _, err := state.FindFqDestChainPDA(sel, addr)
		if err != nil {
			return err
		}
		c.feeQuoterCache.destChainConfig[sel] = destChain
	}

	return nil
}

func (c *pdaCache) updateRMNRemotePDA(addr solana.PublicKey) error {
	curse, _, err := state.FindRMNRemoteCursesPDA(addr)
	if err != nil {
		return err
	}
	c.rmnRemoteCache.curse = curse
	return nil
}

func (c *pdaCache) offampConfigPDA() solana.PublicKey {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	return c.offrampCache.config
}

func (c *pdaCache) offampStatePDA() solana.PublicKey {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	return c.offrampCache.state
}

func (c *pdaCache) offrampRefAddresses() solana.PublicKey {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	return c.offrampCache.refAddresses
}

func (c *pdaCache) offrampSourceChain(sel uint64, offrampAddr solana.PublicKey) (solana.PublicKey, error) {
	c.cacheMu.RLock()
	sourceChain, exists := c.offrampCache.sourceChain[sel]
	c.cacheMu.RUnlock()

	if exists {
		return sourceChain, nil
	}

	// Lazy load PDA into cache if one does not exist for selector
	// Upgrade to write lock
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	// Check if PDA exists in case something changed between lock upgrade
	sourceChain, exists = c.offrampCache.sourceChain[sel]
	if exists {
		return sourceChain, nil
	}
	var err error
	sourceChain, _, err = state.FindOfframpSourceChainPDA(sel, offrampAddr)
	if err != nil {
		return solana.PublicKey{}, err
	}
	c.offrampCache.sourceChain[sel] = sourceChain
	return sourceChain, nil
}

func (c *pdaCache) rmnRemoteCurse() solana.PublicKey {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	return c.rmnRemoteCache.curse
}

func (c *pdaCache) feeQuoterBillingTokenConfig(token solana.PublicKey, feeQuoterAddr solana.PublicKey) (solana.PublicKey, error) {
	c.cacheMu.RLock()
	billingTokenConfig, exists := c.feeQuoterCache.billingTokenConfig[token]
	c.cacheMu.RUnlock()

	if exists {
		return billingTokenConfig, nil
	}

	// Lazy load PDA into cache if one does not exist for selector
	// Upgrade to write lock
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	// Check if PDA exists in case something changed between lock upgrade
	billingTokenConfig, exists = c.feeQuoterCache.billingTokenConfig[token]
	if exists {
		return billingTokenConfig, nil
	}
	var err error
	billingTokenConfig, _, err = state.FindFqBillingTokenConfigPDA(token, feeQuoterAddr)
	if err != nil {
		return solana.PublicKey{}, err
	}
	c.feeQuoterCache.billingTokenConfig[token] = billingTokenConfig
	return billingTokenConfig, nil
}

func (c *pdaCache) feeQuoterConfig() solana.PublicKey {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	return c.feeQuoterCache.config
}

func (c *pdaCache) feeQuoterDestChain(sel uint64, feeQuoterAddr solana.PublicKey) (solana.PublicKey, error) {
	c.cacheMu.RLock()
	destChain, exists := c.feeQuoterCache.destChainConfig[sel]
	c.cacheMu.RUnlock()

	if exists {
		return destChain, nil
	}

	// Lazy load PDA into cache if one does not exist for selector
	// Upgrade to write lock
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	// Check if PDA exists in case something changed between lock upgrade
	destChain, exists = c.feeQuoterCache.destChainConfig[sel]
	if exists {
		return destChain, nil
	}
	var err error
	destChain, _, err = state.FindFqDestChainPDA(sel, feeQuoterAddr)
	if err != nil {
		return solana.PublicKey{}, err
	}
	c.feeQuoterCache.destChainConfig[sel] = destChain
	return destChain, nil
}

func (c *pdaCache) routerConfig() solana.PublicKey {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	return c.routerCache.config
}

func (c *pdaCache) routerDestChain(sel uint64, routerAddr solana.PublicKey) (solana.PublicKey, error) {
	c.cacheMu.RLock()
	destChain, exists := c.routerCache.destChainState[sel]
	c.cacheMu.RUnlock()

	if exists {
		return destChain, nil
	}

	// Lazy load PDA into cache if one does not exist for selector
	// Upgrade to write lock
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	// Check if PDA exists in case something changed between lock upgrade
	destChain, exists = c.routerCache.destChainState[sel]
	if exists {
		return destChain, nil
	}
	var err error
	destChain, err = state.FindDestChainStatePDA(sel, routerAddr)
	if err != nil {
		return solana.PublicKey{}, err
	}
	c.routerCache.destChainState[sel] = destChain
	return destChain, nil
}

func (c *pdaCache) getBinding(contractName string) (solana.PublicKey, error) {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	addr, exists := c.bindings[contractName]
	if !exists {
		return solana.PublicKey{}, contractreader.ErrNoBindings
	}
	return addr, nil
}

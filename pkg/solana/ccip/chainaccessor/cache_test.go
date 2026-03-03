package chainaccessor

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/state"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccip/consts"

	"github.com/smartcontractkit/chainlink-ccip/pkg/contractreader"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
)

func Test_PDACache(t *testing.T) {
	cache := newPDACache(logger.Test(t))
	chainSelector1 := uint64(100)
	chainSelector2 := uint64(101)
	billingToken1 := getRandomPubKey(t)
	billingToken2 := getRandomPubKey(t)

	// Test with 2 address updates
	// 1. Initially add contract address to cache
	// 2. Update the existing contract address
	testAddrs := []solana.PublicKey{
		getRandomPubKey(t),
		getRandomPubKey(t),
	}

	t.Run("update offramp address", func(t *testing.T) {
		for _, offramp := range testAddrs {
			offrampState, _, err := state.FindOfframpStatePDA(offramp)
			require.NoError(t, err)
			offrampConfig, _, err := state.FindOfframpConfigPDA(offramp)
			require.NoError(t, err)
			offrampRefAddress, _, err := state.FindOfframpReferenceAddressesPDA(offramp)
			require.NoError(t, err)
			offrampSourceChainPDA1, _, err := state.FindOfframpSourceChainPDA(chainSelector1, offramp)
			require.NoError(t, err)
			offrampSourceChainPDA2, _, err := state.FindOfframpSourceChainPDA(chainSelector2, offramp)
			require.NoError(t, err)

			err = cache.updateCache(consts.ContractNameOffRamp, offramp)
			require.NoError(t, err)
			sourceChainPDA1, err := cache.offrampSourceChain(chainSelector1, offramp)
			require.NoError(t, err)
			sourceChainPDA2, err := cache.offrampSourceChain(chainSelector2, offramp)
			require.NoError(t, err)

			offrampBinding, err := cache.getBinding(consts.ContractNameOffRamp)
			require.NoError(t, err)

			require.Equal(t, offramp, offrampBinding)
			require.Equal(t, offrampState, cache.offampStatePDA())
			require.Equal(t, offrampConfig, cache.offampConfigPDA())
			require.Equal(t, offrampRefAddress, cache.offrampRefAddresses())
			require.Equal(t, offrampSourceChainPDA1, sourceChainPDA1)
			require.Equal(t, offrampSourceChainPDA2, sourceChainPDA2)
		}
	})

	t.Run("update router address", func(t *testing.T) {
		for _, router := range testAddrs {
			routerConfig, _, err := state.FindConfigPDA(router)
			require.NoError(t, err)
			routerDestChainState1, err := state.FindDestChainStatePDA(chainSelector1, router)
			require.NoError(t, err)
			routerDestChainState2, err := state.FindDestChainStatePDA(chainSelector2, router)
			require.NoError(t, err)

			err = cache.updateCache(consts.ContractNameRouter, router)
			require.NoError(t, err)
			destChainStatePDA1, err := cache.routerDestChain(chainSelector1, router)
			require.NoError(t, err)
			destChainStatePDA2, err := cache.routerDestChain(chainSelector2, router)
			require.NoError(t, err)

			routerBinding, err := cache.getBinding(consts.ContractNameRouter)
			require.NoError(t, err)

			require.Equal(t, router, routerBinding)
			require.Equal(t, routerConfig, cache.routerConfig())
			require.Equal(t, routerDestChainState1, destChainStatePDA1)
			require.Equal(t, routerDestChainState2, destChainStatePDA2)
		}
	})

	t.Run("update onramp address", func(t *testing.T) {
		// Should successfully update the router PDAs
		for _, onramp := range testAddrs {
			onrampConfig, _, err := state.FindConfigPDA(onramp)
			require.NoError(t, err)
			onrampDestChainState1, err := state.FindDestChainStatePDA(chainSelector1, onramp)
			require.NoError(t, err)
			onrampDestChainState2, err := state.FindDestChainStatePDA(chainSelector2, onramp)
			require.NoError(t, err)

			err = cache.updateCache(consts.ContractNameOnRamp, onramp)
			require.NoError(t, err)
			destChainStatePDA1, err := cache.routerDestChain(chainSelector1, onramp)
			require.NoError(t, err)
			destChainStatePDA2, err := cache.routerDestChain(chainSelector2, onramp)
			require.NoError(t, err)

			onrampBinding, err := cache.getBinding(consts.ContractNameOnRamp)
			require.NoError(t, err)

			require.Equal(t, onramp, onrampBinding)
			require.Equal(t, onrampConfig, cache.routerConfig())
			require.Equal(t, onrampDestChainState1, destChainStatePDA1)
			require.Equal(t, onrampDestChainState2, destChainStatePDA2)
		}
	})

	t.Run("update fee quoter address", func(t *testing.T) {
		for _, feequoter := range testAddrs {
			fqConfig, _, err := state.FindFqConfigPDA(feequoter)
			require.NoError(t, err)
			billingTokenConfig1, _, err := state.FindFqBillingTokenConfigPDA(billingToken1, feequoter)
			require.NoError(t, err)
			billingTokenConfig2, _, err := state.FindFqBillingTokenConfigPDA(billingToken2, feequoter)
			require.NoError(t, err)
			destChainConfig1, _, err := state.FindFqDestChainPDA(chainSelector1, feequoter)
			require.NoError(t, err)
			destChainConfig2, _, err := state.FindFqDestChainPDA(chainSelector2, feequoter)
			require.NoError(t, err)

			err = cache.updateCache(consts.ContractNameFeeQuoter, feequoter)
			require.NoError(t, err)
			billingTokenConfigPDA1, err := cache.feeQuoterBillingTokenConfig(billingToken1, feequoter)
			require.NoError(t, err)
			billingTokenConfigPDA2, err := cache.feeQuoterBillingTokenConfig(billingToken2, feequoter)
			require.NoError(t, err)
			destChainPDA1, err := cache.feeQuoterDestChain(chainSelector1, feequoter)
			require.NoError(t, err)
			destChainPDA2, err := cache.feeQuoterDestChain(chainSelector2, feequoter)
			require.NoError(t, err)

			feeQuoterBinding, err := cache.getBinding(consts.ContractNameFeeQuoter)
			require.NoError(t, err)

			require.Equal(t, feequoter, feeQuoterBinding)
			require.Equal(t, fqConfig, cache.feeQuoterConfig())
			require.Equal(t, billingTokenConfig1, billingTokenConfigPDA1)
			require.Equal(t, billingTokenConfig2, billingTokenConfigPDA2)
			require.Equal(t, destChainConfig1, destChainPDA1)
			require.Equal(t, destChainConfig2, destChainPDA2)
		}
	})

	t.Run("update rmn remote address", func(t *testing.T) {
		for _, rmnremote := range testAddrs {
			curse, _, err := state.FindRMNRemoteCursesPDA(rmnremote)
			require.NoError(t, err)

			err = cache.updateCache(consts.ContractNameRMNRemote, rmnremote)
			require.NoError(t, err)

			rmnRemoteBinding, err := cache.getBinding(consts.ContractNameRMNRemote)
			require.NoError(t, err)

			require.Equal(t, rmnremote, rmnRemoteBinding)
			require.Equal(t, curse, cache.rmnRemoteCurse())
		}
	})

	t.Run("update unknown contract address", func(t *testing.T) {
		rmnHome := getRandomPubKey(t)
		err := cache.updateCache(consts.ContractNameRMNHome, rmnHome)
		require.NoError(t, err)
	})

	t.Run("fail to fetch unbound contract", func(t *testing.T) {
		_, err := cache.getBinding(consts.ContractNameRMNProxy)
		require.ErrorIs(t, err, contractreader.ErrNoBindings)
	})
}

func getRandomPubKey(t *testing.T) solana.PublicKey {
	t.Helper()
	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	return privKey.PublicKey()
}

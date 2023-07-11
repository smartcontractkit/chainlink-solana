package cltest

import (
	"context"
	"fmt"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	clientmocks "github.com/smartcontractkit/chainlink-relay/pkg/headtracker/types/mocks"
	commontypes "github.com/smartcontractkit/chainlink-relay/pkg/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/internal/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
)

// TODO: write tests for this package

// Chain Specific Test utils

// Head returns a new Head with the given block height
func Head(val interface{}) *types.Head {
	time := solana.UnixTimeSeconds(0)
	chainId := types.Mainnet
	blockHeight := getBlockHeight(val)
	parentSlot := getParentSlot(blockHeight)
	block := getBlock(blockHeight, parentSlot, time)
	h := createHead(val, blockHeight, block, chainId)
	return h
}

func getBlockHeight(val interface{}) uint64 {
	switch t := val.(type) {
	case int:
		return uint64(t)
	case uint64:
		return t
	case int64:
		return uint64(t)
	case *big.Int:
		return t.Uint64()
	default:
		panic(fmt.Sprintf("Could not convert %v of type %T to Head", val, val))
	}
}

func getParentSlot(blockHeight uint64) uint64 {
	if blockHeight > 1 {
		return blockHeight - 1
	}
	return 0
}

func getBlock(blockHeight, parentSlot uint64, time solana.UnixTimeSeconds) rpc.GetBlockResult {
	return rpc.GetBlockResult{
		Blockhash:         utils.NewHash(),
		PreviousBlockhash: utils.NewHash(),
		ParentSlot:        parentSlot,
		Transactions:      nil,
		Rewards:           nil,
		BlockTime:         &time,
		BlockHeight:       &blockHeight,
	}
}

func createHead(val interface{}, blockHeight uint64, block rpc.GetBlockResult, chainId types.ChainID) *types.Head {
	switch t := val.(type) {
	case int, uint64:
		return types.NewHead(int64(blockHeight), block, nil, chainId)
	case int64:
		return types.NewHead(t, block, nil, chainId)
	case *big.Int:
		return types.NewHead(t.Int64(), block, nil, chainId)
	default:
		panic(fmt.Sprintf("Could not convert %v of type %T to Head", val, val))
	}
}

// MockHeadTrackable allows you to mock HeadTrackable
type MockHeadTrackable struct {
	onNewHeadCount atomic.Int32
}

// OnNewLongestChain increases the OnNewLongestChainCount count by one
func (m *MockHeadTrackable) OnNewLongestChain(context.Context, *types.Head) {
	m.onNewHeadCount.Add(1)
}

// OnNewLongestChainCount returns the count of new heads, safely.
func (m *MockHeadTrackable) OnNewLongestChainCount() int32 {
	return m.onNewHeadCount.Load()
}

type Awaiter chan struct{}

func NewAwaiter() Awaiter { return make(Awaiter) }

func (a Awaiter) ItHappened() { close(a) }

func (a Awaiter) AssertHappened(t *testing.T, expected bool) {
	t.Helper()
	select {
	case <-a:
		if !expected {
			t.Fatal("It happened")
		}
	default:
		if expected {
			t.Fatal("It didn't happen")
		}
	}
}

func (a Awaiter) AwaitOrFail(t testing.TB, durationParams ...time.Duration) {
	t.Helper()

	duration := 10 * time.Second
	if len(durationParams) > 0 {
		duration = durationParams[0]
	}

	select {
	case <-a:
	case <-time.After(duration):
		t.Fatal("Timed out waiting for Awaiter to get ItHappened")
	}
}

func NewClientMock(t *testing.T) *clientmocks.Client[
	*types.Head,
	commontypes.Subscription,
	types.ChainID,
	types.Hash] {
	return clientmocks.NewClient[*types.Head, commontypes.Subscription,
		types.ChainID, types.Hash](t)
}

func NewClientMockWithDefaultChain(t *testing.T) *clientmocks.Client[
	*types.Head,
	commontypes.Subscription,
	types.ChainID,
	types.Hash] {
	c := NewClientMock(t)
	c.On("ConfiguredChainID").Return(types.Mainnet).Maybe()
	c.On("IsL2").Return(false).Maybe()
	return c
}

func ConfigureBlockResult() rpc.GetBlockResult {
	result := rpc.GetBlockResult{
		Blockhash:         utils.NewHash(),
		PreviousBlockhash: utils.NewHash(),
		ParentSlot:        0,
		Transactions:      []rpc.TransactionWithMeta{},
		Signatures:        []solana.Signature{},
		Rewards:           []rpc.BlockReward{},
		BlockTime:         nil,
		BlockHeight:       nil,
	}
	return result
}

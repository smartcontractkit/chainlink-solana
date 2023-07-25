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
		Blockhash:         utils.NewSolanaHash(),
		PreviousBlockhash: utils.NewSolanaHash(),
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

// Blocks - a helper logic to construct a range of linked heads
// and an ability to fork and create logs from them
type Blocks struct {
	t       *testing.T
	Hashes  []types.Hash
	mHashes map[int64]types.Hash
	Heads   map[int64]*types.Head
}

func (b *Blocks) Head(number uint64) *types.Head {
	return b.Heads[int64(number)]
}

func NewBlocks(t *testing.T, numHashes int) *Blocks {
	hashes := make([]types.Hash, 0)
	heads := make(map[int64]*types.Head)
	for i := int64(0); i < int64(numHashes); i++ {
		hash := utils.NewHash()
		hashes = append(hashes, hash)

		heads[i] = Head(i)
		if i > 0 {
			parent := heads[i-1]
			heads[i].Parent = parent
			heads[i].Block.PreviousBlockhash = parent.BlockHash().Hash
		}
	}

	hashesMap := make(map[int64]types.Hash)
	for i := 0; i < len(hashes); i++ {
		hashesMap[int64(i)] = hashes[i]
	}

	return &Blocks{
		t:       t,
		Hashes:  hashes,
		mHashes: hashesMap,
		Heads:   heads,
	}
}

func (b *Blocks) NewHead(number uint64) *types.Head {
	parentNumber := number - 1
	parent, ok := b.Heads[int64(parentNumber)]
	if !ok {
		b.t.Fatalf("Can't find parent block at index: %v", parentNumber)
	}

	head := Head(number)
	head.Parent = parent
	head.Block.PreviousBlockhash = parent.BlockHash().Hash

	return head
}

func (b *Blocks) ForkAt(t *testing.T, blockNum int64, numHashes int) *Blocks {
	forked := NewBlocks(t, len(b.Heads)+numHashes)
	if _, exists := forked.Heads[blockNum]; !exists {
		t.Fatalf("Not enough length for block num: %v", blockNum)
	}

	for i := int64(0); i < blockNum; i++ {
		forked.Heads[i] = b.Heads[i]
	}

	forked.Heads[blockNum].Block.PreviousBlockhash = b.Heads[blockNum].Block.PreviousBlockhash
	forked.Heads[blockNum].Parent = b.Heads[blockNum].Parent
	return forked
}

// HeadBuffer - stores heads in sequence, with increasing timestamps
type HeadBuffer struct {
	t     *testing.T
	Heads []*types.Head
}

func NewHeadBuffer(t *testing.T) *HeadBuffer {
	return &HeadBuffer{
		t:     t,
		Heads: make([]*types.Head, 0),
	}
}

func (hb *HeadBuffer) Append(head *types.Head) {
	// Create a copy of the head, so that we can modify it
	cloned := &types.Head{
		Slot:   head.Slot,
		Block:  head.Block,
		Parent: head.Parent,
		ID:     head.ID,
	}
	hb.Heads = append(hb.Heads, cloned)
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
		Blockhash:         utils.NewSolanaHash(),
		PreviousBlockhash: utils.NewSolanaHash(),
		ParentSlot:        0,
		Transactions:      []rpc.TransactionWithMeta{},
		Signatures:        []solana.Signature{},
		Rewards:           []rpc.BlockReward{},
		BlockTime:         nil,
		BlockHeight:       nil,
	}
	return result
}

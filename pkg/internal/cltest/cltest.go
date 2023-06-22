package cltest

import (
	"context"
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-solana/pkg/internal/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
)

// Head given the value convert it into an Head
func Head(val interface{}) *types.Head {
	var h *types.Head
	time := solana.UnixTimeSeconds(0)
	blockHeight := uint64(0)
	block := rpc.GetBlockResult{
		Blockhash:         utils.NewHash(),
		PreviousBlockhash: utils.NewHash(),
		ParentSlot:        0,
		Transactions:      nil,
		Rewards:           nil,
		BlockTime:         &time,
		BlockHeight:       &blockHeight,
	}
	chainId := types.Mainnet

	switch t := val.(type) {
	case int:
		h = types.NewHead(int64(t), block, nil, chainId)
	case uint64:
		h = types.NewHead(int64(t), block, nil, chainId)
	case int64:
		h = types.NewHead(t, block, nil, chainId)
	case *big.Int:
		h = types.NewHead(t.Int64(), block, nil, chainId)
	default:
		panic(fmt.Sprintf("Could not convert %v of type %T to Head", val, val))
	}
	return h
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

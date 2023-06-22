package cltest

import (
	"fmt"
	"math/big"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-solana/pkg/internal/utils"
	headtracker "github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
)

// Head given the value convert it into an Head
func Head(val interface{}) *headtracker.Head {
	var h *headtracker.Head
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
	chainId := headtracker.Mainnet

	switch t := val.(type) {
	case int:
		h = headtracker.NewHead(int64(t), block, nil, chainId)
	case uint64:
		h = headtracker.NewHead(int64(t), block, nil, chainId)
	case int64:
		h = headtracker.NewHead(t, block, nil, chainId)
	case *big.Int:
		h = headtracker.NewHead(t.Int64(), block, nil, chainId)
	default:
		panic(fmt.Sprintf("Could not convert %v of type %T to Head", val, val))
	}
	return h
}

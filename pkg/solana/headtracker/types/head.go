package headtracker

import (
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	htrktypes "github.com/smartcontractkit/chainlink-solana/pkg/common/headtracker/types"
	commontypes "github.com/smartcontractkit/chainlink-solana/pkg/common/types"
)

var _ commontypes.Head[Hash] = (*SolanaHead)(nil)
var _ htrktypes.Head[Hash, ChainID] = (*SolanaHead)(nil)

type SolanaHead struct {
	Slot   int64
	Block  rpc.GetBlockResult
	Parent *SolanaHead
	ID     ChainID
}

func (h *SolanaHead) BlockNumber() int64 {
	return h.Slot
}

// ChainLength returns the length of the chain followed by recursively looking up parents
func (h *SolanaHead) ChainLength() uint32 {
	if h == nil {
		return 0
	}
	l := uint32(1)

	for {
		if h.Parent != nil {
			l++
			if h == h.Parent {
				panic("circular reference detected")
			}
			h = h.Parent
		} else {
			break
		}
	}
	return l
}

func (h *SolanaHead) EarliestHeadInChain() commontypes.Head[Hash] {
	return h.earliestInChain()
}

func (h *SolanaHead) earliestInChain() *SolanaHead {
	for h.Parent != nil {
		h = h.Parent
	}
	return h
}

func (h *SolanaHead) BlockHash() Hash {
	return Hash{Hash: h.blockHash()}
}

func (h *SolanaHead) blockHash() solana.Hash {
	return h.Block.Blockhash
}

func (h *SolanaHead) GetParent() commontypes.Head[Hash] {
	if h.Parent == nil {
		return nil
	}
	return h.Parent
}

func (h *SolanaHead) GetParentHash() Hash {
	if h.Parent == nil {
		return Hash{}
	}
	return h.Parent.BlockHash()
}

func (h *SolanaHead) HashAtHeight(slotNum int64) Hash {
	for {
		if h.Slot == slotNum {
			return h.BlockHash()
		}
		if h.Parent != nil {
			h = h.Parent
		} else {
			break
		}
	}
	return Hash{}
}

func (h *SolanaHead) ChainID() ChainID {
	return h.ID
}

func (h *SolanaHead) HasChainID() bool {
	return h.ID.String() != "unknown" // TODO: Refactor this into a more coherent check
}

func (h *SolanaHead) IsValid() bool {
	return h != nil
}

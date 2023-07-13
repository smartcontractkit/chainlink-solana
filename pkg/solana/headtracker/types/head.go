package types

import (
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	htrktypes "github.com/smartcontractkit/chainlink-relay/pkg/headtracker/types"
	commontypes "github.com/smartcontractkit/chainlink-relay/pkg/types"
)

var _ commontypes.Head[Hash] = (*Head)(nil)
var _ htrktypes.Head[Hash, ChainID] = (*Head)(nil)

type Head struct {
	Slot   int64
	Block  rpc.GetBlockResult
	Parent *Head
	ID     ChainID
}

// NewHead returns an instance of Head
func NewHead(slot int64, block rpc.GetBlockResult, parent *Head, id ChainID) *Head {
	return &Head{
		Slot:   slot,
		Block:  block,
		Parent: parent,
		ID:     id,
	}
}

func (h *Head) BlockNumber() int64 {
	return h.Slot
}

// ChainLength returns the length of the chain followed by recursively looking up parents
func (h *Head) ChainLength() uint32 {
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

func (h *Head) EarliestHeadInChain() commontypes.Head[Hash] {
	return h.earliestInChain()
}

func (h *Head) earliestInChain() *Head {
	for h.Parent != nil {
		h = h.Parent
	}
	return h
}

func (h *Head) BlockHash() Hash {
	return Hash{Hash: h.blockHash()}
}

func (h *Head) blockHash() solana.Hash {
	return h.Block.Blockhash
}

func (h *Head) GetParent() commontypes.Head[Hash] {
	if h.Parent == nil {
		return nil
	}
	return h.Parent
}

func (h *Head) GetParentHash() Hash {
	return Hash{Hash: h.Block.PreviousBlockhash}
}

func (h *Head) HashAtHeight(slotNum int64) Hash {
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

func (h *Head) ChainID() ChainID {
	return h.ID
}

func (h *Head) HasChainID() bool {
	if h == nil {
		return false
	}
	return h.ChainID().String() != "unknown"
}

func (h *Head) IsValid() bool {
	return h != nil
}

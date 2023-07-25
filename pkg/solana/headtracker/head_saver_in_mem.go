package headtracker

import (
	"context"
	"errors"
	"sync"

	htrktypes "github.com/smartcontractkit/chainlink-relay/pkg/headtracker/types"
	"github.com/smartcontractkit/chainlink-relay/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-relay/pkg/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/headtracker/types"
)

type InMemoryHeadSaver[H htrktypes.Head[BLOCK_HASH, CHAIN_ID], BLOCK_HASH commontypes.Hashable, CHAIN_ID commontypes.ID] struct {
	config      htrktypes.Config
	logger      logger.Logger
	latestHead  H
	Heads       map[BLOCK_HASH]H
	HeadsNumber map[int64][]H
	mu          sync.RWMutex
	getNilHead  func() H
	getNilHash  func() BLOCK_HASH
	setParent   func(H, H)
}

type HeadSaver = InMemoryHeadSaver[*types.Head, types.Hash, types.ChainID]

var _ commontypes.HeadSaver[*types.Head, types.Hash] = (*HeadSaver)(nil)

func NewInMemoryHeadSaver[
	H htrktypes.Head[BLOCK_HASH, CHAIN_ID],
	BLOCK_HASH commontypes.Hashable,
	CHAIN_ID commontypes.ID](
	config htrktypes.Config,
	lggr logger.Logger,
	getNilHead func() H,
	getNilHash func() BLOCK_HASH,
	setParent func(H, H),
) *InMemoryHeadSaver[H, BLOCK_HASH, CHAIN_ID] {
	return &InMemoryHeadSaver[H, BLOCK_HASH, CHAIN_ID]{
		config:      config,
		logger:      logger.Named(lggr, "InMemoryHeadSaver"),
		Heads:       make(map[BLOCK_HASH]H),
		HeadsNumber: make(map[int64][]H),
		getNilHead:  getNilHead,
		getNilHash:  getNilHash,
		setParent:   setParent,
	}
}

// Creates a new In Memory HeadSaver for solana
func NewSaver(config htrktypes.Config, lggr logger.Logger) *HeadSaver {
	return NewInMemoryHeadSaver[*types.Head, types.Hash, types.ChainID](
		config,
		lggr,
		func() *types.Head { return nil },
		func() types.Hash { return types.Hash{} },
		func(head, parent *types.Head) { head.Parent = parent },
	)
}

func (hs *InMemoryHeadSaver[H, BLOCK_HASH, CHAIN_ID]) Save(ctx context.Context, head H) error {
	if !head.IsValid() {
		return errors.New("invalid head passed to Save method of InMemoryHeadSaver")
	}

	historyDepth := int64(hs.config.HeadTrackerHistoryDepth())
	hs.AddHeads(historyDepth, head)

	return nil
}

// No OP function for Solana
func (hs *InMemoryHeadSaver[H, BLOCK_HASH, CHAIN_ID]) Load(ctx context.Context) (H, error) {
	return hs.LatestChain(), nil
}

func (hs *InMemoryHeadSaver[H, BLOCK_HASH, CHAIN_ID]) LatestChain() H {
	head := hs.getLatestHead()

	if head.ChainLength() < hs.config.FinalityDepth() {
		hs.logger.Debugw("chain shorter than EvmFinalityDepth", "chainLen", head.ChainLength(), "evmFinalityDepth", hs.config.FinalityDepth())
	}
	return head
}

func (hs *InMemoryHeadSaver[H, BLOCK_HASH, CHAIN_ID]) Chain(blockHash BLOCK_HASH) H {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	if head, exists := hs.Heads[blockHash]; exists {
		return head
	}

	return hs.getNilHead()
}

func (hs *InMemoryHeadSaver[H, BLOCK_HASH, CHAIN_ID]) HeadByNumber(blockNumber int64) []H {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	if heads, exists := hs.HeadsNumber[blockNumber]; exists {
		return heads
	}

	return []H{}
}

func (hs *InMemoryHeadSaver[H, BLOCK_HASH, CHAIN_ID]) HeadByHash(hash BLOCK_HASH) (H, error) {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	if head, exists := hs.Heads[hash]; exists {
		return head, nil
	}

	return hs.getNilHead(), errors.New("head not found")
}

// Assembles the heads together and populates the Heads Map
func (hs *InMemoryHeadSaver[H, BLOCK_HASH, CHAIN_ID]) AddHeads(historyDepth int64, newHeads ...H) {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	hs.trimHeads(historyDepth)
	for _, head := range newHeads {
		blockHash := head.BlockHash()
		blockNumber := head.BlockNumber()
		parentHash := head.GetParentHash()

		if _, exists := hs.Heads[blockHash]; exists {
			continue
		}

		if parentHash != hs.getNilHash() {
			if parent, exists := hs.Heads[parentHash]; exists {
				hs.setParent(head, parent)
			} else {
				// If parent's head is too old, we should set it to nil
				hs.setParent(head, hs.getNilHead())
			}
		}

		hs.Heads[blockHash] = head
		hs.HeadsNumber[blockNumber] = append(hs.HeadsNumber[blockNumber], head)

		// Set the parent of the existing heads to the new heads added
		for _, existingHead := range hs.Heads {
			parentHash := existingHead.GetParentHash()
			if parentHash != hs.getNilHash() {
				if parent, exists := hs.Heads[parentHash]; exists {
					hs.setParent(existingHead, parent)
				}
			}
		}

		if !hs.latestHead.IsValid() {
			hs.latestHead = head
		} else if head.BlockNumber() > hs.latestHead.BlockNumber() {
			hs.latestHead = head
		}
	}
}

func (hs *InMemoryHeadSaver[H, BLOCK_HASH, CHAIN_ID]) TrimOldHeads(historyDepth int64) {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	hs.trimHeads(historyDepth)
}

// trimHeads() is should only be called by functions with mutex locking.
// trimHeads() is an internal function without locking to prevent deadlocks
func (hs *InMemoryHeadSaver[H, BLOCK_HASH, CHAIN_ID]) trimHeads(historyDepth int64) {
	for headNumber, headNumberList := range hs.HeadsNumber {
		if hs.latestHead.BlockNumber()-headNumber >= historyDepth {
			for _, head := range headNumberList {
				delete(hs.Heads, head.BlockHash())
			}

			delete(hs.HeadsNumber, headNumber)
		}
	}
}

func (hs *InMemoryHeadSaver[H, BLOCK_HASH, CHAIN_ID]) getLatestHead() H {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	return hs.latestHead
}

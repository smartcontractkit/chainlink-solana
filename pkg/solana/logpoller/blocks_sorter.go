package logpoller

import (
	"container/list"
	"context"
	"sync"
	"sync/atomic"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
)

type blocksSorter struct {
	// service state management
	services.Service
	engine *services.Engine
	lggr   logger.Logger

	inBlocks             <-chan Block
	wontReceiveNewBlocks atomic.Bool
	receivedNewBlock     chan struct{}

	outBlocks chan Block

	mu          sync.Mutex
	queue       *list.List
	readyBlocks map[uint64]Block
}

func newBlocksSorter(inBlocks <-chan Block, lggr logger.Logger, expectedBlocks []uint64) (*blocksSorter, <-chan Block) {
	op := &blocksSorter{
		queue:            list.New(),
		readyBlocks:      make(map[uint64]Block),
		inBlocks:         inBlocks,
		outBlocks:        make(chan Block, 16),
		receivedNewBlock: make(chan struct{}, 1),
		lggr:             lggr,
	}

	for _, b := range expectedBlocks {
		op.queue.PushBack(b)
	}

	op.Service, op.engine = services.Config{
		Name:  "blocksSorter",
		Start: op.start,
		Close: nil,
	}.NewServiceEngine(lggr)

	return op, op.outBlocks
}

// ExpectBlocks should be called in block order to preserve block progression.
func (p *blocksSorter) ExpectBlocks(blocks ...uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, b := range blocks {
		p.queue.PushBack(b)
	}
}

func (p *blocksSorter) start(_ context.Context) error {
	p.engine.Go(p.writeOrderedBlocks)
	p.engine.Go(p.readBlocks)
	return nil
}

func (p *blocksSorter) readBlocks(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case block, ok := <-p.inBlocks:
			if !ok {
				p.wontReceiveNewBlocks.Store(true)
				close(p.receivedNewBlock) // trigger last flush of ready blocks
				return
			}

			p.mu.Lock()
			p.readyBlocks[block.SlotNumber] = block
			p.mu.Unlock()
			// try leaving a msg that new block is ready
			select {
			case p.receivedNewBlock <- struct{}{}:
			default:
			}
		}
	}
}

func (p *blocksSorter) writeOrderedBlocks(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.receivedNewBlock:
			if p.sendReadyBlocks(ctx) {
				return
			}
		}
	}
}

func (p *blocksSorter) readNextReadyBlock() *Block {
	p.mu.Lock()
	defer p.mu.Unlock()
	element := p.queue.Front()
	if element == nil {
		return nil
	}

	slotNumber := element.Value.(uint64)
	block, ok := p.readyBlocks[slotNumber]
	if !ok {
		return nil
	}

	p.queue.Remove(element)
	return &block
}

// sendReadyBlocks - sends all blocks in order defined by queue to the consumer.
// Returns true, when blocks provider (inBlocks) signaled that we won't receive any new blocks
// and we've sent all that were ready.
func (p *blocksSorter) sendReadyBlocks(ctx context.Context) bool {
	// start at the lowest block and find ready blocks
	for {
		block := p.readNextReadyBlock()
		if block == nil {
			break
		}

		select {
		case p.outBlocks <- *block:
		case <-ctx.Done():
			break
		}
	}

	if p.wontReceiveNewBlocks.Load() {
		// signal to consumer that work is done
		close(p.outBlocks)
		return true
	}

	return false
}

package logpoller

import (
	"container/list"
	"context"
	"sync"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
)

const blocksChBuffer = 16

type blocksSorter struct {
	// service state management
	services.Service
	engine *services.Engine
	lggr   logger.Logger

	inBlocks         <-chan Block
	receivedNewBlock chan struct{}

	outBlocks chan Block

	mu          sync.Mutex
	queue       *list.List
	readyBlocks map[uint64]Block
}

// newBlocksSorter - returns new instance of blocksSorter that writes blocks into output channel in order defined by expectedBlocks.
func newBlocksSorter(inBlocks <-chan Block, lggr logger.Logger, expectedBlocks []uint64) (*blocksSorter, <-chan Block) {
	op := &blocksSorter{
		queue:            list.New(),
		readyBlocks:      make(map[uint64]Block),
		inBlocks:         inBlocks,
		outBlocks:        make(chan Block, blocksChBuffer),
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
		case _, ok := <-p.receivedNewBlock:
			p.flushReadyBlocks(ctx)
			if !ok {
				p.mu.Lock()
				// signal to consumer that work is done, when it's actually done
				if p.queue.Len() == 0 {
					close(p.outBlocks)
				}
				p.mu.Unlock()
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

// flushReadyBlocks - sends all blocks in order defined by queue to the consumer.
func (p *blocksSorter) flushReadyBlocks(ctx context.Context) {
	for {
		block := p.readNextReadyBlock()
		if block == nil {
			return
		}

		select {
		case p.outBlocks <- *block:
		case <-ctx.Done():
			return
		}
	}
}

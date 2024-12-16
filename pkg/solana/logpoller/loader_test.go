package logpoller_test

import (
	"context"
	"crypto/rand"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	mocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
)

var (
	messages = []string{
		"Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 invoke [1]",
		"Program log: Instruction: CreateLog",
		"Program data: HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA",
		"Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 consumed 1477 of 200000 compute units",
		"Program J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4 success",
	}
)

func TestEncodedLogCollector_StartClose(t *testing.T) {
	t.Parallel()

	client := new(mocks.RPCClient)
	ctx := tests.Context(t)

	collector := logpoller.NewEncodedLogCollector(client, nil, logger.Nop())

	assert.NoError(t, collector.Start(ctx))
	assert.NoError(t, collector.Close())
}

func TestEncodedLogCollector_ParseSingleEvent(t *testing.T) {
	t.Parallel()

	client := new(mocks.RPCClient)
	parser := new(testParser)
	ctx := tests.Context(t)

	collector := logpoller.NewEncodedLogCollector(client, parser, logger.Nop())

	require.NoError(t, collector.Start(ctx))
	t.Cleanup(func() {
		require.NoError(t, collector.Close())
	})

	var latest atomic.Uint64

	latest.Store(uint64(40))

	client.EXPECT().
		GetLatestBlockhash(mock.Anything, rpc.CommitmentFinalized).
		RunAndReturn(latestBlockhashReturnFunc(&latest))

	client.EXPECT().
		GetBlocks(
			mock.Anything,
			mock.MatchedBy(getBlocksStartValMatcher),
			mock.MatchedBy(getBlocksEndValMatcher(&latest)),
			rpc.CommitmentFinalized,
		).
		RunAndReturn(getBlocksReturnFunc(false))

	client.EXPECT().
		GetBlockWithOpts(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, slot uint64, _ *rpc.GetBlockOpts) (*rpc.GetBlockResult, error) {
			height := slot - 1

			result := rpc.GetBlockResult{
				Transactions: []rpc.TransactionWithMeta{},
				Signatures:   []solana.Signature{},
				BlockHeight:  &height,
			}

			_, _ = rand.Read(result.Blockhash[:])

			if slot == 42 {
				var sig solana.Signature
				_, _ = rand.Read(sig[:])

				result.Signatures = []solana.Signature{sig}
				result.Transactions = []rpc.TransactionWithMeta{
					{
						Meta: &rpc.TransactionMeta{
							LogMessages: messages,
						},
					},
				}
			}

			return &result, nil
		})

	tests.AssertEventually(t, func() bool {
		return parser.Called()
	})
}

func TestEncodedLogCollector_MultipleEventOrdered(t *testing.T) {
	t.Parallel()

	client := new(mocks.RPCClient)
	parser := new(testParser)
	ctx := tests.Context(t)

	collector := logpoller.NewEncodedLogCollector(client, parser, logger.Nop())

	require.NoError(t, collector.Start(ctx))
	t.Cleanup(func() {
		require.NoError(t, collector.Close())
	})

	var latest atomic.Uint64

	latest.Store(uint64(40))

	slots := []uint64{44, 43, 42, 41}
	sigs := make([]solana.Signature, len(slots))
	hashes := make([]solana.Hash, len(slots))
	scrambler := &slotUnsync{ch: make(chan struct{})}

	for idx := range len(sigs) {
		_, _ = rand.Read(sigs[idx][:])
		_, _ = rand.Read(hashes[idx][:])
	}

	client.EXPECT().
		GetLatestBlockhash(mock.Anything, rpc.CommitmentFinalized).
		RunAndReturn(latestBlockhashReturnFunc(&latest))

	client.EXPECT().
		GetBlocks(
			mock.Anything,
			mock.MatchedBy(getBlocksStartValMatcher),
			mock.MatchedBy(getBlocksEndValMatcher(&latest)),
			rpc.CommitmentFinalized,
		).
		RunAndReturn(getBlocksReturnFunc(false))

	client.EXPECT().
		GetBlockWithOpts(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, slot uint64, _ *rpc.GetBlockOpts) (*rpc.GetBlockResult, error) {
			slotIdx := -1
			for idx, slt := range slots {
				if slt == slot {
					slotIdx = idx

					break
				}
			}

			// imitate loading block data out of order
			// every other block must wait for the block previous
			scrambler.next()

			height := slot - 1

			if slotIdx == -1 {
				var hash solana.Hash
				_, _ = rand.Read(hash[:])

				return &rpc.GetBlockResult{
					Blockhash:    hash,
					Transactions: []rpc.TransactionWithMeta{},
					Signatures:   []solana.Signature{},
					BlockHeight:  &height,
				}, nil
			}

			return &rpc.GetBlockResult{
				Blockhash: hashes[slotIdx],
				Transactions: []rpc.TransactionWithMeta{
					{
						Meta: &rpc.TransactionMeta{
							LogMessages: messages,
						},
					},
				},
				Signatures:  []solana.Signature{sigs[slotIdx]},
				BlockHeight: &height,
			}, nil
		})

	tests.AssertEventually(t, func() bool {
		return reflect.DeepEqual(parser.Events(), []logpoller.ProgramEvent{
			{
				BlockData: logpoller.BlockData{
					SlotNumber:          41,
					BlockHeight:         40,
					BlockHash:           hashes[3],
					TransactionHash:     sigs[3],
					TransactionIndex:    0,
					TransactionLogIndex: 0,
				},
				Prefix: ">",
				Data:   "HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA",
			},
			{
				BlockData: logpoller.BlockData{
					SlotNumber:          42,
					BlockHeight:         41,
					BlockHash:           hashes[2],
					TransactionHash:     sigs[2],
					TransactionIndex:    0,
					TransactionLogIndex: 0,
				},
				Prefix: ">",
				Data:   "HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA",
			},
			{
				BlockData: logpoller.BlockData{
					SlotNumber:          43,
					BlockHeight:         42,
					BlockHash:           hashes[1],
					TransactionHash:     sigs[1],
					TransactionIndex:    0,
					TransactionLogIndex: 0,
				},
				Prefix: ">",
				Data:   "HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA",
			},
			{
				BlockData: logpoller.BlockData{
					SlotNumber:          44,
					BlockHeight:         43,
					BlockHash:           hashes[0],
					TransactionHash:     sigs[0],
					TransactionIndex:    0,
					TransactionLogIndex: 0,
				},
				Prefix: ">",
				Data:   "HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA",
			},
		})
	})

	client.AssertExpectations(t)
}

type slotUnsync struct {
	ch      chan struct{}
	waiting atomic.Bool
}

func (u *slotUnsync) next() {
	if u.waiting.Load() {
		u.waiting.Store(false)

		<-u.ch

		return
	}

	u.waiting.Store(true)

	u.ch <- struct{}{}
}

func TestEncodedLogCollector_BackfillForAddress(t *testing.T) {
	t.Parallel()

	client := new(mocks.RPCClient)
	parser := new(testParser)
	ctx := tests.Context(t)

	collector := logpoller.NewEncodedLogCollector(client, parser, logger.Nop())

	require.NoError(t, collector.Start(ctx))
	t.Cleanup(func() {
		require.NoError(t, collector.Close())
	})

	pubKey := solana.PublicKey{2, 1, 4, 2}
	slots := []uint64{44, 43, 42}
	sigs := make([]solana.Signature, len(slots)*2)

	for idx := range len(sigs) {
		_, _ = rand.Read(sigs[idx][:])
	}

	var latest atomic.Uint64

	latest.Store(uint64(40))

	// GetLatestBlockhash might be called at start-up; make it take some time because the result isn't needed for this test
	client.EXPECT().
		GetLatestBlockhash(mock.Anything, rpc.CommitmentFinalized).
		RunAndReturn(latestBlockhashReturnFunc(&latest)).
		After(2 * time.Second).
		Maybe()

	client.EXPECT().
		GetBlocks(
			mock.Anything,
			mock.MatchedBy(getBlocksStartValMatcher),
			mock.MatchedBy(getBlocksEndValMatcher(&latest)),
			rpc.CommitmentFinalized,
		).
		RunAndReturn(getBlocksReturnFunc(true))

	client.EXPECT().
		GetSignaturesForAddressWithOpts(mock.Anything, pubKey, mock.Anything).
		RunAndReturn(func(_ context.Context, pk solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error) {
			ret := []*rpc.TransactionSignature{}

			if opts != nil && opts.Before.String() == (solana.Signature{}).String() {
				for idx := range slots {
					ret = append(ret, &rpc.TransactionSignature{Slot: slots[idx], Signature: sigs[idx*2]})
					ret = append(ret, &rpc.TransactionSignature{Slot: slots[idx], Signature: sigs[(idx*2)+1]})
				}
			}

			return ret, nil
		})

	client.EXPECT().
		GetBlockWithOpts(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, slot uint64, _ *rpc.GetBlockOpts) (*rpc.GetBlockResult, error) {
			idx := -1
			for sIdx, slt := range slots {
				if slt == slot {
					idx = sIdx

					break
				}
			}

			height := slot - 1

			if idx == -1 {
				return &rpc.GetBlockResult{
					Transactions: []rpc.TransactionWithMeta{},
					Signatures:   []solana.Signature{},
					BlockHeight:  &height,
				}, nil
			}

			return &rpc.GetBlockResult{
				Transactions: []rpc.TransactionWithMeta{
					{
						Meta: &rpc.TransactionMeta{
							LogMessages: messages,
						},
					},
					{
						Meta: &rpc.TransactionMeta{
							LogMessages: messages,
						},
					},
				},
				Signatures:  []solana.Signature{sigs[idx*2], sigs[(idx*2)+1]},
				BlockHeight: &height,
			}, nil
		})

	assert.NoError(t, collector.BackfillForAddress(ctx, pubKey.String(), 42))

	tests.AssertEventually(t, func() bool {
		return parser.Count() == 6
	})
}

func BenchmarkEncodedLogCollector(b *testing.B) {
	ctx := tests.Context(b)

	ticker := time.NewTimer(500 * time.Millisecond)
	defer ticker.Stop()

	parser := new(testParser)
	blockProducer := &testBlockProducer{
		b:         b,
		nextSlot:  10,
		blockSigs: make(map[uint64][]solana.Signature),
		sigs:      make(map[string]bool),
	}

	collector := logpoller.NewEncodedLogCollector(blockProducer, parser, logger.Nop())

	require.NoError(b, collector.Start(ctx))
	b.Cleanup(func() {
		require.NoError(b, collector.Close())
	})

	b.ReportAllocs()
	b.ResetTimer()

BenchLoop:
	for i := 0; i < b.N; i++ {
		select {
		case <-ticker.C:
			blockProducer.incrementSlot()
		case <-ctx.Done():
			break BenchLoop
		default:
			blockProducer.makeEvent()
		}
	}

	b.ReportMetric(float64(parser.Count())/b.Elapsed().Seconds(), "events/sec")
	b.ReportMetric(float64(blockProducer.Count())/b.Elapsed().Seconds(), "rcp_calls/sec")
}

type testBlockProducer struct {
	b *testing.B

	mu        sync.RWMutex
	nextSlot  uint64
	blockSigs map[uint64][]solana.Signature
	sigs      map[string]bool
	count     uint64
}

func (p *testBlockProducer) incrementSlot() {
	p.b.Helper()

	p.mu.Lock()
	defer p.mu.Unlock()

	p.nextSlot++
	p.blockSigs[p.nextSlot] = make([]solana.Signature, 0, 100)
}

func (p *testBlockProducer) makeEvent() {
	p.b.Helper()

	p.mu.Lock()
	defer p.mu.Unlock()

	var sig solana.Signature

	_, _ = rand.Read(sig[:])

	p.blockSigs[p.nextSlot] = append(p.blockSigs[p.nextSlot], sig)
	p.sigs[sig.String()] = true
}

func (p *testBlockProducer) Count() uint64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.count
}

func (p *testBlockProducer) GetLatestBlockhash(_ context.Context, _ rpc.CommitmentType) (out *rpc.GetLatestBlockhashResult, err error) {
	p.b.Helper()

	p.mu.Lock()
	p.count++
	p.mu.Unlock()

	p.mu.RLock()
	defer p.mu.RUnlock()

	return &rpc.GetLatestBlockhashResult{
		RPCContext: rpc.RPCContext{
			Context: rpc.Context{
				Slot: p.nextSlot,
			},
		},
	}, nil
}

func (p *testBlockProducer) GetBlocks(_ context.Context, startSlot uint64, endSlot *uint64, _ rpc.CommitmentType) (out rpc.BlocksResult, err error) {
	p.b.Helper()

	p.mu.Lock()
	p.count++
	p.mu.Unlock()

	blocks := make([]uint64, *endSlot-startSlot)
	for idx := range blocks {
		blocks[idx] = startSlot + uint64(idx)
	}

	return rpc.BlocksResult(blocks), nil
}

func (p *testBlockProducer) GetBlockWithOpts(_ context.Context, block uint64, opts *rpc.GetBlockOpts) (*rpc.GetBlockResult, error) {
	p.b.Helper()

	p.mu.Lock()
	defer p.mu.Unlock()

	var result rpc.GetBlockResult

	sigs := p.blockSigs[block]

	switch opts.TransactionDetails {
	case rpc.TransactionDetailsFull:
		result.Transactions = make([]rpc.TransactionWithMeta, len(sigs))
		for idx, sig := range sigs {
			delete(p.sigs, sig.String())

			result.Transactions[idx] = rpc.TransactionWithMeta{
				Slot: block,
				Meta: &rpc.TransactionMeta{
					LogMessages: messages,
				},
			}
		}
	case rpc.TransactionDetailsSignatures:
		result.Signatures = sigs
		delete(p.blockSigs, block)
	case rpc.TransactionDetailsNone:
		fallthrough
	default:
	}

	p.count++
	result.BlockHeight = &block

	return &result, nil
}

func (p *testBlockProducer) GetSignaturesForAddressWithOpts(context.Context, solana.PublicKey, *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error) {
	p.b.Helper()

	return nil, nil
}

func (p *testBlockProducer) GetTransaction(_ context.Context, sig solana.Signature, _ *rpc.GetTransactionOpts) (*rpc.GetTransactionResult, error) {
	p.b.Helper()

	p.mu.Lock()
	defer p.mu.Unlock()

	var msgs []string

	p.count++
	_, ok := p.sigs[sig.String()]
	if ok {
		msgs = messages
	}

	delete(p.sigs, sig.String())

	return &rpc.GetTransactionResult{
		Meta: &rpc.TransactionMeta{
			LogMessages: msgs,
		},
	}, nil
}

type testParser struct {
	called atomic.Bool
	mu     sync.Mutex
	events []logpoller.ProgramEvent
}

func (p *testParser) Process(event logpoller.ProgramEvent) error {
	p.called.Store(true)

	p.mu.Lock()
	p.events = append(p.events, event)
	p.mu.Unlock()

	return nil
}

func (p *testParser) Called() bool {
	return p.called.Load()
}

func (p *testParser) Count() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	return uint64(len(p.events))
}

func (p *testParser) Events() []logpoller.ProgramEvent {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.events
}

func latestBlockhashReturnFunc(latest *atomic.Uint64) func(context.Context, rpc.CommitmentType) (*rpc.GetLatestBlockhashResult, error) {
	return func(ctx context.Context, ct rpc.CommitmentType) (*rpc.GetLatestBlockhashResult, error) {
		defer func() {
			latest.Store(latest.Load() + 2)
		}()

		return &rpc.GetLatestBlockhashResult{
			RPCContext: rpc.RPCContext{
				Context: rpc.Context{
					Slot: latest.Load(),
				},
			},
			Value: &rpc.LatestBlockhashResult{
				LastValidBlockHeight: latest.Load() - 1,
			},
		}, nil
	}
}

func getBlocksReturnFunc(empty bool) func(context.Context, uint64, *uint64, rpc.CommitmentType) (rpc.BlocksResult, error) {
	return func(_ context.Context, u1 uint64, u2 *uint64, _ rpc.CommitmentType) (rpc.BlocksResult, error) {
		blocks := []uint64{}

		if !empty {
			blocks = make([]uint64, *u2-u1+1)
			for idx := range blocks {
				blocks[idx] = u1 + uint64(idx)
			}
		}

		return rpc.BlocksResult(blocks), nil
	}
}

func getBlocksStartValMatcher(val uint64) bool {
	return val > uint64(0)
}

func getBlocksEndValMatcher(latest *atomic.Uint64) func(*uint64) bool {
	return func(val *uint64) bool {
		return val != nil && *val <= latest.Load()
	}
}

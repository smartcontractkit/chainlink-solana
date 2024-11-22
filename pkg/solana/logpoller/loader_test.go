package logpoller_test

import (
	"context"
	"crypto/rand"
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
	client := new(mocks.RPCClient)
	ctx := tests.Context(t)

	collector := logpoller.NewEncodedLogCollector(client, nil, logger.Nop())

	assert.NoError(t, collector.Start(ctx))
	assert.NoError(t, collector.Close())
}

func TestEncodedLogCollector_ParseSingleEvent(t *testing.T) {
	client := new(mocks.RPCClient)
	parser := new(testParser)
	ctx := tests.Context(t)

	collector := logpoller.NewEncodedLogCollector(client, parser, logger.Nop())

	require.NoError(t, collector.Start(ctx))
	t.Cleanup(func() {
		require.NoError(t, collector.Close())
	})

	slot := uint64(42)
	sig := solana.Signature{2, 1, 4, 2}
	blockHeight := uint64(21)

	client.EXPECT().GetLatestBlockhash(mock.Anything, rpc.CommitmentFinalized).Return(&rpc.GetLatestBlockhashResult{
		RPCContext: rpc.RPCContext{
			Context: rpc.Context{
				Slot: slot,
			},
		},
	}, nil)

	client.EXPECT().GetBlocks(mock.Anything, uint64(1), mock.MatchedBy(func(val *uint64) bool {
		return val != nil && *val == slot
	}), mock.Anything).Return(rpc.BlocksResult{slot}, nil)

	client.EXPECT().GetBlockWithOpts(mock.Anything, slot, mock.Anything).Return(&rpc.GetBlockResult{
		Transactions: []rpc.TransactionWithMeta{
			{
				Meta: &rpc.TransactionMeta{
					LogMessages: messages,
				},
			},
		},
		Signatures:  []solana.Signature{sig},
		BlockHeight: &blockHeight,
	}, nil).Twice()

	tests.AssertEventually(t, func() bool {
		return parser.Called()
	})

	client.AssertExpectations(t)
}

func TestEncodedLogCollector_BackfillForAddress(t *testing.T) {
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
	blockHeights := []uint64{21, 22, 23, 50}

	for idx := range len(sigs) {
		_, _ = rand.Read(sigs[idx][:])
	}

	// GetLatestBlockhash might be called at start-up; make it take some time because the result isn't needed for this test
	client.EXPECT().GetLatestBlockhash(mock.Anything, mock.Anything).Return(&rpc.GetLatestBlockhashResult{
		RPCContext: rpc.RPCContext{
			Context: rpc.Context{
				Slot: slots[0],
			},
		},
		Value: &rpc.LatestBlockhashResult{
			LastValidBlockHeight: 42,
		},
	}, nil).After(2 * time.Second).Maybe()

	client.EXPECT().
		GetSignaturesForAddressWithOpts(mock.Anything, pubKey, mock.MatchedBy(func(opts *rpc.GetSignaturesForAddressOpts) bool {
			return opts != nil && opts.Before.String() == solana.Signature{}.String()
		})).
		Return([]*rpc.TransactionSignature{
			{Slot: slots[0], Signature: sigs[0]},
			{Slot: slots[0], Signature: sigs[1]},
			{Slot: slots[1], Signature: sigs[2]},
			{Slot: slots[1], Signature: sigs[3]},
			{Slot: slots[2], Signature: sigs[4]},
			{Slot: slots[2], Signature: sigs[5]},
		}, nil)

	client.EXPECT().GetSignaturesForAddressWithOpts(mock.Anything, pubKey, mock.Anything).Return([]*rpc.TransactionSignature{}, nil)

	for idx := range len(slots) {
		client.EXPECT().GetBlockWithOpts(mock.Anything, slots[idx], mock.Anything).Return(&rpc.GetBlockResult{
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
			BlockHeight: &blockHeights[idx],
		}, nil).Twice()
	}

	assert.NoError(t, collector.BackfillForAddress(ctx, pubKey.String(), 42))

	tests.AssertEventually(t, func() bool {
		return parser.Count() == 6
	})

	client.AssertExpectations(t)
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
	count  atomic.Uint64
}

func (p *testParser) Process(event logpoller.ProgramEvent) error {
	p.called.Store(true)
	p.count.Store(p.count.Load() + 1)

	return nil
}

func (p *testParser) Called() bool {
	return p.called.Load()
}

func (p *testParser) Count() uint64 {
	return p.count.Load()
}

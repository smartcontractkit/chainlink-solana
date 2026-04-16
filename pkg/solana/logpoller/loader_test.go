package logpoller_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
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

func TestEncodedLogCollector_MultipleEventOrdered(t *testing.T) {
	t.Parallel()

	client := mocks.NewRPCClient(t)
	ctx := t.Context()

	metrics, err := logpoller.NewSolLpMetrics("test-chain-id")
	require.NoError(t, err)

	collector := logpoller.NewEncodedLogCollector(client, logger.Test(t), "test-chain-id", metrics, nil)

	require.NoError(t, collector.Start(ctx))
	t.Cleanup(func() {
		require.NoError(t, collector.Close())
	})

	var latest atomic.Uint64

	latest.Store(uint64(40))

	address, err := solana.PublicKeyFromBase58("J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4")
	require.NoError(t, err)
	slots := []uint64{44, 43, 42, 41}
	var txSigsResponse []*rpc.TransactionSignature
	for _, slot := range slots {
		txSigsResponse = append(txSigsResponse, &rpc.TransactionSignature{Slot: slot})
	}
	client.EXPECT().GetSignaturesForAddressWithOpts(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, key solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error) {
		switch *opts.MinContextSlot {
		case 44:
			return txSigsResponse, nil
		case 41:
			return nil, nil
		default:
			panic("unexpected call")
		}
	}).Twice()

	sigs := make([]solana.Signature, len(slots))
	hashes := make([]solana.Hash, len(slots))
	scrambler := &slotUnsync{ch: make(chan struct{})}

	timeStamp := solana.UnixTimeSeconds(time.Now().Unix())

	for idx := range len(sigs) {
		_, _ = rand.Read(sigs[idx][:])
		_, _ = rand.Read(hashes[idx][:])
	}

	client.EXPECT().
		GetBlockWithOpts(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, slot uint64, _ *rpc.GetBlockOpts) (*rpc.GetBlockResult, error) {
			slotIdx := slices.Index(slots, slot)
			if slotIdx == -1 {
				require.Fail(t, "trying to get block for unexpected slot", slot)
			}

			// imitate loading block data out of order
			// every other block must wait for the block previous
			scrambler.next()

			height := slot - 1

			tx := solana.Transaction{Signatures: []solana.Signature{sigs[slotIdx]}}
			binaryTx, txErr := tx.MarshalBinary()
			require.NoError(t, txErr)
			return &rpc.GetBlockResult{
				Blockhash: hashes[slotIdx],
				Transactions: []rpc.TransactionWithMeta{
					{
						Transaction: rpc.DataBytesOrJSONFromBytes(binaryTx),
						Meta: &rpc.TransactionMeta{
							LogMessages: messages,
						},
					},
				},
				BlockHeight: &height,
				BlockTime:   &timeStamp,
			}, nil
		})

	results, cleanUp, err := collector.BackfillForAddresses(t.Context(), []types.PublicKey{types.PublicKey(address)}, 41, 44)
	require.NoError(t, err)
	defer cleanUp()
	var events []types.ProgramEvent
	for event := range results {
		events = append(events, event.Events...)
	}

	require.Equal(t, []types.ProgramEvent{
		{
			BlockData: types.BlockData{
				SlotNumber:          41,
				BlockHeight:         40,
				BlockTime:           timeStamp,
				BlockHash:           hashes[3],
				TransactionHash:     sigs[3],
				TransactionIndex:    0,
				TransactionLogIndex: 0,
			},
			Program: "J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4",
			Data:    "HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA",
		},
		{
			BlockData: types.BlockData{
				SlotNumber:          42,
				BlockHeight:         41,
				BlockTime:           timeStamp,
				BlockHash:           hashes[2],
				TransactionHash:     sigs[2],
				TransactionIndex:    0,
				TransactionLogIndex: 0,
			},
			Program: "J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4",
			Data:    "HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA",
		},
		{
			BlockData: types.BlockData{
				SlotNumber:          43,
				BlockHeight:         42,
				BlockTime:           timeStamp,
				BlockHash:           hashes[1],
				TransactionHash:     sigs[1],
				TransactionIndex:    0,
				TransactionLogIndex: 0,
			},
			Program: "J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4",
			Data:    "HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA",
		},
		{
			BlockData: types.BlockData{
				SlotNumber:          44,
				BlockHeight:         43,
				BlockTime:           timeStamp,
				BlockHash:           hashes[0],
				TransactionHash:     sigs[0],
				TransactionIndex:    0,
				TransactionLogIndex: 0,
			},
			Program: "J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4",
			Data:    "HDQnaQjSWwkNAAAASGVsbG8sIFdvcmxkISoAAAAAAAAA",
		},
	}, events)
}

func TestEncodedLogCollector_Backfill_DoesNotBlockOnRPCError(t *testing.T) {
	t.Parallel()

	client := mocks.NewRPCClient(t)

	metrics, err := logpoller.NewSolLpMetrics("test-chain-id")
	require.NoError(t, err)

	collector := logpoller.NewEncodedLogCollector(client, logger.Test(t), "test-chain-id", metrics, nil).WithMaxGroupRetryCount(2)

	ctx := t.Context()
	require.NoError(t, collector.Start(ctx))
	t.Cleanup(func() { require.NoError(t, collector.Close()) })

	address, err := solana.PublicKeyFromBase58("J1zQwrBNBngz26jRPNWsUSZMHJwBwpkoDitXRV95LdK4")
	require.NoError(t, err)

	// Backfill range [41..44]
	slots := []uint64{44, 43, 42, 41}
	txSigsResponse := make([]*rpc.TransactionSignature, 0, len(slots))
	for _, slot := range slots {
		txSigsResponse = append(txSigsResponse, &rpc.TransactionSignature{Slot: slot})
	}
	client.On("GetFirstAvailableBlock", mock.Anything).
		Return(uint64(0), nil).
		Maybe()

	// Same signature paging behavior as your ordered test.
	client.EXPECT().
		GetSignaturesForAddressWithOpts(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, key solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error) {
			switch *opts.MinContextSlot {
			case 44:
				return txSigsResponse, nil
			case 41:
				return nil, nil
			default:
				panic("unexpected call")
			}
		}).Twice()

	// Make one slot fail.
	const failingSlot uint64 = 43

	// Provide valid-looking blocks for other slots.
	sigs := make([]solana.Signature, len(slots))
	hashes := make([]solana.Hash, len(slots))
	for i := range len(slots) {
		_, _ = rand.Read(sigs[i][:])
		_, _ = rand.Read(hashes[i][:])
	}
	timeStamp := solana.UnixTimeSeconds(time.Now().Unix())

	client.EXPECT().
		GetBlockWithOpts(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, slot uint64, _ *rpc.GetBlockOpts) (*rpc.GetBlockResult, error) {
			if slot == failingSlot {
				return nil, fmt.Errorf("rpc boom for slot %d", slot)
			}

			slotIdx := slices.Index(slots, slot)
			if slotIdx == -1 {
				require.Fail(t, "trying to get block for unexpected slot", slot)
			}

			height := slot - 1

			tx := solana.Transaction{Signatures: []solana.Signature{sigs[slotIdx]}}
			binaryTx, txErr := tx.MarshalBinary()
			require.NoError(t, txErr)

			return &rpc.GetBlockResult{
				Blockhash: hashes[slotIdx],
				Transactions: []rpc.TransactionWithMeta{
					{
						Transaction: rpc.DataBytesOrJSONFromBytes(binaryTx),
						Meta: &rpc.TransactionMeta{
							LogMessages: messages,
						},
					},
				},
				BlockHeight: &height,
				BlockTime:   &timeStamp,
			}, nil
		}).Maybe()

	testCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	results, cleanUp, err := collector.BackfillForAddresses(
		testCtx,
		[]types.PublicKey{types.PublicKey(address)},
		41, 44,
	)
	require.NoError(t, err)
	defer cleanUp()

	// If BackfillForAddresses blocks due to missing slot/error, this never finishes.
	done := make(chan struct{})
	var seenAborted bool
	go func() {
		for r := range results {
			if r.Aborted {
				seenAborted = true
			}
		}
		close(done)
	}()
	require.False(t, seenAborted)
	select {
	case <-done:
		// PASS: results channel closed, so no deadlock.
	case <-testCtx.Done():
		t.Fatalf("BackfillForAddresses blocked: results channel did not close after RPC error (ctx err=%v)", testCtx.Err())
	}
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

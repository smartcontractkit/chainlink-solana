package logpoller_test

import (
	"context"
	"crypto/rand"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
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
	ctx := tests.Context(t)

	collector := logpoller.NewEncodedLogCollector(client, logger.TestSugared(t))

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

	results, cleanUp, err := collector.BackfillForAddresses(tests.Context(t), []logpoller.PublicKey{logpoller.PublicKey(address)}, 41, 44)
	require.NoError(t, err)
	defer cleanUp()
	var events []logpoller.ProgramEvent
	for event := range results {
		events = append(events, event.Events...)
	}

	require.Equal(t, []logpoller.ProgramEvent{
		{
			BlockData: logpoller.BlockData{
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
			BlockData: logpoller.BlockData{
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
			BlockData: logpoller.BlockData{
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
			BlockData: logpoller.BlockData{
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

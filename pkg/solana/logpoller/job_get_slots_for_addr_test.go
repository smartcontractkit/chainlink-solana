package logpoller

import (
	"context"
	"errors"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/mocks"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/worker"
)

func TestGetSlotsForAddressJob(t *testing.T) {
	sig, err := solana.SignatureFromBase58("4VJEi7D9ia2R4L6xgPE7bKTtNAtJ2KGHTtq1VEztEMtpcevGPzGpyvnm6EgkMCPhSQTAQ9XwdyqVYzqbf35zJyF")
	require.NoError(t, err)
	rawAddr, err := solana.PublicKeyFromBase58("Cv4T27XbjVoKUYwP72NQQanvZeA7W4YF9L4EnYT9kx5o")
	require.NoError(t, err)
	address := PublicKey(rawAddr)
	const from = uint64(10)
	const to = uint64(20)
	t.Run("String representation contains all details", func(t *testing.T) {
		job := &getSlotsForAddressJob{address: address, from: from, to: to, beforeSig: sig}
		require.Equal(t, "getSlotsForAddress: Cv4T27XbjVoKUYwP72NQQanvZeA7W4YF9L4EnYT9kx5o, from: 10, to: 20, beforeSig: 4VJEi7D9ia2R4L6xgPE7bKTtNAtJ2KGHTtq1VEztEMtpcevGPzGpyvnm6EgkMCPhSQTAQ9XwdyqVYzqbf35zJyF", job.String())
	})
	t.Run("Returns error if RPC request failed", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		expectedError := errors.New("rpc error")
		client.EXPECT().GetSignaturesForAddressWithOpts(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, key solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error) {
				require.Equal(t, address.String(), key.String())
				require.NotNil(t, opts)
				require.True(t, opts.Before.IsZero())
				require.NotNil(t, opts.MinContextSlot)
				require.Equal(t, to, *opts.MinContextSlot)
				return nil, expectedError
			}).Once()
		job := newGetSlotsForAddress(client, nil, nil, address, from, to)
		err := job.Run(tests.Context(t))
		require.ErrorIs(t, err, expectedError)
	})
	requireJobIsDone := func(t *testing.T, done <-chan struct{}, msg string) {
		select {
		case <-done:
		default:
			require.Fail(t, msg)
		}
	}
	t.Run("Completes successfully if there is no signatures", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		client.EXPECT().GetSignaturesForAddressWithOpts(mock.Anything, mock.Anything, mock.Anything).Return([]*rpc.TransactionSignature{}, nil).Once()
		job := newGetSlotsForAddress(client, nil, nil, address, from, to)
		err := job.Run(tests.Context(t))
		require.NoError(t, err)
		requireJobIsDone(t, job.Done(), "expected job to be done")
	})
	t.Run("Stores slots only if they are in range", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		var signatures []*rpc.TransactionSignature
		for _, slot := range []uint64{21, 20, 11, 10, 9} {
			if slot == 20 {
				// must be skipped due to error
				signatures = append(signatures, &rpc.TransactionSignature{Slot: 19, Err: errors.New("transaction failed")})
			}
			if slot == 10 {
				// add errored transaction before a valid into the last slot within range to ensure that we won't skip that slot
				signatures = append(signatures, &rpc.TransactionSignature{Slot: 10, Err: errors.New("transaction failed")})
			}
			signatures = append(signatures, &rpc.TransactionSignature{Slot: slot})
		}
		client.EXPECT().GetSignaturesForAddressWithOpts(mock.Anything, mock.Anything, mock.Anything).Return(signatures, nil).Once()
		var actualSlots []uint64
		job := newGetSlotsForAddress(client, nil, func(s uint64) {
			actualSlots = append(actualSlots, s)
		}, address, from, to)
		err := job.Run(tests.Context(t))
		require.NoError(t, err)
		requireJobIsDone(t, job.Done(), "expected job to be done")
		require.Equal(t, []uint64{20, 11, 10}, actualSlots)
	})
	t.Run("If slot range may have more signatures, schedules a new job", func(t *testing.T) {
		client := mocks.NewRPCClient(t)
		signatures := []*rpc.TransactionSignature{{Slot: 19, Signature: sig}}
		client.EXPECT().GetSignaturesForAddressWithOpts(mock.Anything, mock.Anything, mock.Anything).Return(signatures, nil).Once()
		workers := mocks.NewWorkerGroup(t)
		var secondJob *getSlotsForAddressJob
		workers.EXPECT().Do(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, rawJob worker.Job) error {
			job, ok := rawJob.(*getSlotsForAddressJob)
			require.True(t, ok)
			require.Equal(t, from, job.from)
			require.Equal(t, uint64(19), job.to)
			require.Equal(t, address, job.address)
			require.Equal(t, sig, job.beforeSig)
			secondJob = job
			return nil
		})
		var actualSlots []uint64
		firstJob := newGetSlotsForAddress(client, workers, func(s uint64) {
			actualSlots = append(actualSlots, s)
		}, address, from, to)
		err := firstJob.Run(tests.Context(t))
		require.NoError(t, err)
		select {
		case <-firstJob.Done():
			require.FailNow(t, "expected job to schedule second job and not to be done")
		default:
		}
		require.NotNil(t, secondJob)
		client.EXPECT().GetSignaturesForAddressWithOpts(mock.Anything, mock.Anything, mock.Anything).Return([]*rpc.TransactionSignature{{Slot: 18}, {Slot: 9}}, nil).Once()
		err = secondJob.Run(tests.Context(t))
		require.NoError(t, err)
		requireJobIsDone(t, firstJob.Done(), "expected fist job to be done")
		requireJobIsDone(t, secondJob.Done(), "expected second job to be done")
		require.Equal(t, []uint64{19, 18}, actualSlots)
	})
}

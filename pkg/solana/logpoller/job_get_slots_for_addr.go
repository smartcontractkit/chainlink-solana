package logpoller

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/worker"
)

var _ worker.Job = (*getSlotsForAddressJob)(nil)

// getSlotsForAddressJob - identifies slots that contain transactions for specified address in range [from, to] and
// calls storeSlot for each. If single request was not sufficient to identify all slots - schedules a new job. Channel
// returned by Done() will be closed only when all jobs are done.
type getSlotsForAddressJob struct {
	address   PublicKey
	beforeSig solana.Signature
	from, to  uint64

	client RPCClient

	storeSlot func(slot uint64)
	done      chan struct{}
	workers   WorkerGroup
}

func newGetSlotsForAddress(client RPCClient, workers WorkerGroup, storeSlot func(uint64), address PublicKey, from, to uint64) *getSlotsForAddressJob {
	return &getSlotsForAddressJob{
		address:   address,
		client:    client,
		from:      from,
		to:        to,
		storeSlot: storeSlot,
		workers:   workers,
		done:      make(chan struct{}),
	}
}

func (f *getSlotsForAddressJob) String() string {
	return fmt.Sprintf("getSlotsForAddress: %s, from: %d, to: %d, beforeSig: %s", f.address, f.from, f.to, f.beforeSig)
}

func (f *getSlotsForAddressJob) Done() <-chan struct{} {
	return f.done
}

func (f *getSlotsForAddressJob) Run(ctx context.Context) error {
	isDone, err := f.run(ctx)
	if err != nil {
		return err
	}

	if isDone {
		close(f.done)
	}
	return nil
}

// run - returns true, nil - if job was fully done, and we have not created a child job
func (f *getSlotsForAddressJob) run(ctx context.Context) (bool, error) {
	opts := rpc.GetSignaturesForAddressOpts{
		Commitment:     rpc.CommitmentFinalized,
		MinContextSlot: &f.to, // MinContextSlot is not filter. It defines min slot that RPC is expected to observe to handle the request
	}

	if !f.beforeSig.IsZero() {
		opts.Before = f.beforeSig
	}

	sigs, err := f.client.GetSignaturesForAddressWithOpts(ctx, f.address.ToSolana(), &opts)
	if err != nil {
		return false, fmt.Errorf("failed getting signatures for address: %w", err)
	}

	// NOTE: there is no reliable way for us to verify that RPC has sufficient history depth. Instead of
	// doing additional requests in attempt to verify it, we prefer to just trust RPC and hope that sufficient
	// number of nodes in DON were able to fetch required logs
	if len(sigs) == 0 {
		return true, nil
	}

	// signatures ordered from newest to oldest, defined in the Solana RPC docs
	for _, sig := range sigs {
		// RPC may return slots that are higher than requested. Skip them to simplify mental model.
		if sig.Slot > f.to {
			continue
		}

		if sig.Slot < f.from {
			return true, nil
		}

		// no need to fetch slot, if transaction failed
		if sig.Err == nil {
			f.storeSlot(sig.Slot)
		}
	}

	oldestSig := sigs[len(sigs)-1]
	// to ensure we do not overload RPC perform next call as a separate job
	err = f.workers.Do(ctx, &getSlotsForAddressJob{
		address:   f.address,
		beforeSig: oldestSig.Signature,
		from:      f.from,
		to:        oldestSig.Slot,
		client:    f.client,
		storeSlot: f.storeSlot,
		done:      f.done,
		workers:   f.workers,
	})
	return false, err
}

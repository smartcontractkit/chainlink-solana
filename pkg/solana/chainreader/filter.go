package chainreader

import (
	"context"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
)

type syncedFilter struct {
	// internal state properties
	mu         sync.RWMutex
	addressSet bool
	filter     logpoller.Filter
}

func newSyncedFilter() *syncedFilter {
	return &syncedFilter{}
}

func (r *syncedFilter) Register(ctx context.Context, registrar filterRegistrar) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !registrar.HasFilter(ctx, r.filter.Name) {
		if err := registrar.RegisterFilter(ctx, r.filter); err != nil {
			return FilterError{
				Err:    fmt.Errorf("%w: %s", types.ErrInternal, err.Error()),
				Action: "register",
				Filter: r.filter,
			}
		}
	}

	return nil
}

func (r *syncedFilter) Unregister(ctx context.Context, registrar filterRegistrar) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !registrar.HasFilter(ctx, r.filter.Name) {
		return nil
	}

	if err := registrar.UnregisterFilter(ctx, r.filter.Name); err != nil {
		return FilterError{
			Err:    fmt.Errorf("%w: %s", types.ErrInternal, err.Error()),
			Action: "unregister",
			Filter: r.filter,
		}
	}

	return nil
}

func (r *syncedFilter) SetFilter(filter logpoller.Filter) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.filter = filter
}

func (r *syncedFilter) SetName(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.filter.Name = name
}

func (r *syncedFilter) SetAddress(address solana.PublicKey) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.addressSet = true
	r.filter.Address = logpoller.PublicKey(address)
}

func (r *syncedFilter) AddressSet() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.addressSet
}

type FilterError struct {
	Err    error
	Action string
	Filter logpoller.Filter
}

func (e FilterError) Error() string {
	return fmt.Sprintf("[logpoller filter error] action: %s; err: %s; filter: %+v;", e.Action, e.Err.Error(), e.Filter)
}

func (e FilterError) Unwrap() error {
	return e.Err
}

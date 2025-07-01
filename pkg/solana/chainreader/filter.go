package chainreader

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

type syncedFilter struct {
	// internal state properties
	mu         sync.RWMutex
	addressSet bool
	filter     logpollertypes.Filter

	dirty bool
}

func newSyncedFilter() *syncedFilter {
	return &syncedFilter{}
}

func (r *syncedFilter) Update(ctx context.Context, registrar filterRegistrar, updatedName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.dirty {
		return nil
	}

	oldName := r.filter.Name
	r.filter.Name = updatedName

	if err := r.register(ctx, registrar); err != nil {
		return err
	}

	// filter updated successfully, it's not dirty anymore
	r.dirty = false

	return r.unregister(ctx, registrar, oldName)
}

func (r *syncedFilter) Register(ctx context.Context, registrar filterRegistrar) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.register(ctx, registrar)
}

func (r *syncedFilter) register(ctx context.Context, registrar filterRegistrar) error {
	if err := registrar.RegisterFilter(ctx, r.filter); err != nil && !errors.Is(err, logpoller.ErrFilterNameConflict) {
		return FilterError{
			Err:    fmt.Errorf("%w: %s", types.ErrInternal, err.Error()),
			Action: "register",
			Filter: r.filter,
		}
	}

	return nil
}

func (r *syncedFilter) Unregister(ctx context.Context, registrar filterRegistrar) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	err := r.unregister(ctx, registrar, r.filter.Name)
	if err != nil {
		return err
	}

	r.setAddress(solana.PublicKey{})
	r.setName("")

	r.dirty = false

	return nil
}

func (r *syncedFilter) unregister(ctx context.Context, registrar filterRegistrar, name string) error {
	if !registrar.HasFilter(ctx, name) {
		return nil
	}

	if err := registrar.UnregisterFilter(ctx, name); err != nil {
		return FilterError{
			Err:    fmt.Errorf("%w: %s", types.ErrInternal, err.Error()),
			Action: "unregister",
			Filter: r.filter,
		}
	}

	return nil
}

func (r *syncedFilter) SetFilter(filter logpollertypes.Filter) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.filter = filter
}

func (r *syncedFilter) SetName(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.setName(name)
}

func (r *syncedFilter) setName(name string) {
	r.filter.Name = name
}

func (r *syncedFilter) SetAddress(address solana.PublicKey) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.setAddress(address)
}

func (r *syncedFilter) setAddress(address solana.PublicKey) {
	r.addressSet = true

	pkAddress := logpollertypes.PublicKey(address)
	if r.filter.Address == pkAddress {
		return
	}

	r.dirty = true
	r.filter.Address = logpollertypes.PublicKey(address)
}

func (r *syncedFilter) AddressSet() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.addressSet
}

func (r *syncedFilter) Dirty() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.dirty
}

type FilterError struct {
	Err    error
	Action string
	Filter logpollertypes.Filter
}

func (e FilterError) Error() string {
	return fmt.Sprintf("[logpoller filter error] action: %s; err: %s; filter: %+v;", e.Action, e.Err.Error(), e.Filter)
}

func (e FilterError) Unwrap() error {
	return e.Err
}

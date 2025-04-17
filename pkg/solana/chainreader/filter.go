package chainreader

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
)

type syncedFilter struct {
	// internal state properties
	mu         sync.RWMutex
	addressSet bool
	filter     logpoller.Filter
	hash       string

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
	r.setName(updatedName)

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

func (r *syncedFilter) SetFilter(filter logpoller.Filter) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.filter = filter

	hash, err := filterHash(r.filter)
	if err != nil {
		return err
	}

	r.hash = hash

	return nil
}

func (r *syncedFilter) SetName(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.setName(name)
}

func (r *syncedFilter) setName(name string) {
	r.filter.Name = fmt.Sprintf("%s.%s", name, r.hash)
}

func (r *syncedFilter) SetAddress(address solana.PublicKey) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.setAddress(address)
}

func (r *syncedFilter) setAddress(address solana.PublicKey) {
	r.addressSet = true

	pkAddress := logpoller.PublicKey(address)
	if r.filter.Address == pkAddress {
		return
	}

	r.dirty = true
	r.filter.Address = logpoller.PublicKey(address)
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
	Filter logpoller.Filter
}

func (e FilterError) Error() string {
	return fmt.Sprintf("[logpoller filter error] action: %s; err: %s; filter: %+v;", e.Action, e.Err.Error(), e.Filter)
}

func (e FilterError) Unwrap() error {
	return e.Err
}

func filterHash(filter logpoller.Filter) (string, error) {
	hasher := sha256.New()

	if err := errors.Join(
		onlyError(hasher.Write, putUint64(filter.ID)),
		onlyError(hasher.Write, filter.Address.ToSolana().Bytes()),
		onlyError(hasher.Write, []byte(filter.EventName)),
		onlyError(hasher.Write, []byte(filter.EventSig.String())),
		onlyError(hasher.Write, putUint64(filter.StartingBlock)), // not sure if we should include this
		onlyError(hasher.Write, []byte(filter.SubkeyPaths.String())),
		onlyError(hasher.Write, putUint64(filter.Retention)),
		onlyError(hasher.Write, putUint64(filter.MaxLogsKept)),
		onlyError(hasher.Write, putUint64(binBool(filter.IsDeleted))),
		onlyError(hasher.Write, putUint64(binBool(filter.IsBackfilled))),
		onlyError(hasher.Write, putUint64(binBool(filter.IncludeReverted))),
	); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum([]byte{})), nil
}

func putUint64[T uint8 | int64 | int32 | time.Duration](val T) []byte {
	buf := make([]byte, 8)

	binary.BigEndian.PutUint64(buf, uint64(val))

	return buf
}

func binBool(val bool) uint8 {
	if val {
		return 1
	}

	return 0
}

func onlyError(fnc func([]byte) (int, error), val []byte) error {
	_, err := fnc(val)

	return err
}

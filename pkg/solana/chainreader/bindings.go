package chainreader

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
)

type filterRegistrar interface {
	HasFilter(context.Context, string) bool
	RegisterFilter(context.Context, logpoller.Filter) error
	UnregisterFilter(ctx context.Context, name string) error
}

type readBinding interface {
	Bind(context.Context, solana.PublicKey) error
	Unbind(context.Context) error
	GetAddress(context.Context, any) (solana.PublicKey, error)
	GetGenericName() string
	GetReadDefinition() config.ReadDefinition
	GetIDLInfo() (idl codec.IDL, inputIDLTypeDef interface{}, outputIDLTypeDef codec.IdlTypeDef)
	GetAddressResponseHardCoder() *commoncodec.HardCodeModifierConfig
	SetCodec(types.RemoteCodec)
	SetModifier(commoncodec.Modifier)
	Register(context.Context) error
	Unregister(context.Context) error
	CreateType(bool) (any, error)
	Decode(context.Context, []byte, any) error
	QueryKey(context.Context, query.KeyFilter, query.LimitAndSort, any) ([]types.Sequence, error)
}

type addressShareGroup struct {
	address solana.PublicKey
	mux     sync.Mutex
	group   []string
}

type bindingsRegistry struct {
	mu sync.RWMutex
	// key is namespace
	namespaceBindings map[string]*namespaceBinding
	// key is namespace
	addressShareGroups map[string]*addressShareGroup
}

func newBindingsRegistry() *bindingsRegistry {
	return &bindingsRegistry{
		namespaceBindings: make(map[string]*namespaceBinding),
	}
}

func (r *bindingsRegistry) SetCodecs(codec types.RemoteCodec) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, nbs := range r.namespaceBindings {
		nbs.SetCodecs(codec)
	}
}

func (r *bindingsRegistry) SetModifiers(modifier commoncodec.Modifier) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, nbs := range r.namespaceBindings {
		nbs.SetModifiers(modifier)
	}
}

func (r *bindingsRegistry) RegisterAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, nbs := range r.namespaceBindings {
		if err := nbs.RegisterReaders(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (r *bindingsRegistry) UnregisterAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, nbs := range r.namespaceBindings {
		if err := nbs.UnregisterReaders(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (r *bindingsRegistry) AddReader(namespace, genericName string, reader readBinding) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, nbsExists := r.namespaceBindings[namespace]; !nbsExists {
		r.namespaceBindings[namespace] = newNamespaceBinding(namespace)
	}

	r.namespaceBindings[namespace].AddReader(genericName, reader)
}

func (r *bindingsRegistry) GetReader(namespace, genericName string) (readBinding, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	binding, nbsExists := r.namespaceBindings[namespace]
	if !nbsExists {
		return nil, fmt.Errorf("%w: no read binding exists for %s", types.ErrInvalidConfig, namespace)
	}

	return binding.GetReader(genericName)
}

func (r *bindingsRegistry) GetReaders(namespace string) ([]readBinding, error) {
	rBindings, nameSpaceExists := r.namespaceBindings[namespace]
	if !nameSpaceExists {
		return nil, fmt.Errorf("%w: no read binding exists for namespace: %q", types.ErrInvalidConfig, namespace)
	}

	return rBindings.GetReaders()
}

func (r *bindingsRegistry) Bind(ctx context.Context, reg filterRegistrar, binding types.BoundContract) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	namespace, nbsExist := r.namespaceBindings[binding.Name]
	if !nbsExist {
		return fmt.Errorf("%w: no namespace named %s", types.ErrInvalidConfig, binding.Name)
	}

	address, err := solana.PublicKeyFromBase58(binding.Address)
	if err != nil {
		return err
	}

	if err := errors.Join(
		namespace.Bind(ctx, reg, address),
		namespace.BindReaders(ctx, address),
	); err != nil {
		return err
	}

	return nil
}

func (r *bindingsRegistry) Unbind(ctx context.Context, reg filterRegistrar, bindings []types.BoundContract) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, binding := range bindings {
		namespace, nbsExist := r.namespaceBindings[binding.Name]
		if !nbsExist {
			return fmt.Errorf("%w: no namespace named %s", types.ErrInvalidConfig, binding.Name)
		}

		if err := errors.Join(
			namespace.Unbind(ctx, reg),
			namespace.UnbindReaders(ctx),
		); err != nil {
			return err
		}
	}

	return nil
}

func (r *bindingsRegistry) CreateType(namespace, readName string, forEncoding bool) (any, error) {
	rBinding, err := r.GetReader(namespace, readName)
	if err != nil {
		return nil, err
	}

	return rBinding.CreateType(forEncoding)
}

func (r *bindingsRegistry) initAddressSharing(addressShareGroups [][]string) error {
	r.addressShareGroups = make(map[string]*addressShareGroup)

	for _, group := range addressShareGroups {
		shareGroup := &addressShareGroup{
			address: solana.PublicKey{},
			group:   group,
		}

		for _, namespace := range group {
			if _, alreadySharesAddress := r.addressShareGroups[namespace]; alreadySharesAddress {
				return fmt.Errorf("namespace %q can't share address with two different groups", namespace)
			}

			r.addressShareGroups[namespace] = shareGroup
		}
	}

	return nil
}

func (r *bindingsRegistry) getShareGroup(nameSpace string) (*addressShareGroup, bool) {
	shareGroup, sharesAddress := r.addressShareGroups[nameSpace]
	if !sharesAddress {
		return nil, false
	}

	return shareGroup, sharesAddress
}

type namespaceBinding struct {
	// static data
	name string

	// dynamic thread-safe data
	mu      sync.RWMutex
	readers map[string]readBinding
	bound   map[solana.PublicKey]bool
}

func newNamespaceBinding(namespace string) *namespaceBinding {
	return &namespaceBinding{
		name:    namespace,
		readers: make(map[string]readBinding),
	}
}

func (b *namespaceBinding) SetCodecs(codec types.RemoteCodec) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, rb := range b.readers {
		rb.SetCodec(codec)
	}
}

func (b *namespaceBinding) SetModifiers(modifier commoncodec.Modifier) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, rb := range b.readers {
		rb.SetModifier(modifier)
	}
}

func (b *namespaceBinding) Bind(ctx context.Context, reg filterRegistrar, address solana.PublicKey) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.bindingExists(address) {
		return nil
	}

	b.setBinding(address)

	return nil
}

func (b *namespaceBinding) BindReaders(ctx context.Context, address solana.PublicKey) error {
	var err error

	for _, rb := range b.readers {
		err = errors.Join(err, rb.Bind(ctx, address))
	}

	return nil
}

func (b *namespaceBinding) Unbind(ctx context.Context, reg filterRegistrar) error {
	if !b.isBound() {
		return nil
	}

	b.unsetBinding()

	return nil
}

func (b *namespaceBinding) UnbindReaders(ctx context.Context) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var err error

	for _, reader := range b.readers {
		err = errors.Join(reader.Unbind(ctx))
	}

	return err
}

func (b *namespaceBinding) AddReader(genericName string, reader readBinding) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.readers[genericName] = reader
}

func (b *namespaceBinding) GetReader(genericName string) (readBinding, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	rbs, rbsExists := b.readers[genericName]
	if !rbsExists {
		return nil, fmt.Errorf("%w: no read binding exists for %s", types.ErrInvalidConfig, genericName)
	}

	return rbs, nil
}

func (b *namespaceBinding) GetReaders() ([]readBinding, error) {
	allBindings := make([]readBinding, len(b.readers))

	var idx int

	for _, rBinding := range b.readers {
		allBindings[idx] = rBinding
		idx++
	}

	return allBindings, nil
}

func (b *namespaceBinding) RegisterReaders(ctx context.Context) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, reader := range b.readers {
		if err := reader.Register(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (b *namespaceBinding) UnregisterReaders(ctx context.Context) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, reader := range b.readers {
		if err := reader.Unregister(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (b *namespaceBinding) isBound() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.bound) > 0
}

func (b *namespaceBinding) bindingExists(address solana.PublicKey) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	bound, exists := b.bound[address]

	return exists && bound
}

func (b *namespaceBinding) setBinding(address solana.PublicKey) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.bound[address] = true
}

func (b *namespaceBinding) unsetBinding() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.bound = make(map[solana.PublicKey]bool)
}

func (b *bindingsRegistry) handleAddressSharing(boundContract *types.BoundContract) error {
	shareGroup, isInAGroup := b.getShareGroup(boundContract.Name)
	if !isInAGroup {
		return nil
	}

	shareGroup.mux.Lock()
	defer shareGroup.mux.Unlock()

	// set shared address to the binding address
	if shareGroup.address.IsZero() {
		key, err := solana.PublicKeyFromBase58(boundContract.Address)
		if err != nil {
			return err
		}
		b.addressShareGroups[boundContract.Name].address, shareGroup.address = key, key
	} else if boundContract.Address != shareGroup.address.String() && boundContract.Address != "" {
		return fmt.Errorf("namespace: %q shares address: %q with namespaceBindings: %v and cannot be bound with a new address: %s", boundContract.Name, shareGroup.address, shareGroup.group, boundContract.Address)
	}

	boundContract.Address = shareGroup.address.String()
	return nil
}

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

	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

type filterRegistrar interface {
	HasFilter(context.Context, string) bool
	RegisterFilter(context.Context, logpollertypes.Filter) error
	UnregisterFilter(ctx context.Context, name string) error
}

type readBinding interface {
	Bind(context.Context, solana.PublicKey) error
	Unbind(context.Context) error
	GetAddress(context.Context, any) (solana.PublicKey, error)
	GetGenericName() string
	GetReadDefinition() config.ReadDefinition
	GetIDLInfo() (idl codecv1.IDL, inputIDLTypeDef interface{}, outputIDLTypeDef codecv1.IdlTypeDef)
	GetAddressResponseHardCoder() *commoncodec.HardCodeModifierConfig
	SetCodec(types.RemoteCodec)
	SetModifier(commoncodec.Modifier)
	Register(context.Context) error
	Unregister(context.Context) error
	CreateType(bool) (any, error)
	Decode(context.Context, []byte, any) error
}

type eventBinding interface {
	GetLatestValue(_ context.Context, params, returnVal any) error
	QueryKey(context.Context, query.KeyFilter, query.LimitAndSort, any) ([]types.Sequence, error)
}

type addressShareGroup struct {
	address solana.PublicKey
	mux     sync.RWMutex
	group   []string
}

func (g *addressShareGroup) getAddress() solana.PublicKey {
	g.mux.RLock()
	defer g.mux.RUnlock()

	return g.address
}

func (g *addressShareGroup) setAddress(addr solana.PublicKey) {
	g.mux.Lock()
	defer g.mux.Unlock()

	g.address = addr
}

func (g *addressShareGroup) getGroups() []string {
	g.mux.RLock()
	defer g.mux.RUnlock()

	return g.group
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

// Bind has a side-effect of updating the bound address to a group shared address.
//
// DO NOT CHANGE binding from pointer type.
func (r *bindingsRegistry) Bind(ctx context.Context, reg filterRegistrar, binding *types.BoundContract) error {
	if binding == nil {
		return fmt.Errorf("%w: bound contract is nil", types.ErrInvalidType)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.handleAddressSharing(binding); err != nil {
		return err
	}

	namespace, nbsExist := r.namespaceBindings[binding.Name]
	if !nbsExist {
		return fmt.Errorf("%w: no namespace named %s", types.ErrInvalidConfig, binding.Name)
	}

	address, err := solana.PublicKeyFromBase58(binding.Address)
	if err != nil {
		return err
	}

	return errors.Join(
		namespace.Bind(ctx, reg, address),
		namespace.BindReaders(ctx, address),
	)
}

func (r *bindingsRegistry) Unbind(ctx context.Context, reg filterRegistrar, binding types.BoundContract) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	namespace, nbsExist := r.namespaceBindings[binding.Name]
	if !nbsExist {
		return fmt.Errorf("%w: no namespace named %s", types.ErrInvalidConfig, binding.Name)
	}

	return errors.Join(
		namespace.Unbind(ctx, reg),
		namespace.UnbindReaders(ctx),
	)
}

func (r *bindingsRegistry) CreateType(namespace, readName string, forEncoding bool) (any, error) {
	rBinding, err := r.GetReader(namespace, readName)
	if err != nil {
		return nil, err
	}

	return rBinding.CreateType(forEncoding)
}

func (r *bindingsRegistry) initAddressSharing(addressShareGroups [][]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

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

func (r *bindingsRegistry) GetShares(nameSpace string) (*addressShareGroup, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getShareGroup(nameSpace)
}

func (r *bindingsRegistry) getShareGroup(nameSpace string) (*addressShareGroup, bool) {
	shareGroup, sharesAddress := r.addressShareGroups[nameSpace]
	if !sharesAddress {
		return nil, false
	}

	return shareGroup, sharesAddress
}

func (r *bindingsRegistry) handleAddressSharing(boundContract *types.BoundContract) error {
	shareGroup, isInAGroup := r.getShareGroup(boundContract.Name)
	if !isInAGroup {
		return nil
	}

	// set shared address to the binding address
	if shareGroup.getAddress().IsZero() {
		key, err := solana.PublicKeyFromBase58(boundContract.Address)
		if err != nil {
			return err
		}

		shareGroup.setAddress(key)
	} else if boundContract.Address != shareGroup.getAddress().String() && boundContract.Address != "" {
		return fmt.Errorf("namespace: %q shares address: %q with namespaceBindings: %v and cannot be bound with a new address: %s", boundContract.Name, shareGroup.getAddress(), shareGroup.group, boundContract.Address)
	}

	// side-effect of updating the bound contract address to group-shared address
	boundContract.Address = shareGroup.getAddress().String()

	return nil
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
		bound:   make(map[solana.PublicKey]bool),
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
	if b.bindingExists(address) {
		return nil
	}

	b.setBinding(address)

	return nil
}

func (b *namespaceBinding) BindReaders(ctx context.Context, address solana.PublicKey) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var err error

	for _, rb := range b.readers {
		err = errors.Join(err, rb.Bind(ctx, address))
	}

	return err
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
	b.mu.RLock()
	defer b.mu.RUnlock()

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

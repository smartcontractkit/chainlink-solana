package chainreader

import (
	"context"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

type readBinding interface {
	GetAddress(context.Context, any) (solana.PublicKey, error)
	GetGenericName() string
	GetReadDefinition() config.ReadDefinition
	GetIDLInfo() (idl codec.IDL, inputIDLTypeDef interface{}, outputIDLTypeDef codec.IdlTypeDef)
	GetAddressResponseHardCoder() *commoncodec.HardCodeModifierConfig
	SetAddress(solana.PublicKey)
	SetCodec(types.RemoteCodec)
	SetModifier(commoncodec.Modifier)
	CreateType(bool) (any, error)
	Decode(context.Context, []byte, any) error
	QueryKey(context.Context, query.KeyFilter, query.LimitAndSort, any) ([]types.Sequence, error)
}

type bindingsRegistry struct {
	// key is namespace
	namespaceBindings map[string]readNameBindings
	// key is namespace
	addressShareGroups map[string]*addressShareGroup
}

// key is read name
type readNameBindings map[string]readBinding

type addressShareGroup struct {
	address solana.PublicKey
	mux     sync.Mutex
	group   []string
}

func (b *bindingsRegistry) AddReadBinding(namespace, readName string, rBinding readBinding) {
	if _, nbsExists := b.namespaceBindings[namespace]; !nbsExists {
		b.namespaceBindings[namespace] = readNameBindings{}
	}

	b.namespaceBindings[namespace][readName] = rBinding
}

func (b *bindingsRegistry) GetReadBinding(namespace, readName string) (readBinding, error) {
	rBindings, err := b.GetReadBindings(namespace)
	if err != nil {
		return nil, err
	}

	rBinding, rBindingExists := rBindings[readName]
	if !rBindingExists {
		return nil, fmt.Errorf("%w: no read binding exists for namespace: %q read: %q", types.ErrInvalidConfig, namespace, readName)
	}

	return rBinding, nil
}

func (b *bindingsRegistry) GetReadBindings(namespace string) (readNameBindings, error) {
	rBindings, nameSpaceExists := b.namespaceBindings[namespace]
	if !nameSpaceExists {
		return nil, fmt.Errorf("%w: no read binding exists for namespace: %q", types.ErrInvalidConfig, namespace)
	}
	return rBindings, nil
}

func (b *bindingsRegistry) CreateType(namespace, readName string, forEncoding bool) (any, error) {
	rBinding, err := b.GetReadBinding(namespace, readName)
	if err != nil {
		return nil, err
	}

	return rBinding.CreateType(forEncoding)
}

func (b *bindingsRegistry) Bind(boundContract *types.BoundContract) error {
	if boundContract == nil {
		return fmt.Errorf("%w: bound contract is nil", types.ErrInvalidType)
	}

	if err := b.handleAddressSharing(boundContract); err != nil {
		return err
	}

	rBindings, nameSpaceExists := b.namespaceBindings[boundContract.Name]
	if !nameSpaceExists {
		return fmt.Errorf("%w: no namespace named: %q", types.ErrInvalidConfig, boundContract.Name)
	}

	key, err := solana.PublicKeyFromBase58(boundContract.Address)
	if err != nil {
		return fmt.Errorf("%w: failed to parse address: %q for contract %q", types.ErrInvalidConfig, boundContract.Address, boundContract.Name)
	}

	for _, rBinding := range rBindings {
		rBinding.SetAddress(key)
	}

	return nil
}

func (b *bindingsRegistry) SetCodecs(codec types.RemoteCodec) {
	for _, nbs := range b.namespaceBindings {
		for _, rb := range nbs {
			rb.SetCodec(codec)
		}
	}
}

func (b *bindingsRegistry) SetModifiers(modifier commoncodec.Modifier) {
	for _, nbs := range b.namespaceBindings {
		for _, rb := range nbs {
			rb.SetModifier(modifier)
		}
	}
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

func (b *bindingsRegistry) getShareGroup(nameSpace string) (*addressShareGroup, bool) {
	shareGroup, sharesAddress := b.addressShareGroups[nameSpace]
	if !sharesAddress {
		return nil, false
	}

	return shareGroup, sharesAddress
}

func (b *bindingsRegistry) initAddressSharing(addressShareGroups [][]string) error {
	b.addressShareGroups = make(map[string]*addressShareGroup)
	for _, group := range addressShareGroups {
		shareGroup := &addressShareGroup{
			address: solana.PublicKey{},
			group:   group,
		}

		for _, namespace := range group {
			if _, alreadySharesAddress := b.addressShareGroups[namespace]; alreadySharesAddress {
				return fmt.Errorf("namespace %q can't share address with two different groups", namespace)
			}
			b.addressShareGroups[namespace] = shareGroup
		}
	}

	return nil
}

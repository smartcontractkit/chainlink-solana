package chainreader

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
)

type readBinding interface {
	SetAddress(solana.PublicKey)
	GetAddress(context.Context, any) (solana.PublicKey, error)
	SetCodec(types.RemoteCodec)
	SetModifier(commoncodec.Modifier)
	CreateType(bool) (any, error)
	Decode(context.Context, []byte, any) error
	QueryKey(context.Context, query.KeyFilter, query.LimitAndSort, any) ([]types.Sequence, error)
}

// key is namespace
type namespaceBindings map[string]readNameBindings

// key is method name
type readNameBindings map[string]readBinding

func (b namespaceBindings) AddReadBinding(namespace, readName string, reader readBinding) {
	if _, nbsExists := b[namespace]; !nbsExists {
		b[namespace] = readNameBindings{}
	}

	b[namespace][readName] = reader
}

func (b namespaceBindings) GetReadBinding(namespace, readName string) (readBinding, error) {
	nbs, nbsExists := b[namespace]
	if !nbsExists {
		return nil, fmt.Errorf("%w: no read binding exists for %s", types.ErrInvalidConfig, namespace)
	}

	rbs, rbsExists := nbs[readName]
	if !rbsExists {
		return nil, fmt.Errorf("%w: no read binding exists for %s and %s", types.ErrInvalidConfig, namespace, readName)
	}

	return rbs, nil
}

func (b namespaceBindings) CreateType(namespace, readName string, forEncoding bool) (any, error) {
	binding, err := b.GetReadBinding(namespace, readName)
	if err != nil {
		return nil, err
	}

	return binding.CreateType(forEncoding)
}

func (b namespaceBindings) Bind(binding types.BoundContract) error {
	bnd, nbsExist := b[binding.Name]
	if !nbsExist {
		return fmt.Errorf("%w: no namespace named %s", types.ErrInvalidConfig, binding.Name)
	}

	key, err := solana.PublicKeyFromBase58(binding.Address)
	if err != nil {
		return err
	}

	for _, rb := range bnd {
		rb.SetAddress(key)
	}

	return nil
}

func (b namespaceBindings) SetCodecs(codec types.RemoteCodec) {
	for _, nbs := range b {
		for _, rb := range nbs {
			rb.SetCodec(codec)
		}
	}
}

func (b namespaceBindings) SetModifiers(modifier commoncodec.Modifier) {
	for _, nbs := range b {
		for _, rb := range nbs {
			rb.SetModifier(modifier)
		}
	}
}

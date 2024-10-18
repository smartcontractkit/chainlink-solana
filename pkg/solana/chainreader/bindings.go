package chainreader

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

type readBinding interface {
	PreLoad(context.Context, string, *loadedResult)
	GetLatestValue(ctx context.Context, address string, params, returnVal any, preload *loadedResult) error
	CreateType(bool) (any, error)
}

// key is namespace
type namespaceBindings map[string]methodBindings

// key is method name
type methodBindings map[string]readBindings

// read bindings is a list of bindings by index
type readBindings []readBinding

func (b namespaceBindings) AddReadBinding(namespace, methodName string, reader readBinding) {
	nbs, nbsExists := b[namespace]
	if !nbsExists {
		nbs = methodBindings{}
		b[namespace] = nbs
	}

	rbs, rbsExists := nbs[methodName]
	if !rbsExists {
		rbs = []readBinding{}
	}

	b[namespace][methodName] = append(rbs, reader)
}

func (b namespaceBindings) GetReadBindings(namespace, methodName string) ([]readBinding, error) {
	nbs, nbsExists := b[namespace]
	if !nbsExists {
		return nil, fmt.Errorf("%w: no read binding exists for %s", types.ErrInvalidConfig, namespace)
	}

	rbs, rbsExists := nbs[methodName]
	if !rbsExists {
		return nil, fmt.Errorf("%w: no read binding exists for %s and %s", types.ErrInvalidConfig, namespace, methodName)
	}

	return rbs, nil
}

func (b namespaceBindings) CreateType(namespace, methodName string, forEncoding bool) (any, error) {
	bindings, err := b.GetReadBindings(namespace, methodName)
	if err != nil {
		return nil, err
	}

	if len(bindings) == 1 {
		// get the item type from the binding codec
		return bindings[0].CreateType(forEncoding)
	}

	// build a merged struct from all bindings
	fields := make([]reflect.StructField, 0)
	var fieldIdx int
	fieldNames := make(map[string]struct{})

	for _, binding := range bindings {
		bindingType, err := binding.CreateType(forEncoding)
		if err != nil {
			return nil, err
		}

		tBinding := reflect.TypeOf(bindingType)
		if tBinding.Kind() == reflect.Pointer {
			tBinding = tBinding.Elem()
		}

		// all bindings must be structs to allow multiple bindings
		if tBinding.Kind() != reflect.Struct {
			return nil, fmt.Errorf("%w: support for multiple bindings only applies to all bindings having the type struct", types.ErrInvalidType)
		}

		for idx := 0; idx < tBinding.NumField(); idx++ {
			value := tBinding.FieldByIndex([]int{idx})

			_, exists := fieldNames[value.Name]
			if exists {
				return nil, fmt.Errorf("%w: field name overlap on %s", types.ErrInvalidConfig, value.Name)
			}

			field := reflect.StructField{
				Name:  value.Name,
				Type:  value.Type,
				Index: []int{fieldIdx},
			}

			fields = append(fields, field)

			fieldIdx++
			fieldNames[value.Name] = struct{}{}
		}
	}

	return reflect.New(reflect.StructOf(fields)).Interface(), nil
}

func (b namespaceBindings) Bind(binding types.BoundContract) error {
	_, nbsExist := b[binding.Name]
	if !nbsExist {
		return fmt.Errorf("%w: no namespace named %s", types.ErrInvalidConfig, binding.Name)
	}

	readAddresses, err := decodeAddressMappings(binding.Address)
	if err != nil {
		return err
	}

	for readName, addresses := range readAddresses {
		for idx, address := range addresses {
			if _, err := solana.PublicKeyFromBase58(address); err != nil {
				return fmt.Errorf("%w: invalid address binding for %s at index %d: %s", types.ErrInvalidConfig, readName, idx, err.Error())
			}
		}
	}

	return nil
}

type loadedResult struct {
	value chan []byte
	err   chan error
}

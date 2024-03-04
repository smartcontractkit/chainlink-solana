package solana

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type readBinding interface {
	SetCodec(codec commontypes.RemoteCodec)
	GetLatestValue(ctx context.Context, params, returnVal any) error
	Bind(commontypes.BoundContract) error
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
		return nil, fmt.Errorf("%w: no read binding exists for %s", types.ErrInvalidType, namespace)
	}

	rbs, rbsExists := nbs[methodName]
	if !rbsExists {
		return nil, fmt.Errorf("%w: no read binding exists for %s and %s", types.ErrInvalidType, namespace, methodName)
	}

	return rbs, nil
}

func (b namespaceBindings) Bind(boundContracts []commontypes.BoundContract) error {
	for _, bc := range boundContracts {
		parts := strings.Split(bc.Name, ".")
		if len(parts) != 3 {
			return fmt.Errorf("%w: BoundContract.Name must follow pattern of [namespace.method.procedure_idx]", commontypes.ErrInvalidConfig)
		}

		nbs, nbsExist := b[parts[0]]
		if !nbsExist {
			return fmt.Errorf("%w: no namespace named %s for %s", commontypes.ErrInvalidConfig, parts[0], bc.Name)
		}

		mbs, mbsExists := nbs[parts[1]]
		if !mbsExists {
			return fmt.Errorf("%w: no method named %s for %s", commontypes.ErrInvalidConfig, parts[1], bc.Name)
		}

		val, err := strconv.Atoi(parts[2])
		if err != nil {
			return fmt.Errorf("%w: procedure index not parsable for %s", commontypes.ErrInvalidConfig, bc.Name)
		}

		if len(mbs) <= val {
			return fmt.Errorf("%w: no procedure for index %d for %s", commontypes.ErrInvalidConfig, val, bc.Name)
		}

		if err := mbs[val].Bind(bc); err != nil {
			return err
		}
	}

	return nil
}

package chainreader

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/values"
)

type call struct {
	Namespace, ReadName string
	Params, ReturnVal   any
}

type batchResultWithErr struct {
	address             string
	namespace, readName string
	returnVal           any
	err                 error
}

var (
	ErrMissingAccountData = errors.New("account data not found")
)

type MultipleAccountGetter interface {
	GetMultipleAccountData(context.Context, ...solana.PublicKey) ([][]byte, error)
}

func doMethodBatchCall(ctx context.Context, client MultipleAccountGetter, bindingsRegistry bindingsRegistry, batch []call) ([]batchResultWithErr, error) {
	// Create the list of public keys to fetch
	keys := make([]solana.PublicKey, len(batch))
	for idx, call := range batch {
		rBinding, err := bindingsRegistry.GetReadBinding(call.Namespace, call.ReadName)
		if err != nil {
			return nil, err
		}

		key, err := rBinding.GetAddress(ctx, call.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to get address for %s account read: %w", call.ReadName, err)
		}
		keys[idx] = key
	}

	// Fetch the account data
	data, err := client.GetMultipleAccountData(ctx, keys...)
	if err != nil {
		return nil, err
	}

	results := make([]batchResultWithErr, len(batch))

	// decode batch call results
	for idx, call := range batch {
		results[idx] = batchResultWithErr{
			address:   keys[idx].String(),
			namespace: call.Namespace,
			readName:  call.ReadName,
			returnVal: call.ReturnVal,
		}

		if data[idx] == nil || len(data[idx]) == 0 {
			results[idx].err = ErrMissingAccountData

			continue
		}

		rBinding, err := bindingsRegistry.GetReadBinding(results[idx].namespace, results[idx].readName)
		if err != nil {
			results[idx].err = err

			continue
		}

		ptrToValue, isValue := call.ReturnVal.(*values.Value)
		if !isValue {
			results[idx].err = errors.Join(
				results[idx].err,
				rBinding.Decode(ctx, data[idx], results[idx].returnVal),
			)
			continue
		}

		contractType, err := rBinding.CreateType(false)
		if err != nil {
			results[idx].err = err

			continue
		}

		results[idx].err = errors.Join(
			results[idx].err,
			rBinding.Decode(ctx, data[idx], contractType),
		)

		value, err := values.Wrap(contractType)
		if err != nil {
			results[idx].err = errors.Join(results[idx].err, err)

			continue
		}

		*ptrToValue = value
	}

	return results, nil
}

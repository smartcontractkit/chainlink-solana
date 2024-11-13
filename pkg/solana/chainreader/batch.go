package chainreader

import (
	"context"
	"errors"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/values"
)

type call struct {
	ContractName, ReadName string
	Params, ReturnVal      any
}

type batchResultWithErr struct {
	address                string
	contractName, readName string
	returnVal              any
	err                    error
}

var (
	ErrMissingAccountData = errors.New("account data not found")
)

type MultipleAccountGetter interface {
	GetMultipleAccountData(context.Context, ...solana.PublicKey) ([][]byte, error)
}

func doMethodBatchCall(ctx context.Context, client MultipleAccountGetter, bindings namespaceBindings, batch []call) ([]batchResultWithErr, error) {
	// Create the list of public keys to fetch
	keys := make([]solana.PublicKey, len(batch))
	for idx, call := range batch {
		binding, err := bindings.GetReadBinding(call.ContractName, call.ReadName)
		if err != nil {
			return nil, err
		}

		keys[idx] = binding.GetAddress()
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
			address:      keys[idx].String(),
			contractName: call.ContractName,
			readName:     call.ReadName,
			returnVal:    call.ReturnVal,
		}

		if data[idx] == nil || len(data[idx]) == 0 {
			results[idx].err = ErrMissingAccountData

			continue
		}

		binding, err := bindings.GetReadBinding(results[idx].contractName, results[idx].readName)
		if err != nil {
			results[idx].err = err

			continue
		}

		ptrToValue, isValue := call.ReturnVal.(*values.Value)
		if !isValue {
			results[idx].err = errors.Join(
				results[idx].err,
				binding.Decode(ctx, data[idx], results[idx].returnVal),
			)

			continue
		}

		contractType, err := binding.CreateType(false)
		if err != nil {
			results[idx].err = err

			continue
		}

		results[idx].err = errors.Join(
			results[idx].err,
			binding.Decode(ctx, data[idx], contractType),
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

package chainreader

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
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

// doMultiRead aggregate results from multiple PDAs from the same contract into one result.
func doMultiRead(ctx context.Context, client MultipleAccountGetter, bindings namespaceBindings, rv readValues, returnValue any) error {
	batch := make([]call, len(rv.multiRead))
	for idx, readName := range rv.multiRead {
		batch[idx] = call{
			ContractName: rv.contract,
			ReadName:     readName,
			ReturnVal:    returnValue,
		}
	}

	results, err := doMethodBatchCall(ctx, client, bindings, batch)
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("failed to do a multiRead: %q on contract: %q with address: %q with: %d total calls:\n", rv.multiRead[0], rv.contract, rv.address, len(rv.multiRead)))

	var errCount int
	for i, r := range results {
		if r.err != nil {
			errCount++
			sb.WriteString(fmt.Sprintf("- call: #%d with readName: %q and address: %q failed with err: %s\n", i+1, r.readName, r.address, r.err))
		}
	}

	if errCount != 0 {
		return errors.New(sb.String())
	}

	return nil
}

func doMethodBatchCall(ctx context.Context, client MultipleAccountGetter, bindings namespaceBindings, batch []call) ([]batchResultWithErr, error) {
	// Create the list of public keys to fetch
	keys := make([]solana.PublicKey, len(batch))
	for idx, batchCall := range batch {
		binding, err := bindings.GetReadBinding(batchCall.ContractName, batchCall.ReadName)
		if err != nil {
			return nil, fmt.Errorf("%w: read binding not found for contract: %q read: %q: %w", types.ErrInvalidConfig, batchCall.ContractName, batchCall.ReadName, err)
		}

		keys[idx], err = binding.GetAddress(ctx, batchCall.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to get address for contract: %q read: %q: %w", batchCall.ContractName, batchCall.ReadName, err)
		}
	}

	// Fetch the account data
	data, err := client.GetMultipleAccountData(ctx, keys...)
	if err != nil {
		return nil, err
	}

	results := make([]batchResultWithErr, len(batch))

	// decode batch call results
	for idx, batchCall := range batch {
		results[idx] = batchResultWithErr{
			address:      keys[idx].String(),
			contractName: batchCall.ContractName,
			readName:     batchCall.ReadName,
			returnVal:    batchCall.ReturnVal,
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

		results[idx].err = errors.Join(
			decodeReturnVal(ctx, binding, data[idx], results[idx].returnVal),
			results[idx].err)
	}

	return results, nil
}

// decodeReturnVal checks if returnVal is a *values.Value vs. a normal struct pointer, and decodes accordingly.
func decodeReturnVal(ctx context.Context, binding readBinding, raw []byte, returnVal any) error {
	// If we are not dealing with a `*values.Value`, just decode directly.
	ptrToValue, isValue := returnVal.(*values.Value)
	if !isValue {
		return binding.Decode(ctx, raw, returnVal)
	}

	// Otherwise, we need to create an intermediate type, decode into it,
	// wrap it, and set it back into *values.Value
	contractType, err := binding.CreateType(false)
	if err != nil {
		return err
	}

	if err = binding.Decode(ctx, raw, contractType); err != nil {
		return err
	}

	value, err := values.Wrap(contractType)
	if err != nil {
		return err
	}

	*ptrToValue = value

	return nil
}

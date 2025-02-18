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

// doMultiRead aggregate results from multiple PDAs from the same contract into one result.
func doMultiRead(ctx context.Context, client MultipleAccountGetter, bdRegistry *bindingsRegistry, rv readValues, params, returnValue any) error {
	batch := make([]call, len(rv.reads))
	for idx, r := range rv.reads {
		batch[idx] = call{
			Namespace: rv.contract,
			ReadName:  r.readName,
			ReturnVal: returnValue,
		}
		if r.useParams {
			batch[idx].Params = params
		}
	}

	results, err := doMethodBatchCall(ctx, client, bdRegistry, batch)
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("failed to do a multiRead: %q on contract: %q with address: %q with: %d total calls:\n", rv.reads[0].readName, rv.contract, rv.address, len(rv.reads)))

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

func doMethodBatchCall(ctx context.Context, client MultipleAccountGetter, bdRegistry *bindingsRegistry, batch []call) ([]batchResultWithErr, error) {
	results := make([]batchResultWithErr, len(batch))

	// create the list of public keys to fetch
	keys := []solana.PublicKey{}

	// map batch call index to key index (some calls are event reads and will be handled by a different binding)
	dataMap := make(map[int]int)

	for idx, batchCall := range batch {
		rBinding, err := bdRegistry.GetReader(batchCall.Namespace, batchCall.ReadName)
		if err != nil {
			return nil, fmt.Errorf("%w: read binding not found for contract: %q read: %q: %w", types.ErrInvalidConfig, batchCall.Namespace, batchCall.ReadName, err)
		}

		key, err := rBinding.GetAddress(ctx, batchCall.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to get address for contract: %q read: %q: %w", batchCall.Namespace, batchCall.ReadName, err)
		}

		eBinding, ok := rBinding.(eventBinding)
		if ok {
			results[idx] = batchResultWithErr{
				address:   key.String(),
				namespace: batchCall.Namespace,
				readName:  batchCall.ReadName,
				returnVal: batchCall.ReturnVal,
			}

			results[idx].err = eBinding.GetLatestValue(ctx, batchCall.Params, results[idx].returnVal)

			continue
		}

		// map the idx to the key idx
		dataMap[idx] = len(keys)

		keys = append(keys, key)
	}

	// Fetch the account data
	data, err := client.GetMultipleAccountData(ctx, keys...)
	if err != nil {
		return nil, err
	}

	// decode batch call results
	for idx, batchCall := range batch {
		dataIdx, ok := dataMap[idx]
		if !ok {
			return nil, fmt.Errorf("%w: unexpected data index state", types.ErrInternal)
		}

		results[idx] = batchResultWithErr{
			address:   keys[dataIdx].String(),
			namespace: batchCall.Namespace,
			readName:  batchCall.ReadName,
			returnVal: batchCall.ReturnVal,
		}

		if data[dataIdx] == nil || len(data[dataIdx]) == 0 {
			results[idx].err = ErrMissingAccountData

			continue
		}

		rBinding, err := bdRegistry.GetReader(results[idx].namespace, results[idx].readName)
		if err != nil {
			results[idx].err = err

			continue
		}

		results[idx].err = asValueDotValue(
			ctx,
			rBinding,
			results[dataIdx].returnVal,
			wrapDecodeValuer(rBinding, data[dataIdx]),
		)
	}

	return results, nil
}

// asValueDotValue checks if returnVal is a *values.Value vs. a normal struct pointer, and decodes accordingly.
func asValueDotValue(
	ctx context.Context,
	binding readBinding,
	returnVal any,
	op func(context.Context, any) error,
) error {
	ptrToValue, isValue := returnVal.(*values.Value)
	if !isValue {
		return op(ctx, returnVal)
	}

	contractType, err := binding.CreateType(false)
	if err != nil {
		return err
	}

	if err = op(ctx, contractType); err != nil {
		return err
	}

	value, err := values.Wrap(contractType)
	if err != nil {
		return err
	}

	*ptrToValue = value

	return nil
}

func wrapDecodeValuer(binding readBinding, data []byte) func(context.Context, any) error {
	return func(ctx context.Context, returnVal any) error {
		return binding.Decode(ctx, data, returnVal)
	}
}

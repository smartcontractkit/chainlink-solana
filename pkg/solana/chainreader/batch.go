package chainreader

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
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
func doMultiRead(ctx context.Context, lggr logger.Logger, client MultipleAccountGetter, bdRegistry *bindingsRegistry, rv readValues, params, returnValue any) error {
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

	results, err := doMethodBatchCall(ctx, lggr, client, bdRegistry, batch)
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

func doMethodBatchCall(ctx context.Context, lggr logger.Logger, client MultipleAccountGetter, bdRegistry *bindingsRegistry, batch []call) ([]batchResultWithErr, error) {
	// Create the list of public keys to fetch
	keys := make([]solana.PublicKey, len(batch))
	for idx, batchCall := range batch {
		rBinding, err := bdRegistry.GetReader(batchCall.Namespace, batchCall.ReadName)
		if err != nil {
			return nil, fmt.Errorf("%w: read binding not found for contract: %q read: %q: %w", types.ErrInvalidConfig, batchCall.Namespace, batchCall.ReadName, err)
		}

		keys[idx], err = rBinding.GetAddress(ctx, batchCall.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to get address for contract: %q read: %q: %w", batchCall.Namespace, batchCall.ReadName, err)
		}
	}

	// Fetch the account data
	results := make([]batchResultWithErr, len(batch))
	data, err := client.GetMultipleAccountData(ctx, keys...)
	if err != nil {
		if errors.Is(err, rpc.ErrNotFound) {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("failed to get multiple account data with err: %s, now returning empty responses for: \n", err))
			for i, c := range batch {
				sb.WriteString(fmt.Sprintf("- call: #%d under namespace %s, with readName: %q and address: %q\n", i+1, c.Namespace, c.ReadName, keys[i]))
				results[i] = batchResultWithErr{
					address:   keys[i].String(),
					namespace: c.Namespace,
					readName:  c.ReadName,
					returnVal: c.ReturnVal,
				}
			}
			lggr.Info(sb.String())
			return results, nil
		}
		return nil, err
	}

	// decode batch call results
	for idx, batchCall := range batch {
		results[idx] = batchResultWithErr{
			address:   keys[idx].String(),
			namespace: batchCall.Namespace,
			readName:  batchCall.ReadName,
			returnVal: batchCall.ReturnVal,
		}

		if data[idx] == nil || len(data[idx]) == 0 {
			results[idx].err = ErrMissingAccountData

			continue
		}

		rBinding, err := bdRegistry.GetReader(results[idx].namespace, results[idx].readName)
		if err != nil {
			results[idx].err = err

			continue
		}

		results[idx].err = errors.Join(
			decodeReturnVal(ctx, rBinding, data[idx], results[idx].returnVal),
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

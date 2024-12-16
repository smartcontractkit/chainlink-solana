package chainreader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	codeccommon "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

const ServiceName = "SolanaChainReader"

type SolanaChainReaderService struct {
	types.UnimplementedContractReader

	// provided values
	lggr   logger.Logger
	client MultipleAccountGetter

	// internal values
	bindings namespaceBindings
	lookup   *lookup

	// service state management
	wg sync.WaitGroup
	services.StateMachine
}

var (
	_ services.Service     = &SolanaChainReaderService{}
	_ types.ContractReader = &SolanaChainReaderService{}
)

// NewChainReaderService is a constructor for a new ChainReaderService for Solana. Returns a nil service on error.
func NewChainReaderService(lggr logger.Logger, dataReader MultipleAccountGetter, cfg config.ChainReader) (*SolanaChainReaderService, error) {
	svc := &SolanaChainReaderService{
		lggr:     logger.Named(lggr, ServiceName),
		client:   dataReader,
		bindings: namespaceBindings{},
		lookup:   newLookup(),
	}

	if err := svc.init(cfg.Namespaces); err != nil {
		return nil, err
	}

	return svc, nil
}

// Name implements the services.ServiceCtx interface and returns the logger service name.
func (s *SolanaChainReaderService) Name() string {
	return s.lggr.Name()
}

// Start implements the services.ServiceCtx interface and starts necessary background services.
// An error is returned if starting any internal services fails. Subsequent calls to Start return
// and error.
func (s *SolanaChainReaderService) Start(_ context.Context) error {
	return s.StartOnce(ServiceName, func() error {
		return nil
	})
}

// Close implements the services.ServiceCtx interface and stops all background services and cleans
// up used resources. Subsequent calls to Close return an error.
func (s *SolanaChainReaderService) Close() error {
	return s.StopOnce(ServiceName, func() error {
		s.wg.Wait()

		return nil
	})
}

// Ready implements the services.ServiceCtx interface and returns an error if starting the service
// encountered any errors or if the service is not ready to serve requests.
func (s *SolanaChainReaderService) Ready() error {
	return s.StateMachine.Ready()
}

// HealthReport implements the services.ServiceCtx interface and returns errors for any internal
// function or service that may have failed.
func (s *SolanaChainReaderService) HealthReport() map[string]error {
	return map[string]error{s.Name(): s.Healthy()}
}

// GetLatestValue implements the types.ContractReader interface and requests and parses on-chain
// data named by the provided contract, method, and params.
func (s *SolanaChainReaderService) GetLatestValue(ctx context.Context, readIdentifier string, _ primitives.ConfidenceLevel, params any, returnVal any) error {
	if err := s.Ready(); err != nil {
		return err
	}

	s.wg.Add(1)
	defer s.wg.Done()

	vals, ok := s.lookup.getContractForReadIdentifiers(readIdentifier)
	if !ok {
		return fmt.Errorf("%w: no contract for read identifier %s", types.ErrInvalidType, readIdentifier)
	}

	batch := []call{
		{
			ContractName: vals.contract,
			ReadName:     vals.readName,
			Params:       params,
			ReturnVal:    returnVal,
		},
	}

	results, err := doMethodBatchCall(ctx, s.client, s.bindings, batch)
	if err != nil {
		return err
	}

	if len(results) != len(batch) {
		return fmt.Errorf("%w: unexpected number of results", types.ErrInternal)
	}

	if results[0].err != nil {
		return fmt.Errorf("%w: %s", types.ErrInternal, results[0].err)
	}

	return nil
}

// BatchGetLatestValues implements the types.ContractReader interface.
func (s *SolanaChainReaderService) BatchGetLatestValues(ctx context.Context, request types.BatchGetLatestValuesRequest) (types.BatchGetLatestValuesResult, error) {
	idxLookup := make(map[types.BoundContract][]int)
	batch := []call{}

	for bound, req := range request {
		idxLookup[bound] = make([]int, len(req))

		for idx, readReq := range req {
			idxLookup[bound][idx] = len(batch)
			batch = append(batch, call{
				ContractName: bound.Name,
				ReadName:     readReq.ReadName,
				Params:       readReq.Params,
				ReturnVal:    readReq.ReturnVal,
			})
		}
	}

	results, err := doMethodBatchCall(ctx, s.client, s.bindings, batch)
	if err != nil {
		return nil, err
	}

	if len(results) != len(batch) {
		return nil, errors.New("unexpected number of results")
	}

	result := make(types.BatchGetLatestValuesResult)

	for bound, idxs := range idxLookup {
		result[bound] = make(types.ContractBatchResults, len(idxs))

		for idx, callIdx := range idxs {
			res := types.BatchReadResult{ReadName: results[callIdx].readName}
			res.SetResult(results[callIdx].returnVal, results[callIdx].err)

			result[bound][idx] = res
		}
	}

	return result, nil
}

// QueryKey implements the types.ContractReader interface.
func (s *SolanaChainReaderService) QueryKey(_ context.Context, _ types.BoundContract, _ query.KeyFilter, _ query.LimitAndSort, _ any) ([]types.Sequence, error) {
	return nil, errors.New("unimplemented")
}

// Bind implements the types.ContractReader interface and allows new contract bindings to be added
// to the service.
func (s *SolanaChainReaderService) Bind(_ context.Context, bindings []types.BoundContract) error {
	for _, binding := range bindings {
		if err := s.bindings.Bind(binding); err != nil {
			return err
		}

		s.lookup.bindAddressForContract(binding.Name, binding.Address)
	}

	return nil
}

// Unbind implements the types.ContractReader interface and allows existing contract bindings to be removed
// from the service.
func (s *SolanaChainReaderService) Unbind(_ context.Context, bindings []types.BoundContract) error {
	for _, binding := range bindings {
		s.lookup.unbindAddressForContract(binding.Name, binding.Address)
	}

	return nil
}

// CreateContractType implements the ContractTypeProvider interface and allows the chain reader
// service to explicitly define the expected type for a grpc server to provide.
func (s *SolanaChainReaderService) CreateContractType(readIdentifier string, forEncoding bool) (any, error) {
	values, ok := s.lookup.getContractForReadIdentifiers(readIdentifier)
	if !ok {
		return nil, fmt.Errorf("%w: no contract for read identifier", types.ErrInvalidConfig)
	}

	return s.bindings.CreateType(values.contract, values.readName, forEncoding)
}

func (s *SolanaChainReaderService) init(namespaces map[string]config.ChainReaderMethods) error {
	for namespace, methods := range namespaces {
		for methodName, method := range methods.Methods {
			var idl codec.IDL
			if err := json.Unmarshal([]byte(method.AnchorIDL), &idl); err != nil {
				return err
			}

			idlCodec, err := codec.NewIDLAccountCodec(idl, config.BuilderForEncoding(method.Encoding))
			if err != nil {
				return err
			}

			s.lookup.addReadNameForContract(namespace, methodName)

			procedure := method.Procedure

			injectAddressModifier(procedure.OutputModifications)

			mod, err := procedure.OutputModifications.ToModifier(codec.DecoderHooks...)
			if err != nil {
				return err
			}

			codecWithModifiers, err := codec.NewNamedModifierCodec(idlCodec, procedure.IDLAccount, mod)
			if err != nil {
				return err
			}

			s.bindings.AddReadBinding(namespace, methodName, newAccountReadBinding(
				procedure.IDLAccount,
				codecWithModifiers,
				createRPCOpts(procedure.RPCOpts),
			))
		}
	}

	return nil
}

// injectAddressModifier injects AddressModifier into OutputModifications.
// This is necessary because AddressModifier cannot be serialized and must be applied at runtime.
func injectAddressModifier(outputModifications codeccommon.ModifiersConfig) {
	for i, modConfig := range outputModifications {
		if addrModifierConfig, ok := modConfig.(*codeccommon.AddressBytesToStringModifierConfig); ok {
			addrModifierConfig.Modifier = codec.SolanaAddressModifier{}
			outputModifications[i] = addrModifierConfig
		}
	}
}

func createRPCOpts(opts *config.RPCOpts) *rpc.GetAccountInfoOpts {
	if opts == nil {
		return nil
	}

	result := &rpc.GetAccountInfoOpts{
		DataSlice: opts.DataSlice,
	}

	if opts.Encoding != nil {
		result.Encoding = *opts.Encoding
	}

	if opts.Commitment != nil {
		result.Commitment = *opts.Commitment
	}

	return result
}

type accountDataReader struct {
	client *rpc.Client
}

func NewAccountDataReader(client *rpc.Client) *accountDataReader {
	return &accountDataReader{client: client}
}

func (r *accountDataReader) ReadAll(ctx context.Context, pk solana.PublicKey, opts *rpc.GetAccountInfoOpts) ([]byte, error) {
	result, err := r.client.GetAccountInfoWithOpts(ctx, pk, opts)
	if err != nil {
		return nil, err
	}

	bts := result.Value.Data.GetBinary()

	return bts, nil
}

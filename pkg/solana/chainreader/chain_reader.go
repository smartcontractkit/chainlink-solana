package chainreader

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
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
	parsed   *codec.ParsedTypes
	codec    types.RemoteCodec

	// service state management
	wg sync.WaitGroup
	services.StateMachine
}

var (
	_ services.Service     = &SolanaChainReaderService{}
	_ types.ContractReader = &SolanaChainReaderService{}
)

// NewChainReaderService is a constructor for a new ChainReaderService for Solana. Returns a nil service on error.
func NewChainReaderService(lggr logger.Logger, dataReader MultipleAccountGetter, cfg config.ContractReader) (*SolanaChainReaderService, error) {
	svc := &SolanaChainReaderService{
		lggr:     logger.Named(lggr, ServiceName),
		client:   dataReader,
		bindings: namespaceBindings{},
		lookup:   newLookup(),
		parsed:   &codec.ParsedTypes{EncoderDefs: map[string]codec.Entry{}, DecoderDefs: map[string]codec.Entry{}},
	}

	if err := svc.init(cfg.Namespaces); err != nil {
		return nil, err
	}

	svcCodec, err := svc.parsed.ToCodec()
	if err != nil {
		return nil, err
	}

	svc.codec = svcCodec

	svc.bindings.SetCodec(svcCodec)
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

	values, ok := s.lookup.getContractForReadIdentifiers(readIdentifier)
	if !ok {
		return fmt.Errorf("%w: no contract for read identifier %s", types.ErrInvalidType, readIdentifier)
	}

	batch := []call{
		{
			ContractName: values.contract,
			ReadName:     values.genericName,
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

	return s.bindings.CreateType(values.contract, values.genericName, forEncoding)
}

func (s *SolanaChainReaderService) addCodecDef(forEncoding bool, namespace, genericName string, readType codec.ChainConfigType, idl codec.IDL, idlDefinition interface{}, modCfg commoncodec.ModifiersConfig) error {
	mod, err := modCfg.ToModifier(codec.DecoderHooks...)
	if err != nil {
		return err
	}

	cEntry, err := codec.CreateCodecEntry(idlDefinition, genericName, idl, mod)
	if err != nil {
		return err
	}

	if forEncoding {
		s.parsed.EncoderDefs[codec.WrapItemType(forEncoding, namespace, genericName, readType)] = cEntry
	} else {
		s.parsed.DecoderDefs[codec.WrapItemType(forEncoding, namespace, genericName, readType)] = cEntry
	}
	return nil
}

func (s *SolanaChainReaderService) init(namespaces map[string]config.ChainContractReader) error {
	for namespace, nameSpaceDef := range namespaces {
		for genericName, read := range nameSpaceDef.Reads {
			injectAddressModifier(read.InputModifications, read.OutputModifications)
			idlDef, err := codec.FindDefinitionFromIDL(codec.ChainConfigTypeAccountDef, read.ChainSpecificName, nameSpaceDef.IDL)
			if err != nil {
				return err
			}

			switch read.ReadType {
			case config.Account:
				accountIDLDef, isOk := idlDef.(codec.IdlTypeDef)
				if !isOk {
					return fmt.Errorf("unexpected type %T from IDL definition for account read: %q, with chainSpecificName: %q, of type: %q", accountIDLDef, genericName, read.ChainSpecificName, read.ReadType)
				}
				if err = s.addAccountRead(namespace, genericName, nameSpaceDef.IDL, accountIDLDef, read); err != nil {
					return err
				}
			case config.Event:
				eventIDlDef, isOk := idlDef.(codec.IdlEvent)
				if !isOk {
					return fmt.Errorf("unexpected type %T from IDL definition for log read: %q, with chainSpecificName: %q, of type: %q", eventIDlDef, genericName, read.ChainSpecificName, read.ReadType)
				}
				// TODO s.addLogRead()
				return fmt.Errorf("implement me")
			default:
				return fmt.Errorf("unexpected read type %q for: %q in namespace: %q", read.ReadType, genericName, namespace)
			}
		}
	}

	return nil
}

func (s *SolanaChainReaderService) addAccountRead(namespace string, genericName string, idl codec.IDL, idlType codec.IdlTypeDef, readDefinition config.ReadDefinition) error {
	if err := s.addCodecDef(false, namespace, genericName, codec.ChainConfigTypeAccountDef, idl, idlType, readDefinition.OutputModifications); err != nil {
		return err
	}

	s.lookup.addReadNameForContract(namespace, genericName)

	var reader readBinding
	var inputAccountIDLDef interface{}
	// Create PDA read binding if PDA prefix or seeds configs are populated
	if len(readDefinition.PDADefiniton.Prefix) > 0 || len(readDefinition.PDADefiniton.Seeds) > 0 {
		inputAccountIDLDef = readDefinition.PDADefiniton
		reader = newAccountReadBinding(namespace, genericName, readDefinition.PDADefiniton.Prefix, true)
	} else {
		inputAccountIDLDef = codec.NilIdlTypeDefTy
		reader = newAccountReadBinding(namespace, genericName, "", false)
	}
	if err := s.addCodecDef(true, namespace, genericName, codec.ChainConfigTypeAccountDef, idl, inputAccountIDLDef, readDefinition.InputModifications); err != nil {
		return err
	}
	s.bindings.AddReadBinding(namespace, genericName, reader)

	return nil
}

// injectAddressModifier injects AddressModifier into OutputModifications.
// This is necessary because AddressModifier cannot be serialized and must be applied at runtime.
func injectAddressModifier(inputModifications, outputModifications commoncodec.ModifiersConfig) {
	for i, modConfig := range inputModifications {
		if addrModifierConfig, ok := modConfig.(*commoncodec.AddressBytesToStringModifierConfig); ok {
			addrModifierConfig.Modifier = codec.SolanaAddressModifier{}
			outputModifications[i] = addrModifierConfig
		}
	}

	for i, modConfig := range outputModifications {
		if addrModifierConfig, ok := modConfig.(*commoncodec.AddressBytesToStringModifierConfig); ok {
			addrModifierConfig.Modifier = codec.SolanaAddressModifier{}
			outputModifications[i] = addrModifierConfig
		}
	}
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

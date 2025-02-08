package chainreader

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/gagliardetto/solana-go"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"
	"github.com/smartcontractkit/chainlink-common/pkg/values"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

type EventsReader interface {
	RegisterFilter(context.Context, logpoller.Filter) error
	FilteredLogs(context.Context, []query.Expression, query.LimitAndSort, string) ([]logpoller.Log, error)
}

const ServiceName = "SolanaContractReader"

type ContractReaderService struct {
	types.UnimplementedContractReader

	// provided dependencies
	lggr   logger.Logger
	client MultipleAccountGetter
	reader EventsReader

	// internal values
	bdRegistry bindingsRegistry
	lookup     *lookup
	parsed     *codec.ParsedTypes
	codec      types.RemoteCodec
	filters    []logpoller.Filter

	// service state management
	wg sync.WaitGroup
	services.StateMachine
}

var (
	_ services.Service     = &ContractReaderService{}
	_ types.ContractReader = &ContractReaderService{}
)

// NewContractReaderService is a constructor for a new ContractReaderService for Solana. Returns a nil service on error.
func NewContractReaderService(
	lggr logger.Logger,
	dataReader MultipleAccountGetter,
	cfg config.ContractReader,
	reader EventsReader,
) (*ContractReaderService, error) {
	svc := &ContractReaderService{
		lggr:   logger.Named(lggr, ServiceName),
		client: dataReader,
		bdRegistry: bindingsRegistry{
			namespaceBindings:  make(map[string]readNameBindings),
			addressShareGroups: make(map[string]*addressShareGroup),
		},
		lookup: newLookup(),
		parsed: &codec.ParsedTypes{
			EncoderDefs: map[string]codec.Entry{},
			DecoderDefs: map[string]codec.Entry{},
		},
		filters: []logpoller.Filter{},
		reader:  reader,
	}

	if err := svc.bdRegistry.initAddressSharing(cfg.AddressShareGroups); err != nil {
		return nil, err
	}

	if err := svc.initNamespace(cfg.Namespaces); err != nil {
		return nil, err
	}

	svcCodec, err := svc.parsed.ToCodec()
	if err != nil {
		return nil, err
	}

	svc.codec = svcCodec

	svc.bdRegistry.SetCodecs(svcCodec)
	svc.bdRegistry.SetModifiers(svc.parsed.Modifiers)
	return svc, nil
}

// Name implements the services.ServiceCtx interface and returns the logger service name.
func (s *ContractReaderService) Name() string {
	return s.lggr.Name()
}

// Start implements the services.ServiceCtx interface and starts necessary background services.
// An error is returned if starting any internal services fails. Subsequent calls to Start return
// and error.
func (s *ContractReaderService) Start(ctx context.Context) error {
	return s.StartOnce(ServiceName, func() error {
		// registering filters needs a context so we should be able to use the start function context.
		for _, filter := range s.filters {
			if err := s.reader.RegisterFilter(ctx, filter); err != nil {
				return err
			}
		}

		return nil
	})
}

// Close implements the services.ServiceCtx interface and stops all background services and cleans
// up used resources. Subsequent calls to Close return an error.
func (s *ContractReaderService) Close() error {
	return s.StopOnce(ServiceName, func() error {
		s.wg.Wait()

		return nil
	})
}

// Ready implements the services.ServiceCtx interface and returns an error if starting the service
// encountered any errors or if the service is not ready to serve requests.
func (s *ContractReaderService) Ready() error {
	return s.StateMachine.Ready()
}

// HealthReport implements the services.ServiceCtx interface and returns errors for any internal
// function or service that may have failed.
func (s *ContractReaderService) HealthReport() map[string]error {
	return map[string]error{s.Name(): s.Healthy()}
}

// GetLatestValue implements the types.ContractReader interface and requests and parses on-chain
// data named by the provided contract, method, and params.
func (s *ContractReaderService) GetLatestValue(ctx context.Context, readIdentifier string, _ primitives.ConfidenceLevel, params any, returnVal any) error {
	if err := s.Ready(); err != nil {
		return err
	}

	s.wg.Add(1)
	defer s.wg.Done()

	values, ok := s.lookup.getContractForReadIdentifiers(readIdentifier)
	if !ok {
		return fmt.Errorf("%w: no contract for read identifier: %q", types.ErrInvalidType, readIdentifier)
	}

	if len(values.multiRead) == 0 {
		return fmt.Errorf("%w: no reads defined for readIdentifier: %q", types.ErrInvalidConfig, readIdentifier)
	}

	if len(values.multiRead) > 1 {
		return doMultiRead(ctx, s.client, s.bdRegistry, values, returnVal)
	}

	batch := []call{
		{
			Namespace: values.contract,
			ReadName:  values.multiRead[0],
			Params:    params,
			ReturnVal: returnVal,
		},
	}

	results, err := doMethodBatchCall(ctx, s.client, s.bdRegistry, batch)
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
func (s *ContractReaderService) BatchGetLatestValues(ctx context.Context, request types.BatchGetLatestValuesRequest) (types.BatchGetLatestValuesResult, error) {
	idxLookup := make(map[types.BoundContract][]int)
	var batch []call

	for bound, req := range request {
		idxLookup[bound] = make([]int, len(req))

		for idx, readReq := range req {
			idxLookup[bound][idx] = len(batch)
			batch = append(batch, call{
				Namespace: bound.Name,
				ReadName:  readReq.ReadName,
				Params:    readReq.Params,
				ReturnVal: readReq.ReturnVal,
			})
		}
	}

	results, err := doMethodBatchCall(ctx, s.client, s.bdRegistry, batch)
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
func (s *ContractReaderService) QueryKey(ctx context.Context, contract types.BoundContract, filter query.KeyFilter, limitAndSort query.LimitAndSort, sequenceDataType any) ([]types.Sequence, error) {
	binding, err := s.bdRegistry.GetReadBinding(contract.Name, filter.Key)
	if err != nil {
		return nil, err
	}

	_, isValuePtr := sequenceDataType.(*values.Value)
	if !isValuePtr {
		return binding.QueryKey(ctx, filter, limitAndSort, sequenceDataType)
	}

	dataTypeFromReadIdentifier, err := s.CreateContractType(contract.ReadIdentifier(filter.Key), false)
	if err != nil {
		return nil, err
	}

	sequence, err := binding.QueryKey(ctx, filter, limitAndSort, dataTypeFromReadIdentifier)
	if err != nil {
		return nil, err
	}

	sequenceOfValues := make([]types.Sequence, len(sequence))
	for idx, entry := range sequence {
		value, err := values.Wrap(entry.Data)
		if err != nil {
			return nil, err
		}
		sequenceOfValues[idx] = types.Sequence{
			Cursor: entry.Cursor,
			Head:   entry.Head,
			Data:   &value,
		}
	}

	return sequenceOfValues, nil
}

// Bind implements the types.ContractReader interface and allows new contract namespaceBindings to be added
// to the service.
func (s *ContractReaderService) Bind(_ context.Context, bindings []types.BoundContract) error {
	for i := range bindings {
		if err := s.bdRegistry.Bind(&bindings[i]); err != nil {
			return err
		}

		s.lookup.bindAddressForContract(bindings[i].Name, bindings[i].Address)
		// also bind with an empty address so that we can look up the contract without providing address when calling CR methods
		if _, isInAShareGroup := s.bdRegistry.getShareGroup(bindings[i]); isInAShareGroup {
			s.lookup.bindAddressForContract(bindings[i].Name, "")
		}
	}

	return nil
}

// Unbind implements the types.ContractReader interface and allows existing contract namespaceBindings to be removed
// from the service.
func (s *ContractReaderService) Unbind(_ context.Context, bindings []types.BoundContract) error {
	for _, binding := range bindings {
		s.lookup.unbindAddressForContract(binding.Name, binding.Address)
	}

	return nil
}

// CreateContractType implements the ContractTypeProvider interface and allows the chain reader
// service to explicitly define the expected type for a grpc server to provide.
func (s *ContractReaderService) CreateContractType(readIdentifier string, forEncoding bool) (any, error) {
	values, ok := s.lookup.getContractForReadIdentifiers(readIdentifier)
	if !ok {
		return nil, fmt.Errorf("%w: no contract for read identifier", types.ErrInvalidConfig)
	}

	if len(values.multiRead) == 0 {
		return nil, fmt.Errorf("%w: no reads defined for read identifier", types.ErrInvalidConfig)
	}

	return s.bdRegistry.CreateType(values.contract, values.multiRead[0], forEncoding)
}

func (s *ContractReaderService) addCodecDef(forEncoding bool, namespace, genericName string, idl codec.IDL, idlDefinition interface{}, modCfg commoncodec.ModifiersConfig) error {
	mod, err := modCfg.ToModifier(codec.DecoderHooks...)
	if err != nil {
		return err
	}

	cEntry, err := codec.CreateCodecEntry(idlDefinition, genericName, idl, mod)
	if err != nil {
		return err
	}

	if forEncoding {
		s.parsed.EncoderDefs[codec.WrapItemType(true, namespace, genericName)] = cEntry
	} else {
		s.parsed.DecoderDefs[codec.WrapItemType(false, namespace, genericName)] = cEntry
	}
	return nil
}

func (s *ContractReaderService) initNamespace(namespaces map[string]config.ChainContractReader) error {
	for namespace, nameSpaceDef := range namespaces {
		for genericName, read := range nameSpaceDef.Reads {
			utils.InjectAddressModifier(read.InputModifications, read.OutputModifications)

			switch read.ReadType {
			case config.Account:
				idlDef, err := codec.FindDefinitionFromIDL(codec.ChainConfigTypeAccountDef, read.ChainSpecificName, nameSpaceDef.IDL)
				if err != nil {
					return err
				}

				accountIDLDef, isOk := idlDef.(codec.IdlTypeDef)
				if !isOk {
					return fmt.Errorf("unexpected type %T from IDL definition for account read: %q, with chainSpecificName: %q, of type: %q", accountIDLDef, genericName, read.ChainSpecificName, read.ReadType)
				}
				if err = s.addAccountRead(namespace, genericName, nameSpaceDef.IDL, accountIDLDef, read); err != nil {
					return err
				}
			case config.Event:
				idlDef, err := codec.FindDefinitionFromIDL(codec.ChainConfigTypeEventDef, read.ChainSpecificName, nameSpaceDef.IDL)
				if err != nil {
					return err
				}

				eventIDlDef, isOk := idlDef.(codec.IdlEvent)
				if !isOk {
					return fmt.Errorf("unexpected type %T from IDL definition for event read: %q, with chainSpecificName: %q, of type: %q", eventIDlDef, genericName, read.ChainSpecificName, read.ReadType)
				}

				if err = s.addEventRead(
					namespace, genericName,
					nameSpaceDef.ContractAddress,
					nameSpaceDef.IDL, eventIDlDef,
					read,
					s.reader,
				); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unexpected read type %q for: %q in namespace: %q", read.ReadType, genericName, namespace)
			}
		}
	}

	return nil
}

func (s *ContractReaderService) addAccountRead(namespace string, genericName string, idl codec.IDL, idlType codec.IdlTypeDef, readDefinition config.ReadDefinition) error {
	if err := s.addCodecDef(false, namespace, genericName, idl, idlType, readDefinition.OutputModifications); err != nil {
		return err
	}

	multiRead := []string{genericName}
	if readDefinition.MultiReader != nil {
		reads, err := s.addMultiAccountRead(namespace, readDefinition, idl)
		if err != nil {
			return err
		}
		multiRead = append(multiRead, reads...)
	}

	s.lookup.addReadNameForContract(namespace, genericName, multiRead)

	var (
		reader             readBinding
		inputAccountIDLDef interface{}
	)

	// Create PDA read binding if PDA prefix or seeds configs are populated
	if readDefinition.PDADefinition.Prefix != nil || len(readDefinition.PDADefinition.Seeds) > 0 {
		inputAccountIDLDef = readDefinition.PDADefinition
		reader = newAccountReadBinding(namespace, genericName, readDefinition.PDADefinition.Prefix, true)
	} else {
		inputAccountIDLDef = codec.NilIdlTypeDefTy
		reader = newAccountReadBinding(namespace, genericName, nil, false)
	}
	if err := s.addCodecDef(true, namespace, genericName, idl, inputAccountIDLDef, readDefinition.InputModifications); err != nil {
		return err
	}

	s.bdRegistry.AddReadBinding(namespace, genericName, reader)
	return nil
}

func (s *ContractReaderService) addMultiAccountRead(namespace string, readDefinition config.ReadDefinition, idl codec.IDL) ([]string, error) {
	var reads []string
	for _, mr := range readDefinition.MultiReader.Reads {
		idlDef, err := codec.FindDefinitionFromIDL(codec.ChainConfigTypeAccountDef, mr.ChainSpecificName, idl)
		if err != nil {
			return nil, err
		}

		if mr.ReadType != config.Account {
			return nil, fmt.Errorf("unexpected read type %q for dynamic hard coder read: %q in namespace: %q", mr.ReadType, mr.ChainSpecificName, namespace)
		}

		accountIDLDef, isOk := idlDef.(codec.IdlTypeDef)
		if !isOk {
			return nil, fmt.Errorf("unexpected type %T from IDL definition for account read with chainSpecificName: %q, of type: %q", accountIDLDef, mr.ChainSpecificName, mr.ReadType)
		}

		if err = s.addAccountRead(namespace, mr.ChainSpecificName, idl, accountIDLDef, mr); err != nil {
			return nil, fmt.Errorf("failed to add multi-read %q: %w", mr.ChainSpecificName, err)
		}

		reads = append(reads, mr.ChainSpecificName)
	}
	return reads, nil
}

func (s *ContractReaderService) addEventRead(
	namespace, genericName string,
	contractAddress solana.PublicKey,
	_ codec.IDL,
	_ codec.IdlEvent,
	readDefinition config.ReadDefinition,
	events EventsReader,
) error {
	mappedTuples := make(map[string]uint64)
	subKeys := [4][]string{}

	applyIndexedFieldTuple(mappedTuples, subKeys, readDefinition.IndexedField0, 0)
	applyIndexedFieldTuple(mappedTuples, subKeys, readDefinition.IndexedField1, 1)
	applyIndexedFieldTuple(mappedTuples, subKeys, readDefinition.IndexedField2, 2)
	applyIndexedFieldTuple(mappedTuples, subKeys, readDefinition.IndexedField3, 3)

	filter := toLPFilter(readDefinition.PollingFilter, contractAddress, subKeys[:])

	s.filters = append(s.filters, filter)
	s.bdRegistry.AddReadBinding(namespace, genericName, newEventReadBinding(
		namespace,
		genericName,
		mappedTuples,
		events,
		filter.EventSig,
	))

	return nil
}

func toLPFilter(
	f *config.PollingFilter,
	address solana.PublicKey,
	subKeyPaths [][]string,
) logpoller.Filter {
	return logpoller.Filter{
		Address:     logpoller.PublicKey(address),
		EventName:   f.EventName,
		EventSig:    logpoller.EventSignature([]byte(f.EventName)[:logpoller.EventSignatureLength]),
		SubkeyPaths: logpoller.SubKeyPaths(subKeyPaths),
		Retention:   f.Retention,
		MaxLogsKept: f.MaxLogsKept,
	}
}

func applyIndexedFieldTuple(lookup map[string]uint64, subKeys [4][]string, conf *config.IndexedField, idx uint64) {
	if conf != nil {
		lookup[conf.OffChainPath] = idx
		subKeys[idx] = strings.Split(conf.OnChainPath, ".")
	}
}

package chainreader

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"sync"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/fee_quoter"
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
	Start(ctx context.Context) error
	Ready() error
	RegisterFilter(context.Context, logpoller.Filter) error
	FilteredLogs(context.Context, []query.Expression, query.LimitAndSort, string) ([]logpoller.Log, error)
}

const ServiceName = "SolanaContractReader"

// TODO NONEVM-1320 fix this edge case
const GetTokenPrices = "GetTokenPrices"

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
		if len(s.filters) == 0 {
			// No dependency on EventReader
			return nil
		}
		if s.reader.Ready() != nil {
			// Start EventReader if it hasn't already been
			// Lazily starting it here rather than earlier, since nodes running only ordinary DF jobs don't need it
			err := s.reader.Start(ctx)
			if err != nil &&
				!strings.Contains(err.Error(), "has already been started") { // in case another thread calls Start() after Ready() returns
				return fmt.Errorf("%d event filters defined in ChainReader config, but unable to start event reader: %w", len(s.filters), err)
			}
		}
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

	if len(values.reads) == 0 {
		return fmt.Errorf("%w: no reads defined for readIdentifier: %q", types.ErrInvalidConfig, readIdentifier)
	}

	if len(values.reads) > 1 {
		return doMultiRead(ctx, s.client, s.bdRegistry, values, params, returnVal)
	}

	// TODO this is a temporary edge case - NONEVM-1320
	if values.reads[0].readName == GetTokenPrices {
		return s.handleGetTokenPricesGetLatestValue(ctx, params, values, returnVal)
	}

	batch := []call{
		{
			Namespace: values.contract,
			ReadName:  values.reads[0].readName,
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
			// TODO this is a temporary edge case - NONEVM-1320
			if readReq.ReadName == GetTokenPrices {
				return nil, fmt.Errorf("%w: %s is not supported in batch requests", types.ErrInvalidType, GetTokenPrices)
			}

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
		if sg, isInAShareGroup := s.bdRegistry.getShareGroup(bindings[i].Name); isInAShareGroup {
			s.lookup.bindAddressForContract(bindings[i].Name, "")
			for _, namespace := range sg.group {
				if err := s.addAddressResponseHardCoderModifier(namespace, bindings[i].Address); err != nil {
					return fmt.Errorf("failed to add address response hard coder modifier for contract: %q, : %w", namespace, err)
				}
			}
			return nil
		}

		if err := s.addAddressResponseHardCoderModifier(bindings[i].Name, bindings[i].Address); err != nil {
			return fmt.Errorf("failed to add address response hard coder modifier for contract: %q, : %w", bindings[i].Name, err)
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

	if len(values.reads) == 0 {
		return nil, fmt.Errorf("%w: no reads defined for read identifier", types.ErrInvalidConfig)
	}

	return s.bdRegistry.CreateType(values.contract, values.reads[0].readName, forEncoding)
}

func (s *ContractReaderService) addCodecDef(parsed *codec.ParsedTypes, forEncoding bool, namespace, genericName string, idl codec.IDL, idlDefinition interface{}, modCfg commoncodec.ModifiersConfig) error {
	mod, err := modCfg.ToModifier(codec.DecoderHooks...)
	if err != nil {
		return err
	}

	cEntry, err := codec.CreateCodecEntry(idlDefinition, genericName, idl, mod)
	if err != nil {
		return err
	}

	if forEncoding {
		parsed.EncoderDefs[codec.WrapItemType(true, namespace, genericName)] = cEntry
	} else {
		parsed.DecoderDefs[codec.WrapItemType(false, namespace, genericName)] = cEntry
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

func (s *ContractReaderService) addAccountRead(namespace string, genericName string, idl codec.IDL, outputIDLDef codec.IdlTypeDef, readDefinition config.ReadDefinition) error {
	reads := []read{{readName: genericName, useParams: true}}
	if readDefinition.MultiReader != nil {
		multiRead, err := s.addMultiAccountReadToCodec(namespace, readDefinition, idl)
		if err != nil {
			return err
		}
		reads = append(reads, multiRead...)
	}

	var inputIDLDef interface{} = codec.NilIdlTypeDefTy
	isPDA := false

	// Create PDA read binding if PDA prefix or seeds configs are populated
	if readDefinition.PDADefinition.Prefix != nil || len(readDefinition.PDADefinition.Seeds) > 0 {
		inputIDLDef = readDefinition.PDADefinition
		isPDA = true
	}

	if err := s.addAccountReadToCodec(s.parsed, namespace, genericName, idl, inputIDLDef, outputIDLDef, readDefinition); err != nil {
		return err
	}

	s.bdRegistry.AddReadBinding(namespace, genericName, newAccountReadBinding(namespace, genericName, isPDA, readDefinition.PDADefinition.Prefix, idl, inputIDLDef, outputIDLDef, readDefinition))
	s.lookup.addReadNameForContract(namespace, genericName, reads)
	return nil
}

func (s *ContractReaderService) addAccountReadToCodec(parsed *codec.ParsedTypes, namespace string, genericName string, idl codec.IDL, inputIDLDef interface{}, outputIDLDef codec.IdlTypeDef, readDefinition config.ReadDefinition) error {
	if err := s.addCodecDef(parsed, true, namespace, genericName, idl, inputIDLDef, readDefinition.InputModifications); err != nil {
		return err
	}

	return s.addCodecDef(parsed, false, namespace, genericName, idl, outputIDLDef, readDefinition.OutputModifications)
}

func (s *ContractReaderService) addMultiAccountReadToCodec(namespace string, readDefinition config.ReadDefinition, idl codec.IDL) ([]read, error) {
	var reads []read
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

		var inputIDLDef interface{} = codec.NilIdlTypeDefTy
		isPDA := false

		// Create PDA read binding if PDA prefix or seeds configs are populated
		if mr.PDADefinition.Prefix != nil || len(mr.PDADefinition.Seeds) > 0 {
			inputIDLDef = mr.PDADefinition
			isPDA = true
		}

		// multi read defs don't have a generic name as they are accessed from the parent read which does have a generic name.
		// generic name is used everywhere, so add a prefix to avoid potential collision with generic names of other reads.
		genericName := "multiread-" + mr.ChainSpecificName
		if err = s.addAccountReadToCodec(s.parsed, namespace, genericName, idl, inputIDLDef, accountIDLDef, mr); err != nil {
			return nil, fmt.Errorf("failed to add read to multi read %q: %w", mr.ChainSpecificName, err)
		}

		s.bdRegistry.AddReadBinding(namespace, genericName, newAccountReadBinding(namespace, genericName, isPDA, mr.PDADefinition.Prefix, idl, inputIDLDef, accountIDLDef, readDefinition))
		reads = append(reads, read{
			readName:  genericName,
			useParams: readDefinition.MultiReader.ReuseParams,
		})
	}
	return reads, nil
}

func (s *ContractReaderService) addAddressResponseHardCoderModifier(namespace string, addressToHardCode string) error {
	address, err := solana.PublicKeyFromBase58(addressToHardCode)
	if err != nil {
		return fmt.Errorf("failed to parse address: %q", addressToHardCode)
	}

	rBindings, err := s.bdRegistry.GetReadBindings(namespace)
	if err != nil {
		return fmt.Errorf("failed to get read bindings : %w", err)
	}

	for _, rb := range rBindings {
		if addressResponseHardCoder := rb.GetAddressResponseHardCoder(); addressResponseHardCoder != nil {
			hardCoder := rb.GetAddressResponseHardCoder()
			if hardCoder == nil {
				continue
			}

			for k := range hardCoder.OffChainValues {
				hardCoder.OffChainValues[k] = address
			}

			idl, inputIDlType, outputIDLType := rb.GetIDLInfo()
			parsed := &codec.ParsedTypes{
				EncoderDefs: map[string]codec.Entry{},
				DecoderDefs: map[string]codec.Entry{},
			}

			readDef := rb.GetReadDefinition()
			readDef.OutputModifications = append(readDef.OutputModifications, hardCoder)
			if err = s.addAccountReadToCodec(parsed, namespace, rb.GetGenericName(), idl, inputIDlType, outputIDLType, readDef); err != nil {
				return fmt.Errorf("failed to set codec with address response hardcoder for read: %q: %w", rb.GetGenericName(), err)
			}

			newCodec, err := parsed.ToCodec()
			if err != nil {
				return fmt.Errorf("failed to create codec with address response hardcoder for read: %q: %w", rb.GetGenericName(), err)
			}

			rb.SetCodec(newCodec)
		}
	}
	return nil
}

func (s *ContractReaderService) addEventRead(
	namespace, genericName string,
	contractAddress solana.PublicKey,
	idl codec.IDL,
	eventIdl codec.IdlEvent,
	readDefinition config.ReadDefinition,
	events EventsReader,
) error {
	mappedTuples := make(map[string]uint64)
	subKeys := [4][]string{}

	applyIndexedFieldTuple(mappedTuples, subKeys, readDefinition.IndexedField0, 0)
	applyIndexedFieldTuple(mappedTuples, subKeys, readDefinition.IndexedField1, 1)
	applyIndexedFieldTuple(mappedTuples, subKeys, readDefinition.IndexedField2, 2)
	applyIndexedFieldTuple(mappedTuples, subKeys, readDefinition.IndexedField3, 3)

	filter := toLPFilter(readDefinition.PollingFilter, contractAddress, subKeys[:],
		codec.EventIDLTypes{Event: eventIdl, Types: idl.Types})

	s.filters = append(s.filters, filter)
	s.bdRegistry.AddReadBinding(namespace, genericName, newEventReadBinding(
		namespace,
		genericName,
		mappedTuples,
		events,
		filter.EventSig,
		readDefinition,
	))

	return nil
}

func toLPFilter(
	f *config.PollingFilter,
	address solana.PublicKey,
	subKeyPaths [][]string,
	eventIdl codec.EventIDLTypes,
) logpoller.Filter {
	return logpoller.Filter{
		Address:     logpoller.PublicKey(address),
		EventName:   f.EventName,
		EventSig:    logpoller.NewEventSignatureFromName(f.EventName),
		EventIdl:    logpoller.EventIdl(eventIdl),
		SubkeyPaths: subKeyPaths,
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

func (s *ContractReaderService) handleGetTokenPricesGetLatestValue(
	ctx context.Context,
	params any,
	values readValues,
	returnVal any,
) error {
	pdaAddresses, err := s.getPDAsForGetTokenPrices(params, values)
	if err != nil {
		return err
	}

	data, err := s.client.GetMultipleAccountData(ctx, pdaAddresses...)
	if err != nil {
		return fmt.Errorf(
			"for contract %q read %q: failed to get multiple account data: %w",
			values.contract, values.reads[0].readName, err,
		)
	}

	// -------------- Fill out the returnVal slice with data --------------
	// can't typecast returnVal so we have to use reflection here

	// Ensure `returnVal` is a pointer to a slice we can populate.
	returnSliceVal := reflect.ValueOf(returnVal)
	if returnSliceVal.Kind() == reflect.Ptr {
		returnSliceVal = returnSliceVal.Elem()
		if returnSliceVal.Kind() == reflect.Ptr {
			returnSliceVal = returnSliceVal.Elem()
		}
	}
	if returnSliceVal.Kind() != reflect.Slice {
		return fmt.Errorf(
			"for contract %q read %q: expected `returnVal` to be a slice, got %s",
			values.contract, values.reads[0].readName, returnSliceVal.Kind(),
		)
	}

	elemType := returnSliceVal.Type().Elem()
	for _, d := range data {
		var wrapper fee_quoter.BillingTokenConfigWrapper
		if err = wrapper.UnmarshalWithDecoder(bin.NewBorshDecoder(d)); err != nil {
			return fmt.Errorf(
				"for contract %q read %q: failed to unmarshal account data: %w",
				values.contract, values.reads[0].readName, err,
			)
		}

		newElem := reflect.New(elemType).Elem()

		valueField := newElem.FieldByName("Value")
		if !valueField.IsValid() {
			return fmt.Errorf(
				"for contract %q read %q: struct type missing `Value` field",
				values.contract, values.reads[0].readName,
			)
		}
		valueField.Set(reflect.ValueOf(big.NewInt(0).SetBytes(wrapper.Config.UsdPerToken.Value[:])))

		timestampField := newElem.FieldByName("Timestamp")
		if !timestampField.IsValid() {
			return fmt.Errorf(
				"for contract %q read %q: struct type missing `Timestamp` field",
				values.contract, values.reads[0].readName,
			)
		}

		// nolint:gosec
		// G115: integer overflow conversion int64 -&gt; uint32
		timestampField.Set(reflect.ValueOf(uint32(wrapper.Config.UsdPerToken.Timestamp)))

		returnSliceVal.Set(reflect.Append(returnSliceVal, newElem))
	}

	return nil
}

func (s *ContractReaderService) getPDAsForGetTokenPrices(params any, values readValues) ([]solana.PublicKey, error) {
	val := reflect.ValueOf(params)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf(
			"for contract %q read %q: expected `params` to be a struct, got %s",
			values.contract, values.reads[0].readName, val.Kind(),
		)
	}

	field := val.FieldByName("Tokens")
	if !field.IsValid() {
		return nil, fmt.Errorf(
			"for contract %q read %q: no field named 'Tokens' found in params",
			values.contract, values.reads[0].readName,
		)
	}

	tokens, ok := field.Interface().(*[][32]uint8)
	if !ok {
		return nil, fmt.Errorf(
			"for contract %q read %q: 'Tokens' field is not of type *[][32]uint8",
			values.contract, values.reads[0].readName,
		)
	}

	programAddress, err := solana.PublicKeyFromBase58(values.address)
	if err != nil {
		return nil, fmt.Errorf(
			"for contract %q read %q: %w (could not parse program address %q)",
			values.contract, values.reads[0].readName, types.ErrInvalidConfig, values.address,
		)
	}

	// Build the PDA addresses for all tokens.
	var pdaAddresses []solana.PublicKey
	for _, token := range *tokens {
		tokenAddr := solana.PublicKeyFromBytes(token[:])
		if !tokenAddr.IsOnCurve() || tokenAddr.IsZero() {
			return nil, fmt.Errorf(
				"for contract %q read %q: invalid token address %v (off-curve or zero)",
				values.contract, values.reads[0].readName, tokenAddr,
			)
		}

		pdaAddress, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("fee_billing_token_config"), tokenAddr.Bytes()},
			programAddress,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"for contract %q read %q: %w (failed to find PDA for token %v)",
				values.contract, values.reads[0].readName, types.ErrInvalidConfig, tokenAddr,
			)
		}
		pdaAddresses = append(pdaAddresses, pdaAddress)
	}
	return pdaAddresses, nil
}

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
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/go-viper/mapstructure/v2"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/v0_1_1/fee_quoter"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	ccipconsts "github.com/smartcontractkit/chainlink-common/pkg/types/ccip/consts"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"
	"github.com/smartcontractkit/chainlink-protos/cre/go/values"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"
)

type EventsReader interface {
	Start(ctx context.Context) error
	Ready() error
	HasFilter(context.Context, string) bool
	RegisterFilter(context.Context, logpollertypes.Filter) error
	UnregisterFilter(ctx context.Context, name string) error
	FilteredLogs(context.Context, []query.Expression, query.LimitAndSort, string) ([]logpollertypes.Log, error)
}

const ServiceName = "SolanaContractReader"

// TODO NONEVM-1320 fix this edge case
const GetTokenPrices = ccipconsts.MethodNameFeeQuoterGetTokenPrices

type ContractReaderService struct {
	types.UnimplementedContractReader

	// provided dependencies
	lggr   logger.Logger
	client MultipleAccountGetter
	reader EventsReader

	// internal values
	bdRegistry    *bindingsRegistry
	lookup        *lookup
	parsed        *solcommoncodec.ParsedTypes
	codec         types.RemoteCodec
	shouldStartLP bool

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
		lggr:       logger.Named(lggr, ServiceName),
		client:     dataReader,
		bdRegistry: newBindingsRegistry(),
		lookup:     newLookup(),
		parsed: &solcommoncodec.ParsedTypes{
			EncoderDefs: map[string]solcommoncodec.Entry{},
			DecoderDefs: map[string]solcommoncodec.Entry{},
		},
		reader: reader,
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
		if !s.shouldStartLP {
			// No dependency on EventReader
			return nil
		}

		if s.reader.Ready() != nil {
			// Start EventReader if it hasn't already been
			// Lazily starting it here rather than earlier, since nodes running only ordinary DF jobs don't need it
			err := s.reader.Start(ctx)
			if err != nil &&
				!strings.Contains(err.Error(), "has already been started") { // in case another thread calls Start() after Ready() returns
				return fmt.Errorf("event filters are defined in ChainReader config, but unable to start event reader: %w", err)
			}
		}

		// registering filters needs a context so we should be able to use the start function context.
		return s.bdRegistry.RegisterAll(ctx)
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
		return doMultiRead(ctx, s.lggr, s.client, s.bdRegistry, values, params, returnVal)
	}

	// TODO this is a temporary edge case - NONEVM-1320
	if values.reads[0].readName == GetTokenPrices {
		if err := s.handleGetTokenPricesGetLatestValue(ctx, params, values, returnVal); err != nil {
			return fmt.Errorf("failed to read contract: %q, account: %q err: %w", values.contract, values.reads[0].readName, err)
		}
		return nil
	}

	batch := []call{
		{
			Namespace:               values.contract,
			ReadName:                values.reads[0].readName,
			Params:                  params,
			ReturnVal:               returnVal,
			ErrOnMissingAccountData: values.reads[0].errOnMissingAccountData,
		},
	}

	results, err := doMethodBatchCall(ctx, s.lggr, s.client, s.bdRegistry, batch)
	if err != nil {
		return err
	}

	if len(results) != len(batch) {
		return fmt.Errorf("%w: unexpected number of results", types.ErrInternal)
	}

	if results[0].err != nil {
		if errors.Is(results[0].err, types.ErrNotFound) {
			return types.ErrNotFound
		}

		return fmt.Errorf("%w: %s", types.ErrInternal, results[0].err)
	}

	return nil
}

// BatchGetLatestValues implements the types.ContractReader interface.
func (s *ContractReaderService) BatchGetLatestValues(ctx context.Context, request types.BatchGetLatestValuesRequest) (types.BatchGetLatestValuesResult, error) {
	idxLookup := make(map[types.BoundContract]map[int]int)
	multiIdxLookup := make(map[types.BoundContract]map[int]int)
	result := make(types.BatchGetLatestValuesResult)

	var (
		batch            []call
		multiReadResults []batchResultWithErr
	)

	for bound, req := range request {
		idxLookup[bound] = make(map[int]int)
		multiIdxLookup[bound] = make(map[int]int)
		result[bound] = make(types.ContractBatchResults, len(req))

		for idx, readReq := range req {
			readIdentifier := bound.ReadIdentifier(readReq.ReadName)
			vals, ok := s.lookup.getContractForReadIdentifiers(readIdentifier)
			if !ok {
				return nil, fmt.Errorf("%w: no contract for read identifier: %q", types.ErrInvalidType, readIdentifier)
			}

			// exclude multi read reads from the big batch request and populate them separately and merge results later.
			if len(vals.reads) > 1 {
				err := doMultiRead(ctx, s.lggr, s.client, s.bdRegistry, vals, readReq.Params, readReq.ReturnVal)

				multiIdxLookup[bound][idx] = len(multiReadResults)
				multiReadResults = append(multiReadResults, batchResultWithErr{address: vals.address, namespace: vals.contract, readName: readReq.ReadName, returnVal: readReq.ReturnVal, err: err})

				continue
			}

			idxLookup[bound][idx] = len(batch)

			// TODO: this is a temporary edge case - NONEVM-1320
			if readReq.ReadName == GetTokenPrices {
				return nil, fmt.Errorf("%w: %s is not supported in batch requests", types.ErrInvalidType, GetTokenPrices)
			}

			batch = append(batch, call{
				Namespace:               bound.Name,
				ReadName:                readReq.ReadName,
				Params:                  readReq.Params,
				ReturnVal:               readReq.ReturnVal,
				ErrOnMissingAccountData: vals.reads[0].errOnMissingAccountData,
			})
		}
	}

	results, err := doMethodBatchCall(ctx, s.lggr, s.client, s.bdRegistry, batch)
	if err != nil {
		return nil, err
	}

	if len(results) != len(batch) {
		return nil, errors.New("unexpected number of results")
	}

	populateResultFromLookup(idxLookup, result, results)
	populateResultFromLookup(multiIdxLookup, result, multiReadResults)

	return result, nil
}

// QueryKey implements the types.ContractReader interface.
func (s *ContractReaderService) QueryKey(ctx context.Context, contract types.BoundContract, filter query.KeyFilter, limitAndSort query.LimitAndSort, sequenceDataType any) ([]types.Sequence, error) {
	binding, err := s.bdRegistry.GetReader(contract.Name, filter.Key)
	if err != nil {
		return nil, err
	}

	eBinding, ok := binding.(eventBinding)
	if !ok {
		return nil, fmt.Errorf("%w: invalid binding type for %s", types.ErrInvalidType, contract.Name)
	}

	_, isValuePtr := sequenceDataType.(*values.Value)
	if !isValuePtr {
		return eBinding.QueryKey(ctx, filter, limitAndSort, sequenceDataType)
	}

	dataTypeFromReadIdentifier, err := s.CreateContractType(contract.ReadIdentifier(filter.Key), false)
	if err != nil {
		return nil, err
	}

	sequence, err := eBinding.QueryKey(ctx, filter, limitAndSort, dataTypeFromReadIdentifier)
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

// Bind implements the types.ContractReader interface and allows new contract namespaceBindings to be added to the
// service.
//
// Bind has a side-effect of updating a binding with a shared address if the bound contract has been configured to be
// part of a share group.
func (s *ContractReaderService) Bind(ctx context.Context, bindings []types.BoundContract) error {
	for idx := range bindings {
		if s.lookup.hasAddress(bindings[idx].Name, bindings[idx].Address) {
			continue
		}

		if err := s.bdRegistry.Bind(ctx, s.reader, &bindings[idx]); err != nil {
			return err
		}

		s.lookup.bindAddressForContract(bindings[idx].Name, bindings[idx].Address)

		// also bind with an empty address so that we can look up the contract without providing address when calling CR methods
		if sg, isInAShareGroup := s.bdRegistry.GetShares(bindings[idx].Name); isInAShareGroup {
			s.lookup.bindAddressForContract(bindings[idx].Name, "")

			for _, namespace := range sg.getGroups() {
				if err := s.addAddressResponseHardCoderModifier(namespace, bindings[idx].Address); err != nil {
					return fmt.Errorf("failed to add address response hard coder modifier for contract: %q, : %w", namespace, err)
				}
			}

			continue
		}

		if err := s.addAddressResponseHardCoderModifier(bindings[idx].Name, bindings[idx].Address); err != nil {
			return fmt.Errorf("failed to add address response hard coder modifier for contract: %q, : %w", bindings[idx].Name, err)
		}
	}

	return nil
}

// Unbind implements the types.ContractReader interface and allows existing contract namespaceBindings to be removed
// from the service.
func (s *ContractReaderService) Unbind(ctx context.Context, bindings []types.BoundContract) error {
	for i := range bindings {
		if err := s.bdRegistry.Unbind(ctx, s.reader, bindings[i]); err != nil {
			return err
		}

		s.lookup.unbindAddressForContract(bindings[i].Name, bindings[i].Address)

		// also unbind an empty address if a share group exists
		s.lookup.unbindAddressForContract(bindings[i].Name, "")
	}

	return nil
}

// CreateContractType implements the ContractTypeProvider interface and allows the chain reader
// service to explicitly define the expected type for a grpc server to provide.
func (s *ContractReaderService) CreateContractType(readIdentifier string, forEncoding bool) (any, error) {
	values, ok := s.lookup.getContractForReadIdentifiers(readIdentifier)
	if !ok {
		return nil, fmt.Errorf("%w: no contract for read identifier: %q", types.ErrInvalidConfig, readIdentifier)
	}

	if len(values.reads) == 0 {
		return nil, fmt.Errorf("%w: no reads defined for read identifier: %q", types.ErrInvalidConfig, readIdentifier)
	}

	return s.bdRegistry.CreateType(values.contract, values.reads[0].readName, forEncoding)
}

func (s *ContractReaderService) addCodecDef(parsed *solcommoncodec.ParsedTypes, forEncoding bool, namespace, genericName string, idl codecv1.IDL, idlDefinition interface{}, modCfg commoncodec.ModifiersConfig) error {
	mod, err := modCfg.ToModifier(solcommoncodec.DecoderHooks...)
	if err != nil {
		return err
	}

	cEntry, err := codecv1.CreateCodecEntry(idlDefinition, genericName, idl, mod)
	if err != nil {
		return err
	}

	if forEncoding {
		parsed.EncoderDefs[solcommoncodec.WrapItemType(true, namespace, genericName)] = cEntry
	} else {
		parsed.DecoderDefs[solcommoncodec.WrapItemType(false, namespace, genericName)] = cEntry
	}
	return nil
}

func (s *ContractReaderService) initNamespace(namespaces map[string]config.ChainContractReader) error {
	for namespace, nameSpaceDef := range namespaces {
		for genericName, read := range nameSpaceDef.Reads {
			utils.InjectAddressModifier(read.InputModifications, read.OutputModifications)

			switch read.ReadType {
			case config.Account:
				idlDef, err := codecv1.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeAccountDef, read.ChainSpecificName, nameSpaceDef.IDL)
				if err != nil {
					return err
				}

				accountIDLDef, isOk := idlDef.(codecv1.IdlTypeDef)
				if !isOk {
					return fmt.Errorf("unexpected type %T from IDL definition for account read: %q, with chainSpecificName: %q, of type: %q", accountIDLDef, genericName, read.ChainSpecificName, read.ReadType)
				}
				if err = s.addAccountRead(namespace, genericName, nameSpaceDef.IDL, accountIDLDef, read); err != nil {
					return err
				}
			case config.Event:
				if err := s.addEventRead(
					nameSpaceDef.PollingFilter,
					namespace, genericName,
					nameSpaceDef.IDL,
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

func (s *ContractReaderService) addAccountRead(namespace string, genericName string, idl codecv1.IDL, outputIDLDef codecv1.IdlTypeDef, readDefinition config.ReadDefinition) error {
	reads := []read{{readName: genericName, useParams: true, errOnMissingAccountData: readDefinition.ErrOnMissingAccountData}}
	if readDefinition.MultiReader != nil {
		multiRead, err := s.addMultiAccountReadToCodec(namespace, readDefinition, idl)
		if err != nil {
			return err
		}
		reads = append(reads, multiRead...)
	}

	var inputIDLDef interface{} = codecv1.NilIdlTypeDefTy
	isPDA := false

	// Create PDA read binding if PDA prefix or seeds configs are populated
	if readDefinition.PDADefinition.Prefix != nil || len(readDefinition.PDADefinition.Seeds) > 0 {
		inputIDLDef = readDefinition.PDADefinition
		isPDA = true
	}

	if err := s.addReadToCodec(s.parsed, namespace, genericName, idl, inputIDLDef, outputIDLDef, readDefinition); err != nil {
		return err
	}

	s.bdRegistry.AddReader(namespace, genericName, newAccountReadBinding(namespace, genericName, isPDA, readDefinition.PDADefinition.Prefix, idl, inputIDLDef, outputIDLDef, readDefinition))
	s.lookup.addReadNameForContract(namespace, genericName, reads)

	return nil
}

func (s *ContractReaderService) addReadToCodec(parsed *solcommoncodec.ParsedTypes, namespace string, genericName string, idl codecv1.IDL, inputIDLDef interface{}, outputIDLDef interface{}, readDefinition config.ReadDefinition) error {
	if err := s.addCodecDef(parsed, true, namespace, genericName, idl, inputIDLDef, readDefinition.InputModifications); err != nil {
		return err
	}

	return s.addCodecDef(parsed, false, namespace, genericName, idl, outputIDLDef, readDefinition.OutputModifications)
}

func (s *ContractReaderService) addMultiAccountReadToCodec(namespace string, readDefinition config.ReadDefinition, idl codecv1.IDL) ([]read, error) {
	var reads []read
	for _, mr := range readDefinition.MultiReader.Reads {
		idlDef, err := codecv1.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeAccountDef, mr.ChainSpecificName, idl)
		if err != nil {
			return nil, err
		}

		if mr.ReadType != config.Account {
			return nil, fmt.Errorf("unexpected read type %q for dynamic hard coder read: %q in namespace: %q", mr.ReadType, mr.ChainSpecificName, namespace)
		}

		accountIDLDef, isOk := idlDef.(codecv1.IdlTypeDef)
		if !isOk {
			return nil, fmt.Errorf("unexpected type %T from IDL definition for account read with chainSpecificName: %q, of type: %q", accountIDLDef, mr.ChainSpecificName, mr.ReadType)
		}

		var inputIDLDef interface{} = codecv1.NilIdlTypeDefTy
		isPDA := false

		// Create PDA read binding if PDA prefix or seeds configs are populated
		if mr.PDADefinition.Prefix != nil || len(mr.PDADefinition.Seeds) > 0 {
			inputIDLDef = mr.PDADefinition
			isPDA = true
		}

		// multi read defs don't have a generic name as they are accessed from the parent read which does have a generic name.
		// generic name is used everywhere, so add a prefix to avoid potential collision with generic names of other reads.
		genericName := fmt.Sprintf("multiread-%v-%v-%v", namespace, readDefinition.ChainSpecificName, mr.ChainSpecificName)
		if err = s.addReadToCodec(s.parsed, namespace, genericName, idl, inputIDLDef, accountIDLDef, mr); err != nil {
			return nil, fmt.Errorf("failed to add read to multi read %q: %w", mr.ChainSpecificName, err)
		}

		s.bdRegistry.AddReader(namespace, genericName, newAccountReadBinding(namespace, genericName, isPDA, mr.PDADefinition.Prefix, idl, inputIDLDef, accountIDLDef, readDefinition))
		reads = append(reads, read{
			readName:                genericName,
			useParams:               readDefinition.MultiReader.ReuseParams,
			errOnMissingAccountData: mr.ErrOnMissingAccountData,
		})
	}

	return reads, nil
}

func (s *ContractReaderService) addAddressResponseHardCoderModifier(namespace string, addressToHardCode string) error {
	address, err := solana.PublicKeyFromBase58(addressToHardCode)
	if err != nil {
		return fmt.Errorf("failed to parse address: %q", addressToHardCode)
	}

	rBindings, err := s.bdRegistry.GetReaders(namespace)
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
			parsed := &solcommoncodec.ParsedTypes{
				EncoderDefs: map[string]solcommoncodec.Entry{},
				DecoderDefs: map[string]solcommoncodec.Entry{},
			}

			readDef := rb.GetReadDefinition()
			readDef.OutputModifications = append(readDef.OutputModifications, hardCoder)
			if err = s.addReadToCodec(parsed, namespace, rb.GetGenericName(), idl, inputIDlType, outputIDLType, readDef); err != nil {
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
	common *config.PollingFilter,
	namespace, genericName string,
	idl codecv1.IDL,
	readDefinition config.ReadDefinition,
	events EventsReader,
) error {
	if readDefinition.EventDefinitions == nil {
		return fmt.Errorf("%w: event definitions missing", types.ErrInvalidConfig)
	}

	conf := readDefinition.EventDefinitions

	idlDef, err := codecv1.FindDefinitionFromIDL(solcommoncodec.ChainConfigTypeEventDef, readDefinition.ChainSpecificName, idl)
	if err != nil {
		return err
	}

	pf, err := setPollingFilterOverrides(common, conf.PollingFilter)
	if err != nil {
		return fmt.Errorf("failed to set polling filter overrides: %w", err)
	}

	eventIdl, isOk := idlDef.(codecv1.IdlEvent)
	if !isOk {
		return fmt.Errorf(
			"unexpected type from IDL definition for event read: %q, with chainSpecificName: %q, of type: %q",
			genericName, readDefinition.ChainSpecificName, readDefinition.ReadType,
		)
	}

	subkeys := newIndexedSubkeys()

	applyIndexedFieldTuple(subkeys, conf.IndexedField0, 0)
	applyIndexedFieldTuple(subkeys, conf.IndexedField1, 1)
	applyIndexedFieldTuple(subkeys, conf.IndexedField2, 2)
	applyIndexedFieldTuple(subkeys, conf.IndexedField3, 3)

	eventDef := codecv1.EventIDLTypes{Event: eventIdl, Types: idl.Types}

	if err := s.addReadToCodec(s.parsed, namespace, genericName, idl, eventIdl, eventIdl, readDefinition); err != nil {
		return err
	}

	reader := newEventReadBinding(
		namespace,
		genericName,
		subkeys,
		events,
		readDefinition,
		pf,
	)

	s.shouldStartLP = true
	reader.SetFilter(toLPFilter(readDefinition.ChainSpecificName, pf, subkeys.subKeys[:], eventDef))

	s.bdRegistry.AddReader(namespace, genericName, reader)
	s.lookup.addReadNameForContract(namespace, genericName, []read{{readName: genericName, useParams: false}})

	return nil
}

func populateResultFromLookup(
	idxLookup map[types.BoundContract]map[int]int,
	output types.BatchGetLatestValuesResult,
	results []batchResultWithErr,
) {
	for bound, idxs := range idxLookup {
		for reqIdx, callIdx := range idxs {
			res := types.BatchReadResult{ReadName: results[callIdx].readName}
			res.SetResult(results[callIdx].returnVal, results[callIdx].err)

			output[bound][reqIdx] = res
		}
	}
}

func toLPFilter(
	name string,
	conf config.PollingFilter,
	subKeyPaths [][]string,
	eventIdl codecv1.EventIDLTypes,
) logpollertypes.Filter {
	return logpollertypes.Filter{
		EventName:       name,
		EventSig:        logpollertypes.NewEventSignatureFromName(name),
		StartingBlock:   conf.GetStartingBlock(),
		EventIdl:        logpollertypes.EventIdl(eventIdl),
		SubkeyPaths:     subKeyPaths,
		Retention:       conf.GetRetention(),
		MaxLogsKept:     conf.GetMaxLogsKept(),
		IncludeReverted: conf.GetIncludeReverted(),
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

func applyIndexedFieldTuple(subkeys *indexedSubkeys, conf *config.IndexedField, idx uint64) {
	if conf != nil {
		subkeys.addForIndex(conf.OffChainPath, conf.OnChainPath, idx)
	}
}

func (s *ContractReaderService) handleGetTokenPricesGetLatestValue(
	ctx context.Context,
	params any,
	values readValues,
	returnVal any,
) (err error) {
	// shouldn't happen, but just to be sure
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v", r)
		}
	}()

	pdaAddresses, err := s.getPDAsForGetTokenPrices(params, values)
	if err != nil {
		return err
	}

	if len(pdaAddresses) == 0 {
		s.lggr.Infof("No token addresses found in params: %v that were passed into %q, call to contract: %q with address: %q", params, GetTokenPrices, values.contract, values.address)
		return nil
	}

	accountsRes, err := s.client.GetMultipleAccountData(ctx, pdaAddresses...)
	if err != nil {
		return err
	}

	returnSliceVal := reflect.ValueOf(returnVal)
	if returnSliceVal.Kind() != reflect.Ptr {
		return fmt.Errorf("expected <**[]*struct { Value *big.Int; Timestamp *int64 } Value>, got %q", returnSliceVal.String())
	}

	returnSliceVal = returnSliceVal.Elem()
	// if called directly instead of as a loop
	if returnSliceVal.Kind() == reflect.Slice {
		underlyingType := returnSliceVal.Type().Elem()
		if underlyingType.Kind() == reflect.Struct {
			if _, hasValue := underlyingType.FieldByName("Value"); hasValue {
				if _, hasTimestamp := underlyingType.FieldByName("Timestamp"); hasTimestamp {
					sliceVal := reflect.MakeSlice(returnSliceVal.Type(), 0, 0)
					for _, accRes := range accountsRes {
						var wrapper fee_quoter.BillingTokenConfigWrapper
						// if we got back an empty account then the account must not exist yet, use zero value
						var data []byte
						if accRes != nil && accRes.Data != nil && accRes.Data.GetBinary() != nil {
							data = accRes.Data.GetBinary()
						}

						if len(data) > 0 {
							if err = wrapper.UnmarshalWithDecoder(bin.NewBorshDecoder(data)); err != nil {
								return err
							}
						}
						newElem := reflect.New(underlyingType).Elem()
						newElem.FieldByName("Value").Set(reflect.ValueOf(big.NewInt(0).SetBytes(wrapper.Config.UsdPerToken.Value[:])))
						// nolint:gosec
						// G115: integer overflow conversion int64 -&gt; uint32
						newElem.FieldByName("Timestamp").Set(reflect.ValueOf(uint32(wrapper.Config.UsdPerToken.Timestamp)))
						sliceVal = reflect.Append(sliceVal, newElem)
					}
					return mapstructure.Decode(sliceVal.Interface(), returnVal)
				}
			}
		}
	}

	returnSliceValType := returnSliceVal.Type()
	if returnSliceValType.Kind() != reflect.Ptr {
		return fmt.Errorf("expected <*[]*struct { Value *big.Int; Timestamp *int64 } Value>, got %q", returnSliceValType.String())
	}

	sliceType := returnSliceValType.Elem()
	if sliceType.Kind() != reflect.Slice {
		return fmt.Errorf("expected []*struct { Value *big.Int; Timestamp *int64 }, got %q", sliceType.String())
	}

	if returnSliceVal.IsNil() {
		// init a slice
		sliceVal := reflect.MakeSlice(sliceType, 0, 0)

		// create a pointer to that slice to match what slicePtr
		slicePtr := reflect.New(sliceType)
		slicePtr.Elem().Set(sliceVal)

		returnSliceVal.Set(slicePtr)
		returnSliceVal = returnSliceVal.Elem()
	}

	pointerType := sliceType.Elem()
	if pointerType.Kind() != reflect.Ptr {
		return fmt.Errorf("expected *struct { Value *big.Int; Timestamp *int64 }, got %q", pointerType.String())
	}

	underlyingStruct := pointerType.Elem()
	if underlyingStruct.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct { Value *big.Int; Timestamp *int64 }, got %q", underlyingStruct.String())
	}

	for _, accRes := range accountsRes {
		var wrapper fee_quoter.BillingTokenConfigWrapper

		var data []byte
		if accRes != nil && accRes.Data != nil && accRes.Data.GetBinary() != nil {
			data = accRes.Data.GetBinary()
		}

		if len(data) > 0 {
			if err = wrapper.UnmarshalWithDecoder(bin.NewBorshDecoder(data)); err != nil {
				return err
			}
		}

		newElemPtr := reflect.New(underlyingStruct)
		newElem := newElemPtr.Elem()
		valueField := newElem.FieldByName("Value")
		if !valueField.IsValid() {
			return fmt.Errorf("field `Value` missing from %q", newElem.String())
		}

		valueField.Set(reflect.ValueOf(big.NewInt(0).SetBytes(wrapper.Config.UsdPerToken.Value[:])))
		timestampField := newElem.FieldByName("Timestamp")
		if !timestampField.IsValid() {
			return fmt.Errorf("field `Timestamp` missing from %q", newElem.String())
		}
		// nolint:gosec
		// G115: integer overflow conversion int64 -&gt; uint32
		timestampField.Set(reflect.ValueOf(&wrapper.Config.UsdPerToken.Timestamp))
		returnSliceVal.Set(reflect.Append(returnSliceVal, newElemPtr))
	}

	return nil
}

func (s *ContractReaderService) getPDAsForGetTokenPrices(params any, values readValues) ([]solana.PublicKey, error) {
	val := reflect.ValueOf(params)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	var field reflect.Value
	switch val.Kind() {
	case reflect.Struct:
		field = val.FieldByName("Tokens")
		if !field.IsValid() {
			field = val.FieldByName("tokens")
		}
	case reflect.Map:
		field = val.MapIndex(reflect.ValueOf("Tokens"))
		if !field.IsValid() {
			field = val.MapIndex(reflect.ValueOf("tokens"))
		}
	default:
		return nil, fmt.Errorf(
			"for contract %q read %q: expected `params` to be a struct or map, got %q: %q",
			values.contract, values.reads[0].readName, val.Kind(), val.String(),
		)
	}

	if !field.IsValid() {
		return nil, fmt.Errorf(
			"for contract %q read %q: no field named 'Tokens' found in kind: %q: %q",
			values.contract, values.reads[0].readName, val.Kind(), val.String(),
		)
	}

	var tokens [][]uint8
	switch x := field.Interface().(type) {
	// this is the type when CR is called as LOOP and creates types from IDL
	case *[][32]uint8:
		for _, arr := range *x {
			tokens = append(tokens, arr[:]) // Slice [32]uint8 → []uint8
		}
	// this is the previously expected type when CR is called directly
	case [][]uint8:
		tokens = x
	// this is the expected type when CR is called directly
	case []ccipocr3.UnknownAddress:
		tokens = make([][]uint8, 0, len(x))
		for _, arr := range x {
			tokens = append(tokens, []uint8(arr)) // Cast ccipocr3.UnknownAddress → []uint8
		}
	default:
		return nil, fmt.Errorf(
			"for contract %q read %q: 'Tokens' field is neither *[][32]uint8 nor [][]uint8, got %T",
			values.contract, values.reads[0].readName, x,
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
	for _, token := range tokens {
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

func setPollingFilterOverrides(common *config.PollingFilter, overrides ...*config.PollingFilter) (config.PollingFilter, error) {
	final := reflect.New(reflect.TypeOf(common).Elem())
	valOfF := final.Elem()
	allOverrides := append([]*config.PollingFilter{common}, overrides...)

	for _, override := range allOverrides {
		if override == nil {
			continue
		}

		valOfO := reflect.Indirect(reflect.ValueOf(override))
		for idx := range valOfF.Type().NumField() {
			name := valOfO.Type().Field(idx).Name
			field := valOfF.FieldByName(name)
			valOfFieldO := valOfO.FieldByName(name)

			if valOfFieldO.IsZero() {
				continue
			}

			if field.CanSet() {
				newVal := reflect.New(field.Type().Elem())
				newVal.Elem().Set(valOfO.FieldByName(name).Elem())
				field.Set(newVal)
			}
		}
	}

	filter, ok := final.Elem().Interface().(config.PollingFilter)
	if !ok {
		return config.PollingFilter{}, fmt.Errorf("encountered unexpected type: %T, expected: %T", final.Elem().Interface(), config.PollingFilter{})
	}

	return filter, nil
}

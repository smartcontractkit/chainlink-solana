package chainreader

import (
	"context"
	"crypto/sha3"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/gagliardetto/solana-go"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	logpollertypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

type eventReadBinding struct {
	// dependencies
	reader   EventsReader
	remapper remapHelper
	codec    types.RemoteCodec
	modifier commoncodec.Modifier
	conf     config.PollingFilter
	// filter in eventReadBinding is to be used as an override for lp filter defined in the namespace binding.
	// If filter is nil, this event should be registered with the lp filter defined in the namespace binding.
	filter *syncedFilter

	// static data
	namespace, genericName string
	eventSig               logpollertypes.EventSignature
	indexedSubKeys         *indexedSubkeys
	readDefinition         config.ReadDefinition

	// thread protected fields
	mu             sync.RWMutex
	key            solana.PublicKey
	bound          bool
	registerCalled bool
}

func newEventReadBinding(
	namespace, genericName string,
	indexedSubKeys *indexedSubkeys,
	reader EventsReader,
	readDefinition config.ReadDefinition,
	conf config.PollingFilter,
) *eventReadBinding {
	binding := &eventReadBinding{
		filter:         newSyncedFilter(),
		namespace:      namespace,
		genericName:    genericName,
		indexedSubKeys: indexedSubKeys,
		reader:         reader,
		readDefinition: readDefinition,
		conf:           conf,
	}

	binding.remapper = remapHelper{binding.remapPrimitive}

	return binding
}

func (b *eventReadBinding) Bind(ctx context.Context, address solana.PublicKey) error {
	b.setBinding(address)

	if b.filter == nil {
		return nil
	}

	b.filter.SetAddress(address)

	if !b.filter.Dirty() {
		return nil
	}

	return b.update(ctx)
}

func (b *eventReadBinding) Unbind(ctx context.Context) error {
	if !b.isBound() {
		return nil
	}

	if b.filter == nil {
		return nil
	}

	if err := b.Unregister(ctx); err != nil {
		return err
	}

	b.unsetBinding()

	return nil
}

func (b *eventReadBinding) Register(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.registerCalled = true

	if b.filter == nil {
		return nil
	}

	// can't be true before filters params are set so there is no race with a bad filter outcome
	if !b.bound {
		return nil
	}

	newName := b.deriveName()

	b.filter.SetName(newName)

	return b.filter.Register(ctx, b.reader)
}

func (b *eventReadBinding) update(ctx context.Context) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.filter == nil {
		return nil
	}

	if !b.bound {
		return nil
	}

	if !b.registerCalled {
		return nil
	}

	newName := b.deriveName()

	return b.filter.Update(ctx, b.reader, newName)
}

func (b *eventReadBinding) deriveName() string {
	// include eventSig, readDef, address, subkeyPaths, indexedSubkeys
	data := b.filter.filter.EventSig[:]
	data = append(data, []byte(b.readDefinition.ChainSpecificName)...)
	data = append(data, b.filter.filter.Address.ToSolana().Bytes()...)
	data = append(data, []byte(b.filter.filter.EventName)...)

	for _, sub := range b.filter.filter.SubkeyPaths {
		for _, key := range sub {
			data = append(data, []byte(key)...)
		}
	}

	for _, sub := range b.indexedSubKeys.subKeys {
		for _, key := range sub {
			data = append(data, key...)
		}
	}
	hash := sha3.Sum256(data)

	ret := fmt.Sprintf("%s.%s.%x", b.namespace, b.genericName, hash[:])

	return ret
}

func (b *eventReadBinding) Unregister(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.filter == nil {
		return nil
	}

	if !b.bound {
		return nil
	}

	return b.filter.Unregister(ctx, b.reader)
}

// GetAddress for events returns a static address. Since solana contracts emit events, and not accounts, PDAs are not
// valid for events.
func (b *eventReadBinding) GetAddress(_ context.Context, _ any) (solana.PublicKey, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.key, nil
}

func (b *eventReadBinding) GetGenericName() string {
	return b.genericName
}

func (b *eventReadBinding) GetReadDefinition() config.ReadDefinition {
	return b.readDefinition
}

func (b *eventReadBinding) GetIDLInfo() (idl codecv1.IDL, inputIDLTypeDef interface{}, outputIDLTypeDef codecv1.IdlTypeDef) {
	return codecv1.IDL{}, codecv1.IdlTypeDef{}, codecv1.IdlTypeDef{}
}

func (b *eventReadBinding) GetAddressResponseHardCoder() *commoncodec.HardCodeModifierConfig {
	return nil
}

func (b *eventReadBinding) SetCodec(codec types.RemoteCodec) {
	b.codec = codec
}

func (b *eventReadBinding) SetModifier(modifier commoncodec.Modifier) {
	b.modifier = modifier
}

func (b *eventReadBinding) SetFilter(filter logpollertypes.Filter) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.filter.SetFilter(filter)
	b.eventSig = filter.EventSig
}

func (b *eventReadBinding) CreateType(forEncoding bool) (any, error) {
	itemType := solcommoncodec.WrapItemType(forEncoding, b.namespace, b.genericName)

	return b.codec.CreateType(itemType, forEncoding)
}

func (b *eventReadBinding) Decode(ctx context.Context, bts []byte, outVal any) error {
	itemType := solcommoncodec.WrapItemType(false, b.namespace, b.genericName)

	return b.codec.Decode(ctx, bts, outVal, itemType)
}

func (b *eventReadBinding) GetLatestValue(ctx context.Context, params, returnVal any) error {
	itemType := solcommoncodec.WrapItemType(true, b.namespace, b.genericName)

	pubKey, err := b.GetAddress(ctx, nil)
	if err != nil {
		return err
	}

	offChain, err := b.normalizeParams(params, itemType)
	if err != nil {
		return err
	}

	subkeyFilters, err := b.extractFilterSubkeys(offChain)
	if err != nil {
		return err
	}

	allFilters := []query.Expression{
		logpoller.NewAddressFilter(pubKey),
		logpoller.NewEventSigFilter(b.eventSig),
	}

	if len(subkeyFilters) > 0 {
		allFilters = append(allFilters, query.And(subkeyFilters...))
	}

	limiter := query.NewLimitAndSort(query.CountLimit(1), query.NewSortBySequence(query.Desc))

	filter, err := logpoller.Where(allFilters...)
	if err != nil {
		return err
	}

	logs, err := b.reader.FilteredLogs(ctx, filter, limiter, b.namespace+"-"+pubKey.String()+"-"+b.genericName)
	if err != nil {
		return err
	}

	if len(logs) == 0 {
		return fmt.Errorf("%w: no events found", types.ErrNotFound)
	}

	return asValueDotValue(ctx, b, returnVal, b.wrapDecoderForValuer(&logs[0]))
}

func (b *eventReadBinding) QueryKey(
	ctx context.Context,
	filter query.KeyFilter,
	limitAndSort query.LimitAndSort,
	sequenceDataType any,
) ([]types.Sequence, error) {
	var (
		pubKey solana.PublicKey
		err    error
	)

	if pubKey, err = b.GetAddress(ctx, nil); err != nil {
		return nil, err
	}

	if filter, err = b.remapper.remap(filter); err != nil {
		return nil, err
	}

	// filter should always use the address and event sig
	filter.Expressions = append([]query.Expression{
		logpoller.NewAddressFilter(pubKey),
		logpoller.NewEventSigFilter(b.eventSig),
	}, filter.Expressions...)

	itemType := strings.Join([]string{b.namespace, b.genericName}, "-")

	logs, err := b.reader.FilteredLogs(ctx, filter.Expressions, limitAndSort, itemType)
	if err != nil {
		return nil, err
	}

	sequences, err := b.decodeLogsIntoSequences(ctx, logs, sequenceDataType)
	if err != nil {
		return nil, err
	}

	return sequences, nil
}

func (b *eventReadBinding) normalizeParams(value any, itemType string) (any, error) {
	offChain, err := b.codec.CreateType(itemType, true)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create type: %w", types.ErrInvalidType, err)
	}

	// params can be a singular primitive value, a map of values, or a struct
	// in the case that the input params are presented as a map of values, apply the values to the off-chain type
	// with solana hooks
	if err = solcommoncodec.MapstructureDecode(value, offChain); err != nil {
		return nil, fmt.Errorf("%w: failed to decode offChain value: %s", types.ErrInternal, err.Error())
	}

	return offChain, nil
}

func (b *eventReadBinding) extractFilterSubkeys(offChainParams any) ([]query.Expression, error) {
	var expressions []query.Expression

	for offChainKey, idx := range b.indexedSubKeys.lookup {
		itemType := solcommoncodec.WrapItemType(true, b.namespace, b.genericName+"."+offChainKey)

		fieldVal, err := commoncodec.ValueForPath(reflect.ValueOf(offChainParams), offChainKey)
		if err != nil {
			return nil, fmt.Errorf("%w: no value for path %s; err: %w", types.ErrInternal, b.genericName+"."+offChainKey, err)
		}

		onChainValue, err := b.modifier.TransformToOnChain(fieldVal, itemType)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to apply on-chain transformation for key %s", types.ErrInternal, offChainKey)
		}

		valOf := reflect.ValueOf(onChainValue)

		// check that onChainValue is not zero value for type
		if valOf.IsZero() {
			continue
		}

		expression, err := logpoller.NewEventBySubKeyFilter(
			idx,
			[]primitives.ValueComparator{{Value: reflect.Indirect(valOf).Interface(), Operator: primitives.Eq}},
		)
		if err != nil {
			return nil, err
		}

		expressions = append(expressions, expression)
	}

	return expressions, nil
}

func (b *eventReadBinding) remapPrimitive(expression query.Expression) (query.Expression, error) {
	var (
		comp query.Expression
		err  error
	)

	switch primitive := expression.Primitive.(type) {
	case *primitives.Comparator:
		if comp, err = b.encodeComparator(primitive); err != nil {
			return query.Expression{}, fmt.Errorf("failed to encode comparator %q: %w", primitive.Name, err)
		}
	case *primitives.Timestamp, *primitives.TxHash, *primitives.Block:
		// these seem to work without remapping
		return expression, nil
	case *primitives.Confidence:
		// confidence is ignored in solana
	default:
		return comp, fmt.Errorf("unsupported primitive type: %T", expression.Primitive)
	}

	return comp, nil
}

func (b *eventReadBinding) encodeComparator(comparator *primitives.Comparator) (query.Expression, error) {
	subKeyIndex, ok := b.indexedSubKeys.indexForKey(comparator.Name)
	if !ok {
		return query.Expression{}, fmt.Errorf("%w: unknown indexed subkey mapping %s", types.ErrInvalidConfig, comparator.Name)
	}

	itemType := solcommoncodec.WrapItemType(true, b.namespace, b.genericName+"."+comparator.Name)

	for idx, comp := range comparator.ValueComparators {
		// need to do a transform and then extract the value for the subkey
		newValue, err := b.modifier.TransformToOnChain(comp.Value, itemType)
		if err != nil {
			return query.Expression{}, err
		}

		comparator.ValueComparators[idx].Value = reflect.Indirect(reflect.ValueOf(newValue)).Interface()
	}

	return logpoller.NewEventBySubKeyFilter(subKeyIndex, comparator.ValueComparators)
}

func (b *eventReadBinding) decodeLogsIntoSequences(
	ctx context.Context,
	logs []logpollertypes.Log,
	into any,
) ([]types.Sequence, error) {
	sequences := make([]types.Sequence, len(logs))

	for idx := range logs {
		sequences[idx] = types.Sequence{
			Cursor: logpoller.FormatContractReaderCursor(logs[idx]),
			Head: types.Head{
				Height:    fmt.Sprint(logs[idx].BlockNumber),
				Hash:      solana.PublicKey(logs[idx].BlockHash).Bytes(),
				Timestamp: uint64(logs[idx].BlockTimestamp.Unix()), //nolint:gosec // BlockTimestamp can never be negative so it is safe to cast it to uint64
			},
		}

		var typeVal reflect.Value

		typeInto := reflect.TypeOf(into)
		if typeInto.Kind() == reflect.Pointer {
			typeVal = reflect.New(typeInto.Elem())
		} else {
			typeVal = reflect.Indirect(reflect.New(typeInto))
		}

		// create a new value of the same type as 'into' for the data to be extracted to
		sequences[idx].Data = typeVal.Interface()

		if err := b.decodeLog(ctx, &logs[idx], sequences[idx].Data); err != nil {
			return nil, err
		}
	}

	return sequences, nil
}

func (b *eventReadBinding) decodeLog(ctx context.Context, log *logpollertypes.Log, into any) error {
	itemType := solcommoncodec.WrapItemType(false, b.namespace, b.genericName)

	if err := b.codec.Decode(ctx, log.Data, into, itemType); err != nil {
		return fmt.Errorf("%w: failed to decode log data: %s", types.ErrInvalidType, err.Error())
	}

	return nil
}

func (b *eventReadBinding) isBound() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.bound
}

func (b *eventReadBinding) setBinding(binding solana.PublicKey) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.key = binding
	b.bound = true
}

func (b *eventReadBinding) unsetBinding() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.key = solana.PublicKey{}
	b.bound = false
}

func (b *eventReadBinding) wrapDecoderForValuer(log *logpollertypes.Log) func(context.Context, any) error {
	return func(ctx context.Context, returnVal any) error {
		return b.decodeLog(ctx, log, returnVal)
	}
}

type remapHelper struct {
	primitive func(query.Expression) (query.Expression, error)
}

func (r remapHelper) remap(filter query.KeyFilter) (query.KeyFilter, error) {
	remapped := query.KeyFilter{Key: filter.Key}

	for _, expression := range filter.Expressions {
		remappedExpression, err := r.remapExpression(filter.Key, expression)
		if err != nil {
			return query.KeyFilter{}, err
		}

		remapped.Expressions = append(remapped.Expressions, remappedExpression)
	}

	return remapped, nil
}

func (r remapHelper) remapExpression(key string, expression query.Expression) (query.Expression, error) {
	if !expression.IsPrimitive() {
		remappedBoolExpressions := make([]query.Expression, len(expression.BoolExpression.Expressions))
		for i := range expression.BoolExpression.Expressions {
			remapped, err := r.remapExpression(key, expression.BoolExpression.Expressions[i])
			if err != nil {
				return query.Expression{}, err
			}

			remappedBoolExpressions[i] = remapped
		}

		if expression.BoolExpression.BoolOperator == query.AND {
			return query.And(remappedBoolExpressions...), nil
		}

		return query.Or(remappedBoolExpressions...), nil
	}

	return r.primitive(expression)
}

type indexedSubkeys struct {
	lookup  map[string]uint64
	subKeys [4][]string
}

func newIndexedSubkeys() *indexedSubkeys {
	return &indexedSubkeys{
		lookup: make(map[string]uint64),
	}
}

func (k *indexedSubkeys) addForIndex(offChainPath, onChainPath string, idx uint64) {
	k.lookup[offChainPath] = idx
	k.subKeys[idx] = strings.Split(onChainPath, ".")
}

func (k *indexedSubkeys) indexForKey(key string) (uint64, bool) {
	idx, ok := k.lookup[key]

	return idx, ok
}

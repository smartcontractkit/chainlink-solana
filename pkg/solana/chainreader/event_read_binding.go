package chainreader

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
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
	eventSig               [logpoller.EventSignatureLength]byte
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
	if b.isBound() {
		// we are changing contract address reference, so we need to unregister old filter if it exists
		if err := b.Unregister(ctx); err != nil {
			return err
		}
	}

	// filter isn't required here because the event can also be polled for by the contractBinding common filter.
	if b.filter != nil {
		b.filter.SetName(fmt.Sprintf("%s.%s.%s", b.namespace, b.genericName, uuid.NewString()))
		b.filter.SetAddress(address)
	}

	b.setBinding(address)

	if b.registered() {
		return b.Register(ctx)
	}

	return nil
}

func (b *eventReadBinding) Unbind(ctx context.Context) error {
	if !b.isBound() {
		return nil
	}

	if b.filter != nil {
		b.filter.SetAddress(solana.PublicKey{})
		b.filter.SetName("")
	}

	b.unsetBinding()

	return b.Unregister(ctx)
}

func (b *eventReadBinding) Register(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.filter == nil {
		return nil
	}

	b.registerCalled = true

	// can't be true before filters params are set so there is no race with a bad filter outcome
	if !b.bound {
		return nil
	}

	return b.filter.Register(ctx, b.reader)
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

func (b *eventReadBinding) GetIDLInfo() (idl codec.IDL, inputIDLTypeDef interface{}, outputIDLTypeDef codec.IdlTypeDef) {
	return codec.IDL{}, codec.IdlTypeDef{}, codec.IdlTypeDef{}
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

func (b *eventReadBinding) SetFilter(filter logpoller.Filter) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.filter.SetFilter(filter)
	b.eventSig = filter.EventSig
}

func (b *eventReadBinding) CreateType(forEncoding bool) (any, error) {
	itemType := codec.WrapItemType(forEncoding, b.namespace, b.genericName)

	return b.codec.CreateType(itemType, forEncoding)
}

func (b *eventReadBinding) Decode(ctx context.Context, bts []byte, outVal any) error {
	itemType := codec.WrapItemType(false, b.namespace, b.genericName)

	return b.codec.Decode(ctx, bts, outVal, itemType)
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
		logpoller.NewEventSigFilter(b.eventSig[:]),
	}, filter.Expressions...)

	itemType := strings.Join([]string{b.namespace, b.genericName}, ".")

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
	case *primitives.Confidence:
		// confidence is ignored in solana
	}

	return comp, nil
}

func (b *eventReadBinding) encodeComparator(comparator *primitives.Comparator) (query.Expression, error) {
	subKeyIndex, ok := b.indexedSubKeys.indexForKey(comparator.Name)
	if !ok {
		return query.Expression{}, fmt.Errorf("%w: unknown indexed subkey mapping %s", types.ErrInvalidConfig, comparator.Name)
	}

	itemType := strings.Join([]string{b.namespace, b.genericName, comparator.Name}, ".")

	for idx, comp := range comparator.ValueComparators {
		// need to do a transform and then extract the value for the subkey
		newValue, err := b.modifier.TransformToOnChain(comp.Value, itemType)
		if err != nil {
			return query.Expression{}, err
		}

		comparator.ValueComparators[idx].Value = newValue
	}

	return logpoller.NewEventBySubKeyFilter(subKeyIndex, comparator.ValueComparators)
}

func (b *eventReadBinding) decodeLogsIntoSequences(
	ctx context.Context,
	logs []logpoller.Log,
	into any,
) ([]types.Sequence, error) {
	sequences := make([]types.Sequence, len(logs))

	for idx := range logs {
		sequences[idx] = types.Sequence{
			Cursor: logpoller.FormatContractReaderCursor(logs[idx]),
			Head: types.Head{
				Height:    fmt.Sprint(logs[idx].BlockNumber),
				Hash:      solana.PublicKey(logs[idx].BlockHash).Bytes(),
				Timestamp: uint64(logs[idx].BlockTimestamp.Unix()), //nolint:gosec
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

func (b *eventReadBinding) decodeLog(ctx context.Context, log *logpoller.Log, into any) error {
	itemType := codec.WrapItemType(false, b.namespace, b.genericName)

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

func (b *eventReadBinding) registered() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.registerCalled
}

type remapHelper struct {
	primitive func(query.Expression) (query.Expression, error)
}

func (r remapHelper) remap(filter query.KeyFilter) (query.KeyFilter, error) {
	var remapped query.KeyFilter

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

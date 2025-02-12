package chainreader

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/gagliardetto/solana-go"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query"
	"github.com/smartcontractkit/chainlink-common/pkg/types/query/primitives"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
)

type eventReadBinding struct {
	namespace, genericName string
	codec                  types.RemoteCodec
	modifier               commoncodec.Modifier
	key                    solana.PublicKey
	remapper               remapHelper
	indexedSubKeys         map[string]uint64
	reader                 EventsReader
	eventSig               [logpoller.EventSignatureLength]byte
	readDefinition         config.ReadDefinition
}

func newEventReadBinding(
	namespace, genericName string,
	indexedSubKeys map[string]uint64,
	reader EventsReader,
	eventSig [logpoller.EventSignatureLength]byte,
	readDefinition config.ReadDefinition,
) *eventReadBinding {
	binding := &eventReadBinding{
		namespace:      namespace,
		genericName:    genericName,
		indexedSubKeys: indexedSubKeys,
		reader:         reader,
		eventSig:       eventSig,
		readDefinition: readDefinition,
	}

	binding.remapper = remapHelper{binding.remapPrimitive}

	return binding
}

func (b *eventReadBinding) GetAddress(_ context.Context, _ any) (solana.PublicKey, error) {
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

func (b *eventReadBinding) SetAddress(key solana.PublicKey) {
	b.key = key
}

func (b *eventReadBinding) SetCodec(codec types.RemoteCodec) {
	b.codec = codec
}

func (b *eventReadBinding) SetModifier(modifier commoncodec.Modifier) {
	b.modifier = modifier
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
	subKeyIndex, ok := b.indexedSubKeys[comparator.Name]
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

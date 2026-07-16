package fakes

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"strings"

	solcap "github.com/smartcontractkit/chainlink-common/pkg/capabilities/v2/chain-capabilities/solana"
	codecbinary "github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"

	codecv2 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v2"
)

// subkeyFieldMatches reports whether log satisfies every SubkeyConfig in the
// filter. Each SubkeyConfig decodes one field (by Path) from the Anchor event
// and matches if ANY of its Comparers evaluates true (OR within a field,
// AND across fields) — the same convention used for EVM topic slots.
func subkeyFieldMatches(log *solcap.Log, filter *solcap.FilterLogTriggerRequest) error {
	subkeys := filter.GetSubkeys()
	if len(subkeys) == 0 {
		return nil
	}

	fields, err := decodeAnchorEventFields(filter.GetContractIdlJson(), filter.GetEventName(), log.GetData())
	if err != nil {
		return fmt.Errorf("failed to decode event for subkey filtering: %w", err)
	}

	for _, sk := range subkeys {
		if len(sk.GetPath()) != 1 {
			return fmt.Errorf("subkey path %v: only single-level (top-level scalar field) paths are supported in cre-cli simulate", sk.GetPath())
		}
		fieldName := sk.GetPath()[0]
		val, ok := fields[normalizeFieldName(fieldName)]
		if !ok {
			return fmt.Errorf("subkey path %q: no such top-level field in event %q", fieldName, filter.GetEventName())
		}
		encoded, err := encodeIndexedValue(val)
		if err != nil {
			return fmt.Errorf("subkey path %q: %w", fieldName, err)
		}

		matched := false
		for _, cmp := range sk.GetComparers() {
			ok, err := evaluateComparator(encoded, cmp.GetValue(), cmp.GetOperator())
			if err != nil {
				return fmt.Errorf("subkey path %q: %w", fieldName, err)
			}
			if ok {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("subkey path %q: value does not satisfy any comparer", fieldName)
		}
	}
	return nil
}

// normalizeFieldName strips underscores and lowercases, so IDL snake_case
// field names ("u64_value") and codegen'd Go PascalCase subkey paths
// ("U64Value") compare equal without needing exact case-conversion rules.
func normalizeFieldName(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "_", ""))
}

// decodeAnchorEventFields decodes an Anchor event's top-level fields using
// chainlink-solana's own Anchor IDL codec (codecv2, the same decoder the
// Solana log poller uses to decode events off-chain), keyed by
// normalizeFieldName(field name). Nested structs, vecs, and arrays decode
// successfully but aren't exposed here — only scalar top-level fields are
// usable as subkey paths in cre-cli simulate.
func decodeAnchorEventFields(idlJSON []byte, eventName string, data []byte) (map[string]any, error) {
	if len(idlJSON) == 0 {
		return nil, fmt.Errorf("filter has no contract IDL JSON")
	}
	if eventName == "" {
		return nil, fmt.Errorf("filter has no event name")
	}

	entry, err := codecv2.NewEventArgsEntryWrapper(eventName, string(idlJSON), true, nil, codecbinary.LittleEndian())
	if err != nil {
		return nil, fmt.Errorf("failed to build event codec from contract IDL: %w", err)
	}
	decoded, _, err := entry.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode event data: %w", err)
	}

	rv := reflect.ValueOf(decoded)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil, fmt.Errorf("decoded event %q is nil", eventName)
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("decoded event %q has unexpected type %s", eventName, rv.Type())
	}

	out := make(map[string]any, rv.NumField())
	for i := 0; i < rv.NumField(); i++ {
		fv := rv.Field(i)
		for fv.Kind() == reflect.Ptr {
			if fv.IsNil() {
				break
			}
			fv = fv.Elem()
		}
		if fv.Kind() == reflect.Ptr {
			continue // unset Option field
		}
		out[normalizeFieldName(rv.Type().Field(i).Name)] = fv.Interface()
	}
	return out, nil
}

// encodeIndexedValue mirrors cre-sdk-go's bindings.EncodeIndexedValue byte for
// byte, so a decoded event field compares equal to a workflow-side
// PrepareSubkeyValue-encoded filter value.
func encodeIndexedValue(value any) ([]byte, error) {
	switch v := value.(type) {
	case bool:
		if v {
			return []byte{1}, nil
		}
		return []byte{0}, nil
	case [32]byte:
		return v[:], nil
	case string:
		return []byte(v), nil
	case uint8:
		return encodeUint(uint64(v)), nil
	case uint16:
		return encodeUint(uint64(v)), nil
	case uint32:
		return encodeUint(uint64(v)), nil
	case uint64:
		return encodeUint(v), nil
	case int8:
		return encodeInt(int64(v)), nil
	case int16:
		return encodeInt(int64(v)), nil
	case int32:
		return encodeInt(int64(v)), nil
	case int64:
		return encodeInt(v), nil
	default:
		return nil, fmt.Errorf("unsupported decoded value type %T", value)
	}
}

func encodeUint(v uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, v)
	return buf
}

func encodeInt(v int64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(v)) //nolint:gosec // two's complement encoding, matches bindings.EncodeIndexedValue
	return buf
}

// evaluateComparator compares a decoded, encoded field value against a
// registered comparer value. EQ/NEQ are always exact; ordering operators use
// unsigned big-endian byte comparison, which is exact for unsigned integer
// fields (the common case) but not sign-corrected for negative signed values —
// matching the encoding scheme's own documented scope.
func evaluateComparator(value, want []byte, op solcap.ComparisonOperator) (bool, error) {
	cmp := bytes.Compare(value, want)
	switch op {
	case solcap.ComparisonOperator_COMPARISON_OPERATOR_EQ:
		return cmp == 0, nil
	case solcap.ComparisonOperator_COMPARISON_OPERATOR_NEQ:
		return cmp != 0, nil
	case solcap.ComparisonOperator_COMPARISON_OPERATOR_GT:
		return cmp > 0, nil
	case solcap.ComparisonOperator_COMPARISON_OPERATOR_LT:
		return cmp < 0, nil
	case solcap.ComparisonOperator_COMPARISON_OPERATOR_GTE:
		return cmp >= 0, nil
	case solcap.ComparisonOperator_COMPARISON_OPERATOR_LTE:
		return cmp <= 0, nil
	default:
		return false, fmt.Errorf("unsupported comparison operator %v", op)
	}
}

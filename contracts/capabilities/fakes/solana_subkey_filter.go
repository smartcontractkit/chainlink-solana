package fakes

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	solcap "github.com/smartcontractkit/chainlink-common/pkg/capabilities/v2/chain-capabilities/solana"
	codecbinary "github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"

	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv2 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v2"
)

// subkeyFieldMatches reports whether log satisfies every SubkeyConfig in the
// filter.
func subkeyFieldMatches(log *solcap.Log, filter *solcap.FilterLogTriggerRequest) error {
	subkeys := filter.GetSubkeys()
	if len(subkeys) == 0 {
		return nil
	}

	decoded, err := decodeAnchorEvent(filter.GetContractIdlJson(), filter.GetEventName(), log.GetData())
	if err != nil {
		return fmt.Errorf("failed to decode event for subkey filtering: %w", err)
	}

	for _, sk := range subkeys {
		if len(sk.GetPath()) != 1 {
			return fmt.Errorf("subkey path %v: only single-level (top-level scalar field) paths are supported in cre-cli simulate", sk.GetPath())
		}
		fieldName, ok := resolveTopLevelFieldName(decoded, sk.GetPath()[0])
		if !ok {
			return fmt.Errorf("subkey path %q: no such top-level field in event %q", sk.GetPath()[0], filter.GetEventName())
		}

		val, err := solcommoncodec.ExtractField(decoded, []string{fieldName})
		if err != nil {
			return fmt.Errorf("subkey path %q: %w", fieldName, err)
		}
		encoded, err := solcommoncodec.NewIndexedValue(val)
		if err != nil {
			return fmt.Errorf("subkey path %q: %w", fieldName, err)
		}

		for _, cmp := range sk.GetComparers() {
			ok, err := evaluateComparator(encoded, cmp.GetValue(), cmp.GetOperator())
			if err != nil {
				return fmt.Errorf("subkey path %q: %w", fieldName, err)
			}
			if !ok {
				return fmt.Errorf("subkey path %q: value does not satisfy comparer %v %s", fieldName, cmp.GetOperator(), cmp.GetValue())
			}
		}
	}
	return nil
}

func normalizeFieldName(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "_", ""))
}

func resolveTopLevelFieldName(decoded any, target string) (string, bool) {
	rv := reflect.ValueOf(decoded)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return "", false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return "", false
	}

	want := normalizeFieldName(target)
	for i := 0; i < rv.NumField(); i++ {
		if name := rv.Type().Field(i).Name; normalizeFieldName(name) == want {
			return name, true
		}
	}
	return "", false
}

func decodeAnchorEvent(idlJSON []byte, eventName string, data []byte) (any, error) {
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
	return decoded, nil
}

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

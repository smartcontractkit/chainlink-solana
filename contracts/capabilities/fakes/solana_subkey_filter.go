package fakes

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"

	gbinary "github.com/gagliardetto/binary"

	solcap "github.com/smartcontractkit/chainlink-common/pkg/capabilities/v2/chain-capabilities/solana"
)

// anchorDiscriminatorLen is the size of the discriminator Anchor prepends to
// event log data (sha256("event:<Name>")[:8]).
const anchorDiscriminatorLen = 8

// idlField/idlTypeDef/idlDoc mirror just enough of the Anchor IDL JSON shape
// to look up an event's field layout by name.
type idlField struct {
	Name string          `json:"name"`
	Type json.RawMessage `json:"type"`
}

type idlTypeDef struct {
	Name string `json:"name"`
	Type struct {
		Kind   string     `json:"kind"`
		Fields []idlField `json:"fields"`
	} `json:"type"`
}

type idlDoc struct {
	Types []idlTypeDef `json:"types"`
}

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
// field names ("u64_value") match codegen'd Go PascalCase subkey paths
// ("U64Value") without needing exact case-conversion rules.
func normalizeFieldName(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "_", ""))
}

// decodeAnchorEventFields Borsh-decodes an Anchor event's top-level scalar
// fields (bool, signed/unsigned integers up to 64 bits, string, publicKey)
// keyed by normalizeFieldName(field name). Nested structs, vecs, arrays,
// u128/i128 are not supported — decoding stops with an error if one is
// encountered before all requested-scope fields are read.
func decodeAnchorEventFields(idlJSON []byte, eventName string, data []byte) (map[string]any, error) {
	if len(idlJSON) == 0 {
		return nil, fmt.Errorf("filter has no contract IDL JSON")
	}
	if eventName == "" {
		return nil, fmt.Errorf("filter has no event name")
	}
	if len(data) < anchorDiscriminatorLen {
		return nil, fmt.Errorf("event data too short: expected at least %d bytes, got %d", anchorDiscriminatorLen, len(data))
	}

	var doc idlDoc
	if err := json.Unmarshal(idlJSON, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse contract IDL JSON: %w", err)
	}
	var target *idlTypeDef
	for i := range doc.Types {
		if doc.Types[i].Name == eventName {
			target = &doc.Types[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("event %q not found in contract IDL types", eventName)
	}
	if target.Type.Kind != "struct" {
		return nil, fmt.Errorf("event %q is not a struct type", eventName)
	}

	dec := gbinary.NewBorshDecoder(data[anchorDiscriminatorLen:])
	out := make(map[string]any, len(target.Type.Fields))
	for _, f := range target.Type.Fields {
		var typeName string
		if err := json.Unmarshal(f.Type, &typeName); err != nil {
			return nil, fmt.Errorf("field %q: nested/complex types are not supported in cre-cli simulate", f.Name)
		}
		val, err := decodeScalarField(dec, typeName)
		if err != nil {
			return nil, fmt.Errorf("field %q (%s): %w", f.Name, typeName, err)
		}
		out[normalizeFieldName(f.Name)] = val
	}
	return out, nil
}

func decodeScalarField(dec *gbinary.Decoder, typeName string) (any, error) {
	switch typeName {
	case "bool":
		var v bool
		return v, dec.Decode(&v)
	case "u8":
		var v uint8
		return v, dec.Decode(&v)
	case "u16":
		var v uint16
		return v, dec.Decode(&v)
	case "u32":
		var v uint32
		return v, dec.Decode(&v)
	case "u64":
		var v uint64
		return v, dec.Decode(&v)
	case "i8":
		var v int8
		return v, dec.Decode(&v)
	case "i16":
		var v int16
		return v, dec.Decode(&v)
	case "i32":
		var v int32
		return v, dec.Decode(&v)
	case "i64":
		var v int64
		return v, dec.Decode(&v)
	case "string":
		var v string
		return v, dec.Decode(&v)
	case "publicKey", "pubkey":
		var v [32]byte
		return v, dec.Decode(&v)
	default:
		return nil, fmt.Errorf("unsupported IDL scalar type %q", typeName)
	}
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

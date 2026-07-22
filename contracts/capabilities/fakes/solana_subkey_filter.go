package fakes

import (
	"bytes"
	"fmt"

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
		val, err := solcommoncodec.ExtractField(decoded, sk.GetPath())
		if err != nil {
			return fmt.Errorf("subkey path %v: %w", sk.GetPath(), err)
		}
		encoded, err := solcommoncodec.NewIndexedValue(val)
		if err != nil {
			return fmt.Errorf("subkey path %v: %w", sk.GetPath(), err)
		}

		for _, cmp := range sk.GetComparers() {
			ok, err := evaluateComparator(encoded, cmp.GetValue(), cmp.GetOperator())
			if err != nil {
				return fmt.Errorf("subkey path %v: %w", sk.GetPath(), err)
			}
			if !ok {
				return fmt.Errorf("subkey path %v: value does not satisfy comparer %v %s", sk.GetPath(), cmp.GetOperator(), cmp.GetValue())
			}
		}
	}
	return nil
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

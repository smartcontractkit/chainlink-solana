package utils

import (
	"fmt"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings/binary"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codecv2"
	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/commoncodec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

// NewEventCodecEntry creates a codec entry for decoding event data from a filter.
// It tries codecv2 (Anchor 0.30+) first, then falls back to codec (legacy Anchor).
// If ContractIdl is empty but EventIdl is provided, it uses the legacy EventIdl approach.
func NewEventCodecEntry(filter types.Filter) (solcommoncodec.Entry, error) {
	if filter.ContractIdl != "" {
		// Try codecv2 first (Anchor 0.30+ IDL format)
		entry, err := codecv2.NewEventArgsEntryWrapper(filter.EventName, filter.ContractIdl, true, nil, binary.LittleEndian())
		if err == nil {
			return entry, nil
		}
		// Fall back to codec (legacy Anchor IDL format)
		entry, err = codec.NewEventArgsEntryWrapper(filter.EventName, filter.ContractIdl, true, nil, binary.LittleEndian())
		if err != nil {
			return nil, fmt.Errorf("failed to create event codec entry: %w", err)
		}
		return entry, nil
	}

	// Backward compatibility: use legacy EventIdl if ContractIdl is empty
	entry, err := codec.NewEventArgsEntry(filter.EventName, codec.EventIDLTypes(filter.EventIdl), true, nil, binary.LittleEndian())
	if err != nil {
		return nil, fmt.Errorf("failed to create event codec entry from EventIdl: %w", err)
	}

	return entry, nil
}

func NewCreateCodecEntryWrapper(cfgType solcommoncodec.ChainConfigType, mod commoncodec.Modifier, onChainName, offChainName, idlString string) (entry solcommoncodec.Entry, err error) {
	input, err := codecv2.CreateCodecEntryWrapper(cfgType, mod, onChainName, offChainName, idlString)
	if err == nil {
		return input, nil
	}
	input, err = codec.CreateCodecEntryWrapper(cfgType, mod, onChainName, offChainName, idlString)
	if err != nil {
		return nil, fmt.Errorf("failed to create codec entry for %s, error: %w", offChainName, err)
	}
	return input, nil
}

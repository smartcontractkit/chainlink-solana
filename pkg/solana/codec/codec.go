//nolint:revive // utils is an established package name in this codebase
package codec

import (
	"fmt"

	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	codecv2 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v2"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

// NewEventCodecEntry creates a codec entry for decoding event data from a filter.
// It tries codecv2 (Anchor 0.30+) first, then falls back to codec (legacy Anchor).
// If ContractIdl is empty but EventIdl is provided, it uses the legacy EventIdl approach.
func NewEventCodecEntry(filter types.Filter, includeDiscriminator bool, mod commoncodec.Modifier, builder encodings.Builder) (solcommoncodec.Entry, error) {
	if filter.ContractIdl != "" {
		// Try codecv2 first (Anchor 0.30+ IDL format)
		entry, err := codecv2.NewEventArgsEntryWrapper(filter.EventName, filter.ContractIdl, includeDiscriminator, mod, builder)
		if err == nil {
			return entry, nil
		}
		// Fall back to codec (legacy Anchor IDL format)
		entry, err = codecv1.NewEventArgsEntryWrapper(filter.EventName, filter.ContractIdl, includeDiscriminator, mod, builder)
		if err != nil {
			return nil, fmt.Errorf("failed to create event codec entry: %w", err)
		}
		return entry, nil
	}

	// Backward compatibility: use legacy EventIdl if ContractIdl is empty
	//nolint:staticcheck // intentional use of deprecated EventIdl for backward compatibility
	entry, err := codecv1.NewEventArgsEntry(filter.EventName, codecv1.EventIDLTypes(filter.EventIdl), includeDiscriminator, mod, builder)
	if err != nil {
		return nil, fmt.Errorf("failed to create event codec entry from EventIdl: %w", err)
	}

	return entry, nil
}

// NewCreateCodecEntry creates a codec entry based on config type
// It tries codecv2 (Anchor 0.30+) first, then falls back to codec (legacy Anchor).
func NewCreateCodecEntry(cfgType solcommoncodec.ChainConfigType, mod commoncodec.Modifier, onChainName, offChainName, idlString string) (entry solcommoncodec.Entry, err error) {
	input, err := codecv2.CreateCodecEntryWrapper(cfgType, mod, onChainName, offChainName, idlString)
	if err == nil {
		return input, nil
	}
	input, err = codecv1.CreateCodecEntryWrapper(cfgType, mod, onChainName, offChainName, idlString)
	if err != nil {
		return nil, fmt.Errorf("failed to create codec entry for %s, error: %w", offChainName, err)
	}
	return input, nil
}

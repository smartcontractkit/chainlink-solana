//nolint:revive // utils is an established package name in this codebase
package codec

import (
	"fmt"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	solcommoncodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/common"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
	codecv2 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v2"
)

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

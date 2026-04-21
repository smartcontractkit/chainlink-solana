package types //nolint:revive // package name matches existing convention

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscriminatorStability(t *testing.T) {
	t.Run("AnchorCPIEventDiscriminator matches Anchor EVENT_IX_TAG_LE", func(t *testing.T) {
		expected := EventSignature{0xe4, 0x45, 0xa5, 0x2e, 0x51, 0xcb, 0x9a, 0x1d}
		require.Equal(t, expected, AnchorCPIEventDiscriminator())
	})

	t.Run("cpiEvent method signature matches CCIP RMN Remote Instruction_CpiEvent", func(t *testing.T) {
		expected := EventSignature{0xbc, 0xd8, 0xa6, 0x6c, 0x1a, 0xa6, 0x8e, 0xb6}
		require.Equal(t, expected, NewMethodSignatureFromName("cpiEvent"))
	})

	t.Run("AnchorCPIEventDiscriminator differs from cpiEvent method signature", func(t *testing.T) {
		require.NotEqual(t, AnchorCPIEventDiscriminator(), NewMethodSignatureFromName("cpiEvent"))
	})
}

package types //nolint:revive // package name matches existing convention

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
)

func TestMatchSameLogs(t *testing.T) {
	baseFilter := Filter{
		Address:     PublicKey{1},
		EventSig:    EventSignature{2},
		EventName:   "Transfer",
		SubkeyPaths: SubKeyPaths{{"from"}},
		ContractIdl: `{"version":"0.1.0"}`,
		EventIdl: EventIdl{
			Event: codecv1.IdlEvent{Name: "Transfer"},
		},
	}

	t.Run("identical filters match", func(t *testing.T) {
		assert.True(t, baseFilter.MatchSameLogs(baseFilter))
	})

	t.Run("empty ContractIdl does not match populated ContractIdl", func(t *testing.T) {
		empty := baseFilter
		empty.ContractIdl = ""
		assert.False(t, baseFilter.MatchSameLogs(empty))
		assert.False(t, empty.MatchSameLogs(baseFilter))
	})

	t.Run("both empty ContractIdl match", func(t *testing.T) {
		a := baseFilter
		a.ContractIdl = ""
		b := baseFilter
		b.ContractIdl = ""
		assert.True(t, a.MatchSameLogs(b))
	})

	t.Run("different ContractIdl do not match", func(t *testing.T) {
		other := baseFilter
		other.ContractIdl = `{"version":"0.2.0"}`
		assert.False(t, baseFilter.MatchSameLogs(other))
	})

	t.Run("empty EventIdl does not match populated EventIdl", func(t *testing.T) {
		empty := baseFilter
		empty.EventIdl = EventIdl{}
		assert.False(t, baseFilter.MatchSameLogs(empty))
		assert.False(t, empty.MatchSameLogs(baseFilter))
	})

	t.Run("both empty EventIdl match", func(t *testing.T) {
		a := baseFilter
		a.EventIdl = EventIdl{}
		b := baseFilter
		b.EventIdl = EventIdl{}
		assert.True(t, a.MatchSameLogs(b))
	})

	t.Run("different EventIdl do not match", func(t *testing.T) {
		other := baseFilter
		other.EventIdl = EventIdl{
			Event: codecv1.IdlEvent{Name: "Approval"},
		}
		assert.False(t, baseFilter.MatchSameLogs(other))
	})

	t.Run("different Address does not match", func(t *testing.T) {
		other := baseFilter
		other.Address = PublicKey{99}
		assert.False(t, baseFilter.MatchSameLogs(other))
	})

	t.Run("different EventSig does not match", func(t *testing.T) {
		other := baseFilter
		other.EventSig = EventSignature{99}
		assert.False(t, baseFilter.MatchSameLogs(other))
	})

	t.Run("different EventName does not match", func(t *testing.T) {
		other := baseFilter
		other.EventName = "Approval"
		assert.False(t, baseFilter.MatchSameLogs(other))
	})

	t.Run("different SubkeyPaths does not match", func(t *testing.T) {
		other := baseFilter
		other.SubkeyPaths = SubKeyPaths{{"to"}}
		assert.False(t, baseFilter.MatchSameLogs(other))
	})

	t.Run("non-matching fields ignored by MatchSameLogs", func(t *testing.T) {
		other := baseFilter
		other.StartingBlock = 999
		other.Retention = 60
		other.MaxLogsKept = 100
		other.IncludeReverted = true
		assert.True(t, baseFilter.MatchSameLogs(other))
	})
}

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

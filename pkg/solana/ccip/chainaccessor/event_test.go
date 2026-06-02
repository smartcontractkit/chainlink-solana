package chainaccessor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

func Test_deriveName(t *testing.T) {
	const legacyFalseName = "MyEvent.11111111111111111111111111111111.c6962b9b24636b9e8a6ab96d9cb3934e03905cd657cb872452720c7338420f69"

	f1 := types.Filter{
		EventName:       "MyEvent",
		SubkeyPaths:     [][]string{{"a"}, {"b"}, {"c"}},
		IncludeReverted: false,
	}
	name1, err := deriveName(f1)
	require.NoError(t, err)
	require.Equal(t, legacyFalseName, name1)

	f2 := f1
	f2.SubkeyPaths = [][]string{{"a"}, {"b", "c"}}
	name2, err2 := deriveName(f2)
	require.NoError(t, err2)
	require.NotEqual(t, name1, name2)

	f3 := f1
	f3.IncludeReverted = true
	name3, err3 := deriveName(f3)
	require.NoError(t, err3)
	require.NotEqual(t, name1, name3)
}

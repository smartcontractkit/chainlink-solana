package chainaccessor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

func Test_deriveName(t *testing.T) {
	var f1 types.Filter
	f1.SubkeyPaths = [][]string{{"a"}, {"b"}, {"c"}}
	name1, err := deriveName(f1)
	require.NoError(t, err)
	var f2 types.Filter
	f1.SubkeyPaths = [][]string{{"a"}, {"b", "c"}}
	name2, err2 := deriveName(f2)
	require.NoError(t, err2)
	require.NotEqual(t, name1, name2)
}

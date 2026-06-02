package chainaccessor

import (
	"crypto/sha3"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

func Test_deriveName(t *testing.T) {
	f1 := types.Filter{
		EventName:       "MyEvent",
		SubkeyPaths:     [][]string{{"a"}, {"b"}, {"c"}},
		IncludeReverted: false,
	}
	name1, err := deriveName(f1)
	require.NoError(t, err)
	require.Equal(t, legacyDerivedNameForTest(t, f1), name1)

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

func legacyDerivedNameForTest(t *testing.T, filter types.Filter) string {
	t.Helper()

	data := filter.EventSig[:]
	data = append(data, filter.Address.ToSolana().Bytes()...)
	data = append(data, []byte(filter.EventName)...)

	if len(filter.SubkeyPaths) > 0 {
		b, err := json.Marshal(filter.SubkeyPaths)
		require.NoError(t, err)
		data = append(data, b...)
	}

	if filter.IsCPIFilter() {
		data = append(data, filter.ExtraFilterConfig.DestProgram[:]...)
		data = append(data, filter.ExtraFilterConfig.MethodSignature[:]...)
	}

	hash := sha3.Sum256(data)
	if filter.IsCPIFilter() {
		return fmt.Sprintf("cpi.%s.%s.%x", filter.EventName, filter.Address.String(), hash[:])
	}

	return fmt.Sprintf("%s.%s.%x", filter.EventName, filter.Address.String(), hash[:])
}

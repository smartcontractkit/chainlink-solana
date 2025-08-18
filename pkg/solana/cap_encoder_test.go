package solana

import (
	"math/big"
	"testing"

	"github.com/smartcontractkit/chainlink-common/pkg/values"
	"github.com/stretchr/testify/require"
)

func Test_capEncoder(t *testing.T) {
	enc, err := NewEncoder(nil)
	require.NoError(t, err, "failed to create encoder")

	m := map[string]any{
		"Reports": []any{
			map[string]any{
				"Timestamp": uint32(10),
				"Answer":    big.NewInt(10),
			},
		},
	}

	in, err := values.NewMap(m)
	require.NoError(t, err, "failed to create in map")

	_, err = enc.Encode(t.Context(), *in)
	require.NoError(t, err, "failed to encode payload")
}

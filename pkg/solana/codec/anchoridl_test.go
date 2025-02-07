package codec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana"
)

func ensureUnmarshal[T any](t *testing.T, originalStrIDL string) {
	var firstReadIDL T
	require.NoError(t, json.Unmarshal([]byte(originalStrIDL), &firstReadIDL))
	marshaledIDL, err := json.Marshal(firstReadIDL)
	require.NoError(t, err)
	var secondReadIDL T
	require.NoError(t, json.Unmarshal(marshaledIDL, &secondReadIDL))
	require.Equal(t, firstReadIDL, secondReadIDL)
}

func TestIDLTypes_JSONMarshalUnmarshal(t *testing.T) {
	t.Run("Array IDL Filed", func(t *testing.T) {
		idl := `{ "name": "OracleIds", "type": { "array": ["u8", 32] } }`
		ensureUnmarshal[IdlField](t, idl)
	})
	t.Run("CCIP IDL", func(t *testing.T) {
		ensureUnmarshal[IDL](t, solana.FetchCCIPRouterIDL())
	})
}

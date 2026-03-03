package codecv1

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
	t.Run("Array IDL Field", func(t *testing.T) {
		idl := `{ "name": "OracleIds", "type": { "array": ["u8", 32] } }`
		ensureUnmarshal[IdlField](t, idl)
	})
	t.Run("CCIP Offramp IDL", func(t *testing.T) {
		ensureUnmarshal[IDL](t, solana.FetchCCIPOfframpIDL())
	})
	t.Run("CCIP FeeQuoter IDL", func(t *testing.T) {
		ensureUnmarshal[IDL](t, solana.FetchFeeQuoterIDL())
	})
	t.Run("CCIP RMNRemote IDL", func(t *testing.T) {
		ensureUnmarshal[IDL](t, solana.FetchRMNRemoteIDL())
	})
	t.Run("Invalid JSON: multiple IDLTypes provided", func(t *testing.T) {
		idl := `{ "array": ["u8", 32], "vec": {"string": "test"}}`
		var readIdlType IdlType
		require.ErrorContains(t, json.Unmarshal([]byte(idl), &readIdlType), "multiple types found for IdlType:")
	})
}

// When both accounts and isMut are provided
func TestIDLAccountItem_Invalid(t *testing.T) {
	raw := `{
			"isMut": true,
			"accounts": {
				"name": "myAccounts",
				"accounts": [{ "name": "subAccount" }]
			}
		}`
	var item IdlAccountItem
	err := json.Unmarshal([]byte(raw), &item)
	require.Error(t, err)
}

func TestIDLAccountItem_Circular(t *testing.T) {
	t.Run("Circular Dependency", func(t *testing.T) {
		// Parent structure
		root := IdlAccountItem{
			IdlAccounts: &IdlAccounts{
				Name:     "RootAccount",
				Docs:     []string{"This is the root account"},
				Accounts: []IdlAccountItem{},
			},
		}

		// Child structure
		child := IdlAccountItem{
			IdlAccounts: &IdlAccounts{
				Name:     "ChildAccount",
				Docs:     []string{"This is a child account"},
				Accounts: []IdlAccountItem{},
			},
		}

		_, err := root.MarshalJSON()
		require.NoError(t, err)

		// Child points back to root
		child.IdlAccounts.Accounts = append(child.IdlAccounts.Accounts, root)
		// Root points to child
		root.IdlAccounts.Accounts = append(root.IdlAccounts.Accounts, child)

		_, err = root.MarshalJSON()
		require.Error(t, err)
	})
}

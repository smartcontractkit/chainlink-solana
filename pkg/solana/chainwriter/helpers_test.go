package chainwriter_test

import (
	"bytes"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_offramp"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
	"github.com/stretchr/testify/require"
)

func TestToSnakeCase(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"testCamelCase", "test_camel_case"},
		{"oneword", "oneword"},
		{"", ""},
		{"testCamelCaseWithCAPS", "test_camel_case_with_caps"},
		{"testCamelCaseWithCAPSAndNumbers123", "test_camel_case_with_caps_and_numbers123"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			actual := chainwriter.ToSnakeCase(tc.input)
			if actual != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, actual)
			}
		})
	}
}

func Test_GetValuesAtLocation(t *testing.T) {
	type ParentStruct struct {
		ExtraArgs []byte
	}

	t.Run("decodes encoded structs found in path", func(t *testing.T) {
		extraArgs := ccip_offramp.Any2SVMRampExtraArgs{IsWritableBitmap: 0}
		buf := new(bytes.Buffer)
		err := bin.NewBorshEncoder(buf).Encode(extraArgs)
		require.NoError(t, err)
		parentStruct := ParentStruct{
			ExtraArgs: buf.Bytes(),
		}

		results, err := chainwriter.GetValuesAtLocation(parentStruct, "ExtraArgs.IsWritableBitmap")
		require.NoError(t, err)
		require.Len(t, results, 1)
	})
}

package chainwriterutils_test

import (
	"testing"

	chainwriterutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/chain_writer_utils"
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
			actual := chainwriterutils.ToSnakeCase(tc.input)
			if actual != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, actual)
			}
		})
	}
}

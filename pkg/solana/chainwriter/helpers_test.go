package chainwriter_test

import (
	"testing"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/chainwriter"
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

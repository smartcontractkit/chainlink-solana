package logpoller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexedValue(t *testing.T) {
	cases := []struct {
		typeName string
		lower    any
		higher   any
	}{
		{"int32", int32(-5), int32(5)},
		{"int32", int32(-8), int32(-5)},
		{"int32", int32(5), int32(8)},
		{"int64", int64(-5), int64(5)},
		{"int64", int64(-8), int64(-5)},
		{"int64", int64(5), int64(8)},
		{"float32", float32(-5), float32(5)},
		{"float32", float32(-8), float32(-5)},
		{"float32", float32(5), float32(8)},
		{"float64", float64(-5), float64(5)},
		{"float64", float64(-8), float64(-5)},
		{"float64", float64(5), float64(8)},
		{"string", "abcc", "abcd"},
		{"string", "abcd", "abcdef"},
		{"[]byte", []byte("abcc"), []byte("abcd")},
		{"[]byte", []byte("abcd"), []byte("abcdef")},
	}
	for _, c := range cases {
		t.Run(c.typeName, func(t *testing.T) {
			iVal1, err := NewIndexedValue(c.lower)
			require.NoError(t, err)
			iVal2, err := NewIndexedValue(c.higher)
			require.NoError(t, err)
			assert.Less(t, iVal1, iVal2)
		})
	}
}

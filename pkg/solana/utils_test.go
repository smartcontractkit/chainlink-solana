package solana

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLamportsToSol(t *testing.T) {
	tests := []struct {
		name string
		in   uint64
		out  float64
	}{
		{"happypath", 1234567890, 1.23456789},
		{"zero", 0, 0},
		{"maxUint64", ^uint64(0), 18446744073.709551615},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.out, LamportsToSol(test.in))
		})
	}
}

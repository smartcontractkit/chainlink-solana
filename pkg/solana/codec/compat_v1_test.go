package codec_test

import (
	"testing"

	solanacodec "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
)

func TestCompatV1IdlTypeConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  codecv1.IdlTypeAsString
		want codecv1.IdlTypeAsString
	}{
		{name: "IdlTypeBool", got: solanacodec.IdlTypeBool, want: codecv1.IdlTypeBool},
		{name: "IdlTypeU8", got: solanacodec.IdlTypeU8, want: codecv1.IdlTypeU8},
		{name: "IdlTypeI8", got: solanacodec.IdlTypeI8, want: codecv1.IdlTypeI8},
		{name: "IdlTypeU16", got: solanacodec.IdlTypeU16, want: codecv1.IdlTypeU16},
		{name: "IdlTypeI16", got: solanacodec.IdlTypeI16, want: codecv1.IdlTypeI16},
		{name: "IdlTypeU32", got: solanacodec.IdlTypeU32, want: codecv1.IdlTypeU32},
		{name: "IdlTypeI32", got: solanacodec.IdlTypeI32, want: codecv1.IdlTypeI32},
		{name: "IdlTypeU64", got: solanacodec.IdlTypeU64, want: codecv1.IdlTypeU64},
		{name: "IdlTypeI64", got: solanacodec.IdlTypeI64, want: codecv1.IdlTypeI64},
		{name: "IdlTypeU128", got: solanacodec.IdlTypeU128, want: codecv1.IdlTypeU128},
		{name: "IdlTypeI128", got: solanacodec.IdlTypeI128, want: codecv1.IdlTypeI128},
		{name: "IdlTypeBytes", got: solanacodec.IdlTypeBytes, want: codecv1.IdlTypeBytes},
		{name: "IdlTypeString", got: solanacodec.IdlTypeString, want: codecv1.IdlTypeString},
		{name: "IdlTypePublicKey", got: solanacodec.IdlTypePublicKey, want: codecv1.IdlTypePublicKey},
		{name: "IdlTypeUnixTimestamp", got: solanacodec.IdlTypeUnixTimestamp, want: codecv1.IdlTypeUnixTimestamp},
		{name: "IdlTypeHash", got: solanacodec.IdlTypeHash, want: codecv1.IdlTypeHash},
		{name: "IdlTypeDuration", got: solanacodec.IdlTypeDuration, want: codecv1.IdlTypeDuration},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Fatalf("%s mismatch: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestCompatV1DefaultHashBitLength(t *testing.T) {
	t.Parallel()

	if solanacodec.DefaultHashBitLength != codecv1.DefaultHashBitLength {
		t.Fatalf("DefaultHashBitLength mismatch: got %d, want %d", solanacodec.DefaultHashBitLength, codecv1.DefaultHashBitLength)
	}
}

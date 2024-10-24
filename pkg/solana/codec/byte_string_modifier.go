package codec

import (
	"fmt"

	"github.com/gagliardetto/solana-go"

	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

// SolanaAddressModifier implements the AddressModifier interface for Solana addresses.
// It handles encoding and decoding Solana addresses using Base58 encoding.
type SolanaAddressModifier struct{}

// EncodeAddress encodes a Solana address (32-byte array) into a Base58 string.
func (s SolanaAddressModifier) EncodeAddress(bytes []byte) (string, error) {
	if len(bytes) != s.Length() {
		return "", fmt.Errorf("%w: got length %d, expected 32 for bytes %x", commontypes.ErrInvalidType, len(bytes), bytes)
	}
	return solana.PublicKeyFromBytes(bytes).String(), nil
}

// DecodeAddress decodes a Base58-encoded Solana address into a 32-byte array.
func (s SolanaAddressModifier) DecodeAddress(str string) ([]byte, error) {
	if len(str) != 44 {
		return nil, fmt.Errorf("%w: got length %d, expected 44 for address %s", commontypes.ErrInvalidType, len(str), str)
	}

	pubkey, err := solana.PublicKeyFromBase58(str)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode Base58 address: %s", commontypes.ErrInvalidType, err)
	}

	return pubkey.Bytes(), nil
}

// Length returns the expected length of a Solana address in bytes (32 bytes).
func (s SolanaAddressModifier) Length() int {
	return solana.PublicKeyLength
}

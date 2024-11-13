package types

import (
	"database/sql/driver"
	"fmt"

	"github.com/gagliardetto/solana-go"
)

type PublicKey solana.PublicKey

// Scan implements Scanner for database/sql.
func (a *PublicKey) Scan(src interface{}) error {
	srcB, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("can't scan %T into Address", src)
	}
	if len(srcB) != solana.PublicKeyLength {
		return fmt.Errorf("can't scan []byte of len %d into Address, want %d", len(srcB), solana.PublicKeyLength)
	}
	copy(a[:], srcB)
	return nil
}

// Value implements valuer for database/sql.
func (a PublicKey) Value() (driver.Value, error) {
	return a[:], nil
}

package monitoring

import (
	"math/rand"
	"time"

	"github.com/gagliardetto/solana-go"
)

func generatePublicKey() solana.PublicKey {
	arr := generate32ByteArr()
	return solana.PublicKeyFromBytes(arr[:])
}

func generateSolanaConfig() SolanaConfig {
	return SolanaConfig{
		RPCEndpoint:  "http://solana:6969",
		NetworkName:  "solana-mainnet-beta",
		NetworkID:    "1",
		ChainID:      "solana-mainnet-beta",
		ReadTimeout:  100 * time.Millisecond,
		PollInterval: time.Duration(1+rand.Intn(5)) * time.Second,
	}
}

// This utilities are used primarely in tests but are present in the monitoring package because they are not inside a file ending in _test.go.
// This is done in order to expose NewRandomDataReader for use in cmd/monitoring.
// The following code is added to comply with the "unused" linter:
var (
	_ = generateSolanaConfig()
	_ = generatePublicKey()
)

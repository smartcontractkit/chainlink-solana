package monitoring

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/gagliardetto/solana-go"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
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

func generateSolanaFeedConfig() SolanaFeedConfig {
	coins := []string{"btc", "eth", "matic", "link", "avax", "ftt", "srm", "usdc", "sol", "ray"}
	coin := coins[rand.Intn(len(coins))]
	contract, transmissions, state := generatePublicKey(), generatePublicKey(), generatePublicKey()
	return SolanaFeedConfig{
		Name:           fmt.Sprintf("%s / usd", coin),
		Path:           fmt.Sprintf("%s-usd", coin),
		Symbol:         "$",
		HeartbeatSec:   1,
		ContractType:   "ocr2",
		ContractStatus: "status",

		ContractAddressBase58:      contract.String(),
		TransmissionsAccountBase58: transmissions.String(),
		StateAccountBase58:         state.String(),

		ContractAddress:      contract,
		TransmissionsAccount: transmissions,
		StateAccount:         state,
	}
}

func generate32ByteArr() [32]byte {
	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	if err != nil {
		panic("unable to generate [32]byte from rand")
	}
	var out [32]byte
	copy(out[:], buf[:32])
	return out
}

func NewFakeRDDSource(minFeeds, maxFeeds uint8) relayMonitoring.Source {
	return &fakeRddSource{minFeeds, maxFeeds}
}

type fakeRddSource struct {
	minFeeds, maxFeeds uint8
}

func (f *fakeRddSource) Fetch(_ context.Context) (interface{}, error) {
	numFeeds := int(f.minFeeds) + rand.Intn(int(f.maxFeeds-f.minFeeds))
	feeds := make([]relayMonitoring.FeedConfig, numFeeds)
	for i := 0; i < numFeeds; i++ {
		feeds[i] = generateSolanaFeedConfig()
	}
	return feeds, nil
}

func NewFakeBalancesSourceFactory(log relayMonitoring.Logger) relayMonitoring.SourceFactory {
	return &fakeSourceFactory{log}
}

type fakeSourceFactory struct {
	log relayMonitoring.Logger
}

func (f *fakeSourceFactory) NewSource(
	_ relayMonitoring.ChainConfig,
	_ relayMonitoring.FeedConfig,
) (relayMonitoring.Source, error) {
	return &fakeSource{f.log}, nil
}

type fakeSource struct {
	log relayMonitoring.Logger
}

func (f *fakeSource) Fetch(ctx context.Context) (interface{}, error) {
	return generateBalances(), nil
}

func generateBalances() Balances {
	out := Balances{
		make(map[string]uint64),
		make(map[string]solana.PublicKey),
	}
	for _, key := range BalanceAccountNames {
		out.Values[key] = rand.Uint64()
		out.Addresses[key] = generatePublicKey()
	}
	return out
}

// This utilities are used primarely in tests but are present in the monitoring package because they are not inside a file ending in _test.go.
// This is done in order to expose NewRandomDataReader for use in cmd/monitoring.
// The following code is added to comply with the "unused" linter:
var (
	_ = generateSolanaConfig()
	_ = generatePublicKey()
	_ = generateSolanaFeedConfig()
	_ = generate32ByteArr()
	_ = fakeRddSource{}
	_ = fakeSourceFactory{}
	_ = fakeSource{}
)

package testutils

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commonMonitoring "github.com/smartcontractkit/chainlink-common/pkg/monitoring"

	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/types"
)

// Generators

func GeneratePublicKey() solana.PublicKey {
	arr := Generate32ByteArr()
	return solana.PublicKeyFromBytes(arr[:])
}

func GenerateChainConfig() config.SolanaConfig {
	return config.SolanaConfig{
		RPCEndpoint:  "http://solana:6969",
		NetworkName:  "solana-mainnet-beta",
		NetworkID:    "1",
		ChainID:      "solana-mainnet-beta",
		ReadTimeout:  100 * time.Millisecond,
		PollInterval: time.Duration(1+rand.Intn(5)) * time.Second,
	}
}

func GenerateFeedConfig() config.SolanaFeedConfig {
	coins := []string{"btc", "eth", "matic", "link", "avax", "ftt", "srm", "usdc", "sol", "ray"}
	coin := coins[rand.Intn(len(coins))]
	contract, transmissions, state := GeneratePublicKey(), GeneratePublicKey(), GeneratePublicKey()
	return config.SolanaFeedConfig{
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

func Generate32ByteArr() [32]byte {
	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	if err != nil {
		panic("unable to Generate [32]byte from rand")
	}
	var out [32]byte
	copy(out[:], buf[:32])
	return out
}

func GenerateBalances() types.Balances {
	out := types.Balances{
		Values:    make(map[string]uint64),
		Addresses: make(map[string]solana.PublicKey),
	}
	for _, key := range types.FeedBalanceAccountNames {
		out.Values[key] = rand.Uint64()
		out.Addresses[key] = GeneratePublicKey()
	}
	return out
}

func GenerateTransactionSignatures() (success, fail int, out []*rpc.TransactionSignature) {
	success = int(math.Mod(float64(rand.Uint32()), 100))
	fail = 100 - success
	for i := 0; i < success; i++ {
		out = append(out, &rpc.TransactionSignature{Err: nil})
	}
	for i := 0; i < fail; i++ {
		out = append(out, &rpc.TransactionSignature{Err: fmt.Errorf("error %d", i)})
	}

	// randomize
	for i := range out {
		j := rand.Intn(i + 1)
		out[i], out[j] = out[j], out[i]
	}

	return success, fail, out
}

// Sources

func NewFakeRDDSource(minFeeds, maxFeeds uint8) commonMonitoring.Source {
	return &fakeRddSource{minFeeds, maxFeeds}
}

type fakeRddSource struct {
	minFeeds, maxFeeds uint8
}

func (f *fakeRddSource) Fetch(_ context.Context) (interface{}, error) {
	numFeeds := int(f.minFeeds) + rand.Intn(int(f.maxFeeds-f.minFeeds))
	feeds := make([]commonMonitoring.FeedConfig, numFeeds)
	for i := 0; i < numFeeds; i++ {
		feeds[i] = GenerateFeedConfig()
	}
	return feeds, nil
}

func NewFakeBalancesSourceFactory(log commonMonitoring.Logger) commonMonitoring.SourceFactory {
	return &fakeSourceFactory{log}
}

type fakeSourceFactory struct {
	log commonMonitoring.Logger
}

func (f *fakeSourceFactory) NewSource(
	_ commonMonitoring.ChainConfig,
	_ commonMonitoring.FeedConfig,
) (commonMonitoring.Source, error) {
	return &fakeSource{f.log}, nil
}

func (f *fakeSourceFactory) GetType() string {
	return "fake"
}

type fakeSource struct {
	log commonMonitoring.Logger
}

func (f *fakeSource) Fetch(ctx context.Context) (interface{}, error) {
	return GenerateBalances(), nil
}

func NewNullLogger() logger.Logger {
	return logger.Nop()
}

// This utilities are used primarely in tests but are present in the monitoring package because they are not inside a file ending in _test.go.
// This is done in order to expose NewRandomDataReader for use in cmd/monitoring.
// The following code is added to comply with the "unused" linter:
var (
	_ = GenerateChainConfig()
	_ = GeneratePublicKey()
	_ = GenerateFeedConfig()
	_ = Generate32ByteArr()
	_ = fakeRddSource{}
	_ = fakeSourceFactory{}
	_ = fakeSource{}
)

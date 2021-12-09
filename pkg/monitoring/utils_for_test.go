package monitoring

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"time"

	gbinary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/linkedin/goavro"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/pb"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"google.golang.org/protobuf/proto"
)

func generateNumericalMedianPluginConfig() ([]byte, *pb.NumericalMedianConfigProto) {
	out := &pb.NumericalMedianConfigProto{
		AlphaReportInfinite: true,
		AlphaReportPpb:      100,
		AlphaAcceptInfinite: true,
		AlphaAcceptPpb:      110,
		DeltaCNanoseconds:   uint64(120 * time.Millisecond),
	}
	buf, err := proto.Marshal(out)
	if err != nil {
		panic(fmt.Errorf("failed to marshal median plugin config: %w", err))
	}
	return buf, out
}

func generateOffchainConfig(oracles [19]pkgSolana.Oracle) ([]byte, *pb.OffchainConfigProto, *pb.NumericalMedianConfigProto) {
	offchainPublicKeys := [][]byte{} // TODO
	encodedNumericalMedianConfig, numericalMedianConfig := generateNumericalMedianPluginConfig()
	out := &pb.OffchainConfigProto{
		DeltaProgressNanoseconds:          uint64(100 * time.Millisecond),
		DeltaResendNanoseconds:            uint64(110 * time.Millisecond),
		DeltaRoundNanoseconds:             uint64(120 * time.Millisecond),
		DeltaGraceNanoseconds:             uint64(130 * time.Millisecond),
		DeltaStageNanoseconds:             uint64(140 * time.Millisecond),
		RMax:                              10,
		S:                                 []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9},
		OffchainPublicKeys:                offchainPublicKeys,
		PeerIds:                           []string{"1", "2", "3", "4"},
		ReportingPluginConfig:             encodedNumericalMedianConfig,
		MaxDurationQueryNanoseconds:       uint64(140 * time.Millisecond),
		MaxDurationObservationNanoseconds: uint64(150 * time.Millisecond),
		MaxDurationReportNanoseconds:      uint64(160 * time.Millisecond),

		MaxDurationShouldAcceptFinalizedReportNanoseconds:  uint64(170 * time.Millisecond),
		MaxDurationShouldTransmitAcceptedReportNanoseconds: uint64(180 * time.Millisecond),

		SharedSecretEncryptions: &pb.SharedSecretEncryptionsProto{
			DiffieHellmanPoint: []byte{'d', 'i', 'f', 'f', 'i', 'e'},
			SharedSecretHash:   []byte{'h', 'a', 's', 'h'},
			Encryptions:        [][]byte{[]byte("encryption")},
		},
	}
	buf, err := proto.Marshal(out)
	if err != nil {
		panic(fmt.Errorf("failed to marshal offchain config: %w", err))
	}
	return buf, out, numericalMedianConfig
}

func generateState() (
	pkgSolana.State,
	*pb.OffchainConfigProto,
	*pb.NumericalMedianConfigProto,
) {
	var oracles [19]pkgSolana.Oracle
	for i := 0; i < 19; i++ {
		oracles[i] = pkgSolana.Oracle{
			Transmitter: generatePublicKey(),
			Signer: pkgSolana.SigningKey{
				Key: generate20ByteArr(),
			},
			Payee:         generatePublicKey(),
			ProposedPayee: generatePublicKey(),
			Payment:       100,
			FromRoundID:   1,
		}
	}

	var leftovers [19]pkgSolana.LeftoverPayment
	var leftoversLen uint8 = 10
	var i uint8
	for i = 0; i < leftoversLen; i++ {
		leftovers[i] = pkgSolana.LeftoverPayment{
			Payee:  generatePublicKey(),
			Amount: 100,
		}
	}

	encodedOffchainConfig, offchainConfig, numericalMedianConfig := generateOffchainConfig(oracles)
	var enlargedOffchainConfig [4096]byte
	copy(enlargedOffchainConfig[:len(encodedOffchainConfig)], encodedOffchainConfig[:])

	state := pkgSolana.State{
		AccountDiscriminator: [8]byte{'0', '1', '2', '3', '4', '5', '6', '7'},
		Nonce:                42,
		Config: pkgSolana.Config{
			Version:                   1,
			Owner:                     generatePublicKey(),
			TokenMint:                 generatePublicKey(),
			TokenVault:                generatePublicKey(),
			RequesterAccessController: generatePublicKey(),
			BillingAccessController:   generatePublicKey(),
			MinAnswer:                 gbinary.Int128{Lo: 10, Hi: 10},
			MaxAnswer:                 gbinary.Int128{Lo: 100, Hi: 100},
			Decimals:                  1,
			Description:               generate32ByteArr(),
			F:                         10,
			ConfigCount:               1,
			LatestConfigDigest:        generate32ByteArr(),
			LatestConfigBlockNumber:   1,
			LatestAggregatorRoundID:   1,
			Epoch:                     1,
			Round:                     1,
			Billing: pkgSolana.Billing{
				ObservationPayment: 100,
			},
			Validator:         generatePublicKey(),
			FlaggingThreshold: 10,
			OffchainConfig: pkgSolana.OffchainConfig{
				Version: 1,
				Raw:     enlargedOffchainConfig,
				Len:     uint64(len(encodedOffchainConfig)),
			},
		},
		Oracles:            pkgSolana.Oracles{Raw: oracles, Len: 19},
		LeftoverPayment:    leftovers,
		LeftoverPaymentLen: leftoversLen,
		Transmissions:      generatePublicKey(),
	}
	return state, offchainConfig, numericalMedianConfig
}

func generateSolanaConfig() SolanaConfig {
	return SolanaConfig{
		RPCEndpoint: "",
		NetworkName: "solana-mainnet-beta",
		NetworkID:   "1",
		ChainID:     "solana-mainnet-beta",
	}
}

func generateFeedConfig() FeedConfig {
	return FeedConfig{
		FeedName:       "sol-link",
		FeedPath:       "sol/link",
		Symbol:         "l",
		HeartbeatSec:   1,
		ContractType:   "ocr2",
		ContractStatus: "status",

		ContractAddress:      generatePublicKey(),
		TransmissionsAccount: generatePublicKey(),
		StateAccount:         generatePublicKey(),

		PollInterval: 5 * time.Second,
	}
}

func generate20ByteArr() [20]byte {
	buf := make([]byte, 20)
	_, err := rand.Read(buf)
	if err != nil {
		panic("unable to generate [32]byte from rand")
	}
	var out [20]byte
	copy(out[:], buf[:20])
	return out
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

func generatePublicKey() solana.PublicKey {
	arr := generate32ByteArr()
	return solana.PublicKeyFromBytes(arr[:])
}

type fakeReader struct {
	readCh chan interface{}
}

func (f *fakeReader) Read(ctx context.Context, _ solana.PublicKey) (interface{}, error) {
	ans := <-f.readCh
	return ans, nil
}

// runGenerateRandomData should be executed as a goroutine.
// This method publishes random data as fast as the reader asks for it.
// Only run this if you're not using f.readCh dirrectly!
func (f *fakeReader) runGenerateRandomData(ctx context.Context, typ string) {
	log.Printf("generating fake data for account reader of type %s", typ)
	for {
		var payload interface{}
		if typ == "state" {
			state, _, _ := generateState()
			payload = StateEnvelope{state, 100}
		} else if typ == "transmission" {
			payload = generateTransmissionEnvelope(42)
		} else {
			panic(fmt.Errorf("unknown reader type %s", typ))
		}
		select {
		case f.readCh <- payload:
			log.Printf("send payload of type %s", typ)
		case <-ctx.Done():
			return
		}
	}
}

func generateTransmissionEnvelope(seed int) TransmissionEnvelope {
	return TransmissionEnvelope{
		pkgSolana.Answer{
			Data:      big.NewInt(int64(seed)),
			Timestamp: uint32(time.Now().Unix()),
		},
		1000, // BlockNumber
	}
}

type producedMessage struct{ key, value []byte }

type fakeProducer struct {
	sendCh chan producedMessage
}

func (f fakeProducer) Produce(key, value []byte) error {
	f.sendCh <- producedMessage{value, value}
	return nil
}

type fakeSchema struct {
	codec *goavro.Codec
}

func (f fakeSchema) Encode(value interface{}) ([]byte, error) {
	return f.codec.BinaryFromNative(nil, value)
}

func (f fakeSchema) Decode(buf []byte) (interface{}, error) {
	value, _, err := f.codec.NativeFromBinary(buf)
	return value, err
}

type devnullMetrics struct{}

var _ Metrics = (*devnullMetrics)(nil)

func (d *devnullMetrics) SetHeadTrackerCurrentHead(blockNumber uint64, networkName, chainID, networkID string) {
}

func (d *devnullMetrics) SetFeedContractMetadata(chainID, contractAddress, contractStatus, contractType, feedName, feedPath, networkID, networkName, symbol string) {
}

func (d *devnullMetrics) SetNodeMetadata(chainID, networkID, networkName, oracleName, sender string) {
}

func (d *devnullMetrics) SetOffchainAggregatorAnswers(answer *big.Int, contractAddress, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string) {
}

func (d *devnullMetrics) IncOffchainAggregatorAnswersTotal(contractAddress, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string) {
}

func (d *devnullMetrics) SetOffchainAggregatorSubmissionReceivedValues(value *big.Int, contractAddress, sender, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string) {
}

func (d *devnullMetrics) SetOffchainAggregatorAnswerStalled(isSet bool, contractAddress, chainID, contractStatus, contractType, feedName, feedPath, networkID, networkName string) {
}

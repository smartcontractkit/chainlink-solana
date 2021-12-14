// This file contains data generators and utilities to simplify tests.
// The data generated here shouldn't be used to run OCR instances
package monitoring

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"
	"sync"
	"time"

	gbinary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/linkedin/goavro"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring/pb"
	pkgSolana "github.com/smartcontractkit/chainlink-solana/pkg/solana"
	"github.com/smartcontractkit/chainlink/core/logger"
	"google.golang.org/protobuf/proto"
)

func generateNumericalMedianOffchainConfig() (*pb.NumericalMedianConfigProto, []byte, error) {
	out := &pb.NumericalMedianConfigProto{
		AlphaReportInfinite: ([]bool{true, false})[rand.Intn(2)],
		AlphaReportPpb:      rand.Uint64(),
		AlphaAcceptInfinite: ([]bool{true, false})[rand.Intn(2)],
		AlphaAcceptPpb:      rand.Uint64(),
		DeltaCNanoseconds:   rand.Uint64(),
	}
	buf, err := proto.Marshal(out)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal median plugin config: %w", err)
	}
	return out, buf, nil
}

func generateOffchainConfig(oracles [19]pkgSolana.Oracle, numOracles int) (
	*pb.OffchainConfigProto, *pb.NumericalMedianConfigProto, []byte, error,
) {
	numericalMedianOffchainConfig, encodedNumericalMedianOffchainConfig, err := generateNumericalMedianOffchainConfig()
	if err != nil {
		return nil, nil, nil, err
	}
	schedule := []uint32{}
	for i := 0; i < 10; i++ {
		schedule = append(schedule, 1)
	}
	offchainPublicKeys := [][]byte{}
	for i := 0; i < numOracles; i++ {
		randArr := generate32ByteArr()
		offchainPublicKeys = append(offchainPublicKeys, randArr[:])
	}
	peerIDs := []string{}
	for i := 0; i < numOracles; i++ {
		peerIDs = append(peerIDs, fmt.Sprintf("peer#%d", i))
	}
	config := &pb.OffchainConfigProto{
		DeltaProgressNanoseconds:          rand.Uint64(),
		DeltaResendNanoseconds:            rand.Uint64(),
		DeltaRoundNanoseconds:             rand.Uint64(),
		DeltaGraceNanoseconds:             rand.Uint64(),
		DeltaStageNanoseconds:             rand.Uint64(),
		RMax:                              rand.Uint32(),
		S:                                 schedule,
		OffchainPublicKeys:                offchainPublicKeys,
		PeerIds:                           peerIDs,
		ReportingPluginConfig:             encodedNumericalMedianOffchainConfig,
		MaxDurationQueryNanoseconds:       rand.Uint64(),
		MaxDurationObservationNanoseconds: rand.Uint64(),
		MaxDurationReportNanoseconds:      rand.Uint64(),

		MaxDurationShouldAcceptFinalizedReportNanoseconds:  rand.Uint64(),
		MaxDurationShouldTransmitAcceptedReportNanoseconds: rand.Uint64(),

		SharedSecretEncryptions: &pb.SharedSecretEncryptionsProto{
			DiffieHellmanPoint: []byte{'p', 'o', 'i', 'n', 't'},
			SharedSecretHash:   []byte{'h', 'a', 's', 'h'},
			Encryptions:        [][]byte{[]byte("encryption")},
		},
	}
	encodedConfig, err := proto.Marshal(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal offchain config: %w", err)
	}
	return config, numericalMedianOffchainConfig, encodedConfig, nil
}

func generateState() (
	pkgSolana.State,
	*pb.OffchainConfigProto,
	*pb.NumericalMedianConfigProto,
	error,
) {
	numOracles := 1 + rand.Intn(18) // At least one oracle.
	var oracles [19]pkgSolana.Oracle
	for i := 0; i < numOracles; i++ {
		oracles[i] = pkgSolana.Oracle{
			Transmitter: generatePublicKey(),
			Signer: pkgSolana.SigningKey{
				Key: generate20ByteArr(),
			},
			Payee:         generatePublicKey(),
			ProposedPayee: generatePublicKey(),
			Payment:       rand.Uint64(),
			FromRoundID:   rand.Uint32(),
		}
	}

	var numLeftovers = rand.Intn(numOracles)
	var leftovers [19]pkgSolana.LeftoverPayment
	for i := 0; i < numLeftovers; i++ {
		leftovers[i] = pkgSolana.LeftoverPayment{
			Payee:  generatePublicKey(),
			Amount: rand.Uint64(),
		}
	}

	offchainConfig, numericalMedianConfig, encodedOffchainConfig, err := generateOffchainConfig(oracles, numOracles)
	if err != nil {
		return pkgSolana.State{}, nil, nil, err
	}

	var enlargedOffchainConfig [4096]byte
	copy(enlargedOffchainConfig[:len(encodedOffchainConfig)], encodedOffchainConfig[:])

	state := pkgSolana.State{
		AccountDiscriminator: [8]byte{'0', '1', '2', '3', '4', '5', '6', '7'},
		Version:              uint8(rand.Intn(256)),
		Nonce:                uint8(rand.Intn(256)),
		Config: pkgSolana.Config{
			Owner:                     generatePublicKey(),
			ProposedOwner:             generatePublicKey(),
			TokenMint:                 generatePublicKey(),
			TokenVault:                generatePublicKey(),
			RequesterAccessController: generatePublicKey(),
			BillingAccessController:   generatePublicKey(),
			MinAnswer:                 gbinary.Int128{Lo: rand.Uint64(), Hi: rand.Uint64()},
			MaxAnswer:                 gbinary.Int128{Lo: rand.Uint64(), Hi: rand.Uint64()},
			Description:               generate32ByteArr(),
			Decimals:                  uint8(rand.Intn(256)),
			F:                         uint8(10),
			Round:                     uint8(rand.Intn(256)),
			Epoch:                     rand.Uint32(),
			LatestAggregatorRoundID:   rand.Uint32(),
			LatestTransmitter:         generatePublicKey(),
			ConfigCount:               rand.Uint32(),
			LatestConfigDigest:        generate32ByteArr(),
			LatestConfigBlockNumber:   rand.Uint64(),
			Billing: pkgSolana.Billing{
				ObservationPayment: rand.Uint32(),
			},
			Validator:         generatePublicKey(),
			FlaggingThreshold: rand.Uint32(),
			OffchainConfig: pkgSolana.OffchainConfig{
				Version: rand.Uint64(),
				Raw:     enlargedOffchainConfig,
				Len:     uint64(len(encodedOffchainConfig)),
			},
		},
		Oracles: pkgSolana.Oracles{
			Raw: oracles,
			Len: uint64(numOracles),
		},
		LeftoverPayments: pkgSolana.LeftoverPayments{
			Raw: leftovers,
			Len: uint64(numLeftovers),
		},
		Transmissions: generatePublicKey(),
	}
	return state, offchainConfig, numericalMedianConfig, nil
}

func generateSolanaConfig() SolanaConfig {
	return SolanaConfig{
		RPCEndpoint: "http://solana:6969",
		NetworkName: "solana-mainnet-beta",
		NetworkID:   "1",
		ChainID:     "solana-mainnet-beta",
	}
}

func generateFeedConfig() FeedConfig {
	coins := []string{"btc", "eth", "matic", "link", "avax", "ftt", "srm", "usdc", "sol", "ray"}
	coin := coins[rand.Intn(len(coins))]
	return FeedConfig{
		FeedName:       fmt.Sprintf("%s / usd", coin),
		FeedPath:       fmt.Sprintf("%s-usd", coin),
		Symbol:         "$",
		HeartbeatSec:   1,
		ContractType:   "ocr2",
		ContractStatus: "status",

		ContractAddress:      generatePublicKey(),
		TransmissionsAccount: generatePublicKey(),
		StateAccount:         generatePublicKey(),

		PollInterval: time.Duration(1+rand.Intn(5)) * time.Second,
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

func generateStateEnvelope() (StateEnvelope, error) {
	state, _, _, err := generateState()
	if err != nil {
		return StateEnvelope{}, err
	}
	return StateEnvelope{
		state,
		rand.Uint64(), // block number
	}, nil
}

func generateTransmissionEnvelope() TransmissionEnvelope {
	return TransmissionEnvelope{
		pkgSolana.Answer{
			Data:      big.NewInt(rand.Int63()),
			Timestamp: uint32(time.Now().Unix()),
		},
		rand.Uint64(), // BlockNumber
	}
}

// Test implementations of interfaces in monitoring.

type fakeReader struct {
	readCh chan interface{}
}

// NewRandomDataReader produces an AccountReader that generates random data for "state" and "transmission" types.
func NewRandomDataReader(ctx context.Context, wg *sync.WaitGroup, typ string, log logger.Logger) AccountReader {
	f := &fakeReader{make(chan interface{})}
	wg.Add(1)
	go func() {
		defer wg.Done()
		f.runRandomDataGenerator(ctx, typ, log)
	}()
	return f
}

func (f *fakeReader) Read(ctx context.Context, _ solana.PublicKey) (interface{}, error) {
	ans := <-f.readCh
	return ans, nil
}

// RunRandomDataGenerator should be executed as a goroutine.
// This method publishes random data as fast as the reader asks for it.
// Only run this if you're not using f.readCh dirrectly!
func (f *fakeReader) runRandomDataGenerator(ctx context.Context, typ string, log logger.Logger) {
	var err error
	for {
		var payload interface{}
		if typ == "state" {
			payload, err = generateStateEnvelope()
			if err != nil {
				log.Errorw("failed to generate state", "error", err)
				continue
			}
		} else if typ == "transmission" {
			payload = generateTransmissionEnvelope()
		} else {
			log.Critical(fmt.Errorf("unknown reader type %s", typ))
		}
		select {
		case f.readCh <- payload:
			log.Infof("sent payload of type %s", typ)
		case <-ctx.Done():
			return
		}
	}
}

type producerMessage struct{ key, value []byte }

type fakeProducer struct {
	sendCh chan producerMessage
}

func (f fakeProducer) Produce(key, value []byte) error {
	f.sendCh <- producerMessage{key, value}
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

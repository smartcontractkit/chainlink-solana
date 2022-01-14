package solana

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/libocr/offchainreporting2/confighelper"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	"golang.org/x/sync/singleflight"
)

const (
	XXXLogNone = iota
	XXXLogBasic
	XXXLogAll
)

// XXXInspectStates prints out state data, it should only be used for inspection
func XXXInspectStates(state, transmission, program, rpc string, log int) (answer *big.Int, timestamp time.Time, err error) {
	tracker := ContractTracker{
		StateID:         solana.MustPublicKeyFromBase58(state),
		TransmissionsID: solana.MustPublicKeyFromBase58(transmission),
		client:          NewClient(rpc),
		lggr:            logger.NullLogger,
		requestGroup:    &singleflight.Group{},
		ProgramID:       solana.MustPublicKeyFromBase58(program),
	}

	digester := OffchainConfigDigester{
		ProgramID: tracker.ProgramID,
	}

	cfg, err := tracker.LatestConfig(context.TODO(), 0)
	if err != nil {
		return answer, timestamp, errors.Wrap(err, "error in tracker.LatestConfig")
	}

	digest, err := digester.ConfigDigest(cfg)
	if err != nil {
		return answer, timestamp, errors.Wrap(err, "error in digester.ConfigDigest")
	}
	if cfg.ConfigDigest != digest {
		return answer, timestamp, errors.Errorf("config digest mismatch: %s (onchain) != %s (calculated)", cfg.ConfigDigest.Hex(), digest.Hex())
	}

	digest, epoch, round, answer, timestamp, err := tracker.LatestTransmissionDetails(context.TODO())
	if err != nil {
		return answer, timestamp, errors.Wrap(err, "error in tracker.LatestTransmissionDetails")
	}

	bh, err := tracker.LatestBlockHeight(context.TODO())
	if err != nil {
		return answer, timestamp, errors.Wrap(err, "error in tracker.LatestBlockHeight")
	}

	var txs TransmissionsPartial
	err = tracker.client.rpc.GetAccountDataInto(context.TODO(), tracker.state.Transmissions, &txs)
	seeds := [][]byte{[]byte("store"), tracker.StateID.Bytes()}
	storeAuthority, _, err := solana.FindProgramAddress(seeds, tracker.ProgramID)
	if err != nil {
		return answer, timestamp, errors.Wrap(err, "error in solana.FindProgramAddress")
	}

	nodeLen := len(tracker.state.Oracles.Data())
	if log > XXXLogNone {
		fmt.Println("LatestBlockHeight", bh)
		fmt.Println("LatestTransmissionDetails", digest, epoch, round, answer, timestamp)
		fmt.Println("LatestConfigBlockNumber", tracker.state.Config.LatestConfigBlockNumber)
		fmt.Println("OffchainConfig", tracker.state.Config.OffchainConfig.Data())
		fmt.Println("AccessControllers", tracker.state.Config.RequesterAccessController, tracker.state.Config.BillingAccessController)
		fmt.Println("BillingConfig", tracker.state.Config.Billing.ObservationPayment, tracker.state.Config.Billing.TransmissionPayment)
		fmt.Printf("OracleConfigs Len: %d, Data: %+v\n", nodeLen, tracker.state.Oracles.Data())
		fmt.Println("Transmissions Account", tracker.state.Transmissions)
		fmt.Printf("Transmissions %+v\n", tracker.answer)

		// data from transmission account
		fmt.Println("Transmissions writer permission", txs.Writer, storeAuthority)
		fmt.Printf("Transmissions Partial: %+v\n", txs)
		fmt.Println("Parsed Description:", string(txs.Description[:]))
	}

	if log > XXXLogBasic {
		// parsed config data
		config, err := confighelper.PublicConfigFromContractConfig(false, types.ContractConfig{
			OffchainConfig:        tracker.state.Config.OffchainConfig.Data(),
			OffchainConfigVersion: 2,
			Signers:               make([]types.OnchainPublicKey, nodeLen),
			Transmitters:          make([]types.Account, nodeLen),
		})
		if err != nil {
			return answer, timestamp, errors.Wrap(err, "error in confighelper.PublicConfigFromContractConfig")
		}
		dataOracles := tracker.state.Oracles.Data()
		dataConfig := config.OracleIdentities
		if len(dataOracles) != len(dataConfig) {
			return answer, timestamp, errors.New("mismatch oracle length in offchain config and retrieved oracle data")
		}
		for i := range dataOracles {
			fmt.Println("ORACLE:", i)
			fmt.Println("Transmitter:", dataOracles[i].Transmitter)
			fmt.Println("OnchainPublicKey:", hex.EncodeToString(dataOracles[i].Signer.Key[:]))
			fmt.Println(dataOracles[i].Signer.Key)
			fmt.Println("OffchainPublicKey:", hex.EncodeToString(dataConfig[i].OffchainPublicKey))
			fmt.Println(dataConfig[i].OffchainPublicKey)
			fmt.Println("PeerID:", dataConfig[i].PeerID)
			fmt.Println("----------------------------------------------")
		}
	}

	return answer, timestamp, nil
}

// Partial transmissions state, does not include actual transmissions
type TransmissionsPartial struct {
	Prefix           [8]byte
	Version          uint8
	Store            solana.PublicKey
	Writer           solana.PublicKey
	Description      [32]byte
	Decimals         uint8
	FlagThreshold    uint32
	RoundID          uint32
	Granularity      uint8
	LiveLength       uint32
	LiveCursor       uint32
	HistoricalCursor uint32
}

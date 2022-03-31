package main

import (
	"context"
	"fmt"
	"log"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	relayMonitoring "github.com/smartcontractkit/chainlink-relay/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-solana/pkg/monitoring"
	"github.com/smartcontractkit/chainlink/core/logger"
)

func main() {
	rpcEndpoint := "https://floral-morning-sun.solana-devnet.quiknode.pro/d874b0e33834d6babaa1e60a5b6181f22dd0409e/"
	client := rpc.New(rpcEndpoint)

	coreLog, closeLggr := logger.NewLogger()
	defer func() {
		if err := closeLggr(); err != nil {
			log.Println(fmt.Sprintf("Error while closing Logger: %v", err))
		}
	}()
	log := logWrapper{coreLog}

	//accountPubKeyBase58 := "2TQmhSnGK5NwdXBKEmJ8wfwH17rNSQgH11SVdHkYC1ZD" // testnet LINK/USD state
	accountPubKeyBase58 := "HoLknTuGPcjsVDyEAu92x1njFKc5uUXuYLYFuhiEatF1" // testnet LINK/USD transmissions

	accounts := []solana.PublicKey{
		solana.MustPublicKeyFromBase58(accountPubKeyBase58),
	}

	commitment := rpc.CommitmentFinalized

	//source := monitoring.NewAccountSource(
	//	client,
	//	accounts,
	//	log.With("source", "state"),
	//	commitment,
	//)
	source := monitoring.NewTransmissionAccountSource(
		client,
		accounts,
		log.With("source", "transmissions"),
		commitment,
	)

	ctx := context.Background()
	data, err := source.Fetch(ctx)
	fmt.Println(">>>>>>>>>>>", data, err)
}

// adapt core logger to monitoring logger.

type logWrapper struct {
	logger.Logger
}

func (l logWrapper) With(values ...interface{}) relayMonitoring.Logger {
	return logWrapper{l.Logger.With(values...)}
}

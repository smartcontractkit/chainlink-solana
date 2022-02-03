package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	solanaGo "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	solanaRelay "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

var inspectType string
var stateAccount string
var transmissionsAccount string
var ocr2Program string
var network string

func init() {
	flag.StringVar(&inspectType, "type", "", "specify the type of inspection")
	flag.StringVar(&network, "network", "", "specify the solana network")
	flag.StringVar(&stateAccount, "state", "", "specify the ocr2 state account for inspection")
	flag.StringVar(&transmissionsAccount, "transmissions", "", "specify the ocr2 transmissions account for inspection")
	flag.StringVar(&ocr2Program, "program", "", "specify the ocr2 program account for inspection")
}

func main() {
	flag.Parse()

	switch strings.ToLower(network) {
	case "mainnet":
		network = rpc.MainNetBeta_RPC
	case "testnet":
		network = rpc.TestNet_RPC
	case "devnet":
		network = rpc.DevNet_RPC
	case "localnet":
		network = rpc.LocalNet_RPC
	default:
		// allows for option to pass url
		if network == "" {
			log.Fatal(errors.New("Unknown network"))
		}
	}

	var err error
	switch strings.ToLower(inspectType) {
	case "feed":
		_, _, err = solanaRelay.XXXInspectStates(
			stateAccount,
			transmissionsAccount,
			ocr2Program,
			network,
			solanaRelay.XXXLogBasic, // print data
		)

		if err != nil {
			log.Fatal(err)
		}
	case "txs":
		err = XXXInspectTxs(network, stateAccount)
	default:
		log.Fatal(errors.New("Unknown type"))
	}

	if err != nil {
		log.Fatal(err)
	}
}

func XXXInspectTxs(network string, state string) error {
	client := rpc.New(network)
	txSigs, err := client.GetSignaturesForAddressWithOpts(
		context.TODO(),
		solanaGo.MustPublicKeyFromBase58(state),
		&rpc.GetSignaturesForAddressOpts{
			Commitment: rpc.CommitmentConfirmed,
		},
	)
	if err != nil {
		return err
	}

	chunkStart := txSigs[len(txSigs)-1].BlockTime.Time()
	reverts := map[string]int{}
	var revertCount int
	var passCount int
	var pass []int
	var fail []int

	// parse all txs
	for i := len(txSigs) - 1; i >= 0; i-- {
		tx := txSigs[i]
		if tx.BlockTime.Time().Sub(chunkStart) > 1*time.Minute {
			fmt.Printf("%s: Success - %d, Reverted - %d\n", chunkStart, passCount, revertCount)
			pass = append(pass, passCount)
			fail = append(fail, revertCount)

			chunkStart = tx.BlockTime.Time()
			revertCount = 0
			passCount = 0
		}

		if tx.Err == nil {
			passCount++
			continue
		}

		revertCount++
		// fetch additional data if revert (hits the rate limit)
		txRaw, err := client.GetTransaction(
			context.TODO(),
			tx.Signature,
			&rpc.GetTransactionOpts{
				Commitment: rpc.CommitmentConfirmed,
			},
		)
		if err != nil {
			return err
		}
		txDetails := txRaw.Transaction.GetParsedTransaction()

		// use first address: https://docs.solana.com/developing/clients/jsonrpc-api#transaction-structure
		// The first message.header.numRequiredSignatures public keys must sign the transaction (therefore it is the transmitter)
		reverts[txDetails.Message.AccountKeys[0].String()]++
	}

	// calculate averages
	var avgPass int
	var avgFail int
	for i := range pass {
		avgPass += pass[i]
		avgFail += fail[i]
	}

	t := len(pass)
	fmt.Printf("\n---------------SUMMARY-----------------\n")
	fmt.Printf("Minutes: %d\n", t)
	fmt.Printf("Success: %d/min\n", avgPass/t)
	fmt.Printf("Reverts: %d/min\n", avgFail/t)

	fmt.Printf("\n----------REVERTS/ADDRESS---------------\n")
	for k,v := range reverts {
		fmt.Printf("%s: %d\n", k, v)
	}


	return nil
}

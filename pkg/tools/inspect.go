package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	bin "github.com/gagliardetto/binary"
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

type Transmission struct {
	Epoch uint32
	Round uint8
	Oracle string
	Success bool
	ObservationsTimestamp time.Time
	BlockTimestamp time.Time	
}

func (r *Transmission) Delay() time.Duration {
	return r.BlockTimestamp.Sub(r.ObservationsTimestamp)
}

// takes a list of transmissions and filters them by the provided predicate,
// then generates a textual summary
func Summary(rs []Transmission, filter func(r Transmission) bool) string {
	succeeded := 0

	filtered := 0
	filteredSucceeded := 0

	filteredDelaySum := time.Duration(0)
	filteredDelays := []time.Duration{}


	for _, r := range rs {
		f := filter(r)
		if f {
			filtered++
		}
		if r.Success {
			succeeded++
			if f {
				filteredSucceeded++
			}
		}
		if f {
			filteredDelaySum += r.Delay()
			filteredDelays = append(filteredDelays, r.Delay())
		}
	}

	sort.Slice(filteredDelays, func(i, j int) bool {
		return filteredDelays[i] <= filteredDelays[j]
	})

	s := ""
	// s += fmt.Sprintf("total    count: %3d success: %3d fail: %3d success-%%: %5.2f\n", 
	// 	len(rs), succeeded, len(rs)-succeeded, float64(suceeded)/float64(len(rs)))
	s += fmt.Sprintf("  success-%%: %6.2f count: %3d success: %3d fail: %3d\n", 
		100*float64(filteredSucceeded)/float64(filtered), filtered, filteredSucceeded, filtered-filteredSucceeded)
	s += fmt.Sprintf("  rel success-%%: %6.2f fail-%%: %6.2f\n", 
		100*float64(filteredSucceeded)/float64(succeeded), 100*float64(filtered-filteredSucceeded)/float64(len(rs)-succeeded))
	s += fmt.Sprintf("  avg delay in s:         %6.2f\n", filteredDelaySum.Seconds()/float64(filtered))
	s += fmt.Sprintf("  50-th %%ile delay in s:  %6.2f\n", filteredDelays[len(filteredDelays)/2].Seconds())
	s += fmt.Sprintf("  95-th %%ile delay in s:  %6.2f\n", filteredDelays[len(filteredDelays)*95/100].Seconds())
	return s
}


func XXXInspectTxs(network string, state string) error {
	client := rpc.New(network)

	// fetch 0-999
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

	// modify this to change the number of transactions being considered, e.g. with i < 4 we consider 5k txs
	for i := 0; i < 4; i++ {
		// fetch x000-x999
		txSigsNext, err := client.GetSignaturesForAddressWithOpts(
			context.TODO(),
			solanaGo.MustPublicKeyFromBase58(state),
			&rpc.GetSignaturesForAddressOpts{
				Commitment: rpc.CommitmentConfirmed,
				Before:     txSigs[len(txSigs)-1].Signature,
			},
		)
		if err != nil {
			return err
		}

		txSigs = append(txSigs, txSigsNext...)
	}

	var transmissions []Transmission

	// parse all txs
	for i := len(txSigs) - 1; i >= 0; i-- {
	// for i := 0; i < len(txSigs); i++ {
		tx := txSigs[i]

		// fetch additional data about tx (hits the rate limit for public endpoint)
		txRaw, err := client.GetTransaction(
			context.TODO(),
			tx.Signature,
			&rpc.GetTransactionOpts{
				Commitment: rpc.CommitmentConfirmed,
				Encoding:   solanaGo.EncodingBase64,
			},
		)
		if err != nil {
			return err
		}
		txData, err := solanaGo.TransactionFromDecoder(bin.NewBinDecoder(txRaw.Transaction.GetBinary()))
		if err != nil {
			return err
		}


		// use first address: https://docs.solana.com/developing/clients/jsonrpc-api#transaction-structure
		// The first message.header.numRequiredSignatures public keys must sign the transaction (therefore it is the transmitter)
		transmitter := txData.Message.AccountKeys[0].String()

		// store nonce (byte) + config digest ([32]byte) + epoch/round prefix ([32-4-1]byte)
		transmitData := []byte(txData.Message.Instructions[0].Data)

		// fmt.Printf("%x", []byte(txData.Message.Instructions[0].Data))

		epochRoundOffset := 1 + 32 + 27
		obsTsOffset := 97

		if len(transmitData) < obsTsOffset+4 {
			fmt.Println("WARN: Unable to parse tx", tx.Signature)
			continue
		}

		epoch := binary.BigEndian.Uint32(transmitData[epochRoundOffset:epochRoundOffset+4])
		round := transmitData[epochRoundOffset+4]
		obsTs := time.Unix(int64(binary.BigEndian.Uint32(transmitData[97:97+4])), 0)


		// fmt.Println("Epoch", epoch, "Round", round)
		// fmt.Printf("     Observations Timestamp: %v\n", obsTs.Format(time.Stamp))
		// fmt.Printf("     Block Timestamp:        %v\n", tx.BlockTime.Time().Format(time.Stamp))
		// fmt.Printf("     Delay:                  %v\n", tx.BlockTime.Time().Sub(obsTs))

		transmissions = append(transmissions, Transmission{
			epoch,
			round,
			transmitter,
			tx.Err == nil,
			obsTs,
			tx.BlockTime.Time(),
		})

		if i % 500 == 0 {
			fmt.Fprintf(os.Stderr, "%d left...\n", i)
		}
	}

	fmt.Printf("\n---------------totals-----------------\n")


	fmt.Println("total")
	fmt.Println(Summary(transmissions, func (r Transmission) bool {
		return true
	}))	

	fmt.Printf("\n---------------breakdown by success/revert-----------------\n")

	fmt.Println("success")
	fmt.Println(Summary(transmissions, func (r Transmission) bool {
		return r.Success
	}))	

	fmt.Println("revert")
	fmt.Println(Summary(transmissions, func (r Transmission) bool {
		return !r.Success
	}))		


	fmt.Printf("\n---------------breakdown by transmitting oracle-----------------\n")

	oracles := make(map[string]struct{})
	for _, r := range transmissions {
		oracles[r.Oracle] = struct{}{}
	}
	var sortedOracles []string
	for o := range oracles {
		sortedOracles = append(sortedOracles, o)
	}
	sort.StringSlice(sortedOracles).Sort()
	for _, oracle := range sortedOracles {
		fmt.Println(oracle)
		fmt.Println(Summary(transmissions, func (r Transmission) bool {
			return r.Oracle == oracle
		}))
	}

	fmt.Printf("\n---------------breakdown by delay-----------------\n")

	for minDelaySecs := 0;  minDelaySecs <= 50; minDelaySecs += 5 {
		maxDelaySecs := minDelaySecs + 5
		fmt.Printf("%vs < delay <= %vs\n", minDelaySecs, maxDelaySecs)
		fmt.Println(Summary(transmissions, func (r Transmission) bool {
			return float64(minDelaySecs) < r.Delay().Seconds() && r.Delay().Seconds() <= float64(maxDelaySecs)
		}))
	}

	fmt.Printf("\n---------------breakdown by hour-----------------\n")

	startTimestamp := transmissions[0].BlockTimestamp
	stopTimestamp := transmissions[len(transmissions)-1].BlockTimestamp
	for ts := startTimestamp; ts.Before(stopTimestamp); ts = ts.Add(time.Hour) {
		tsTo := ts.Add(time.Hour)
		fmt.Printf("block timestamp between %v and %v\n", ts.Format(time.Stamp), tsTo.Format(time.Stamp))
		fmt.Println(Summary(transmissions, func (r Transmission) bool {
			return ts.Before(r.BlockTimestamp) && r.BlockTimestamp.Before(tsTo)
		}))		
	}

	fmt.Printf("\n---------------rate stats-----------------\n")
	durationSeconds := stopTimestamp.Sub(startTimestamp).Seconds()
	successCount := 0;
	for _, r := range transmissions {
		if r.Success {
			successCount++
		}
	}
	fmt.Printf("interval between txs in s:       %5.2f\n", durationSeconds/float64(len(transmissions)))
	fmt.Printf("tx rate in 1/s:                  %5.2f\n", float64(len(transmissions))/durationSeconds)	
	fmt.Printf("interval between successes in s: %5.2f\n", durationSeconds/float64(successCount))
	fmt.Printf("success rate in 1/s:             %5.2f\n", float64(successCount)/durationSeconds)
	fmt.Printf("interval between reverts in s:   %5.2f\n", durationSeconds/float64(len(transmissions)-successCount))
	fmt.Printf("revert rate in 1/s:              %5.2f\n", float64(len(transmissions)-successCount)/durationSeconds)


	fmt.Printf("\n---------------successful update interval stats-----------------\n")
	successTransmissions := []Transmission{}
	for _, r := range transmissions {
		if r.Success {
			successTransmissions = append(successTransmissions, r)
		}
	}

	successTransmissionIntervals := []time.Duration{}
	for i := 1; i < len(successTransmissions); i++ {
		betweenUpdates := successTransmissions[i].BlockTimestamp.Sub(successTransmissions[i-1].BlockTimestamp)
		successTransmissionIntervals = append(successTransmissionIntervals, betweenUpdates)
	}

	sort.Slice(successTransmissionIntervals, func(i, j int) bool {
		return successTransmissionIntervals[i] < successTransmissionIntervals[j]
	})

	fmt.Printf("50-th %%ile successful update interval in s:    %6.2f\n", successTransmissionIntervals[len(successTransmissionIntervals)/2].Seconds())
	fmt.Printf("95-th %%ile successful update interval in s:    %6.2f\n", successTransmissionIntervals[len(successTransmissionIntervals)*95/100].Seconds())
	fmt.Printf("99.5-th %%ile successful update interval in s:  %6.2f\n", successTransmissionIntervals[len(successTransmissionIntervals)*995/1000].Seconds())
	fmt.Printf("worst successful update interval in s:          %6.2f\n", successTransmissionIntervals[len(successTransmissionIntervals)-1].Seconds())


	fmt.Printf("\n---------------age of latest value in contract-----------------\n")
	totalAgeSeconds := float64(0)
	for i := 1; i < len(successTransmissions); i++ {
		secondsBetweenUpdates := successTransmissions[i].BlockTimestamp.Sub(successTransmissions[i-1].BlockTimestamp).Seconds()
		ageSeconds := successTransmissions[i-1].Delay().Seconds() + secondsBetweenUpdates/2
		totalAgeSeconds += ageSeconds * secondsBetweenUpdates
	}
	fmt.Printf("average age in seconds: %v\n", totalAgeSeconds/durationSeconds)


	return nil
}

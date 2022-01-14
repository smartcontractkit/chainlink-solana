package main

import (
	"log"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

func main() {
	// _, _, err := solana.XXXInspectStates(
	// 	"5mfAsrt5MLU1q1ZWWVi68KoSSZGwCQBvLqYdpjEdHdpe", // state
	// 	"5zzLD6uuEkZjQmGGQ3kxjpBvjussTp3WkgzthtgfdjCj", // transmissions
	// 	"CF13pnKGJ1WJZeEgVAtFdUi4MMndXm9hneiHs8azUaZt", // ocr2 program
	// 	rpc.LocalNet_RPC, // localnet
	// 	true,             // print data
	// )
	_, _, err := solana.XXXInspectStates(
		"4PndafEthP58AUsQccPvyy7eXAnYr9M7MDv1V6M4ugzX", // state
		"69qS5PTKse6fvFyj7CWGtn8Vc2hWUv2usgNdrnmdtyzX", // transmissions
		"AZDmDi2CL8NpX6F8EYKuuMd1iGEfL5FpByXjhiXocQbY", // ocr2 program
		rpc.DevNet_RPC, // devnet
		true,           // print data
	)

	if err != nil {
		log.Fatal(err)
	}
}

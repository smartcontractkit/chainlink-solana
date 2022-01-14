package main

import (
	"log"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

func main() {
	err := solana.XXXInspectStates(
		"5mfAsrt5MLU1q1ZWWVi68KoSSZGwCQBvLqYdpjEdHdpe", // state
		"5zzLD6uuEkZjQmGGQ3kxjpBvjussTp3WkgzthtgfdjCj", // transmissions
		"CF13pnKGJ1WJZeEgVAtFdUi4MMndXm9hneiHs8azUaZt", // ocr2 program
		rpc.LocalNet_RPC,
	)

	if err != nil {
		log.Fatal(err)
	}
}

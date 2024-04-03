package types

import "github.com/gagliardetto/solana-go"

var FeedBalanceAccountNames = []string{
	"contract",
	"state",
	"transmissions",
	"token_vault",
	"requester_access_controller",
	"billing_access_controller",
}

type Balances struct {
	Values    map[string]uint64
	Addresses map[string]solana.PublicKey
}

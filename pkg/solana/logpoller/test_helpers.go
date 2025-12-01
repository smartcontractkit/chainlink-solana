package logpoller

import (
	"encoding/json"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

func mustMarshalEventIdl(idl codec.EventIDLTypes) string {
	b, err := json.Marshal(idl)
	if err != nil {
		panic(err)
	}
	return string(b)
}


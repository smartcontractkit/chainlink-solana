package config

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
)

type ContractReader struct {
	Namespaces map[string]ChainContractReader `json:"namespaces" toml:"namespaces"`
}

type ChainContractReader struct {
	IDL string `json:"anchorIDL" toml:"anchorIDL"`
	// Reads key is the off-chain name for this read.
	Reads map[string]ReadDefinition
	// TODO ContractPollingFilter same as EVM?
}

type ReadDefinition struct {
	ChainSpecificName   string                      `json:"chainSpecificName"`
	ReadType            ReadType                    `json:"readType,omitempty"`
	InputModifications  commoncodec.ModifiersConfig `json:"inputModifications,omitempty"`
	OutputModifications commoncodec.ModifiersConfig `json:"outputModifications,omitempty"`
	RPCOpts             *RPCOpts                    `json:"rpcOpts,omitempty"`

	// TODO EventDefinitions    *EventDefinitions similar to EVM?
	// TODO Lookup details for PDAs and lookup tables to be merged with CW
	//LookupTables *LookupTables
	//Accounts     *[]Lookup
}

type ReadType int

const (
	Account ReadType = iota
	Log
)

func (r ReadType) String() string {
	switch r {
	case Account:
		return "Account"
	case Log:
		return "Log"
	default:
		return fmt.Sprintf("Unknown(%d)", r)
	}
}

type RPCOpts struct {
	Encoding   *solana.EncodingType `json:"encoding,omitempty"`
	Commitment *rpc.CommitmentType  `json:"commitment,omitempty"`
	DataSlice  *rpc.DataSlice       `json:"dataSlice,omitempty"`
}

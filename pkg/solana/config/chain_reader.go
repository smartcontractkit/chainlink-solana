package config

import (
	"encoding/json"
	"fmt"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

type ContractReader struct {
	Namespaces map[string]ChainContractReader `json:"namespaces"`
}

type ChainContractReader struct {
	codec.IDL `json:"anchorIDL"`
	// Reads key is the off-chain name for this read.
	Reads map[string]ReadDefinition `json:"reads"`
	// TODO ContractPollingFilter same as EVM?
}

type ReadDefinition struct {
	ChainSpecificName   string                      `json:"chainSpecificName"`
	ReadType            ReadType                    `json:"readType,omitempty"`
	InputModifications  commoncodec.ModifiersConfig `json:"inputModifications,omitempty"`
	OutputModifications commoncodec.ModifiersConfig `json:"outputModifications,omitempty"`
}

type ReadType int

const (
	Account ReadType = iota
	Event
)

func (r ReadType) String() string {
	switch r {
	case Account:
		return "Account"
	case Event:
		return "Event"
	default:
		return fmt.Sprintf("Unknown(%d)", r)
	}
}

func (c *ChainContractReader) UnmarshalJSON(bytes []byte) error {
	rawJSON := make(map[string]json.RawMessage)
	if err := json.Unmarshal(bytes, &rawJSON); err != nil {
		return err
	}

	idlBytes := rawJSON["anchorIDL"]
	var rawString string
	if err := json.Unmarshal(idlBytes, &rawString); err == nil {
		if err = json.Unmarshal([]byte(rawString), &c.IDL); err != nil {
			return fmt.Errorf("failed to parse anchorIDL string as IDL struct: %w", err)
		}
		return nil
	}

	// If we didn't get a string, attempt to parse directly as an IDL object
	if err := json.Unmarshal(idlBytes, &c.IDL); err != nil {
		return fmt.Errorf("anchorIDL field is neither a valid JSON string nor a valid IDL object: %w", err)
	}

	if len(c.Accounts) == 0 && len(c.Events) == 0 {
		return fmt.Errorf("namespace idl must have at least one account or event: %w", commontypes.ErrInvalidConfig)
	}

	if err := json.Unmarshal(rawJSON["reads"], &c.Reads); err != nil {
		return err
	}

	if len(c.Reads) == 0 {
		return fmt.Errorf("namespace must have at least one read: %w", commontypes.ErrInvalidConfig)
	}

	return nil
}

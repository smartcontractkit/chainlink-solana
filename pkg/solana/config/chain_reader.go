package config

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
)

type ContractReader struct {
	Namespaces map[string]ChainContractReader `json:"namespaces"`
	// AddressShareGroups lists namespaces groups that share the same address.
	// Whichever namespace or i.e. Binding from the list is Bound first will share that address with the rest of the group.
	// Namespaces that were bound after the first one still have to be Bound to be initialised.
	// If they are Bound with an empty address string, they will use the address of the first Bound contract.
	// If they are Bound with a non-empty address string, an error will be thrown unless the address matches the address of the first Bound shared contract.
	AddressShareGroups [][]string `json:"addressShareGroups,omitempty"`
}

type ChainContractReader struct {
	codec.IDL       `json:"anchorIDL"`
	ContractAddress solana.PublicKey `json:"contractAddress"`
	// Reads key is the off-chain name for this read.
	Reads map[string]ReadDefinition `json:"reads"`
	// TODO ContractPollingFilter same as EVM?
}

type MultiReader struct {
	// Reads is a list of reads that is sequentially read to fill out a complete response for the parent read.
	// Parent ReadDefinition has to define codec modifiers which adds fields that are to be filled out by the reads in Reads.
	Reads []ReadDefinition `json:"reads,omitempty"`
	// ReuseParams If true, params from parent read will be reused for all MultiReader Reads.
	ReuseParams bool `json:"reuseParams"`
}

type ReadDefinition struct {
	ChainSpecificName   string                      `json:"chainSpecificName"`
	ReadType            ReadType                    `json:"readType,omitempty"`
	InputModifications  commoncodec.ModifiersConfig `json:"inputModifications,omitempty"`
	OutputModifications commoncodec.ModifiersConfig `json:"outputModifications,omitempty"`
	PDADefinition       codec.PDATypeDef            `json:"pdaDefinition,omitempty"` // Only used for PDA account reads
	MultiReader         *MultiReader                `json:"multiReader,omitempty"`
	IndexedField0       *IndexedField               `json:"indexedField0"`
	IndexedField1       *IndexedField               `json:"indexedField1"`
	IndexedField2       *IndexedField               `json:"indexedField2"`
	IndexedField3       *IndexedField               `json:"indexedField3"`
	// ResponseAddressHardCoder hardcodes the address of the contract into the defined field in the response.
	ResponseAddressHardCoder *commoncodec.HardCodeModifierConfig `json:"responseAddressHardCoder,omitempty"`
	// This will create a log poller filter for this event.
	*PollingFilter `json:"pollingFilter,omitempty"`
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

type IndexedField struct {
	OffChainPath string `json:"offChainPath"`
	OnChainPath  string `json:"onChainPath"`
}

func (c *ContractReader) UnmarshalJSON(bytes []byte) error {
	rawJSON := make(map[string]json.RawMessage)
	if err := json.Unmarshal(bytes, &rawJSON); err != nil {
		return err
	}

	c.Namespaces = make(map[string]ChainContractReader)
	if err := json.Unmarshal(rawJSON["namespaces"], &c.Namespaces); err != nil {
		return err
	}

	if rawJSON["addressShareGroups"] != nil {
		if err := json.Unmarshal(rawJSON["addressShareGroups"], &c.AddressShareGroups); err != nil {
			return err
		}
	}

	if c.AddressShareGroups != nil {
		seen := make(map[string][]string)
		for _, group := range c.AddressShareGroups {
			for _, namespace := range group {
				if seenIn, alreadySeen := seen[namespace]; alreadySeen {
					return fmt.Errorf("namespace %s is already in share group %v: %w", namespace, seenIn, commontypes.ErrInvalidConfig)
				}
				seen[namespace] = group
			}
		}
	}

	return nil
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

	if len(c.IDL.Accounts) == 0 && len(c.IDL.Events) == 0 {
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

type PollingFilter struct {
	EventName   string        `json:"eventName"`
	Retention   time.Duration `json:"retention"`   // maximum amount of time to retain logs
	MaxLogsKept int64         `json:"maxLogsKept"` // maximum number of logs to retain ( 0 = unlimited )
}

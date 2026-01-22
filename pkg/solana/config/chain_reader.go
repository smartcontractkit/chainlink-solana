package config

import (
	"encoding/json"
	"fmt"
	"time"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"

	codecv1 "github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/v1"
)

const (
	DefaultLogPollerRetention = 30 * 24 * time.Hour
	DefaultMaxLogsKept        = 0
	DefaultStartingBlock      = 0
	DefaultIncludeReverted    = false
)

type PollingFilter struct {
	Retention       *time.Duration `json:"retention,omitempty"`     // maximum amount of time to retain logs
	MaxLogsKept     *int64         `json:"maxLogsKept,omitempty"`   // maximum number of logs to retain ( 0 = unlimited )
	StartingBlock   *int64         `json:"startingBlock,omitempty"` // which block to start looking for logs
	IncludeReverted *bool          `json:"includeReverted"`         // whether to include logs emitted by transactions which failed while executing on chain
}

func (f PollingFilter) GetRetention() time.Duration {
	if f.Retention == nil {
		return DefaultLogPollerRetention
	}

	return *f.Retention
}

func (f PollingFilter) GetMaxLogsKept() int64 {
	if f.MaxLogsKept == nil {
		return DefaultMaxLogsKept
	}

	return *f.MaxLogsKept
}

func (f PollingFilter) GetStartingBlock() int64 {
	if f.StartingBlock == nil {
		return DefaultStartingBlock
	}

	return *f.StartingBlock
}

func (f PollingFilter) GetIncludeReverted() bool {
	if f.IncludeReverted == nil {
		return DefaultIncludeReverted
	}
	return *f.IncludeReverted
}

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
	codecv1.IDL    `json:"anchorIDL"`
	*PollingFilter `json:"pollingFilter,omitempty"`
	// Reads key is the off-chain name for this read.
	Reads map[string]ReadDefinition `json:"reads"`
}

type EventDefinitions struct {
	IndexedField0 *IndexedField `json:"indexedField0"`
	IndexedField1 *IndexedField `json:"indexedField1"`
	IndexedField2 *IndexedField `json:"indexedField2"`
	IndexedField3 *IndexedField `json:"indexedField3"`
	// PollingFilter should be defined on a contract level in ContractPollingFilter, unless event needs to override the
	// contract level filter options.
	*PollingFilter `json:"pollingFilter,omitempty"`
}

type MultiReader struct {
	// Reads is a list of reads that is sequentially read to fill out a complete response for the parent read.
	// Parent ReadDefinition has to define codec modifiers which adds fields that are to be filled out by the reads in Reads.
	Reads []ReadDefinition `json:"reads,omitempty"`
	// ReuseParams If true, params from parent read will be reused for all MultiReader Reads.
	ReuseParams bool `json:"reuseParams"`
}

type ReadDefinition struct {
	ChainSpecificName       string                      `json:"chainSpecificName"`
	ReadType                ReadType                    `json:"readType,omitempty"`
	ErrOnMissingAccountData bool                        `json:"errOnMissingAccountData,omitempty"`
	InputModifications      commoncodec.ModifiersConfig `json:"inputModifications,omitempty"`
	OutputModifications     commoncodec.ModifiersConfig `json:"outputModifications,omitempty"`
	PDADefinition           codecv1.PDATypeDef          `json:"pdaDefinition,omitempty"` // Only used for PDA account reads
	MultiReader             *MultiReader                `json:"multiReader,omitempty"`
	EventDefinitions        *EventDefinitions           `json:"eventDefinitions,omitempty"`
	// ResponseAddressHardCoder hardcodes the address of the contract into the defined field in the response.
	ResponseAddressHardCoder *commoncodec.HardCodeModifierConfig `json:"responseAddressHardCoder,omitempty"`
}

func (d ReadDefinition) HasPollingFilter() bool {
	return d.EventDefinitions != nil && d.EventDefinitions.PollingFilter != nil
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

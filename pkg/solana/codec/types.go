package codec

import commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"

type ChainConfigType string

const (
	ChainConfigTypeAccountDef     ChainConfigType = "account"
	ChainConfigTypeInstructionDef ChainConfigType = "instruction"
	ChainConfigTypeEventDef       ChainConfigType = "event"
)

type Config struct {
	// Configs key is the type's offChainName for the codec
	Configs map[string]ChainConfig `json:"configs" toml:"configs"`
}

type ChainConfig struct {
	IDL         string `json:"typeIdl" toml:"typeIdl"`
	OnChainName string `json:"onChainName" toml:"onChainName"`
	// Type can be Solana Account, Instruction args, or TODO Event
	Type            ChainConfigType             `json:"type" toml:"type"`
	ModifierConfigs commoncodec.ModifiersConfig `json:"modifierConfigs,omitempty" toml:"modifierConfigs,omitempty"`
}

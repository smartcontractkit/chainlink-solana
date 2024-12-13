package codec

import commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"

type ChainConfigType string

const (
	ChainConfigTypeAccountDef     ChainConfigType = "account"
	ChainConfigTypeInstructionDef ChainConfigType = "instruction"
	ChainConfigTypeEventDef       ChainConfigType = "event"
)

type Config struct {
	// Configs key is the type's name for the codec
	Configs map[string]ChainConfig `json:"configs" toml:"configs"`
}

type ChainConfig struct {
	IDL             string                      `json:"IDL" toml:"IDL"`
	IDLTypeName     string                      `json:"IDLTypeName" toml:"IDLTypeName"`
	Type            ChainConfigType             `json:"type" toml:"type"`
	ModifierConfigs commoncodec.ModifiersConfig `json:"modifierConfigs,omitempty" toml:"modifierConfigs,omitempty"`
}

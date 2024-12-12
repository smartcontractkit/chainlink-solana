package codec

import commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"

type Config struct {
	// Configs key is the type's name for the codec
	Configs map[string]ChainConfig `json:"configs" toml:"configs"`
}

type ChainConfig struct {
	IDL             string                      `json:"IDL" toml:"IDL"`
	AccountDef      *IdlTypeDef                 `json:"account" toml:"IDL"`
	EventDef        *IdlEvent                   `json:"event" toml:"IDL"`
	ModifierConfigs commoncodec.ModifiersConfig `json:"modifierConfigs,omitempty" toml:"modifierConfigs,omitempty"`
}

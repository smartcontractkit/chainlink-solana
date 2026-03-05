package devenv

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// EnvOutput captures the environment state after setup so that the test phase
// can run independently. Written to env-out.toml by the setup phase and read
// by the test/assertion phase.
type EnvOutput struct {
	OcrAddress     string `toml:"ocr_address"`
	FeedAddress    string `toml:"feed_address"`
	RPCURLExternal string `toml:"rpc_url_external"`
	WSURLExternal  string `toml:"ws_url_external"`
	GauntletPath   string `toml:"gauntlet_path"`
}

const DefaultEnvOutFile = "env-out.toml"

func (e *EnvOutput) Write(path string) error {
	b, err := toml.Marshal(e)
	if err != nil {
		return fmt.Errorf("failed to marshal env output: %w", err)
	}
	return os.WriteFile(path, b, 0644)
}

func LoadEnvOutput(path string) (*EnvOutput, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read env output file: %w", err)
	}
	var out EnvOutput
	if err := toml.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal env output: %w", err)
	}
	return &out, nil
}

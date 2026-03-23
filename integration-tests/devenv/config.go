// copy of chainlink/devenv/products/config.go
package devenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	DefaultConfigDir         = "."
	EnvVarTestConfigs        = "CTF_CONFIGS"
	DefaultOverridesFilePath = "overrides.toml"
)

var L = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).Level(zerolog.InfoLevel)

func Load[T any]() (*T, error) {
	var config T
	paths := strings.Split(os.Getenv(EnvVarTestConfigs), ",")
	for _, path := range paths {
		L.Info().Str("Path", path).Msg("Loading configuration input")
		data, err := os.ReadFile(filepath.Join(DefaultConfigDir, path))
		if err != nil {
			if path == DefaultOverridesFilePath {
				L.Info().Str("Path", path).Msg("Overrides file not found or empty")
				continue
			}
			return nil, fmt.Errorf("error reading config file %s: %w", path, err)
		}
		if L.GetLevel() == zerolog.TraceLevel {
			fmt.Println(string(data))
		}
		decoder := toml.NewDecoder(strings.NewReader(string(data)))
		if err := decoder.Decode(&config); err != nil {
			return nil, fmt.Errorf("failed to decode TOML config, strict mode: %w", err)
		}
	}
	return &config, nil
}

func Store[T any](cfg *T) error {
	baseConfigPath, err := BaseConfigPath()
	if err != nil {
		return err
	}
	newCacheName := strings.ReplaceAll(baseConfigPath, ".toml", "")
	var outCacheName string
	if strings.Contains(newCacheName, "cache") {
		L.Info().Str("Cache", baseConfigPath).Msg("Cache file already exists, overriding")
		outCacheName = baseConfigPath
	} else {
		outCacheName = strings.ReplaceAll(baseConfigPath, ".toml", "") + "-out.toml"
	}
	L.Info().Str("OutputFile", outCacheName).Msg("Storing configuration output")
	d, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(DefaultConfigDir, outCacheName), d, 0600)
}

func LoadOutput[T any](path string) (*T, error) {
	_ = os.Setenv(EnvVarTestConfigs, path)
	return Load[T]()
}

func BaseConfigPath() (string, error) {
	configs := os.Getenv(EnvVarTestConfigs)
	if configs == "" {
		return "", fmt.Errorf("no %s env var is provided, you should provide at least one test config in TOML", EnvVarTestConfigs)
	}
	L.Debug().Str("Configs", configs).Msg("Getting base config path")
	return strings.Split(configs, ",")[0], nil
}

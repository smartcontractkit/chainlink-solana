package testconfig

import (
	"embed"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/smartcontractkit/chainlink-testing-framework/docker/test_env"
	"github.com/smartcontractkit/seth"

	"github.com/barkimedes/go-deepcopy"
	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	ocr2_config "github.com/smartcontractkit/chainlink-solana/integration-tests/testconfig/ocr2"
	ctf_config "github.com/smartcontractkit/chainlink-testing-framework/config"
	k8s_config "github.com/smartcontractkit/chainlink-testing-framework/k8s/config"
	"github.com/smartcontractkit/chainlink-testing-framework/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/utils/osutil"
)

type TestConfig struct {
	ChainlinkImage        *ctf_config.ChainlinkImageConfig `toml:"ChainlinkImage"`
	Logging               *ctf_config.LoggingConfig        `toml:"Logging"`
	ChainlinkUpgradeImage *ctf_config.ChainlinkImageConfig `toml:"ChainlinkUpgradeImage"`
	Network               *ctf_config.NetworkConfig        `toml:"Network"`
	Common                *Common                          `toml:"Common"`
	OCR2                  *ocr2_config.Config              `toml:"OCR2"`
	SolanaConfig          *SolanaConfig                    `toml:"SolanaConfig"`
	ConfigurationName     string                           `toml:"-"`
}

func (c *TestConfig) GetLoggingConfig() *ctf_config.LoggingConfig {
	return c.Logging
}

func (c *TestConfig) GetPrivateEthereumNetworkConfig() *test_env.EthereumNetwork {
	return &test_env.EthereumNetwork{}
}

func (c *TestConfig) GetPyroscopeConfig() *ctf_config.PyroscopeConfig {
	return &ctf_config.PyroscopeConfig{}
}

func (c *TestConfig) GetSethConfig() *seth.Config {
	return nil
}

var embeddedConfigs embed.FS
var areConfigsEmbedded bool

func init() {
	embeddedConfigs = embeddedConfigsFs
}

// Saves Test Config to a local file
func (c *TestConfig) Save() (string, error) {
	filePath := fmt.Sprintf("test_config-%s.toml", uuid.New())

	content, err := toml.Marshal(*c)
	if err != nil {
		return "", errors.Wrapf(err, "error marshaling test config")
	}

	err = os.WriteFile(filePath, content, 0600)
	if err != nil {
		return "", errors.Wrapf(err, "error writing test config")
	}

	return filePath, nil
}

// MustCopy Returns a deep copy of the Test Config or panics on error
func (c TestConfig) MustCopy() any {
	return deepcopy.MustAnything(c).(TestConfig)
}

// MustCopy Returns a deep copy of struct passed to it and returns a typed copy (or panics on error)
func MustCopy[T any](c T) T {
	return deepcopy.MustAnything(c).(T)
}

func (c TestConfig) GetNetworkConfig() *ctf_config.NetworkConfig {
	return c.Network
}

func (c TestConfig) GetChainlinkImageConfig() *ctf_config.ChainlinkImageConfig {
	return c.ChainlinkImage
}

func (c TestConfig) GetCommonConfig() *Common {
	return c.Common
}

func (c TestConfig) GetChainlinkUpgradeImageConfig() *ctf_config.ChainlinkImageConfig {
	return c.ChainlinkUpgradeImage
}

func (c TestConfig) GetConfigurationName() string {
	return c.ConfigurationName
}

func (c *TestConfig) AsBase64() (string, error) {
	content, err := toml.Marshal(*c)
	if err != nil {
		return "", errors.Wrapf(err, "error marshaling test config")
	}

	return base64.StdEncoding.EncodeToString(content), nil
}

type Common struct {
	Network   *string `toml:"network"`
	InsideK8s *bool   `toml:"inside_k8"`
	User      *string `toml:"user"`
	// if rpc requires api key to be passed as an HTTP header
	RPC_URL            *string `toml:"rpc_url"`
	WS_URL             *string `toml:"ws_url"`
	PrivateKey         *string `toml:"private_key"`
	Stateful           *bool   `toml:"stateful_db"`
	InternalDockerRepo *string `toml:"internal_docker_repo"`
	DevnetImage        *string `toml:"devnet_image"`
}

type SolanaConfig struct {
	Secret                    *string `toml:"secret"`
	OCR2ProgramId             *string `toml:"ocr2_program_id"`
	AccessControllerProgramId *string `toml:"access_controller_program_id"`
	StoreProgramId            *string `toml:"store_program_id"`
	LinkTokenAddress          *string `toml:"link_token_address"`
	VaultAddress              *string `toml:"vault_address"`
}

func (c *SolanaConfig) Validate() error {
	if c.Secret == nil {
		return fmt.Errorf("secret must be set")
	}
	if c.OCR2ProgramId == nil {
		return fmt.Errorf("ocr2_program_id must be set")
	}
	if c.AccessControllerProgramId == nil {
		return fmt.Errorf("access_controller_program_id must be set")
	}
	if c.StoreProgramId == nil {
		return fmt.Errorf("store_program_id must be set")
	}
	if c.LinkTokenAddress == nil {
		return fmt.Errorf("link_token_address must be set")
	}
	if c.VaultAddress == nil {
		return fmt.Errorf("vault_address must be set")
	}
	return nil
}

func (c *Common) Validate() error {
	if c.Network == nil {
		return fmt.Errorf("network must be set")
	}

	switch *c.Network {
	case "localnet":
		if c.DevnetImage == nil {
			return fmt.Errorf("devnet_image must be set")
		}
	case "devnet":
		if c.PrivateKey == nil {
			return fmt.Errorf("private_key must be set")
		}
		if c.RPC_URL == nil {
			return fmt.Errorf("rpc_url must be set")
		}
		if c.WS_URL == nil {
			return fmt.Errorf("rpc_url must be set")
		}

	default:
		return fmt.Errorf("network must be either 'localnet' or 'devnet'")
	}

	if c.InsideK8s == nil {
		return fmt.Errorf("inside_k8 must be set")
	}

	if c.InternalDockerRepo == nil {
		return fmt.Errorf("internal_docker_repo must be set")
	}

	err := os.Setenv("INTERNAL_DOCKER_REPO", *c.InternalDockerRepo)
	if err != nil {
		return fmt.Errorf("could not set INTERNAL_DOCKER_REPO env var")
	}

	if c.User == nil {
		return fmt.Errorf("user must be set")
	}

	err = os.Setenv("CHAINLINK_ENV_USER", *c.User)
	if err != nil {
		return fmt.Errorf("could not set CHAINLINK_ENV_USER env var")
	}

	if c.Stateful == nil {
		return fmt.Errorf("stateful_db state for db must be set")
	}

	return nil
}

type Product string

const (
	OCR2 Product = "ocr2"
)

const TestTypeEnvVarName = "TEST_TYPE"

const (
	Base64OverrideEnvVarName = k8s_config.EnvBase64ConfigOverride
	NoKey                    = "NO_KEY"
)

func GetConfig(configurationName string, product Product) (TestConfig, error) {
	logger := logging.GetTestLogger(nil)

	configurationName = strings.ReplaceAll(configurationName, "/", "_")
	configurationName = strings.ReplaceAll(configurationName, " ", "_")
	configurationName = cases.Title(language.English, cases.NoLower).String(configurationName)
	fileNames := []string{
		"default.toml",
		fmt.Sprintf("%s.toml", product),
		"overrides.toml",
	}

	testConfig := TestConfig{}
	testConfig.ConfigurationName = configurationName
	logger.Debug().Msgf("Will apply configuration named '%s' if it is found in any of the configs", configurationName)

	var handleSpecialOverrides = func(logger zerolog.Logger, filename, configurationName string, target *TestConfig, content []byte, product Product) error {
		switch product {
		default:
			err := ctf_config.BytesToAnyTomlStruct(logger, filename, configurationName, &testConfig, content)
			if err != nil {
				return errors.Wrapf(err, "error reading file %s", filename)
			}

			return nil
		}
	}

	// read embedded configs is build tag "embed" is set
	// this makes our life much easier when using a binary
	if areConfigsEmbedded {
		logger.Info().Msg("Reading embedded configs")
		embeddedFiles := []string{"default.toml", fmt.Sprintf("%s/%s.toml", product, product)}
		for _, fileName := range embeddedFiles {
			file, err := embeddedConfigs.ReadFile(fileName)
			if err != nil && errors.Is(err, os.ErrNotExist) {
				logger.Debug().Msgf("Embedded config file %s not found. Continuing", fileName)
				continue
			} else if err != nil {
				return TestConfig{}, errors.Wrapf(err, "error reading embedded config")
			}

			err = handleSpecialOverrides(logger, fileName, configurationName, &testConfig, file, product)
			if err != nil {
				return TestConfig{}, errors.Wrapf(err, "error unmarshalling embedded config")
			}
		}
	}

	logger.Info().Msg("Reading configs from file system")
	for _, fileName := range fileNames {
		logger.Debug().Msgf("Looking for config file %s", fileName)
		filePath, err := osutil.FindFile(fileName, osutil.DEFAULT_STOP_FILE_NAME, 3)

		if err != nil && errors.Is(err, os.ErrNotExist) {
			logger.Debug().Msgf("Config file %s not found", fileName)
			continue
		} else if err != nil {
			return TestConfig{}, errors.Wrapf(err, "error looking for file %s", filePath)
		}
		logger.Debug().Str("location", filePath).Msgf("Found config file %s", fileName)

		content, err := readFile(filePath)
		if err != nil {
			return TestConfig{}, errors.Wrapf(err, "error reading file %s", filePath)
		}

		err = handleSpecialOverrides(logger, fileName, configurationName, &testConfig, content, product)
		if err != nil {
			return TestConfig{}, errors.Wrapf(err, "error reading file %s", filePath)
		}
	}

	logger.Info().Msg("Reading configs from Base64 override env var")
	configEncoded, isSet := os.LookupEnv(Base64OverrideEnvVarName)
	if isSet && configEncoded != "" {
		logger.Debug().Msgf("Found base64 config override environment variable '%s' found", Base64OverrideEnvVarName)
		decoded, err := base64.StdEncoding.DecodeString(configEncoded)
		if err != nil {
			return TestConfig{}, err
		}

		err = handleSpecialOverrides(logger, Base64OverrideEnvVarName, configurationName, &testConfig, decoded, product)
		if err != nil {
			return TestConfig{}, errors.Wrapf(err, "error unmarshaling base64 config")
		}
	} else {
		logger.Debug().Msg("Base64 config override from environment variable not found")
	}

	// it neede some custom logic, so we do it separately
	err := testConfig.readNetworkConfiguration()
	if err != nil {
		return TestConfig{}, errors.Wrapf(err, "error reading network config")
	}

	logger.Debug().Msg("Validating test config")
	err = testConfig.Validate()
	if err != nil {
		return TestConfig{}, errors.Wrapf(err, "error validating test config")
	}

	if testConfig.Common == nil {
		testConfig.Common = &Common{}
	}

	logger.Debug().Msg("Correct test config constructed successfully")
	return testConfig, nil
}

func (c *TestConfig) readNetworkConfiguration() error {
	// currently we need to read that kind of secrets only for network configuration
	if c == nil {
		c.Network = &ctf_config.NetworkConfig{}
	}

	c.Network.UpperCaseNetworkNames()
	err := c.Network.Default()
	if err != nil {
		return errors.Wrapf(err, "error reading default network config")
	}

	return nil
}

func (c *TestConfig) Validate() error {
	defer func() {
		if r := recover(); r != nil {
			panic(fmt.Errorf("Panic during test config validation: '%v'. Most probably due to presence of partial product config", r))
		}
	}()
	if c.ChainlinkImage == nil {
		return fmt.Errorf("chainlink image config must be set")
	}
	if err := c.ChainlinkImage.Validate(); err != nil {
		return errors.Wrapf(err, "chainlink image config validation failed")
	}
	if c.ChainlinkUpgradeImage != nil {
		if err := c.ChainlinkUpgradeImage.Validate(); err != nil {
			return errors.Wrapf(err, "chainlink upgrade image config validation failed")
		}
	}
	if err := c.Network.Validate(); err != nil {
		return errors.Wrapf(err, "network config validation failed")
	}

	if c.Common == nil {
		return fmt.Errorf("common config must be set")
	}

	if err := c.Common.Validate(); err != nil {
		return errors.Wrapf(err, "Common config validation failed")
	}

	if c.OCR2 == nil {
		return fmt.Errorf("OCR2 config must be set")
	}

	if err := c.OCR2.Validate(); err != nil {
		return errors.Wrapf(err, "OCR2 config validation failed")
	}
	if c.SolanaConfig == nil {
		return fmt.Errorf("SolanaConfig config must be set")
	}

	if err := c.SolanaConfig.Validate(); err != nil {
		return errors.Wrapf(err, "SolanaConfig config validation failed")
	}
	return nil
}

func readFile(filePath string) ([]byte, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading file %s", filePath)
	}

	return content, nil
}

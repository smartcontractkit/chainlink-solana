package solana

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
	tc "github.com/testcontainers/testcontainers-go"
	tcwait "github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/exp/slices"

	"github.com/smartcontractkit/chainlink-testing-framework/framework"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
)

const (
	SolHTTPPort = "8899"
	SolWSPort   = "8900"
)

var configYmlRaw = `
json_rpc_url: http://0.0.0.0:8899
websocket_url: ws://0.0.0.0:8900
keypair_path: /root/.config/solana/cli/id.json
address_labels:
  "11111111111111111111111111111111": ""
commitment: finalized
`

var idJSONRaw = `
[94,214,238,83,144,226,75,151,226,20,5,188,42,110,64,180,196,244,6,199,29,231,108,112,67,175,110,182,3,242,102,83,103,72,221,132,137,219,215,192,224,17,146,227,94,4,173,67,173,207,11,239,127,174,101,204,65,225,90,88,224,45,205,117]
`

type Input struct {
	Image      string  `toml:"image"`
	ChainID    string  `toml:"chain_id"`
	PublicKey  string  `toml:"-"`
	PrivateKey string  `toml:"-"`
	Secret     string  `toml:"secret"`
	Out        *Output `toml:"out"`
}

type Output struct {
	UseCache        bool         `toml:"use_cache"`
	ContainerName   string       `toml:"container_name"`
	ExternalHTTPURL string       `toml:"external_http_url"`
	InternalHTTPURL string       `toml:"internal_http_url"`
	ExternalWsURL   string       `toml:"external_ws_url"`
	InternalWsURL   string       `toml:"internal_ws_url"`
	Container       tc.Container `toml:"-"`
}

func NewSolana(ctx context.Context, image, publicKey string, cachedOut *Output) (*Output, error) {
	if cachedOut != nil && cachedOut.UseCache {
		framework.L.Info().Msg("Using cached Solana container")
		return cachedOut, nil
	}

	containerName := framework.DefaultTCName("solana")

	inactiveMainnetFeatures, err := GetInactiveFeatureHashes("mainnet-beta")
	if err != nil {
		return nil, err
	}

	cReq, err := getContainerRequest(containerName, image, publicKey, inactiveMainnetFeatures)
	if err != nil {
		return nil, err
	}

	c, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: *cReq,
		Reuse:            true,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot start Solana container: %w", err)
	}

	host, err := framework.GetHostWithContext(ctx, c)
	if err != nil {
		return nil, err
	}
	httpPort, err := c.MappedPort(ctx, nat.Port(fmt.Sprintf("%s/tcp", SolHTTPPort)))
	if err != nil {
		return nil, err
	}
	wsPort, err := c.MappedPort(ctx, nat.Port(fmt.Sprintf("%s/tcp", SolWSPort)))
	if err != nil {
		return nil, err
	}

	out := &Output{
		ContainerName:   containerName,
		ExternalHTTPURL: fmt.Sprintf("http://%s:%s", host, httpPort.Port()),
		InternalHTTPURL: fmt.Sprintf("http://%s:%s", containerName, SolHTTPPort),
		ExternalWsURL:   fmt.Sprintf("ws://%s:%s", host, wsPort.Port()),
		InternalWsURL:   fmt.Sprintf("ws://%s:%s", containerName, SolWSPort),
		Container:       c,
	}
	framework.L.Info().
		Str("ExternalHTTPURL", out.ExternalHTTPURL).
		Str("InternalHTTPURL", out.InternalHTTPURL).
		Str("ExternalWsURL", out.ExternalWsURL).
		Str("InternalWsURL", out.InternalWsURL).
		Str("ContainerName", out.ContainerName).
		Msg("Started Solana container")

	inactiveLocalFeatures, err := GetInactiveFeatureHashes(out.ExternalHTTPURL)
	if err != nil {
		return nil, err
	}
	if !slices.Equal(inactiveMainnetFeatures, inactiveLocalFeatures) {
		return nil, fmt.Errorf("localnet features does not match mainnet features")
	}
	return out, nil
}

func getContainerRequest(containerName, image, publicKey string, inactiveFeatures InactiveFeatures) (*tc.ContainerRequest, error) {
	configYml, err := os.CreateTemp("", "config.yml")
	if err != nil {
		return nil, err
	}
	_, err = configYml.WriteString(configYmlRaw)
	if err != nil {
		return nil, err
	}

	idJSON, err := os.CreateTemp("", "id.json")
	if err != nil {
		return nil, err
	}
	_, err = idJSON.WriteString(idJSONRaw)
	if err != nil {
		return nil, err
	}

	return &tc.ContainerRequest{
		Name:  containerName,
		Image: image,
		ExposedPorts: []string{
			fmt.Sprintf("%s/tcp", SolHTTPPort),
			fmt.Sprintf("%s/tcp", SolWSPort),
		},
		Env: map[string]string{
			"SERVER_PORT": "1080",
		},
		Networks: []string{framework.DefaultNetworkName},
		NetworkAliases: map[string][]string{
			framework.DefaultNetworkName: {containerName},
		},
		Labels: framework.DefaultTCLabels(),
		WaitingFor: tcwait.ForLog("Processed Slot:").
			WithStartupTimeout(30 * time.Second).
			WithPollInterval(100 * time.Millisecond),
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   utils.ContractsDir,
				Target:   "/programs",
				ReadOnly: false,
			})
		},
		LifecycleHooks: []tc.ContainerLifecycleHooks{
			{
				PostStarts: []tc.ContainerHook{
					func(ctx context.Context, container tc.Container) error {
						err = container.CopyFileToContainer(ctx, configYml.Name(), "/root/.config/solana/cli/config.yml", 0644)
						if err != nil {
							return err
						}
						err = container.CopyFileToContainer(ctx, idJSON.Name(), "/root/.config/solana/cli/id.json", 0644)
						return err
					},
				},
			},
		},
		Entrypoint: []string{"sh", "-c", "mkdir -p /root/.config/solana/cli && solana-test-validator -r --mint=" + publicKey + " " + inactiveFeatures.CLIString()},
	}, nil
}

// FindSolanaByName looks up a running Solana container by its Docker name
// and returns an Output with the same external/internal URLs and container ref.
func FindSolanaByName(ctx context.Context, name string) (*Output, error) {
	c, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: tc.ContainerRequest{Name: name},
		Reuse:            true,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find solana container %q: %w", name, err)
	}

	host, err := framework.GetHostWithContext(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("failed to get host for container %q: %w", name, err)
	}
	httpPort, err := c.MappedPort(ctx, nat.Port(fmt.Sprintf("%s/tcp", SolHTTPPort)))
	if err != nil {
		return nil, fmt.Errorf("failed to get http port for container %q: %w", name, err)
	}
	wsPort, err := c.MappedPort(ctx, nat.Port(fmt.Sprintf("%s/tcp", SolWSPort)))
	if err != nil {
		return nil, fmt.Errorf("failed to get ws port for container %q: %w", name, err)
	}

	return &Output{
		ContainerName:   name,
		ExternalHTTPURL: fmt.Sprintf("http://%s:%s", host, httpPort.Port()),
		InternalHTTPURL: fmt.Sprintf("http://%s:%s", name, SolHTTPPort),
		ExternalWsURL:   fmt.Sprintf("ws://%s:%s", host, wsPort.Port()),
		InternalWsURL:   fmt.Sprintf("ws://%s:%s", name, SolWSPort),
		Container:       c,
	}, nil
}

type FeatureStatuses struct {
	Features []FeatureStatus
}

type FeatureStatus struct {
	ID          string
	Description string
	Status      string
	SinceSlot   int
}

type InactiveFeatures []string

func (f InactiveFeatures) CLIString() string {
	return "--deactivate-feature=" + strings.Join(f, " --deactivate-feature=")
}

// GetInactiveFeatureHashes uses the solana CLI to fetch inactive solana features
func GetInactiveFeatureHashes(url string) (output InactiveFeatures, err error) {
	cmd := exec.Command("solana", "feature", "status", "-u="+url, "--output=json") //nolint:gosec
	stdout, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Failed to get feature status: %w", err)
	}

	statuses := FeatureStatuses{}
	if err = json.Unmarshal(stdout, &statuses); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal feature status: %w", err)
	}

	for _, f := range statuses.Features {
		if f.Status == "inactive" {
			output = append(output, f.ID)
		}
	}

	slices.Sort(output)
	return output, err
}

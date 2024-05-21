package test_env

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	tc "github.com/testcontainers/testcontainers-go"
	tcwait "github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/exp/slices"

	"github.com/smartcontractkit/chainlink-testing-framework/docker/test_env"
	"github.com/smartcontractkit/chainlink-testing-framework/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/utils/testcontext"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
)

const (
	SOL_HTTP_PORT = "8899"
	SOL_WS_PORT   = "8900"
)

var config_yml = `
json_rpc_url: http://0.0.0.0:8899
websocket_url: ws://0.0.0.0:8900
keypair_path: /root/.config/solana/cli/id.json
address_labels:
  "11111111111111111111111111111111": ""
commitment: finalized
`

var id_json = `
[205,246,252,222,193,57,3,13,164,146,52,162,143,135,8,254,37,4,250,48,137,61,49,57,187,210,209,118,108,125,81,235,136,69,202,17,24,209,91,226,206,92,80,45,83,14,222,113,229,190,94,142,188,124,102,122,15,246,40,190,24,247,69,133]
`

type Solana struct {
	test_env.EnvComponent
	ExternalHttpUrl string
	ExternalWsUrl   string
	InternalHttpUrl string
	InternalWsUrl   string
	t               *testing.T
	l               zerolog.Logger
}

func NewSolana(networks []string, opts ...test_env.EnvComponentOption) *Solana {
	ms := &Solana{
		EnvComponent: test_env.EnvComponent{
			ContainerName: fmt.Sprintf("%s-%s", "solana", uuid.NewString()[0:8]),
			Networks:      networks,
		},
		l: log.Logger,
	}
	for _, opt := range opts {
		opt(&ms.EnvComponent)
	}
	return ms
}

func (s *Solana) WithTestLogger(t *testing.T) *Solana {
	s.l = logging.GetTestLogger(t)
	s.t = t
	return s
}

func (s *Solana) StartContainer() error {
	l := tc.Logger
	if s.t != nil {
		l = logging.CustomT{
			T: s.t,
			L: s.l,
		}
	}

	// get disabled/unreleased features on mainnet
	inactiveMainnetFeatures, err := GetInactiveFeatureHashes("mainnet-beta")
	if err != nil {
		return err
	}

	cReq, err := s.getContainerRequest(inactiveMainnetFeatures)
	if err != nil {
		return err
	}
	c, err := tc.GenericContainer(testcontext.Get(s.t), tc.GenericContainerRequest{
		ContainerRequest: *cReq,
		Reuse:            true,
		Started:          true,
		Logger:           l,
	})
	if err != nil {
		return fmt.Errorf("cannot start Solana container: %w", err)
	}
	s.Container = c
	host, err := test_env.GetHost(testcontext.Get(s.t), c)
	if err != nil {
		return err
	}
	httpPort, err := c.MappedPort(testcontext.Get(s.t), test_env.NatPort(SOL_HTTP_PORT))
	if err != nil {
		return err
	}
	wsPort, err := c.MappedPort(testcontext.Get(s.t), test_env.NatPort(SOL_WS_PORT))
	if err != nil {
		return err
	}
	s.ExternalHttpUrl = fmt.Sprintf("http://%s:%s", host, httpPort.Port())
	s.InternalHttpUrl = fmt.Sprintf("http://%s:%s", s.ContainerName, SOL_HTTP_PORT)
	s.ExternalWsUrl = fmt.Sprintf("ws://%s:%s", host, wsPort.Port())
	s.InternalWsUrl = fmt.Sprintf("ws://%s:%s", s.ContainerName, SOL_WS_PORT)

	s.l.Info().
		Any("ExternalHttpUrl", s.ExternalHttpUrl).
		Any("InternalHttpUrl", s.InternalHttpUrl).
		Any("ExternalWsUrl", s.ExternalWsUrl).
		Any("InternalWsUrl", s.InternalWsUrl).
		Str("containerName", s.ContainerName).
		Msgf("Started Solana container")

	// validate features are properly set
	inactiveLocalFeatures, err := GetInactiveFeatureHashes(s.ExternalHttpUrl)
	if err != nil {
		return err
	}
	if !slices.Equal(inactiveMainnetFeatures, inactiveLocalFeatures) {
		return fmt.Errorf("Localnet features does not match mainnet features")
	}
	return nil
}

func (ms *Solana) getContainerRequest(inactiveFeatures InactiveFeatures) (*tc.ContainerRequest, error) {
	configYml, err := os.CreateTemp("", "config.yml")
	if err != nil {
		return nil, err
	}
	_, err = configYml.WriteString(config_yml)
	if err != nil {
		return nil, err
	}

	idJson, err := os.CreateTemp("", "id.json")
	if err != nil {
		return nil, err
	}
	_, err = idJson.WriteString(id_json)
	if err != nil {
		return nil, err
	}

	return &tc.ContainerRequest{
		Name:         ms.ContainerName,
		Image:        "solanalabs/solana:v1.17.34",
		ExposedPorts: []string{test_env.NatPortFormat(SOL_HTTP_PORT), test_env.NatPortFormat(SOL_WS_PORT)},
		Env: map[string]string{
			"SERVER_PORT": "1080",
		},
		Networks: ms.Networks,
		WaitingFor: tcwait.ForLog("Processed Slot: 1").
			WithStartupTimeout(30 * time.Second).
			WithPollInterval(100 * time.Millisecond),
		Mounts: tc.ContainerMounts{
			tc.ContainerMount{
				Source: tc.GenericBindMountSource{
					HostPath: utils.ContractsDir,
				},
				Target: "/programs",
			},
		},
		LifecycleHooks: []tc.ContainerLifecycleHooks{
			{
				PostStarts: []tc.ContainerHook{
					func(ctx context.Context, container tc.Container) error {
						err = container.CopyFileToContainer(ctx, configYml.Name(), "/root/.config/solana/cli/config.yml", 0644)
						if err != nil {
							return err
						}
						err = container.CopyFileToContainer(ctx, idJson.Name(), "/root/.config/solana/cli/id.json", 0644)
						return err
					},
				},
			},
		},
		Entrypoint: []string{"sh", "-c", "mkdir -p /root/.config/solana/cli && solana-test-validator -r --mint=AAxAoGfkbWnbgsiQeAanwUvjv6bQrM5JS8Vxv1ckzVxg " + inactiveFeatures.CLIString()},
	}, nil
}

type FeatureStatuses struct {
	Features []FeatureStatus
	// note: there are other unused params in the json response
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
// This is used in conjunction with the solana-test-validator command to produce a solana network that has the same features as mainnet
// the solana-test-validator has all features on by default (released + unreleased)
func GetInactiveFeatureHashes(url string) (output InactiveFeatures, err error) {
	cmd := exec.Command("solana", "feature", "status", "-u="+url, "--output=json") // -um is for mainnet url
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

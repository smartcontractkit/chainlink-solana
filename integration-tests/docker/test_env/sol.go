package test_env

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	tc "github.com/testcontainers/testcontainers-go"
	tcwait "github.com/testcontainers/testcontainers-go/wait"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
	"github.com/smartcontractkit/chainlink-testing-framework/docker/test_env"
	"github.com/smartcontractkit/chainlink-testing-framework/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/utils/testcontext"
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
	cReq, err := s.getContainerRequest()
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

	return nil
}

func (ms *Solana) getContainerRequest() (*tc.ContainerRequest, error) {
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
		Image:        "solanalabs/solana:v1.17.22",
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
		Entrypoint: []string{"sh", "-c", "mkdir -p /root/.config/solana/cli && solana-test-validator -r --mint AAxAoGfkbWnbgsiQeAanwUvjv6bQrM5JS8Vxv1ckzVxg"},
	}, nil
}

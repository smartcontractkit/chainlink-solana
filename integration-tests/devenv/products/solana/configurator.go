package solana

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/guregu/null.v4"

	"github.com/smartcontractkit/chainlink-testing-framework/framework/clclient"
	ns "github.com/smartcontractkit/chainlink-testing-framework/framework/components/simple_node_set"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/components/solana"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv/products"

	ocr_config "github.com/smartcontractkit/chainlink-solana/integration-tests/config"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/gauntlet"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
)

var L = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).Level(zerolog.InfoLevel).With().Fields(map[string]any{"component": "ocr2_solana"}).Logger()

type Configurator struct {
	Config []*OCR2Solana `toml:"ocr2_solana"`
}

func NewConfigurator() *Configurator {
	return &Configurator{}
}

func (m *Configurator) Load() error {
	cfg, err := products.Load[Configurator]()
	if err != nil {
		return fmt.Errorf("failed to load product config: %w", err)
	}
	m.Config = cfg.Config
	return nil
}

func (m *Configurator) Store(_ string, _ int) error {
	return products.Store(".", m)
}

var solanaNodeConfigTmpl = template.Must(template.New("solana-cl-config").Parse(`
[Log]
Level = 'debug'

[WebServer]
HTTPPort = 6688
SecureCookies = false
SessionTimeout = '999h0m0s'
[WebServer.TLS]
HTTPSPort = 0
[WebServer.RateLimit]
Authenticated = 2000
Unauthenticated = 100

[Feature]
FeedsManager = true
LogPoller = true
UICSAKeys = true

[OCR2]
Enabled = true

[P2P.V2]
Enabled = true
DeltaDial = '5s'
DeltaReconcile = '5s'
ListenAddresses = ['0.0.0.0:6690']

[[Solana]]
Enabled = true
ChainID = '{{ .ChainID }}'
TxTimeout = '2m0s'

[Solana.MultiNode]
Enabled = true
SyncThreshold = 170
VerifyChainID = false
{{ range $i, $url := .RPCNodes }}
[[Solana.Nodes]]
Name = 'primary-{{ $i }}'
URL = '{{ $url }}'
{{ end }}
`))

func (m *Configurator) GenerateNodesConfig(
	_ context.Context,
	sol *solana.Input,
	_ []*ns.Input,
) (string, error) {
	L.Info().Msg("Generating Solana CL node configuration")

	var buf bytes.Buffer
	err := solanaNodeConfigTmpl.Execute(&buf, struct {
		ChainID  string
		RPCNodes []string
	}{
		ChainID:  sol.ChainID,
		RPCNodes: []string{sol.Out.InternalHTTPURL},
	})
	if err != nil {
		return "", fmt.Errorf("failed to render node config template: %w", err)
	}
	return buf.String(), nil
}

func (m *Configurator) GenerateNodesSecrets(
	_ context.Context,
	_ *solana.Input,
	_ []*ns.Input,
) (string, error) {
	return "", nil
}

// ConfigureJobsAndContracts deploys OCR2 contracts via Gauntlet and creates
// CL node jobs. Anchor program binaries are already deployed by NewEnvironment.
func (m *Configurator) ConfigureJobsAndContracts(
	ctx context.Context,
	_ int,
	sol *solana.Input,
	fakesURL string,
	nodeSets []*ns.Input,
) error {
	L.Info().Msg("Configuring OCR2 Solana jobs and contracts")
	cfg := m.Config[0]

	cl, err := clclient.New(nodeSets[0].Out.CLNodes)
	if err != nil {
		return fmt.Errorf("failed to create CL clients: %w", err)
	}
	for i, c := range cl {
		c.Config.InternalIP = nodeSets[0].Out.CLNodes[i].Node.InternalIP
	}

	nKeys, err := createNodeKeysBundle(cl, "solana", sol.ChainID)
	if err != nil {
		return fmt.Errorf("failed to create node keys: %w", err)
	}

	gauntletCopyPath := utils.ProjectRoot + "/gauntlet-ocr2-setup"
	if out, cpErr := exec.Command("cp", "-r", utils.ProjectRoot+"/gauntlet", gauntletCopyPath).Output(); cpErr != nil { //nolint:gosec
		return fmt.Errorf("failed to copy gauntlet: %s: %w", string(out), cpErr)
	}

	sg, err := gauntlet.NewSolanaGauntlet(gauntletCopyPath)
	if err != nil {
		return fmt.Errorf("failed to create gauntlet: %w", err)
	}
	if cfg.GauntletNetwork != "" {
		sg.G.Network = cfg.GauntletNetwork
	}

	gauntletConfig := map[string]string{
		"SECRET":      fmt.Sprintf("\"%s\"", sol.Secret),
		"NODE_URL":    sol.Out.ExternalHTTPURL,
		"WS_URL":      sol.Out.ExternalWsURL,
		"PRIVATE_KEY": sol.PrivateKey,
	}
	if err := sg.SetupNetwork(gauntletConfig); err != nil {
		return fmt.Errorf("failed to setup gauntlet network: %w", err)
	}
	if err := sg.InstallDependencies(); err != nil {
		return fmt.Errorf("failed to install gauntlet dependencies: %w", err)
	}

	if cfg.ProgramAddresses == nil {
		cfg.ProgramAddresses = &ProgramAddresses{
			OCR2:             "E3j24rx12SyVsG6quKuZPbQqZPkhAUCh8Uek4XrKYD2x",
			AccessController: "2ckhep7Mvy1dExenBqpcdevhRu7CLuuctMcx7G9mWEvo",
			Store:            "9kRNTZmoZSiTBuXC62dzK9E7gC7huYgcmRRhYv3i4osC",
		}
	}

	if err := sg.DeployLinkToken(); err != nil {
		return fmt.Errorf("failed to deploy link token: %w", err)
	}

	if err := sg.G.WriteNetworkConfigVar(sg.NetworkFilePath, "PROGRAM_ID_OCR2", cfg.ProgramAddresses.OCR2); err != nil {
		return fmt.Errorf("failed to write PROGRAM_ID_OCR2: %w", err)
	}
	if err := sg.G.WriteNetworkConfigVar(sg.NetworkFilePath, "PROGRAM_ID_ACCESS_CONTROLLER", cfg.ProgramAddresses.AccessController); err != nil {
		return fmt.Errorf("failed to write PROGRAM_ID_ACCESS_CONTROLLER: %w", err)
	}
	if err := sg.G.WriteNetworkConfigVar(sg.NetworkFilePath, "PROGRAM_ID_STORE", cfg.ProgramAddresses.Store); err != nil {
		return fmt.Errorf("failed to write PROGRAM_ID_STORE: %w", err)
	}
	if err := sg.G.WriteNetworkConfigVar(sg.NetworkFilePath, "LINK", sg.LinkAddress); err != nil {
		return fmt.Errorf("failed to write LINK: %w", err)
	}
	if err := sg.G.WriteNetworkConfigVar(sg.NetworkFilePath, "VAULT_ADDRESS", sg.VaultAddress); err != nil {
		return fmt.Errorf("failed to write VAULT_ADDRESS: %w", err)
	}

	if _, err := sg.DeployOCR2(); err != nil {
		return fmt.Errorf("failed to deploy OCR2: %w", err)
	}

	ocr2Config := ocr_config.NewOCR2Config(nKeys, sg.ProposalAddress, sg.VaultAddress, sol.Secret)
	ocr2Config.Default()
	sg.OCR2Config = ocr2Config
	if err := sg.ConfigureOCR2(); err != nil {
		return fmt.Errorf("failed to configure OCR2: %w", err)
	}

	if err := m.createJobs(sol, fakesURL, cl, nKeys, sg); err != nil {
		return fmt.Errorf("failed to create jobs: %w", err)
	}

	cfg.OcrAddress = sg.OcrAddress
	cfg.FeedAddress = sg.FeedAddress
	cfg.LinkAddress = sg.LinkAddress
	cfg.VaultAddress = sg.VaultAddress
	cfg.ProposalAddress = sg.ProposalAddress
	cfg.GauntletPath = gauntletCopyPath

	L.Info().
		Str("OcrAddress", cfg.OcrAddress).
		Str("FeedAddress", cfg.FeedAddress).
		Msg("OCR2 Solana deployment complete")
	return nil
}

func (m *Configurator) createJobs(
	sol *solana.Input,
	fakesURL string,
	cl []*clclient.ChainlinkClient,
	nKeys []clclient.NodeKeysBundle,
	sg *gauntlet.SolanaGauntlet,
) error {
	cfg := m.Config[0]
	relayConfig := JSONConfig{
		"nodeEndpointHTTP": []string{sol.Out.InternalHTTPURL},
		"ocr2ProgramID":    cfg.ProgramAddresses.OCR2,
		"transmissionsID":  sg.FeedAddress,
		"storeProgramID":   cfg.ProgramAddresses.Store,
		"chainID":          sol.ChainID,
	}

	bootstrapInternalIP := cl[0].InternalIP()
	bootstrapPeers := []clclient.P2PData{
		{
			InternalIP:   bootstrapInternalIP,
			InternalPort: "6690",
			PeerID:       nKeys[0].PeerID,
		},
	}

	mockBridgeURL := fmt.Sprintf("%s/%s", fakesURL, "mockserver-bridge")
	sourceValueBridge := clclient.BridgeTypeAttributes{
		Name:        "mockserver-bridge",
		URL:         mockBridgeURL,
		RequestData: "{}",
	}
	observationSource := clclient.ObservationSourceSpecBridge(&sourceValueBridge)

	bootstrapSpec := &TaskJobSpec{
		Name:    fmt.Sprintf("sol-OCRv2-%s-%s", "bootstrap", uuid.New().String()),
		JobType: "bootstrap",
		OCR2OracleSpec: OracleSpec{
			ContractID:                        sg.OcrAddress,
			Relay:                             "solana",
			RelayConfig:                       relayConfig,
			P2PV2Bootstrappers:                pq.StringArray{bootstrapPeers[0].P2PV2Bootstrapper()},
			OCRKeyBundleID:                    null.StringFrom(nKeys[0].OCR2Key.Data.ID),
			TransmitterID:                     null.StringFrom(nKeys[0].TXKey.Data.ID),
			ContractConfigConfirmations:       1,
			ContractConfigTrackerPollInterval: *NewInterval(15 * time.Second),
		},
	}

	if err := cl[0].MustCreateBridge(&sourceValueBridge); err != nil {
		return fmt.Errorf("failed to create bridge on bootstrap: %w", err)
	}
	if _, err := cl[0].MustCreateJob(bootstrapSpec); err != nil {
		return fmt.Errorf("failed to create bootstrap job: %w", err)
	}

	solClient := &solclient.Client{}
	solClient.Config = solClient.Config.Default()
	solClient.Config.URLs = []string{sol.Out.ExternalHTTPURL, sol.Out.ExternalWsURL}
	solClient, err := solclient.NewClient(solClient.Config)
	if err != nil {
		return fmt.Errorf("failed to create solana client for funding: %w", err)
	}

	for nIdx := 1; nIdx < len(cl); nIdx++ {
		if err := solClient.Fund(nKeys[nIdx].TXKey.Data.ID, big.NewFloat(1e4)); err != nil {
			return fmt.Errorf("failed to fund node %d: %w", nIdx, err)
		}

		workerBridge := clclient.BridgeTypeAttributes{
			Name:        "mockserver-bridge",
			URL:         mockBridgeURL,
			RequestData: "{}",
		}
		if _, err := cl[nIdx].CreateBridge(&workerBridge); err != nil {
			return fmt.Errorf("failed to create bridge on node %d: %w", nIdx, err)
		}

		pluginConfig := JSONConfig{
			"juelsPerFeeCoinSource": fmt.Sprintf("\"\"\"\n%s\n\"\"\"", observationSource),
		}

		workerSpec := &TaskJobSpec{
			Name:              fmt.Sprintf("sol-OCRv2-%d-%s", nIdx, uuid.New().String()),
			JobType:           "offchainreporting2",
			ObservationSource: observationSource,
			OCR2OracleSpec: OracleSpec{
				ContractID:                        sg.OcrAddress,
				Relay:                             "solana",
				RelayConfig:                       relayConfig,
				P2PV2Bootstrappers:                pq.StringArray{bootstrapPeers[0].P2PV2Bootstrapper()},
				OCRKeyBundleID:                    null.StringFrom(nKeys[nIdx].OCR2Key.Data.ID),
				TransmitterID:                     null.StringFrom(nKeys[nIdx].TXKey.Data.ID),
				ContractConfigConfirmations:       1,
				ContractConfigTrackerPollInterval: *NewInterval(15 * time.Second),
				PluginType:                        "median",
				PluginConfig:                      pluginConfig,
			},
		}
		if _, err := cl[nIdx].MustCreateJob(workerSpec); err != nil {
			return fmt.Errorf("failed to create job on node %d: %w", nIdx, err)
		}
	}
	return nil
}

func createNodeKeysBundle(clients []*clclient.ChainlinkClient, chainName, chainID string) ([]clclient.NodeKeysBundle, error) {
	nkb := make([]clclient.NodeKeysBundle, 0, len(clients))
	for _, n := range clients {
		p2pkeys, err := n.MustReadP2PKeys()
		if err != nil {
			return nil, err
		}
		peerID := p2pkeys.Data[0].Attributes.PeerID
		txKey, _, err := n.CreateTxKey(chainName, chainID)
		if err != nil {
			return nil, err
		}
		ocrKey, _, err := n.CreateOCR2Key(chainName)
		if err != nil {
			return nil, err
		}
		nkb = append(nkb, clclient.NodeKeysBundle{
			PeerID:  peerID,
			OCR2Key: *ocrKey,
			TXKey:   *txKey,
		})
	}
	return nkb, nil
}

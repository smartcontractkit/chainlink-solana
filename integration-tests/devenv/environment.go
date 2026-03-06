package devenv

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	solanago "github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-testing-framework/framework"
	ns "github.com/smartcontractkit/chainlink-testing-framework/framework/components/simple_node_set"
	testenvctf "github.com/smartcontractkit/chainlink-testing-framework/lib/docker/test_env"
	"github.com/smartcontractkit/chainlink-testing-framework/parrot"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv/products/solana"
	testenvsol "github.com/smartcontractkit/chainlink-solana/integration-tests/docker/testenv"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/solclient"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
)

func newProduct(name string) (Product, error) {
	switch name {
	case "ocr2_solana":
		return solana.NewConfigurator(), nil
	default:
		return nil, fmt.Errorf("unknown product type: %s", name)
	}
}

// NewEnvironment mirrors devenv/environment.go NewEnvironment() phase-by-phase.
func NewEnvironment(ctx context.Context) error {
	// Phase 1: Docker network
	if err := framework.DefaultNetwork(nil); err != nil {
		return err
	}

	// Phase 2: Load config
	in, err := Load[Cfg]()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Derive localnet keys if not provided
	if in.Solana.PublicKey == "" || in.Solana.PrivateKey == "" {
		pk, pkErr := solanago.PrivateKeyFromBase58(solclient.DefaultPrivateKeysSolValidator[1])
		if pkErr != nil {
			return fmt.Errorf("failed to decode default localnet private key: %w", pkErr)
		}
		in.Solana.PublicKey = pk.PublicKey().String()
		in.Solana.PrivateKey = fmt.Sprintf("[%s]", formatBuffer([]byte(pk)))
	}

	// Phase 3: Start Solana container (replaces blockchain.NewBlockchainNetwork)
	if in.Solana.Image == "" {
		in.Solana.Image = "anzaxyz/agave:v2.1.21"
	}
	sol := testenvsol.NewSolana(
		[]string{framework.DefaultNetworkName},
		in.Solana.Image,
		in.Solana.PublicKey,
	)
	if err := sol.StartContainer(); err != nil {
		return fmt.Errorf("failed to start solana container: %w", err)
	}
	in.Solana.Out = &solana.SolanaOutput{
		InternalHTTPURL: sol.InternalHTTPURL,
		ExternalHTTPURL: sol.ExternalHTTPURL,
		ExternalWsURL:   sol.ExternalWsURL,
		ContainerName:   sol.ContainerName,
	}

	// Deploy anchor program binaries (infra-level, before product configurators)
	solClient := &solclient.Client{}
	solClient.Config = solClient.Config.Default()
	solClient.Config.URLs = []string{sol.ExternalHTTPURL, sol.ExternalWsURL}
	solClient, err = solclient.NewClient(solClient.Config)
	if err != nil {
		return fmt.Errorf("failed to create solana client: %w", err)
	}
	cd, err := solclient.NewContractDeployer(solClient, nil)
	if err != nil {
		return fmt.Errorf("failed to create contract deployer: %w", err)
	}
	if err := cd.DeployAnchorProgramsRemoteDocker(utils.ContractsDir, "", sol, solclient.BuildProgramIDKeypairPath); err != nil {
		return fmt.Errorf("failed to deploy anchor programs: %w", err)
	}

	// Phase 4: Start Parrot (replaces fake.NewDockerFakeDataProvider)
	mockAdapter := testenvctf.NewParrot([]string{framework.DefaultNetworkName})
	if err := mockAdapter.StartContainer(); err != nil {
		return fmt.Errorf("failed to start parrot: %w", err)
	}
	if in.Parrot == nil {
		in.Parrot = &solana.ParrotInput{}
	}
	in.Parrot.Out = &solana.ParrotOutput{
		InternalEndpoint: mockAdapter.InternalEndpoint,
		ExternalEndpoint: mockAdapter.ExternalEndpoint,
	}
	if err := setupParrotRoutes(mockAdapter); err != nil {
		return fmt.Errorf("failed to setup parrot routes: %w", err)
	}

	// Phase 5: Product configurators -- generate node config overrides
	productConfigurators := make([]Product, 0)
	clNodeProductConfigOverrides := make([]string, 0)
	clNodeProductSecretsOverrides := make([]string, 0)
	for _, product := range in.Products {
		p, err := newProduct(product.Name)
		if err != nil {
			return err
		}
		if err := p.Load(); err != nil {
			return fmt.Errorf("failed to load product config: %w", err)
		}

		configOverrides, err := p.GenerateNodesConfig(ctx, in.Solana, in.Parrot, in.NodeSets)
		if err != nil {
			return fmt.Errorf("failed to generate CL nodes config: %w", err)
		}

		secretsOverrides, err := p.GenerateNodesSecrets(ctx, in.Solana, in.Parrot, in.NodeSets)
		if err != nil {
			return fmt.Errorf("failed to generate CL nodes secrets: %w", err)
		}

		productConfigurators = append(productConfigurators, p)
		clNodeProductConfigOverrides = append(clNodeProductConfigOverrides, configOverrides)
		clNodeProductSecretsOverrides = append(clNodeProductSecretsOverrides, secretsOverrides)
	}

	// Phase 6: Apply config overrides to node specs, start node sets
	nodeSet := in.NodeSets[0]
	for _, spec := range nodeSet.NodeSpecs {
		spec.Node.TestConfigOverrides = strings.Join(clNodeProductConfigOverrides, "\n")
		spec.Node.TestSecretsOverrides = strings.Join(clNodeProductSecretsOverrides, "\n")
		if os.Getenv("CHAINLINK_IMAGE") != "" {
			spec.Node.Image = os.Getenv("CHAINLINK_IMAGE")
		}
	}

	if _, err := ns.NewSharedDBNodeSet(nodeSet, nil); err != nil {
		return fmt.Errorf("failed to create new shared db node set: %w", err)
	}

	// Phase 7: Store infra state
	if err := Store(in); err != nil {
		return err
	}

	// Phase 8: Deploy products
	for productIdx, productInfo := range in.Products {
		for productInstance := range productInfo.Instances {
			if err := productConfigurators[productIdx].ConfigureJobsAndContracts(
				ctx,
				productInstance,
				in.Solana,
				in.Parrot,
				in.NodeSets,
			); err != nil {
				return fmt.Errorf("failed to setup product deployment: %w", err)
			}
			if err := productConfigurators[productIdx].Store("env-out.toml", productInstance); err != nil {
				return fmt.Errorf("failed to store product config: %w", err)
			}
		}
	}

	L.Info().Str("BootstrapNode", in.NodeSets[0].Out.CLNodes[0].Node.ExternalURL).Send()
	for _, n := range in.NodeSets[0].Out.CLNodes[1:] {
		L.Info().Str("Node", n.Node.ExternalURL).Send()
	}
	return nil
}

func setupParrotRoutes(p interface{ SetAdapterRoute(route *parrot.Route) error }) error {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		if err := p.SetAdapterRoute(&parrot.Route{
			Path:               "/mockserver-bridge",
			Method:             method,
			ResponseBody:       5,
			ResponseStatusCode: http.StatusOK,
		}); err != nil {
			return fmt.Errorf("failed to set parrot route %s: %w", method, err)
		}
	}
	return nil
}

func formatBuffer(buf []byte) string {
	if len(buf) == 0 {
		return ""
	}
	result := fmt.Sprintf("%d", buf[0])
	for _, b := range buf[1:] {
		result += fmt.Sprintf(",%d", b)
	}
	return result
}

package smoke

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/solclient"
	"github.com/smartcontractkit/helmenv/environment"
	"github.com/smartcontractkit/helmenv/tools"
	"github.com/smartcontractkit/integrations-framework/actions"
	"github.com/smartcontractkit/integrations-framework/client"
	"github.com/smartcontractkit/integrations-framework/contracts"
	"math/big"
)

var _ = Describe("Solana OCRv2", func() {
	var (
		e              *environment.Environment
		chainlinkNodes []client.Chainlink
		cd             contracts.ContractDeployer
		store          contracts.OCRv2Store
		billingAC      contracts.OCRv2AccessController
		requesterAC    contracts.OCRv2AccessController
		ocr2           contracts.OCRv2
		ocConfig       contracts.OffChainAggregatorV2Config
		nkb            []NodeKeysBundle
		mockserver     *client.MockserverClient
		nets           *client.Networks
		err            error
	)

	BeforeEach(func() {
		By("Deploying the environment", func() {
			e, err = environment.DeployOrLoadEnvironment(
				solclient.NewChainlinkSolOCRv2(),
				tools.ChartsRoot,
			)
			Expect(err).ShouldNot(HaveOccurred())
			err = e.ConnectAll()
			Expect(err).ShouldNot(HaveOccurred())
			err = UploadProgramBinaries(e)
			Expect(err).ShouldNot(HaveOccurred())
		})
		By("Getting the clients", func() {
			networkRegistry := client.NewNetworkRegistry()
			networkRegistry.RegisterNetwork(
				"solana",
				solclient.ClientInitFunc(),
				solclient.ClientURLSFunc(),
			)
			nets, err = networkRegistry.GetNetworks(e)
			Expect(err).ShouldNot(HaveOccurred())
			mockserver, err = client.ConnectMockServer(e)
			Expect(err).ShouldNot(HaveOccurred())
			chainlinkNodes, err = client.ConnectChainlinkNodes(e)
			Expect(err).ShouldNot(HaveOccurred())
			ocConfig, nkb, err = DefaultOffChainConfigParamsFromNodes(chainlinkNodes)
			Expect(err).ShouldNot(HaveOccurred())
		})
		By("Deploying contracts", func() {
			cd, err = solclient.NewContractDeployer(nets.Default, e)
			Expect(err).ShouldNot(HaveOccurred())
			lt, err := cd.DeployLinkTokenContract()
			Expect(err).ShouldNot(HaveOccurred())
			err = FundOracles(nets.Default, nkb, big.NewFloat(5e4))
			Expect(err).ShouldNot(HaveOccurred())
			billingAC, err = cd.DeployOCRv2AccessController()
			Expect(err).ShouldNot(HaveOccurred())
			requesterAC, err = cd.DeployOCRv2AccessController()
			Expect(err).ShouldNot(HaveOccurred())
			err = nets.Default.WaitForEvents()
			Expect(err).ShouldNot(HaveOccurred())

			store, err = cd.DeployOCRv2Store(billingAC.Address())
			Expect(err).ShouldNot(HaveOccurred())
			ocr2, err = cd.DeployOCRv2(billingAC.Address(), requesterAC.Address(), lt.Address())
			Expect(err).ShouldNot(HaveOccurred())
			err = nets.Default.WaitForEvents()
			Expect(err).ShouldNot(HaveOccurred())

			err = store.CreateFeed("Feed", uint8(9), 10, 1024)
			Expect(err).ShouldNot(HaveOccurred())

			err = ocr2.SetBilling(uint32(1), uint32(1), billingAC.Address())
			Expect(err).ShouldNot(HaveOccurred())
			storeAuth, err := ocr2.AuthorityAddr("store")
			Expect(err).ShouldNot(HaveOccurred())
			err = billingAC.AddAccess(storeAuth)
			Expect(err).ShouldNot(HaveOccurred())
			err = ocr2.SetOracles(ocConfig)
			Expect(err).ShouldNot(HaveOccurred())
			err = nets.Default.WaitForEvents()
			Expect(err).ShouldNot(HaveOccurred())

			err = store.SetWriter(storeAuth)
			Expect(err).ShouldNot(HaveOccurred())
			err = store.SetValidatorConfig(80000)
			Expect(err).ShouldNot(HaveOccurred())
			err = nets.Default.WaitForEvents()
			Expect(err).ShouldNot(HaveOccurred())

			err = ocr2.SetOffChainConfig(ocConfig)
			Expect(err).ShouldNot(HaveOccurred())
			err = ocr2.DumpState()
			Expect(err).ShouldNot(HaveOccurred())
		})

		By("Creating OCR2 jobs", func() {
			err = mockserver.SetValuePath("/variable", 5)
			Expect(err).ShouldNot(HaveOccurred())
			err = mockserver.SetValuePath("/juels", 1)
			Expect(err).ShouldNot(HaveOccurred())

			err = CreateOCR2Jobs(
				chainlinkNodes,
				nkb,
				mockserver,
				ocr2,
				store,
			)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	Describe("with Solana", func() {
		It("performs OCR round", func() {
			err = TriggerNewRound(mockserver, 1, 2, 10)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func(g Gomega) {
				a, err := store.GetLatestRoundData()
				g.Expect(err).ShouldNot(HaveOccurred())
				log.Debug().Interface("Answer", a).Msg("OCR Round answer")
				g.Expect(a).Should(Equal(uint64(10)))
			}, "2m", "5s").Should(Succeed())
		})
	})

	AfterEach(func() {
		By("Tearing down the environment", func() {
			err = actions.TeardownSuite(e, nil, "logs")
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})

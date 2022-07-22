package smoke

import (
	"github.com/smartcontractkit/chainlink/integration-tests/actions"
	"time"

	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/common"
)

var _ = Describe("Solana OCRv2", func() {
	var state = common.NewOCRv2State(1, 5)
	BeforeEach(func() {
		By("Deploying OCRv2 cluster", func() {
			state.DeployCluster(5, false, utils.ContractsDir)
			state.SetAllAdapterResponsesToTheSameValue(10)
		})
	})
	Describe("with Solana", func() {
		It("performs OCR round", func() {
			state.ValidateRoundsAfter(time.Now(), common.NewRoundCheckTimeout, 1)
		})
	})
	AfterEach(func() {
		By("Tearing down the environment", func() {
			err := actions.TeardownSuite(state.Env, "logs", state.ChainlinkNodes, nil, nil)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})

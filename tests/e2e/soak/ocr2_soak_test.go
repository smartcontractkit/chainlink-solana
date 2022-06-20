package smoke

import (
	"time"

	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/common"
	"github.com/smartcontractkit/chainlink-testing-framework/actions"
)

var _ = Describe("Solana OCRv2 soak test @ocr2-soak", func() {
	var state = common.NewOCRv2State(30, 5)
	BeforeEach(func() {
		By("Deploying OCRv2 cluster", func() {
			state.DeployCluster(5, false, utils.ContractsDir)
			state.SetAllAdapterResponsesToTheSameValue(10)
		})
	})
	Describe("with Solana", func() {
		It("performs OCR rounds", func() {
			state.ValidateRoundsAfter(time.Now(), common.NewSoakRoundsCheckTimeout, 8000)
		})
	})
	AfterEach(func() {
		By("Tearing down the environment", func() {
			err := actions.TeardownSuite(state.Env, nil, "logs", nil, nil)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})

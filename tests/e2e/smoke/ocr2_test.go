package smoke

import (
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/common"
	"github.com/smartcontractkit/integrations-framework/actions"
)

var _ = Describe("Solana OCRv2", func() {
	var state = &common.OCRv2TestState{}
	BeforeEach(func() {
		By("Deploying OCRv2 cluster", func() {
			state.DeployCluster(5, false, utils.ContractsDir)
			state.SetAllAdapterResponsesToTheSameValue(10)
		})
	})
	Describe("with Solana", func() {
		It("performs OCR round", func() {
			Eventually(func(g Gomega) {
				a, ts, _, err := state.Store.GetLatestRoundData()
				log.Debug().Uint64("Answer", a).Time("Time", time.Unix(int64(ts), 0)).Msg("Round data")
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(a).Should(Equal(uint64(10)))
			}, common.NewRoundCheckTimeout, common.NewRoundCheckPollInterval).Should(Succeed())
		})
	})
	AfterEach(func() {
		By("Tearing down the environment", func() {
			err := actions.TeardownSuite(state.Env, nil, "logs", nil, nil)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})

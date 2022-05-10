package chaos

import (
	"time"

	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/common"
	"github.com/smartcontractkit/chainlink-testing-framework/actions"
)

var _ = Describe("Solana chaos suite", func() {
	var state = common.NewOCRv2State(1, 19)
	BeforeEach(func() {
		By("Deploying OCRv2 cluster", func() {
			state.DeployCluster(19, true, utils.ContractsDir)
			state.LabelChaosGroups()
			state.SetAllAdapterResponsesToTheSameValue(10)
		})
	})
	It("Can tolerate chaos experiments", func() {
		By("Stable and working", func() {
			state.ValidateRoundsAfter(time.Now(), common.NewRoundCheckTimeout, 10)
		})
		By("Can work with faulty nodes offline", func() {
			state.CanWorkWithFaultyNodesOffline()
		})
		By("Can't work when more than faulty nodes are offline", func() {
			state.CantWorkWithMoreThanFaultyNodesSplit()
		})
		By("Can't work with two parts network split, restored after", func() {
			state.RestoredAfterNetworkSplit()
		})
		By("Can recover from yellow group loss connection to validator", func() {
			state.CanWorkYellowGroupNoValidatorConnection()
		})
		By("Can recover after all nodes lost connection to validator", func() {
			state.CanRecoverAllNodesValidatorConnectionLoss()
		})
		By("Can work after all nodes restarted", func() {
			state.CanWorkAfterAllNodesRestarted()
		})
	})
	AfterEach(func() {
		By("Tearing down the environment", func() {
			err := actions.TeardownSuite(state.Env, nil, "logs", nil, nil)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})

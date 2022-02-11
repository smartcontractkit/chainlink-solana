package chaos

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/common"
	"github.com/smartcontractkit/integrations-framework/actions"
)

var _ = Describe("Solana chaos suite", func() {
	var state = &common.OCRv2TestState{}
	BeforeEach(func() {
		By("Deploying OCRv2 cluster", func() {
			state.DeployCluster(19, true)
			state.LabelChaosGroups()
			state.ImitateSource(1*time.Second, 2, 10)
		})
	})
	It("Can tolerate chaos experiments", func() {
		By("Stable and working", func() {
			state.ValidateRoundsAfter(time.Now(), 10)
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
			err := actions.TeardownSuite(state.Env, nil, "logs")
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})

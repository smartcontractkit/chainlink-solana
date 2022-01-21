package common

import (
	. "github.com/onsi/gomega"
	"github.com/smartcontractkit/helmenv/chaos/experiments"
	"time"
)

func (m *OCRv2TestState) CanRecoverAllNodesValidatorConnectionLoss() {
	// nolint
	defer m.Env.ClearAllChaosExperiments()
	_, err := m.Env.ApplyChaosExperiment(
		&experiments.NetworkPartition{
			FromMode:       "all",
			FromLabelKey:   ChaosGroupOnline,
			FromLabelValue: "1",
			ToMode:         "all",
			ToLabelKey:     "app",
			ToLabelValue:   "sol",
		},
	)
	Expect(err).ShouldNot(HaveOccurred())
	time.Sleep(ChaosAwaitingApply)
	err = m.Env.ClearAllChaosExperiments()
	Expect(err).ShouldNot(HaveOccurred())
	m.ValidateRoundsAfter(time.Now(), 10)
}

func (m *OCRv2TestState) CanWorkYellowGroupNoValidatorConnection() {
	// nolint
	defer m.Env.ClearAllChaosExperiments()
	_, err := m.Env.ApplyChaosExperiment(
		&experiments.NetworkPartition{
			FromMode:       "all",
			FromLabelKey:   ChaosGroupYellow,
			FromLabelValue: "1",
			ToMode:         "all",
			ToLabelKey:     "app",
			ToLabelValue:   "sol",
		},
	)
	Expect(err).ShouldNot(HaveOccurred())
	time.Sleep(ChaosAwaitingApply)
	m.ValidateRoundsAfter(time.Now(), 10)
}

func (m *OCRv2TestState) CantWorkWithFaultyNodesFailed() {
	// nolint
	defer m.Env.ClearAllChaosExperiments()
	_, err := m.Env.ApplyChaosExperiment(
		&experiments.PodFailure{
			Mode:       "all",
			LabelKey:   ChaosGroupYellow,
			LabelValue: "1",
			Duration:   UntilStop,
		},
	)
	Expect(err).ShouldNot(HaveOccurred())
	time.Sleep(ChaosAwaitingApply)
	m.ValidateNoRoundsAfter(time.Now())
}

func (m *OCRv2TestState) CanWorkWithFaultyNodesOffline() {
	// nolint
	defer m.Env.ClearAllChaosExperiments()
	_, err := m.Env.ApplyChaosExperiment(
		&experiments.NetworkPartition{
			FromMode:       "all",
			FromLabelKey:   ChaosGroupFaulty,
			FromLabelValue: "1",
			ToMode:         "all",
			ToLabelKey:     ChaosGroupOnline,
			ToLabelValue:   "1",
		},
	)
	Expect(err).ShouldNot(HaveOccurred())
	time.Sleep(ChaosAwaitingApply)
	m.ValidateRoundsAfter(time.Now(), 10)
}

func (m *OCRv2TestState) CantWorkWithMoreThanFaultyNodesOffline() {
	// nolint
	defer m.Env.ClearAllChaosExperiments()
	_, err := m.Env.ApplyChaosExperiment(
		&experiments.NetworkLoss{
			Mode:        "all",
			LabelKey:    ChaosGroupYellow,
			Loss:        100,
			Correlation: 100,
			LabelValue:  "1",
			Duration:    UntilStop,
		},
	)
	Expect(err).ShouldNot(HaveOccurred())
	time.Sleep(ChaosAwaitingApply)
	m.ValidateRoundsAfter(time.Now(), 30)
}

func (m *OCRv2TestState) CantWorkWithMoreThanFaultyNodesSplit() {
	// nolint
	defer m.Env.ClearAllChaosExperiments()
	_, err := m.Env.ApplyChaosExperiment(
		&experiments.NetworkPartition{
			FromMode:       "all",
			FromLabelKey:   ChaosGroupYellow,
			FromLabelValue: "1",
			ToMode:         "all",
			ToLabelKey:     ChaosGroupOnline,
			ToLabelValue:   "1",
		},
	)
	Expect(err).ShouldNot(HaveOccurred())
	time.Sleep(ChaosAwaitingApply)
	m.ValidateNoRoundsAfter(time.Now())
}

func (m *OCRv2TestState) NetworkCorrupt(group string, corrupt int, rounds int) {
	// nolint
	defer m.Env.ClearAllChaosExperiments()
	_, err := m.Env.ApplyChaosExperiment(
		&experiments.NetworkCorrupt{
			Mode:        "all",
			LabelKey:    group,
			LabelValue:  "1",
			Corrupt:     corrupt,
			Correlation: 100,
			Duration:    UntilStop,
		},
	)
	Expect(err).ShouldNot(HaveOccurred())
	time.Sleep(ChaosAwaitingApply)
	m.ValidateRoundsAfter(time.Now(), rounds)
}

func (m *OCRv2TestState) CanWorkAfterAllNodesRestarted() {
	// nolint
	defer m.Env.ClearAllChaosExperiments()
	_, err := m.Env.ApplyChaosExperiment(
		&experiments.ContainerKill{
			Mode:       "all",
			LabelKey:   "app",
			LabelValue: "chainlink-node",
			Container:  "node",
		},
	)
	Expect(err).ShouldNot(HaveOccurred())
	time.Sleep(ChaosAwaitingApply)
	m.ValidateRoundsAfter(time.Now(), 10)
}

func (m *OCRv2TestState) RestoredAfterNetworkSplit() {
	// nolint
	defer m.Env.ClearAllChaosExperiments()
	_, err := m.Env.ApplyChaosExperiment(
		&experiments.NetworkPartition{
			FromMode:       "all",
			FromLabelKey:   ChaosGroupLeftHalf,
			FromLabelValue: "1",
			ToMode:         "all",
			ToLabelKey:     ChaosGroupRightHalf,
			ToLabelValue:   "1",
		},
	)
	Expect(err).ShouldNot(HaveOccurred())
	time.Sleep(ChaosAwaitingApply)
	m.ValidateNoRoundsAfter(time.Now())
	err = m.Env.ClearAllChaosExperiments()
	Expect(err).ShouldNot(HaveOccurred())
	m.ValidateRoundsAfter(time.Now(), 10)
}

package smoke_test

import (
	"github.com/smartcontractkit/chainlink-testing-framework/actions"
	"testing"

	. "github.com/onsi/ginkgo/v2"
)

func Test_Suite(t *testing.T) {
	actions.GinkgoSuite()
	RunSpecs(t, "Soak")
}

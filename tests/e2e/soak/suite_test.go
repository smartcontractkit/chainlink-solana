package smoke_test

import (
	"testing"

	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"

	. "github.com/onsi/ginkgo/v2"
)

func Test_Suite(t *testing.T) {
	utils.GinkgoSuite()
	RunSpecs(t, "Soak")
}

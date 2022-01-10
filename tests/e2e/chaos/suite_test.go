package chaos_test

import (
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"
	"testing"

	. "github.com/onsi/ginkgo/v2"
)

func Test_Suite(t *testing.T) {
	utils.GinkgoSuite()
	RunSpecs(t, "Chaos")
}

package utils

import (
	"github.com/smartcontractkit/integrations-framework/config"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// GinkgoSuite provides the default setup for running a Ginkgo test suite
func GinkgoSuite() {
	RegisterFailHandler(Fail)

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	fConf, err := config.LoadFrameworkConfig(filepath.Join(TestsDir, "framework.yaml"))
	if err != nil {
		log.Fatal().
			Str("Path", ProjectRoot).
			Msg("Failed to load config")
		return
	}
	_, err = config.LoadNetworksConfig(filepath.Join(TestsDir, "networks.yaml"))
	if err != nil {
		log.Fatal().
			Str("Path", ProjectRoot).
			Err(err).
			Msg("Failed to load config")
		return
	}
	log.Logger = log.Logger.Level(zerolog.Level(fConf.Logging.Level))
}

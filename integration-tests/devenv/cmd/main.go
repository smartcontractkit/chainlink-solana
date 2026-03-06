// copy of chainlink/devenv/cmd
package main

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/smartcontractkit/chainlink-testing-framework/framework"

	de "github.com/smartcontractkit/chainlink-solana/integration-tests/devenv"
)

var rootCmd = &cobra.Command{
	Use:   "sol",
	Short: "Solana local environment tool",
}

var upCmd = &cobra.Command{
	Use:     "up",
	Aliases: []string{"u"},
	Short:   "Spin up the Solana development environment",
	Args:    cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile := "env.toml"
		if len(args) > 0 {
			configFile = args[0]
		}
		de.L.Info().Str("Config", configFile).Msg("Creating development environment")
		_ = os.Setenv("CTF_CONFIGS", configFile)
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		return de.NewEnvironment(ctx)
	},
}

var downCmd = &cobra.Command{
	Use:     "down",
	Aliases: []string{"d"},
	Short:   "Tear down the Solana development environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		de.L.Info().Msg("Tearing down the development environment")
		return framework.RemoveTestContainers()
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		de.L.Err(err).Send()
		os.Exit(1)
	}
}

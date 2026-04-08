// copy of chainlink/devenv/cmd
package main

import (
	"context"
	"fmt"
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

var obsCmd = &cobra.Command{
	Use:   "obs",
	Short: "Manage the observability stack",
	Long:  "Spin up or down the observability stack with subcommands 'up' and 'down'",
}

var obsUpCmd = &cobra.Command{
	Use:     "up",
	Aliases: []string{"u"},
	Short:   "Spin up the observability stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		full, _ := cmd.Flags().GetBool("full")
		var err error
		if full {
			err = framework.ObservabilityUpFull()
		} else {
			err = framework.ObservabilityUp()
		}
		if err != nil {
			return fmt.Errorf("observability up failed: %w", err)
		}
		return nil
	},
}

var obsDownCmd = &cobra.Command{
	Use:     "down",
	Aliases: []string{"d"},
	Short:   "Spin down the observability stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		return framework.ObservabilityDown()
	},
}

var obsRestartCmd = &cobra.Command{
	Use:     "restart",
	Aliases: []string{"r"},
	Short:   "Restart the observability stack (data wipe)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := framework.ObservabilityDown(); err != nil {
			return fmt.Errorf("observability down failed: %w", err)
		}
		full, _ := cmd.Flags().GetBool("full")
		var err error
		if full {
			err = framework.ObservabilityUpFull()
		} else {
			err = framework.ObservabilityUp()
		}
		if err != nil {
			return fmt.Errorf("observability up failed: %w", err)
		}
		return nil
	},
}

func init() {
	// observability
	obsCmd.PersistentFlags().BoolP("full", "f", false, "Enable full observability stack with additional components")
	obsCmd.AddCommand(obsRestartCmd)
	obsCmd.AddCommand(obsUpCmd)
	obsCmd.AddCommand(obsDownCmd)
	rootCmd.AddCommand(obsCmd)

	// main env commands
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		de.L.Err(err).Send()
		os.Exit(1)
	}
}

// Package gauntlet enables the framework to interface with the chainlink gauntlet project
// Copied from chainlink-testing-framework/lib/gauntlet to avoid depending on deprecated CTF packages.
package gauntlet

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	retry "github.com/avast/retry-go/v4"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

var (
	execDir string
)

// Gauntlet contains helpful data to run gauntlet commands
type Gauntlet struct {
	exec          string
	Command       string
	Network       string
	NetworkConfig map[string]string
}

// NewGauntlet Sets up a gauntlet struct and checks if the yarn executable exists.
func NewGauntlet() (*Gauntlet, error) {
	yarn, err := exec.LookPath("yarn")
	if err != nil {
		return &Gauntlet{}, errors.New("'yarn' is not installed")
	}
	log.Debug().Str("PATH", yarn).Msg("Executable Path")
	os.Setenv("SKIP_PROMPTS", "true")
	g := &Gauntlet{
		exec:          yarn,
		Command:       "gauntlet",
		NetworkConfig: make(map[string]string),
	}
	g.GenerateRandomNetwork()
	return g, nil
}

// Flag returns a string formatted in the expected gauntlet's flag form
func (g *Gauntlet) Flag(flag, value string) string {
	return fmt.Sprintf("--%s=%s", flag, value)
}

func (g *Gauntlet) SetWorkingDir(workDir string) {
	execDir = workDir
}

// GenerateRandomNetwork Creates and sets a random network prepended with test
func (g *Gauntlet) GenerateRandomNetwork() {
	r := uuid.NewString()[0:8]
	t := time.Now().UnixMilli()
	g.Network = fmt.Sprintf("test%v%s", t, r)
	log.Debug().Str("Network", g.Network).Msg("Generated Network Name")
}

type ExecCommandOptions struct {
	ErrHandling       []string
	CheckErrorsInRead bool
	RetryCount        int
	RetryDelay        time.Duration
}

// ExecCommand Executes a gauntlet or yarn command with the provided arguments.
//
//	It will also check for any errors you specify in the output via the errHandling slice.
func (g *Gauntlet) ExecCommand(args []string, options ExecCommandOptions) (string, error) {
	output := ""
	var updatedArgs []string
	if g.Command == "gauntlet" {
		updatedArgs = append([]string{g.Command}, args...)
		updatedArgs = insertArg(updatedArgs, 2, g.Flag("network", g.Network))
	} else {
		updatedArgs = args
	}

	printArgs(updatedArgs)

	cmd := exec.Command(g.exec, updatedArgs...) // #nosec G204
	if execDir != "" {
		cmd.Dir = execDir
	}
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return output, err
	}

	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	for err == nil {
		log.Info().Str("stdout", line).Msg(g.Command)
		output = fmt.Sprintf("%s%s", output, line)
		if options.CheckErrorsInRead {
			rerr := checkForErrors(options.ErrHandling, output)
			if rerr != nil {
				return output, rerr
			}
		}
		line, err = reader.ReadString('\n')
	}

	reader = bufio.NewReader(stderr)
	line, err = reader.ReadString('\n')
	for err == nil {
		log.Info().Str("stderr", line).Msg(g.Command)
		output = fmt.Sprintf("%s%s", output, line)
		if options.CheckErrorsInRead {
			rerr := checkForErrors(options.ErrHandling, output)
			if rerr != nil {
				return output, rerr
			}
		}
		line, err = reader.ReadString('\n')
	}

	rerr := checkForErrors(options.ErrHandling, output)
	if rerr != nil {
		return output, rerr
	}

	if strings.Compare("EOF", err.Error()) > 0 {
		return output, err
	}

	err = cmd.Wait()

	log.Debug().Str("Command", g.Command).Msg("command Completed")
	return output, err
}

// ExecCommandWithRetries Some commands are safe to retry and in ci this can be even more so needed.
func (g *Gauntlet) ExecCommandWithRetries(args []string, options ExecCommandOptions) (string, error) {
	var output string
	var err error
	if options.RetryDelay == 0 {
		options.RetryDelay = time.Second * 5
	}
	err = retry.Do(
		func() error {
			output, err = g.ExecCommand(args, options)
			return err
		},
		retry.Delay(options.RetryDelay),
		retry.MaxDelay(options.RetryDelay),
		retry.Attempts(uint(options.RetryCount)), //nolint
	)

	return output, err
}

// WriteNetworkConfigMap write a network config file for gauntlet testing.
func (g *Gauntlet) WriteNetworkConfigMap(networkDirPath string) error {
	file := filepath.Join(networkDirPath, fmt.Sprintf(".env.%s", g.Network))
	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	for k, v := range g.NetworkConfig {
		log.Debug().Str(k, v).Msg("Gauntlet .env config value:")
		_, err = f.WriteString(fmt.Sprintf("\n%s=%s", k, v))
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *Gauntlet) WriteNetworkConfigVar(networkDirPath string, k string, v string) error {
	file := filepath.Join(networkDirPath, fmt.Sprintf(".env.%s", g.Network))
	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	log.Debug().Str(k, v).Msg("Gauntlet .env config value:")
	_, err = f.WriteString(fmt.Sprintf("\n%s=%s", k, v))
	if err != nil {
		return err
	}
	return nil
}

func checkForErrors(errHandling []string, line string) error {
	for _, e := range errHandling {
		if strings.Contains(line, e) {
			log.Debug().Str("Error", line).Msg("Gauntlet Error Found")
			return fmt.Errorf("found a gauntlet error")
		}
	}
	return nil
}

func insertArg(args []string, index int, valueToInsert string) []string {
	if len(args) <= index {
		return append(args, valueToInsert)
	}
	args = append(args[:index+1], args[index:]...)
	args[index] = valueToInsert
	return args
}

func printArgs(args []string) {
	out := "yarn"
	for _, arg := range args {
		out = fmt.Sprintf("%s %s", out, arg)
	}
	log.Info().Str("Command", out).Msg("Gauntlet")
}

func (g *Gauntlet) AddNetworkConfigVar(k string, v string) {
	if existingVal, exists := g.NetworkConfig[k]; exists && existingVal == v {
		log.Info().Str("stdout", "gauntlet network").Msg("Skipping adding duplicate key to network config")
		return
	}
	g.NetworkConfig[k] = v
}

package client

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/freeport"
)

func SetupLocalSolNode(t *testing.T) string {
	t.Helper()

	url, _ := SetupLocalSolNodeWithFlags(t)

	return url
}

// SetupLocalSolNode sets up a local solana node via solana cli, and returns the url
func SetupLocalSolNodeWithFlags(t *testing.T, flags ...string) (string, string) {
	t.Helper()

	port := freeport.GetN(t, 2)
	portStr := strconv.Itoa(port[0])

	faucetPort := freeport.GetOne(t)
	url := "http://127.0.0.1:" + portStr
	wsURL := "ws://127.0.0.1:" + strconv.Itoa(port[1]) //there is no way to define ws port on Solana validation. It must be +1 from rpc port.

	args := append([]string{
		"--reset",
		"--rpc-port", portStr,
		"--faucet-port", strconv.Itoa(faucetPort),
		"--ledger", t.TempDir(),
		// Configurations to make the local cluster faster
		"--ticks-per-slot", "8", // value in mainnet: 64
	}, flags...)

	cmd := exec.Command("solana-test-validator", args...)

	var stdErr bytes.Buffer
	cmd.Stderr = &stdErr
	var stdOut bytes.Buffer
	cmd.Stdout = &stdOut
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		assert.NoError(t, cmd.Process.Kill())
		if err2 := cmd.Wait(); assert.Error(t, err2) {
			if t.Failed() || !assert.Contains(t, err2.Error(), "signal: killed", cmd.ProcessState.String()) {
				t.Logf("solana-test-validator\n stdout: %s\n stderr: %s", stdOut.String(), stdErr.String())
			}
		}
	})

	// Wait for api server to boot
	var ready bool
	for i := 0; i < 30; i++ {
		time.Sleep(time.Second)
		client := rpc.New(url)
		out, err := client.GetHealth(t.Context())
		if err != nil || out != rpc.HealthOk {
			t.Logf("API server not ready yet (attempt %d)\n", i+1)
			t.Logf("Cmd output: %s\nCmd error: %s\n", stdOut.String(), stdErr.String())
			continue
		}
		ready = true
		break
	}
	if !ready {
		t.Logf("Cmd output: %s\nCmd error: %s\n", stdOut.String(), stdErr.String())
	}
	require.True(t, ready)

	return url, wsURL
}

func FundTestAccountsWithRetry(t *testing.T, keys []solana.PublicKey, url string, attempts int) error {
	t.Helper()

	var errKeys []solana.PublicKey
	for i, key := range keys {
		account := keys[i].String()
		_, err := exec.Command("solana", "airdrop", "100",
			account,
			"--url", url,
		).Output()
		if err != nil {
			if attempts <= 0 {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					return fmt.Errorf("failed to fund solana account: %w; stderr: %s", err, string(exitErr.Stderr))
				}
				return err
			}
			errKeys = append(errKeys, key)
		}
	}
	// call FundTestAccountsWithRetry recursively with keys that errored, decrement attempts to cap the number of retries
	if len(errKeys) > 0 {
		if attempts <= 0 {
			return fmt.Errorf("failed to fund solana accounts")
		}
		time.Sleep(500 * time.Millisecond)
		return FundTestAccountsWithRetry(t, errKeys, url, attempts-1)
	}

	return nil
}

func FundTestAccounts(t *testing.T, keys []solana.PublicKey, url string) {
	t.Helper()
	err := FundTestAccountsWithRetry(t, keys, url, 5)
	require.NoError(t, err)
}

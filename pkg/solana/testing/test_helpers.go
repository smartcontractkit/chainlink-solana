package testing

import (
	"bytes"
	"context"
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

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/freeport"
)

const (
	fundingTimeout    = 30 * time.Second
	fundingTimestep   = 500 * time.Millisecond
	fundingMaxRetries = 5
)

type SolanaValidatorNode struct {
	URL     string
	WS_URL  *string
	chainID *string
}

func (svn *SolanaValidatorNode) GetChainID(ctx context.Context) (string, error) {
	if svn.chainID != nil {
		return *svn.chainID, nil
	}
	clientConfig := config.NewDefault()
	requestTimeout := 5 * time.Second
	logger, _ := logger.New()
	nodeClient, err := client.NewClient(svn.URL, clientConfig, requestTimeout, logger)
	if err != nil {
		return "", err
	}
	chainID, err := nodeClient.ChainID(ctx)
	if err != nil {
		return "", err
	}
	strChainID := chainID.String()
	svn.chainID = &strChainID
	return strChainID, nil
}

func SetupLocalSolNode(t *testing.T) SolanaValidatorNode {
	t.Helper()

	url, wsURL := SetupLocalSolNodeWithFlags(t)

	return SolanaValidatorNode{
		URL:    url,
		WS_URL: &wsURL,
	}
}

// SetupLocalSolNode sets up a local solana node via solana cli, and returns the url
func SetupLocalSolNodeWithFlags(t *testing.T, flags ...string) (string, string) {
	t.Helper()

	ports, err := TwoConsecutiveFreeports(t)
	require.NoError(t, err)
	portStr := strconv.Itoa(ports[0])

	faucetPort := freeport.GetOne(t)
	url := "http://127.0.0.1:" + portStr
	wsURL := "ws://127.0.0.1:" + strconv.Itoa(ports[1]) //there is no way to define ws port on Solana validation. It must be +1 from rpc port.

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

func FundTestAccountsWithRetry(t *testing.T, keys []solana.PublicKey, client *rpc.Client, attempts int) error {
	t.Helper()

	if attempts <= 0 {
		return fmt.Errorf("failed to fund accounts within %d tries", fundingMaxRetries)
	}

	out, err := client.GetHealth(t.Context())
	if err != nil || out != rpc.HealthOk {
		t.Log("client RPC not healthy when trying to fund account")
		return errors.New("client not healthy when funding account")
	}

	sigs := []solana.Signature{}
	for _, v := range keys {
		sig, err := client.RequestAirdrop(t.Context(), v, 100*solana.LAMPORTS_PER_SOL, rpc.CommitmentFinalized)
		require.NoError(t, err)
		sigs = append(sigs, sig)
	}

	// wait for confirmation so later transactions don't fail
	remaining := keys
	initTime := time.Now()

	for elapsed := time.Since(initTime); elapsed < fundingTimeout; elapsed = time.Since(initTime) {
		time.Sleep(fundingTimestep)

		statusRes, sigErr := client.GetSignatureStatuses(t.Context(), true, sigs...)
		require.NoError(t, sigErr)
		require.NotNil(t, statusRes)
		require.NotNil(t, statusRes.Value)

		accountsWithNonFinalizedFunding := []solana.PublicKey{}
		for i, res := range statusRes.Value {
			if res == nil || res.ConfirmationStatus != rpc.ConfirmationStatusFinalized {
				accountsWithNonFinalizedFunding = append(accountsWithNonFinalizedFunding, keys[i])
			}
		}
		remaining = accountsWithNonFinalizedFunding

		if len(remaining) == 0 {
			return nil // all done!
		}
	}

	return FundTestAccountsWithRetry(t, remaining, client, attempts-1) // recursive call with only remaining & with fewer attempts
}

func FundTestAccounts(t *testing.T, keys []solana.PublicKey, url string) {
	t.Helper()
	client := rpc.New(url)
	err := FundTestAccountsWithRetry(t, keys, client, fundingMaxRetries)
	require.NoError(t, err)
}

func TwoConsecutiveFreeports(t *testing.T) ([]int, error) {
	t.Helper()
	// track unused ports until consecutive ones are found or max retries is reached
	// ports are not immediately returned to avoid re-fetching the same ones again
	var unusedPorts []int
	// try a maximum of 5 times
	for range 5 {
		ports := freeport.GetN(t, 2)
		if ports[0]+1 == ports[1] {
			freeport.Return(unusedPorts)
			return ports, nil
		}
		unusedPorts = append(unusedPorts, ports...)
	}
	freeport.Return(unusedPorts)
	return nil, errors.New("failed to fetch 2 consecutive ports")
}

package testing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/freeport"
)

const (
	fundingTimestep   = 500 * time.Millisecond
	fundingTimeout    = 30 * time.Second
	fundingMaxRetries = 5
	fundingAmount     = 100 * solana.LAMPORTS_PER_SOL
)

// Funder receives the genesis token supply (--mint) on validators started by
// SetupLocalSolNodeWithFlags. It is the same well-known test key as
// chainlink-testing-framework's blockchain.DefaultSolanaPrivateKey.
var Funder = solana.MustPrivateKeyFromBase58("DmPfeHBC8Brf8s5qQXi25bmJ996v6BHRtaLc6AH51yFGSqQpUMy1oHkbbXobPNBdgGH2F29PAmoq9ZZua4K9vCc")

func SetupLocalSolNode(t *testing.T) string {
	t.Helper()

	url, _ := SetupLocalSolNodeWithFlags(t)

	return url
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

	// args1 := []string{"--version"}

	args := append([]string{
		"--reset",
		"--rpc-port", portStr,
		"--faucet-port", strconv.Itoa(faucetPort),
		"--ledger", t.TempDir(),
		"--mint", Funder.PublicKey().String(),
		// Configurations to make the local cluster faster
		"--ticks-per-slot", "8", // value in mainnet: 64
	}, flags...)

	cmd := exec.Command("solana-test-validator", args...)
	// Prevent macOS from creating metadata files (._*) that interfere with validator startup
	cmd.Env = append(os.Environ(), "COPYFILE_DISABLE=1")

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

// Transfer sends lamports from the funder to the recipient.
func Transfer(ctx context.Context, client *rpc.Client, funder solana.PrivateKey, recipient solana.PublicKey, lamports uint64) (solana.Signature, error) {
	recent, err := client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(lamports, funder.PublicKey(), recipient).Build(),
		},
		recent.Value.Blockhash,
		solana.TransactionPayer(funder.PublicKey()),
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to build fund transaction: %w", err)
	}

	if _, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if funder.PublicKey().Equals(key) {
			return &funder
		}
		return nil
	}); err != nil {
		return solana.Signature{}, fmt.Errorf("failed to sign fund transaction: %w", err)
	}

	return client.SendTransaction(ctx, tx)
}

func fundTestAccounts(t *testing.T, keys []solana.PublicKey, url string, attempts int) error {
	t.Helper()
	ctx := t.Context()
	client := rpc.New(url)

	// transfer from Funder when it holds the genesis supply (validators started by
	// SetupLocalSolNodeWithFlags); otherwise fall back to a faucet airdrop
	bal, balErr := client.GetBalance(ctx, Funder.PublicKey(), rpc.CommitmentConfirmed)
	useFunder := balErr == nil && bal.Value > uint64(len(keys))*fundingAmount

	var errKeys []solana.PublicKey
	var sentKeys []solana.PublicKey
	var sigs []solana.Signature
	for _, key := range keys {
		var sig solana.Signature
		var err error
		if useFunder {
			sig, err = Transfer(ctx, client, Funder, key, fundingAmount)
		} else {
			sig, err = client.RequestAirdrop(ctx, key, fundingAmount, rpc.CommitmentFinalized)
		}
		if err != nil {
			if attempts <= 0 {
				return fmt.Errorf("failed to fund solana account %s: %w", key, err)
			}
			errKeys = append(errKeys, key)
			continue
		}
		sentKeys = append(sentKeys, key)
		sigs = append(sigs, sig)
	}

	// wait for the transfers to finalize so later transactions don't fail
	for deadline := time.Now().Add(fundingTimeout); len(sigs) > 0 && time.Now().Before(deadline); {
		time.Sleep(fundingTimestep)

		statusRes, err := client.GetSignatureStatuses(ctx, true, sigs...)
		if err != nil || statusRes == nil {
			continue
		}

		var pendingKeys []solana.PublicKey
		var pendingSigs []solana.Signature
		for i, res := range statusRes.Value {
			if res == nil || res.ConfirmationStatus != rpc.ConfirmationStatusFinalized {
				pendingKeys = append(pendingKeys, sentKeys[i])
				pendingSigs = append(pendingSigs, sigs[i])
			}
		}
		sentKeys, sigs = pendingKeys, pendingSigs
	}
	errKeys = append(errKeys, sentKeys...)

	// call fundTestAccounts recursively with keys that errored, decrement attempts to cap the number of retries
	if len(errKeys) > 0 {
		if attempts <= 0 {
			return fmt.Errorf("failed to fund solana accounts")
		}
		time.Sleep(fundingTimestep)
		return fundTestAccounts(t, errKeys, url, attempts-1)
	}

	return nil
}

// FundTestAccounts funds each key with 100 SOL and waits for finalization.
func FundTestAccounts(t *testing.T, keys []solana.PublicKey, url string) {
	t.Helper()
	err := fundTestAccounts(t, keys, url, fundingMaxRetries)
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

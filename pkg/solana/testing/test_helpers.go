package testing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	mrand "math/rand"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/retry"
	"github.com/smartcontractkit/freeport"
)

const (
	fundingTimeout = 30 * time.Second
	fundingAmount  = 100 * solana.LAMPORTS_PER_SOL
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

	// Each validator needs its own gossip/TPU ports: the defaults (gossip 8000,
	// dynamic range next to it) collide when several validators share a host, and
	// SO_REUSEPORT then silently routes transactions to the wrong validator's TPU.
	gossipBase := findContiguousFreePorts(t, gossipPortWindow)

	args := append([]string{
		"--reset",
		"--rpc-port", portStr,
		"--faucet-port", strconv.Itoa(faucetPort),
		"--gossip-port", strconv.Itoa(gossipBase),
		"--dynamic-port-range", fmt.Sprintf("%d-%d", gossipBase+1, gossipBase+gossipPortWindow-1),
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
	tx, err := buildTransfer(ctx, client, funder, recipient, lamports)
	if err != nil {
		return solana.Signature{}, err
	}
	return client.SendTransaction(ctx, tx)
}

// buildTransfer returns a signed transfer transaction, ready to send.
func buildTransfer(ctx context.Context, client *rpc.Client, funder solana.PrivateKey, recipient solana.PublicKey, lamports uint64) (*solana.Transaction, error) {
	recent, err := client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(lamports, funder.PublicKey(), recipient).Build(),
		},
		recent.Value.Blockhash,
		solana.TransactionPayer(funder.PublicKey()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build fund transaction: %w", err)
	}

	if _, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if funder.PublicKey().Equals(key) {
			return &funder
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to sign fund transaction: %w", err)
	}

	return tx, nil
}

// FundTestAccounts funds each key with 100 SOL from Funder and waits for confirmation.
// The validator must mint its genesis supply to Funder (SetupLocalSolNodeWithFlags does).
func FundTestAccounts(t *testing.T, keys []solana.PublicKey, url string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), fundingTimeout)
	defer cancel()
	client := rpc.New(url)
	lggr := logger.Test(t)

	// signed transfers by recipient, kept so unconfirmed txs re-broadcast the
	// identical tx (same signature) rather than double-funding with a new one
	txs := make(map[solana.PublicKey]*solana.Transaction, len(keys))
	_, err := retry.Do(ctx, lggr, func(ctx context.Context) (any, error) {
		for _, key := range keys {
			if _, sent := txs[key]; sent {
				continue
			}
			tx, err := buildTransfer(ctx, client, Funder, key, fundingAmount)
			if err != nil {
				return nil, fmt.Errorf("failed to build funding transfer for %s: %w", key, err)
			}
			if _, err = client.SendTransaction(ctx, tx); err != nil {
				return nil, fmt.Errorf("failed to fund solana account %s from Funder %s (is the validator running with --mint?): %w", key, Funder.PublicKey(), err)
			}
			lggr.Infow("Sent funding transfer", "recipient", key, "sig", tx.Signatures[0])
			txs[key] = tx
		}

		keyList := make([]solana.PublicKey, 0, len(txs))
		sigList := make([]solana.Signature, 0, len(txs))
		for key, tx := range txs {
			keyList = append(keyList, key)
			sigList = append(sigList, tx.Signatures[0])
		}
		statusRes, err := client.GetSignatureStatuses(ctx, true, sigList...)
		if err != nil {
			return nil, err
		}
		pending := 0
		for i, res := range statusRes.Value {
			switch {
			case res == nil:
				// not on-chain yet: still propagating, or dropped (e.g. sent while the
				// validator was warming up); re-broadcasting is safe and covers both
				pending++
				key := keyList[i]
				lggr.Warnw("Funding transfer not yet on-chain, re-broadcasting", "recipient", key, "sig", sigList[i])
				if _, err = client.SendTransaction(ctx, txs[key]); err != nil && strings.Contains(err.Error(), "Blockhash not found") {
					// the tx expired and can never land, so a fresh one cannot double-fund
					lggr.Warnw("Funding transfer expired, will re-send with a fresh blockhash", "recipient", key, "sig", sigList[i])
					delete(txs, key)
				}
			case res.ConfirmationStatus == rpc.ConfirmationStatusConfirmed || res.ConfirmationStatus == rpc.ConfirmationStatusFinalized:
				// confirmed is sufficient on a single-node local validator; finalization
				// takes ~31 more slots, which times out on slow CI runners
			default:
				pending++
			}
		}
		if pending > 0 {
			return nil, fmt.Errorf("waiting for %d of %d funding transfers to confirm", pending, len(keys))
		}
		lggr.Infow("All funding transfers confirmed", "accounts", len(keys))
		return nil, nil
	})
	require.NoError(t, err)
}

// gossipPortWindow is the number of ports reserved per validator for gossip plus
// the dynamic TPU/TVU port range (agave needs ~13; extra is headroom).
const gossipPortWindow = 32

// findContiguousFreePorts returns the first port of a contiguous range of n free
// ports, verified by binding each one (TCP and UDP) like freeport does.
func findContiguousFreePorts(t *testing.T, n int) int {
	t.Helper()
	ctx := t.Context()
	var lc net.ListenConfig
	for attempt := 0; attempt < 50; attempt++ {
		base := 20000 + mrand.Intn(40000-n)
		free := true
		for p := base; p < base+n; p++ {
			l, err := lc.Listen(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", p))
			if err != nil {
				free = false
				break
			}
			_ = l.Close()
			pc, err := lc.ListenPacket(ctx, "udp", fmt.Sprintf("127.0.0.1:%d", p))
			if err != nil {
				free = false
				break
			}
			_ = pc.Close()
		}
		if free {
			return base
		}
	}
	require.Fail(t, "failed to find a contiguous free port range")
	return 0
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

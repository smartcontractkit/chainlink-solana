// init_mock_forwarder calls `initialize` on a deployed mock_forwarder program,
// creating a fresh `ForwarderState` account. Prints the resulting pubkeys for
// pasting into cre-cli's supported_chains.go.
//
// Usage:
//
//	go run . -rpc https://api.devnet.solana.com -wallet ~/.config/solana/id.json
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	mock_forwarder "github.com/smartcontractkit/chainlink-solana/contracts/generated/mock_forwarder"
	solanaclient "github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

const (
	confirmTimeout = 90 * time.Second
	pollInterval   = 2 * time.Second
)

func main() {
	rpcURL := flag.String("rpc", rpc.DevNet_RPC, "Solana RPC URL")
	walletPath := flag.String("wallet", os.ExpandEnv("$HOME/.config/solana/id.json"), "Path to Solana CLI keygen JSON wallet file")
	stateOut := flag.String("state-out", "", "Optional: path to write the new state account keypair JSON")
	flag.Parse()

	wallet, err := loadKeygenJSON(*walletPath)
	must(err, "load wallet")

	state := solana.NewWallet()
	if *stateOut != "" {
		must(writeKeygenJSON(*stateOut, state.PrivateKey), "write state keypair")
	}

	fmt.Printf("Program ID:       %s\n", mock_forwarder.ProgramID)
	fmt.Printf("Owner (wallet):   %s\n", wallet.PublicKey())
	fmt.Printf("State (new):      %s\n", state.PublicKey())
	fmt.Println()

	lggr, err := logger.New()
	must(err, "logger")

	cfg := config.NewDefault()
	client, err := solanaclient.NewClient(*rpcURL, cfg, 30*time.Second, lggr)
	must(err, "build solana client")

	ctx := context.Background()

	ix, err := mock_forwarder.NewInitializeInstruction(
		state.PublicKey(),
		wallet.PublicKey(),
		solana.SystemProgramID,
	)
	must(err, "build initialize instruction")

	blockhash, err := client.LatestBlockhash(ctx)
	must(err, "get latest blockhash")

	tx, err := solana.NewTransaction(
		[]solana.Instruction{ix},
		blockhash.Value.Blockhash,
		solana.TransactionPayer(wallet.PublicKey()),
	)
	must(err, "build tx")

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		switch key {
		case wallet.PublicKey():
			return &wallet
		case state.PublicKey():
			return &state.PrivateKey
		}
		return nil
	})
	must(err, "sign tx")

	sig, err := client.SendTx(ctx, tx)
	must(err, "SendTx")
	fmt.Printf("Submitted:        %s\n", sig)

	must(waitForConfirmation(ctx, client, sig), "wait for confirmation")

	fmt.Println()
	fmt.Println("✓ initialized")
	fmt.Println()
	fmt.Println("Paste into cre-cli/cmd/workflow/simulate/chain/solana/supported_chains.go:")
	fmt.Printf("  devnetProgramID    = %q\n", mock_forwarder.ProgramID.String())
	fmt.Printf("  devnetStateAccount = %q\n", state.PublicKey().String())
}

// waitForConfirmation polls SignatureStatuses until the tx reaches confirmed/finalized
// or hits a fatal error. Mirrors what rpc/confirm.SendAndConfirmTransaction does, but
// without requiring a websocket connection.
func waitForConfirmation(ctx context.Context, client *solanaclient.Client, sig solana.Signature) error {
	deadline := time.Now().Add(confirmTimeout)
	for {
		statuses, err := client.SignatureStatuses(ctx, []solana.Signature{sig})
		if err != nil {
			return fmt.Errorf("SignatureStatuses: %w", err)
		}
		if len(statuses) == 1 && statuses[0] != nil {
			s := statuses[0]
			if s.Err != nil {
				return fmt.Errorf("tx failed on-chain: %v", s.Err)
			}
			switch s.ConfirmationStatus {
			case rpc.ConfirmationStatusConfirmed, rpc.ConfirmationStatusFinalized:
				return nil
			}
		}
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for confirmation")
		}
		time.Sleep(pollInterval)
	}
}

// loadKeygenJSON reads Solana CLI's keygen JSON format: a JSON array of 64
// bytes representing the ed25519 secret key (seed || pubkey).
func loadKeygenJSON(path string) (solana.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var arr []byte
	if err := json.Unmarshal(b, &arr); err != nil {
		return nil, fmt.Errorf("expected JSON byte array (Solana CLI keygen format): %w", err)
	}
	if len(arr) != 64 {
		return nil, fmt.Errorf("expected 64-byte secret, got %d", len(arr))
	}
	return solana.PrivateKey(arr), nil
}

func writeKeygenJSON(path string, key solana.PrivateKey) error {
	b, err := json.Marshal([]byte(key))
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func must(err error, what string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s: %v\n", what, err)
		os.Exit(1)
	}
}

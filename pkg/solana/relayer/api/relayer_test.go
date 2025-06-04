package api_test

import (
	"testing"

	solanago "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	commonconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/relayer/api"
	soltesting "github.com/smartcontractkit/chainlink-solana/pkg/solana/testing"
)

func TestSubmitTransactionAndGetStatusUntilFinality(t *testing.T) {
	ctx := t.Context()

	// Create local Solana validator node
	solanaValidatorNode := soltesting.SetupLocalSolNode(t)
	// Create an account
	firstAccount := solanago.NewWallet()
	// Create another account
	secondAccount := solanago.NewWallet()
	
	// Add funds to the created accounts
	soltesting.FundTestAccounts(t, []solanago.PublicKey{firstAccount.PublicKey(), secondAccount.PublicKey()}, solanaValidatorNode.URL)

	nodeURL, err := commonconfig.ParseURL(solanaValidatorNode.URL)
	if err != nil {
		t.Fatalf("Failed to parse node URL: %v", err)
	}
	chainID, err := solanaValidatorNode.GetChainID(ctx)
	if err != nil {
		t.Fatalf("Failed to get ChainID: %v", err)
	}

	// Create a config for initializing the relayer API
	relayerConfig, err := api.NewRelayerConfigBuilder().
		WithNode(api.NewNodeConfigBuilder().WithURL(*nodeURL)).
		WithChainID(chainID).
		Build()
	if err != nil {
		t.Fatalf("Failed creating tx %v", err)
	}

	// Configure the required APIs from the relayer. Simple implementations are provided for getting started quickly with the relayer
	requiredAPIs := &api.RequiredAPIs{
		KeystoreAPI: api.NewDummyKeystoreService([]solanago.PrivateKey{firstAccount.PrivateKey}),
	}

	// Instantiate the relayer using the config and the required APIs.
	solanaRelayerAPI, err := api.NewSolanaRelayerAPI(ctx, relayerConfig, requiredAPIs)
	if err != nil {
		t.Fatalf("Failure creating Solana relayer API %v", err)
	}

	instructions := []solanago.Instruction{
		system.NewTransferInstruction(
			// 1 SOL = 1e9 lamports
			1e6, // 0.001 SOL
			firstAccount.PublicKey(),
			secondAccount.PublicKey(),
		).Build(),
	}

	accountsMeta := []solanago.AccountMeta{
		*solanago.Meta(firstAccount.PublicKey()).SIGNER().WRITE(), *solanago.Meta(secondAccount.PublicKey()).WRITE(),
	}

	// Create a Solana Transaction Request
	txRequest := api.TransactionRequest{
		Instructions: instructions,
		Accounts:     accountsMeta,
		Payer:        firstAccount.PublicKey(),
	}

	// Submit the TX and get an ID
	txID, err := solanaRelayerAPI.SubmitTransaction(ctx, txRequest)
	if err != nil {
		t.Fatalf("Failed submitting TX %v", err)
	}

	// Wait until TX reaches finality
	err = solanaRelayerAPI.WaitUntilState(ctx, api.UntilFinalizedRequest(txID))
	if err != nil {
		t.Fatalf("Failed to get TX status to finalized in expected time: %v", err)
	}
}

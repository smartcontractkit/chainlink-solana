# Testing Tools

The package `github.com/smartcontractkit/chainlink-solana/pkg/solana/testing` includes useful tools to setup local Solana Test Validators node for testing purposes

## Quick Start Guide

```go
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
    
    // Create a relayer API and start testing
}
```
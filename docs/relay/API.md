# Solana Relayer API

This is the main API to be used by clients when importing chainlink-solana module for blockchain connectivity.

## Quick Start Guide

```go
func TestSubmitTransactionAndGetStatusUntilFinality(t *testing.T, nodeURL config.URL, chainID string) {
    // Create a config for initializing the relayer API
    relayerConfig, _ := api.NewRelayerConfigBuilder().
        WithNode(api.NewNodeConfigBuilder().WithURL(*nodeURL)).
        WithChainID(chainID).
        Build()

    // Configure the required APIs from the relayer. Simple implementations are provided for getting started quickly with the relayer
    requiredAPIs := &api.RequiredAPIs{
        KeystoreAPI: api.NewDummyKeystoreService([]solanago.PrivateKey{firstAccount.PrivateKey}),
    }

    solanaRelayerAPI, _ := api.NewSolanaRelayerAPI(ctx, relayerConfig, requiredAPIs)

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

    // Submit a TX and get the ID
    txID, _ := solanaRelayerAPI.SubmitTransaction(ctx, txRequest)

    // Wait until TX reaches finality
    err = solanaRelayerAPI.WaitUntilState(ctx, api.UntilFinalizedRequest(txID))
    if err != nil {
        t.Fatalf("Failed to get TX status to finalized in expected time: %v", err)
    }
}
```
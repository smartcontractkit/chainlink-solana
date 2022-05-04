# Relay Configuration Parameters

The relay can be configured with various specific parameters. This document discusses the optional parameters and their impact.

## Job Spec Parameters

```toml
[relayConfig]
chainID            = "<insert solana chain id>"
ocr2ProgramID      = "<insert solana ocr2 program ID>"
transmissionsID    = "<insert solana ocr2 transmissions account>"
storeProgramID     = "<insert solana ocr2 store account>"
```

### Parameters

| Parameter         | Description                                                                                                                                                                                     | Default      | Options                                    |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------ | ------------------------------------------ |
| `chainID`         | chainID is used to find corresponding chains/nodes for endpoints and configuration, based on genesis blockhash, unrecognized hashes default to `localnet`                                       | **required** | `mainnet`, `testnet`, `devnet`, `localnet` |
| `ocr2ProgramID`   | the deployed OCR2 program (for production services typically: [cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ](https://explorer.solana.com/address/cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ))   | **required** |                                            |
| `transmissionsID` | the transmission account for the specific feed                                                                                                                                                  | **required** |                                            |
| `storeProgramID`  | the deployed OCR2 program (for production services typically: [HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny](https://explorer.solana.com/address/HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny)) | **required** |                                            |

## Chains & Nodes Configuration

Additional configuration for the solana chain and endpoints are handled via the nodes and chains configuration in the Chainlink core node

```bash
chainlink chains solana create --id=<chain-id> {}

chainlink chains solana configure --id=<chain-id> <parameter>=<value> <parameter>=<value> ...

chainlink nodes solana create --name=<node-name> --chain-id=<chain-id> --url=<url>
```

### Chain Parameters

| Parameter             | Description                                                                                                                                                                                                        | Default     | Options                               |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------- | ------------------------------------- |
| `BalancePollPeriod`   | rate for polling SOL balance and updating Prometheus metric                                                                                                                                                      | 5s          |                                       |
| `ConfirmPollPeriod`   | rate for polling for signature confirmation                                                                                                                                                                        | 500ms       |                                       |
| `OCR2CachePollPeriod` | rate for polling state for OCR2 cache                                                                                                                                                                              | 1s          |                                       |
| `OCR2CacheTTL`        | stale OCR2 cache deadline                                                                                                                                                                                          | 1m          |                                       |
| `TxTimeout`           | timeout to send tx to rpc endpoint                                                                                                                                                                                 | 1m          |                                       |
| `TxRetryTimeout`      | duration for tx to be rebroadcast to rpc, txm stops rebroadcast after timeout                                                                                                                                      | 5s          |                                       |
| `TxConfirmTimeout`    | duration when confirming a tx signature before signature is discarded as unconfirmed                                                                                                                               | 15s         |                                       |
| `SkipPreflight`       | enable or disable preflight checks when sending tx                                                                                                                                                                 | `true`      | `true`, `false`                       |
| `Commitment`          | Confirmation level for solana state and transactions. ([documentation](https://docs.solana.com/developing/clients/jsonrpc-api#configuring-state-commitment))                                                       | `confirmed` | `processed`, `confirmed`, `finalized` |
| `MaxRetries`          | Parameter when sending transactions, how many times the RPC node will automatically rebroadcast a tx, default = `0` for custom txm rebroadcasting method, set to `-1` to use the RPC node's default retry strategy | `0`         |                                       |

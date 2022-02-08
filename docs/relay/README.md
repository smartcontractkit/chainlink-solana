# Relay Configuration Parameters

The relay can be configured with various specific parameters. This document discusses the optional parameters and their impact.

```toml
[relayConfig]
nodeEndpointHTTP   = "http:..."
ocr2ProgramID      = "<insert solana ocr2 program ID>"
transmissionsID    = "<insert solana ocr2 transmissions account>"
storeProgramID     = "<insert solana ocr2 store account>"
usePreflight       = false       # optional, defaults to false
commitment         = "confirmed" # optional, defaults to "confirmed"
txTimeout          = "1m"        # optional, defaults to "1m"
pollingInterval    = "1s"        # optional, defaults to "1s"
pollingCtxTimeout  = "2s"        # optional, defaults to `2x ${pollingInterval}`
staleTimeout       = "1m"        # optional, defaults to "1m"
```

### Parameters

| Parameter           | Description                                                                                                                                                  | Default                      | Options                               |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------- | ------------------------------------- |
| `usePreflight`      | Controls if simulating transactions are enabled when transmitting (disabled for speed)                                                                       | `false`                      | `false`, `true`                       |
| `commitment`        | Confirmation level for solana state and transactions. ([documentation](https://docs.solana.com/developing/clients/jsonrpc-api#configuring-state-commitment)) | `confirmed`                  | `processed`, `confirmed`, `finalized` |
| `txTimeout`         | Timeout for sending a transaction                                                                                                                            | `"1m"`                       |                                       |
| `pollingInterval`   | State polling interval                                                                                                                                       | `"1s"`                       |                                       |
| `pollingCtxTimeout` | Request timeout during state polling                                                                                                                         | `2 x pollingInterval = "2s"` |                                       |
| `staleTimeout`      | Used when polling is unsuccessful to indicate that consumed data is stale, and will cause errors to throw                                                    | `"1m"`                       |                                       |

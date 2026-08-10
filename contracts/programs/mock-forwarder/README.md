# mock-forwarder

Dev/test mock of `keystone-forwarder`. Used by `cre workflow simulate` to
exercise Solana chain-write workflows locally without needing a registered
DON config or OCR-signed reports. Solana analog of
`chainlink-aptos/contracts/platform_mock/sources/mock_forwarder.move`.

**Not for production.** Skips ECDSA signature verification and the
`oracles_config` account check entirely. Account-hash check, replay
protection via `execution_state`, and the `invoke_signed` CPI into the
workflow-supplied receiver all mirror prod, so the on-chain instruction
shape and event timeline match what a workflow sees in production.

## Build

From `chainlink-solana/contracts/` (same prerequisites as the existing
programs — Anchor 0.31, Solana SBF toolchain; see `contracts/README.md`):

```bash
# Generate the program keypair, then sync declare_id! + Anchor.toml entry.
solana-keygen new -o target/deploy/mock_forwarder-keypair.json --no-bip39-passphrase
anchor keys sync

anchor build
```

`anchor keys sync` overwrites the `11111111111111111111111111111111`
placeholder in both `programs/mock-forwarder/src/lib.rs` and
`contracts/Anchor.toml`.

## Deploy to devnet

```bash
solana program deploy target/deploy/mock_forwarder.so \
  --program-id target/deploy/mock_forwarder-keypair.json \
  --url devnet

# Note the program id printed; this is `forwarderProgramId` for the cre-cli
# `supported_chains.go` entry.
```

## Initialize the forwarder state account

The `report` instruction requires a `ForwarderState` account owned by this
program. Initialize once per environment:

```bash
# TODO: add a small Go helper under cmd/ (mirror of
# chainlink-deployments/.../canary/contracts/solana/cmd/deploy_receiver) that
# calls the Anchor `initialize` instruction with a fresh state keypair and
# prints the resulting `forwarderState` pubkey.
```

Record the resulting `(forwarderProgramId, forwarderState)` pair — both go
into `cre-cli/cmd/workflow/simulate/chain/solana/supported_chains.go`.

## Differences from `keystone-forwarder`

| Feature                                                 | keystone-forwarder | mock-forwarder       |
| ------------------------------------------------------- | ------------------ | -------------------- |
| ECDSA signature verification                            | required           | **skipped**          |
| `oracles_config` PDA                                    | required           | **removed**          |
| `init_oracles_config` / `update` / `close` instructions | yes                | **removed**          |
| Account-hash check (`ForwarderReport`)                  | yes                | yes                  |
| `invoke_signed` into receiver                           | yes                | yes (identical path) |
| Events (`ReportInProgress`, `ReportProcessed`)          | yes                | yes                  |

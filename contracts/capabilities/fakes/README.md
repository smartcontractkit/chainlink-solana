# Solana chain fakes

In-process implementation of the Solana chain capability used by cre-cli's
`cre workflow simulate`.

Reads (`GetAccountInfoWithOpts`, `GetBalance`, `GetSlotHeight`) proxy to the
configured RPC. Writes are the interesting part.

## How `WriteReport` works

Real DON writes go through the keystone-forwarder program, which verifies
`f+1` DON signatures against on-chain config. A local simulation cannot
produce those signatures, so the fake writes through a **mock forwarder**
instead — a permissionless program with the same instruction shape and CPI
behavior that skips signature verification (see
`programs/mock-forwarder`). The mock forwarder's program id and state account
come from the simulator config (cre-cli hardcodes per-chain defaults; see
`cre workflow supported-chains`).

SDK-generated bindings (Go and TypeScript) build reports for **production**:

- `remainingAccounts` layout: index 0 = forwarder state, index 1 = forwarder
  authority PDA, index 2+ = receiver-specific accounts — taken from the
  workflow config, i.e. the _real_ keystone forwarder;
- the report embeds `account_hash = sha256(concat(remainingAccount pubkeys))`,
  which the forwarder recomputes on-chain over
  `[state, authority, ...remaining]` and rejects on mismatch
  (`Custom:6002 InvalidAccountHash`).

Since the fake substitutes the mock forwarder's state and authority, the
workflow-computed hash would never match. The fake therefore:

1. strips remaining accounts 0–1 (mirroring the real transmitter's
   `forwarder_client.go`, which maps them onto named instruction accounts);
2. **rewrites `account_hash` in place** over the list the mock forwarder will
   actually see: `[mock state, mock authority, ...receiver accounts]`
   (`patchReportAccountHash`). This is safe precisely because the mock
   forwarder verifies no DON signatures, and the transaction signature is
   produced by the fake's transmitter key afterwards.

Net effect: workflows keep their production config — no simulation-specific
forwarder addresses, no binding changes. An `info` log is emitted whenever the
hash is rewritten.

## What workflow/receiver authors still need

The fake cannot influence the **receiver program's own caller validation**.
A receiver following the keystone pattern verifies that the CPI comes from
its trusted forwarder (state-account owner + authority PDA derived under a
stored or compiled-in forwarder program id). For simulation to reach
`on_report` successfully, that trust anchor must accept the mock forwarder —
e.g. initialize the receiver's state once against the mock forwarder program
id on devnet. This mirrors EVM, where receivers are deployed with the
documented (mock) forwarder address as a constructor argument.

## Modes

`dryRunWrites=true` routes through `SimulateTransaction` (no fees, no
signature verification); otherwise the transaction is sent and confirmed on
the live cluster and a real signature is returned.

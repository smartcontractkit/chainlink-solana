# Data Feeds Store: scan findings response

## Scope

This document responds to the 16 findings in `scan-findings-2026-11-20-17-24-36.csv`, produced against commit `4646f9ec19739ea5b721bd5dc69800d535c93ff1` of `smartcontractkit/chainlink-solana`.

The findings cover the new `data-feeds-store` program and its reader crate. DF1, OCR2 and the keystone forwarder are unchanged.

Dispositions below mean:

- **Fixed:** the implementation and tests changed as described.
- **Intended behavior:** the reported behavior is deliberate and consistent with the documented design.
- **Accepted limitation:** the behavior exists, but its risk or operational value does not justify a code change now.
- **Not a defect:** the finding's security premise does not hold for this implementation.

## Summary

| Finding                                                       | Severity      | Disposition                    |
| ------------------------------------------------------------- | ------------- | ------------------------------ |
| `12d84b23` — frozen feed still writes DF1                     | Medium        | Intended behavior              |
| `01b26999` — DF1 truncates small answers to zero              | Low           | Fixed                          |
| `02686b61` — setters implicitly create default-writable feeds | Low           | Intended behavior              |
| `57cea78d` — no batch/DF1-size caps                           | Low           | Accepted limitation            |
| `5ac024d5` — markers share ACCOUNT_VERSION                    | Low           | Fixed                          |
| `a28f0c1b` — unbounded forward window                         | Low           | Intended behavior              |
| `c17efe16` — zero/small forward window                        | Low           | Intended behavior              |
| `c4ba0cdd` — transfer-hook recovery and token-account closing | Low           | Accepted limitation            |
| `388a13fb` — DF1 nonzero answer truncates to zero             | Low           | Fixed; duplicate of `01b26999` |
| `11ea9681` — direct decoder skips state-version check         | Informational | Not a defect                   |
| `17e74b19` — direct decoder does not check the feed PDA       | Informational | Not a defect                   |
| `2b584cb6` — clock timestamp cast to u32                      | Informational | Fixed                          |
| `2d558e6f` — link change aborts in-flight batches             | Informational | Intended behavior              |
| `a8cdfb21` — unchanged link rechecks DF1 layout               | Informational | Fixed                          |
| `c430d2e2` — marker rent remains locked                       | Informational | Accepted limitation            |
| `dbaad088` — skip events are spoofable                        | Informational | Intended behavior              |

## Fixed findings

### `01b26999` and `388a13fb`: DF1 nonzero-to-zero truncation

**Response: fixed by skipping only the DF1 write.**

`df1::check_round` returns `Df1SkipReason::RoundsToZero` when a nonzero 18-decimal answer would become zero at the linked DF1 feed's decimals. The store still stores its exact round. DF1 is not called, so the DF1 feed may be passed read-only in this case. At supported decimal settings, a genuine zero answer remains valid and is still written. At DF1 decimals 57 and above, the scale factor itself overflows and every answer, including zero, is rejected as `AnswerOverflow`; zero-only support would not make those feeds usable, so no special case was added.

Implementation:

- `contracts/programs/data-feeds-store/src/df1.rs:125-164`
- `contracts/programs/data-feeds-store/src/state.rs:308-342`
- Generated Go enum bindings in `contracts/generated/data_feeds_store/types.go`

We deliberately did **not** set the store's minimum answer from the DF1 feed's decimals. That would reject valid store rounds merely because a legacy DF1 feed could not represent them, coupling the canonical store to DF1's precision. Aborting the whole report was rejected for the same reason.

Evidence considered:

- **Stellar** stores answers unchanged at 18 decimals and returns `RoundsToZero` only when a proxy read would truncate a nonzero answer to zero. It has no DF1-style write-through path. See `data-feeds-proxy/src/contract.rs` in [smartcontractkit/chainlink-stellar](https://github.com/smartcontractkit/chainlink-stellar/blob/main/contracts/data-feeds/data-feeds-proxy/src/contract.rs).
- **EVM** `DataFeedsStore._downscaleAnswer` divides without a zero check, so a legacy proxy can return zero at lower decimals. See [DataFeedsStore.sol:1709-1722](https://github.com/smartcontractkit/svr-auction-don/blob/main/contracts/src/v0.2-df-store-spf/DataFeedsStore.sol#L1709-L1722). EVM needs no write-through because its legacy proxies downscale on reads; its callbacks receive the original 18-decimal answer.
- **Current Solana DF1** accepts zero in `submit`, and its deviation check can flag a large change. Skipping DF1 avoids presenting a false zero to legacy readers, at the documented cost that DF1 retains its previous round until a representable answer arrives.

Tests cover positive and negative tiny answers, genuine zero, the smallest representable positive answer, preservation of the store round, and passing DF1 read-only when its write is skipped. A property test asserts that no nonzero input produces a successful zero DF1 write:

- `contracts/programs/data-feeds-store/src/df1.rs:384-422,498-513`
- `contracts/tests/data-feeds-store.spec.ts`, test "skips only DF1 when a non-zero answer would become zero"

### `5ac024d5`: shared account version and marker migration

**Response: fixed with independent account-type versions.**

The program has:

```rust
pub const STORE_STATE_VERSION: u8 = 1;
pub const FEED_ACCOUNT_VERSION: u8 = 1;
pub const DF1_LINK_VERSION: u8 = 1;
```

See `contracts/programs/data-feeds-store/src/common.rs:4-13`.

Store-state, feed and DF1-link checks use their respective constants. Store states and markers remain at version `1`; feeds use version `2` because their layout changed. A future incompatible layout change can bump only the affected account type rather than invalidating unchanged markers through an unrelated constant.

The current layout is the initial public version 1. Development-era staging accounts are abandoned and recreated rather than migrated. The Go e2e upgrade step checks the upgradeable loader with the current binary, not old-layout migration. Deployment reads reject feed versions other than 1 before consuming decoded fields.

### `a8cdfb21`: rechecking an unchanged DF1 link

**Response: fixed for the disable-only path.**

`set_df1_feeds` still validates a DF1 feed when creating, changing or enabling a link. It skips the layout recheck only when disabling an existing link. This preserves fail-fast validation when a write is enabled while allowing an admin to disable an existing write if DF1's layout becomes unsupported. Reports still run `check_round` before every `submit`.

Implementation: `contracts/programs/data-feeds-store/src/instructions/feeds.rs:399-421`. The deployment planner applies the same predicate, so a closed or unsupported DF1 feed no longer blocks a disable-only change during planning.

The Anchor test "can disable an existing DF1 link after the DF1 feed closes" closes a linked DF1 feed, verifies that disabling the existing link succeeds, and verifies that re-enabling it fails with `InvalidDf1Feed`. The invalid-link test also covers creation and changes with `write_disabled = true`, proving that disabled writes do not bypass validation.

### `2b584cb6`: clock timestamp cast to u32

**Response: fixed by using u64 Unix-second timestamps throughout the new store.**

`ReceivedReport`, the feed header, historical rounds, `Round`, events and the internal clock representation use `u64`. The signed Solana clock value is converted with `u64::try_from`, rejecting a negative value with `StoreError::InvalidTimestamp` rather than silently wrapping it. Consumers receive full u64 Unix-second timestamps, with the same width as Stellar's rounds.

DF1 stores transmission timestamps as `u32`. Its `submit` argument is `u64`, but DF1 casts it internally, so `check_round` returns `Df1SkipReason::TimestampOverflow` when an observation timestamp exceeds `u32::MAX`. The exact store round still lands; only DF1 is skipped, preventing a wrapped legacy timestamp.

The report entry is 40 bytes. A historical round is 48 bytes after explicit Rust alignment padding, and the feed header is 1,344 bytes. A newly created feed's ring holds at most 185 rounds within the 10 KiB creation limit; an existing ring can grow by 213 rounds per instruction before its first wrap. The measured V1 batches still hold 40 store-only or 25 DF1-backed reports.

## Read interface behavior

The customer-facing read interface has these behaviors:

- **Error precedence:** `InvalidDecimals` precedes `FeedFrozen` for both round reads and direct decoding.
- **Metadata freeze:** `decimals` and `description` refuse a frozen feed, while `get_min_decimals` remains unfrozen.
- **Missing feeds:** with otherwise valid arguments, a correctly derived but uninitialized feed returns `NoDataPresent` for `latest_round`, `get_round`, `decimals`, `description` and `get_min_decimals`. For round reads, decimals are validated first against the default minimum of 18, so invalid decimals return `InvalidDecimals`. `get_min_decimals` deliberately has no 18 fallback for an unset feed, keeping all missing-feed data reads consistent.
- **Wrong-address precedence:** a wrong empty feed address returns `AccountMismatch` before the fallback decimals check, even when the decimals argument is also invalid. The client crate applies the same behavior, so CPI and direct decoding agree.

Implementation and regression coverage are in `contracts/programs/data-feeds-store/src/instructions/reads.rs`, `contracts/programs/data-feeds-store/src/context.rs`, `contracts/programs/data-feeds-store/src/utils.rs`, `contracts/crates/chainlink-solana-data-feeds-store/src/lib.rs` and `contracts/tests/data-feeds-store.spec.ts`.

## No-change findings

### `12d84b23`: a frozen feed keeps receiving rounds, including DF1 writes

**Intended behavior.** `frozen` is a read freeze only. Round reads, `decimals` and `description` fail while reports continue to the store and any linked DF1 feed. `get_min_decimals` does not check the freeze and returns `NoDataPresent` for an unset feed. `writes_disabled` is the separate write freeze. The two flags are documented in `state.rs`, `feeds.rs` and the README. Treating the read freeze as a write freeze would remove the intended ability to keep history current while consumers cannot read prices.

### `02686b61`: per-feed setters can create feeds writable by the default workflow list

**Intended behavior.** Only a feed admin can invoke these setters and pay for feed creation. A new feed without a specific workflow list deliberately uses the store's default list. This is not an access-control bypass. The deployment changesets separately require existing feeds for setters where accidental creation would be undesirable.

### `57cea78d`: no explicit cap on batch CPIs or DF1 account size

**Accepted operational limitation.** Transaction size, account locks, instruction count, compute and loaded-account-data limits are runtime constraints. An oversized signed transaction cannot be made landable by adding a program-side report-count check. The workflow must size batches correctly. The README documents the measured limits, and the e2e tests assert transaction size and account-count limits.

### `a28f0c1b` and `c17efe16`: forward window has no lower or upper bound

**Intended EVM-compatible behavior.** The EVM constructor and `setMaxObservationForwardDurationSeconds` accept every `uint32` value without validation. See [DataFeedsStore.sol:224-236](https://github.com/smartcontractkit/svr-auction-don/blob/main/contracts/src/v0.2-df-store-spf/DataFeedsStore.sol#L224-L236) and [DataFeedsStore.sol:399-404](https://github.com/smartcontractkit/svr-auction-don/blob/main/contracts/src/v0.2-df-store-spf/DataFeedsStore.sol#L399-L404). Solana retains that feed-admin-controlled behavior, including zero for a strict no-future-timestamps policy. No validation was added.

### `c4ba0cdd`: transfer-hook mints and token-account closing

**Accepted limitation.** Normal store operation creates no token accounts. This recovery path matters only if an external party creates and funds a token account owned by the store authority, typically by mistake. Ordinary SPL and compatible Token-2022 tokens can already be recovered. Transfer-hook support would execute additional external programs and require extra account privileges, which is not justified for this rare path. Closing a standard emptied token account would recover only about 0.002 SOL of rent. Neither addition is warranted now; the program is upgradeable if a real recovery case requires one later.

### `11ea9681`: direct decoding does not check the store state's version

**Not a defect.** Direct decoding receives a trusted state pubkey, not a state account. It validates the feed's owner, discriminator, feed-account version, state and data ID. Round reads depend on the feed, not on state contents. The CPI path additionally checks the state account through `FeedRead`, as expected.

### `17e74b19`: direct decoding does not bind the feed address to its PDA

**Not a defect under the program's account-creation invariant.** Every valid feed is owned by the store program, and the program creates feeds only at the canonical PDA. `decode_feed_into` checks the discriminator, version, state, data ID and ring length. An external caller cannot create a forged program-owned feed at another address. An additional address derivation would be optional defense in depth, not a required security check.

### `2d558e6f`: a DF1 link change aborts in-flight signed batches

**Intended behavior.** The DON signs the account keys. Changing a link changes the account list expected by an already-signed batch, so that batch aborts atomically rather than risking a wrong account interpretation. Other reports in the aborted transaction are rolled back consistently and can be delivered by the next report. The cutover runbook explicitly says to change links between rounds.

### `c430d2e2`: cleared DF1 link markers retain their rent

**Accepted limitation.** A marker holds roughly 0.001 SOL and can be reused if the DF1 feed is linked again. Keeping cleared markers avoids manual lamport movement interacting with later CPIs in the same instruction. A separate close instruction would add privileged code and audit surface for a very small amount of rent, so markers remain allocated.

### `dbaad088`: arbitrary signers can cause skip events

**Intended behavior with explicit indexer guidance.** No caller can store a round without matching the feed's permitted sender, workflow owner and workflow name. Events emitted before the permission check carry `sender`; indexers must accept report events only from trusted forwarder authorities. Post-permission events are not arbitrarily spoofable. CPI events prevent log truncation from silently dropping these signals.

## Mainnet production context

We queried the production DF1 program `HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny` and cross-checked the [Solana mainnet reference-data directory](https://github.com/smartcontractkit/reference-data-directory/tree/master/solana-mainnet/contracts).

The directory marks 14 feeds as live:

- 10 use 8 decimals;
- 1 uses 6 decimals;
- 3 use 18 decimals.

At 8 decimals, nonzero answers below `0.00000001` truncate to zero; at 6 decimals the threshold is `0.000001`. The 18-decimal feeds do not downscale. Current answers are far above these thresholds: for example, JUP/USD was approximately `0.2248` at 8 decimals and SYRUPUSDC-USDC was approximately `1.1945` at 6 decimals. No current production feed is near its threshold under normal conditions.

All 14 directory-listed live feeds have `minSubmissionValue = 1` at their DF1 decimals, and Solana OCR2 enforces `report.median >= min_answer` onchain (`contracts/programs/ocr_2/src/lib.rs:747-751`). This is useful migration context, but it does not justify applying a DF1-derived minimum to the independent store.

## Verification

- `cargo test -p data-feeds-store -p chainlink_solana_data_feeds_store`: **33 passed** (22 program tests and 11 reader-crate tests), including property tests.
- `cargo test -p data-feeds-store -p chainlink_solana_data_feeds_store --all-features`: **62 passed** (51 program tests, including the IDL-build tests, and 11 reader-crate tests).
- `anchor build -p data_feeds_store`: passed.
- `cargo clippy -p data-feeds-store -p chainlink_solana_data_feeds_store --all-targets -- -D warnings`: passed, with standard and all features.
- `tests/data-feeds-store.spec.ts` on an isolated local validator: **30 passed**, including ownership, recovery, bounds-clearing and read regression tests.
- `../scripts/test-data-feeds-store-wrong-versions.sh`: **3 passed**, covering wrong state, feed and link-marker versions through read and admin instructions.
- `go test ./generated/data_feeds_store`: passed.
- `go test ./gotests -run '^$'`: the e2e package compiled successfully.
- `go test ./gotests -run 'TestDataFeedsStore$' -v`: passed, including real-forwarder reports and the in-place program upgrade.
- Workflow `bun test chains/solana`: **76 passed**; `bunx tsc --noEmit`: passed.
- Full workflow `bun test`: **773 passed**.
- Deployment `go test ./changesets/solana` against the local updated bindings: passed.
- Devnet staging: both zones landed non-skipped post-cutover reports in the version 1 store and its existing DF1 feeds. Store and DF1 timestamps and rescaled answers matched, and public `latest_round`, `get_round`, `decimals`, `description` and `get_min_decimals` reads matched direct account decoding.
- Changed TypeScript specs compile and run under `ts-mocha`. A focused TypeScript 5.9.3 check reports no errors in the changed store specs, only the pre-existing `tests/utils.ts` `Buffer`/`EthKeypairInfo` mismatch. The full workspace check also contains pre-existing errors in other specs, examples and Web3 typings.
- `git diff --check`: passed.

The deployment integration harness currently stops during MCMS setup with `DeclaredProgramIdMismatch`, before any store operation. This is the pre-existing harness issue noted in the deployment PR, not a result of the store changes.

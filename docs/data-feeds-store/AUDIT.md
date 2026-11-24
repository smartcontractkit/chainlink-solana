# Data Feeds Store on Solana: Audit brief

This brief is for the internal review, then the external audit. It says what the program does, what it trusts, the invariants it keeps and the risks we know about. The reasons behind the design are in the design decisions ([design-alignment.md](design-alignment.md)), the program's behavior in [README.md](README.md), and the full detail (layouts, every instruction, error and event, cases reviewed) in the audit reference ([AUDIT-REFERENCE.md](AUDIT-REFERENCE.md)). Any difference between this brief and the code is a finding: report it, since either one may be wrong.

**Pull requests:**

- the program: chainlink-solana [#1676](https://github.com/smartcontractkit/chainlink-solana/pull/1676);
- the deployment changesets: chainlink-data-feeds [#291](https://github.com/smartcontractkit/chainlink-data-feeds/pull/291);
- the workflow's Solana adapter: chainlink-data-feeds [#290](https://github.com/smartcontractkit/chainlink-data-feeds/pull/290).

The program already runs on DF CRE staging against Solana devnet. §12 has the evidence, with explorer links.

## 1. What it is and scope

Chainlink Data Feeds are moving on Solana from OCR2 to CRE (Chainlink Runtime Environment). A CRE DON (decentralized oracle network) signs reports, and the keystone forwarder verifies them and calls the **store**, which keeps a history per feed and serves it to readers. The store ports the EVM `DataFeedsStore` (`svr-auction-don/contracts/src/v0.2-df-store-spf`) without SVR. It is also the program readers integrate with, with no separate proxy: each data ID has one feed account, owned by the store, that readers pass to the store's reads or decode themselves.

Existing customers read the feed accounts of the current Solana store program, called **DF1** here. A data ID can be linked to one of those accounts, and the store then writes each of its rounds there too, through DF1's unchanged `submit` instruction, signed by the store's own writer PDA.

In scope, in `smartcontractkit/chainlink-solana` (Anchor 0.31), on PR [#1676](https://github.com/smartcontractkit/chainlink-solana/pull/1676):

- `programs/data-feeds-store` (about 3,500 lines with tests, new), program ID `9qCsUGTdKCanybkroJuPzAhuRU8P2SZN4NCexevSZAwu`.
- `crates/chainlink-solana-data-feeds-store` (about 590 lines with tests), the client crate for programs that read feeds.

DF1 (`programs/store`) is unchanged and out of scope, except for what the store assumes about it (§5). Also out of scope: the keystone forwarder (audited separately; what we rely on is in §2), CRE nodes and the workflow, and the deployment changesets, which drive the owner and feed admin instructions.

## 2. Architecture and trust

```
CRE DON
   |  signed report
   v
forwarder
   |  CPI on_report
   v
data-feeds-store  --CPI submit-->  DF1 store
   ^
   |  CPI reads, or decode the feed account
readers
```

Each **zone** (an independent CRE deployment with its own DON, forwarder and transmitters) writes the same feeds. The program relies on these assumptions and doesn't re-check them on-chain:

- **Forwarder.** It accepts a report only with F+1 valid signatures from its DON, over the report and the account addresses it is sent with. It calls the store as its authority PDA `["forwarder", forwarder_state, receiver_program]`, delivers each report at most once, passes its 64-byte metadata header unchanged, and passes every other account unsigned.
- **The store trusts senders through its allowed-workflow list.** Each entry is `(allowed_sender, workflow_owner, workflow_name)`, and each zone's forwarder authority is the sender of one entry. Nothing on-chain checks that a sender is a forwarder: only the deployment process ensures it (risk 3).
- **DON.** Answers are correct and at 18 decimals; observation timestamps are correct. The store checks ordering, freshness and answer bounds, not correctness.
- **Transmitters are untrusted.** The DON's signature covers the report and the account addresses, but not which accounts are marked writable, so a transmitter chooses the writable flags, the compute budget, and whether to send. They can make a transaction fail (risk 5), but not change what it writes.
- **Runtime.** A program can't be re-entered through a chain of calls (A→B→A). Calls nest at most 5 deep, an account grows by at most 10 KiB per instruction, the 32 KiB heap is never freed, and a transaction has at most 64 instructions (inner calls and events included) and 64 accounts.
- **Privileged roles.** The program limits what the upgrade authority, a store's owner and its feed admins can do, but not whether they do it: a compromised role can do anything its role allows (risk 2). On mainnet the MCMS timelock holds all three, so every change is a timelock proposal that signers approve and that waits out a delay. MCMS also owns the DF1 feeds and switches their writer; in an incident, a rollback goes through the MCMS bypasser, which skips the delay.

## 3. Where to start

In order of the harm a bug would do:

1. **Report path** (`instructions/on_report.rs`, `logic.rs`, `utils.rs`), the only path that writes rounds. Check that each report is skipped or aborted correctly, that a skipped report steps past exactly its own accounts, and that the one feed buffer reused for every report never carries one feed's data into the next.
2. **DF1 write** (`df1.rs`). A missed way for `submit` to fail aborts a whole batch, and a wrong check or conversion writes a wrong price to a DF1 feed customers read.
3. **Account checks and growth** (`state.rs`, `context.rs`, `instructions/feeds.rs`). A wrong layout, address or version check could let one account pass for another; feed creation, ring growth and the link markers (`set_df1_feeds`) change accounts in place.
4. **Reads** (`instructions/reads.rs` and the client crate). A wrong decimals, rescaling or freeze check returns a wrong price, and a weak account check in a direct decode returns another feed's price.
5. **Owner and store-wide settings** (`instructions/admin.rs`): who can create a store, transfer ownership, change feed admins and recover funds.

## 4. Report path

`on_report(metadata, report)` takes:

- **Arguments:** the forwarder's metadata header (at least 62 bytes; the store reads only the workflow name and owner, as on EVM) and a Borsh `Vec<ReceivedReport { data_id: [u8;16], timestamp: u64, answer: i128 }>`. Each report entry is 40 bytes, and timestamps are Unix seconds.
- **Fixed accounts:** the forwarder state and authority, the store state, the DF1 program and DF1 writer, and the event authority and program. The DF1 program and writer are optional. Anchor marks an optional account as absent by passing the program's own ID in its place, so a transaction in which no report writes DF1 passes the store's program ID for both.
- **Per-report accounts:** the report's feed account and, if the feed has a DF1 link, the DF1 feed.

Reports take their accounts from this list in order, so a skipped report must still step past exactly the accounts it would have used; otherwise the next report would read the wrong feed. Each report is checked in this order:

1. Data ID non-zero with the SVR path bytes zero, else **skip** (`InvalidDataIdIgnored`). The report used one account.
2. Feed account created, else **skip** (`FeedNotInitializedIgnored`): an empty account still owned by the system program. The report used one account. The address isn't checked against the data ID's feed address; risk 4 explains why that is harmless.
3. Feed account owned by the store, with the right discriminator, length, version, `state` and data ID, else **abort**. The program doesn't recompute the feed's address on each report. It trusts the store state and data ID recorded in the account, because only a feed admin can create a feed, and creation checks the address.
4. If the feed has a DF1 link, the next account is that DF1 feed, else **abort**.
5. Write freeze off, else **skip** (`FeedWritesDisabledIgnored`).
6. Sender, workflow owner and name match the feed's own workflow list, or the store's default list if it has none, else **skip**.
7. Timestamp no later than now plus the forward window (`max_observation_forward_secs`), else **skip**.
8. Timestamp newer than the latest, else **skip** (`StaleReport`).
9. Answer within the answer bounds, if set, else **skip**.
10. Feed passed writable, else **abort**. Checked before the DF1 write, so DF1 is never written for a round the store can't keep.
11. DF1 write, if linked: either `submit` is called, or it is skipped with `Df1WriteSkipped { reason }` (§5). The round is stored either way. If `submit` would be called, the DF1 feed must be writable, else **abort**; when DF1 won't be written, a read-only DF1 feed is fine. The abort stops a transmitter from making the store skip DF1 just by passing the feed read-only.
12. Round `latest + 1` written into the ring and the header (`FeedUpdated`).

Leftover accounts abort, as do a malformed payload, a short header, bad fixed accounts, a negative chain-clock timestamp (`InvalidTimestamp`) and running out of compute or memory. The signed clock value is converted with `u64::try_from`. A skip emits an event and lets the rest of the batch land, as on EVM; an abort covers input or account state that can't be trusted and required call or runtime failures. Reports never create feeds, because nothing in a report can pay rent.

**Batch size.** F is the number of faulty nodes a DON tolerates (a report needs F+1 signatures), and V1 is Solana's 4,096-byte transaction format. At F=5, a V1 transaction holds 40 store-only or 25 DF1-backed reports; reference §9 explains the limits.

**CPI depth.** Forwarder (1) → store (2) → store event (3), or → DF1 `submit` (3). Reader → store (2).

## 5. DF1 write

A feed admin links a data ID to a DF1 feed with `set_df1_feeds`. Creating, changing or enabling a link checks that the account is a DF1 feed `submit` could write. Disabling an existing unchanged link skips that layout check, so it remains available if DF1's layout becomes unsupported. The DF1 feed owner then sets its writer to the store's `df1_writer` PDA `["df1_writer", state]`. Solana can't contain a failed CPI, so before calling `submit` the store checks everything that would make it fail, and skips only the DF1 write if any check fails:

- The feed's own switch for the DF1 write is off (`Disabled`).
- The transaction didn't include the DF1 program and writer (`NotPassed`).
- The account isn't a DF1 feed `submit` can write: wrong owner, type, layout or version (`InvalidAccount`, `InvalidVersion`).
- The DF1 feed's writer isn't the store's `df1_writer` (`NotWriter`).
- DF1's round counter is used up (`RoundIdOverflow`).
- The observation timestamp exceeds `u32::MAX` (`TimestampOverflow`). DF1's `submit` takes `u64` but stores it as `u32`; skipping prevents that cast from wrapping.
- The round isn't newer than DF1's latest (`Stale`). `submit` doesn't check ordering itself: OCR2, DF1's current writer, orders reports before calling it, so the store must too.
- The answer, converted from 18 decimals to the DF1 feed's decimals, doesn't fit DF1's `i128` (`AnswerOverflow`).
- A nonzero answer would become zero at the DF1 feed's decimals (`RoundsToZero`).

Every skip still stores the exact store round. In particular, DF1 retains its previous round when the store's answer or timestamp cannot be represented safely there.

The checks read DF1's layout at fixed offsets (reference §5), so a DF1 upgrade that changes the layout or adds failure modes to `submit` must be checked against them (risk 1).

- **One data ID per DF1 feed.** If two data IDs linked the same DF1 feed, both would write their rounds into it and mix two price histories, and DF1 couldn't tell them apart because both write as the same `df1_writer`. So `set_df1_feeds` keeps a link marker `["df1_link", state, df1_feed]` that records which data ID links the DF1 feed, and refuses a second one (`Df1FeedAlreadyLinked`). The marker is created on the first link, with the feed admin paying, and cleared on unlink. Reports never read it.
- **Cutover and rollback, per feed.** The store's feed is created and written by CRE while OCR2 still writes DF1. Once the two agree, the data ID is linked, the DF1 feed owner sets the writer to `df1_writer`, and the OCR2 job is stopped. A rollback sets the writer back to OCR2's store authority. OCR2's first round afterwards may be older than the store's last round, and `submit` doesn't check order, so DF1 readers can briefly see the price step back until OCR2 catches up.
- **New feeds** launched after the cutover live in the store; a DF1 feed is created for one only on request, with `df1_writer` as its writer. The checks accept a DF1 feed with no rounds.

## 6. Roles and governance

- **Upgrade authority** is the only key that can `initialize` a store, checked through the program's ProgramData account, so every store under this program ID is ours. The authority becomes the store's owner. `initialize` creates the state account itself, at the PDA `["state", store ID]`, rather than filling in one created beforehand. An empty, program-owned state account waiting for an MCMS proposal could be taken over by anyone through Anchor's built-in IDL instructions (`IdlCreateBuffer`, then `IdlCloseAccount`), which would make `initialize` fail; creating the account inside `initialize` leaves no such window.
- **Owner:** two-step ownership transfer, feed admins (up to 64), token and SOL recovery from the store authority PDA.
- **Feed admins** create feeds and change every per-feed setting (README "Feed settings"). They also set the default workflow list (up to 16 entries) and the forward window. Unlike on EVM, where the default list can only be replaced, it can be emptied, which revokes a compromised default workflow without naming a replacement. Every per-feed setter but `remove_specific_feed_metadata` creates missing feeds, checking each address is the feed PDA, with the feed admin paying.
- **Ring size** only grows, and only before the ring has wrapped (`MaxStoredReportsLocked` after). Solana lets an account grow by at most 10 KiB per instruction, so each change adds at most 213 rounds. Inside an MCMS batch, the runtime counts that limit for the whole batch, which the program can't see, so a batch should grow each feed once.
- **Upgrades.** The program is upgradeable at a fixed address. Store state, DF1 link and feed layouts have independent versions: `STORE_STATE_VERSION = 1`, `DF1_LINK_VERSION = 1` and `FEED_ACCOUNT_VERSION = 2`. Every account carries its version, checked on load, so an incompatible layout requires explicit migration or replacement accounts. Readers decode the feed layout, so its fields move only with a version bump, and spare bytes are kept for later fields such as SVR's. Version 1 feeds are not migrated: the version 2 layout requires a new store and newly created version 2 feeds. New code runs first as a shadow deployment under a second program ID, then the main program is upgraded to that same verified build (README "Releasing a new build").

## 7. Reads

`latest_round(data_id, decimals)` and `get_round(data_id, round_id, decimals)` return `Round { round_id, answer, observations_timestamp, recorded_timestamp }` at the decimals the caller asks for, as Stellar's reads do (EVM returns a fixed number of decimals). When several checks fail, the first in this order decides the error, as on Stellar:

1. `FeedFrozen` while the read freeze is set;
2. `InvalidDecimals` below the feed's minimum (18 until lowered) or above 18;
3. `NoDataPresent` for a round never written or no longer in the ring;
4. `RoundsToZero` when a non-zero answer would read as zero.

`decimals`, `description` and `get_min_decimals` complete the feed reads.

`check_feed_permission` takes the data ID's feed account (required) and answers from the feed's own list, or the default list when it has none. It answers false for a feed that isn't set. Like the EVM function it mirrors, it ignores the write freeze, which EVM doesn't have.

The client crate offers each of these reads except `check_feed_permission` two ways: by calling the store, or by decoding the feed account in the caller's program (`read_latest_round` and so on). Every function checks that the account is a feed of the store program for the store state and data ID the caller trusts, and the round reads apply the program's own checks and rescaling. A caller that decodes without the crate must make the same checks.

## 8. Invariants

1. A feed's round ID rises by exactly 1 per stored report, and its observation timestamp strictly increases. Only `on_report` writes rounds.
2. A skipped report writes nothing to the store or DF1; an aborted transaction writes nothing.
3. A report is stored only if `(sender, workflow_owner, workflow_name)` matches the feed's own workflow list, or the default list when it has none, and the write freeze is off.
4. A feed account is created only by a feed admin, at the PDA of its padded data ID, and is then bound to `(state, data_id)` by its stored fields.
5. A stored timestamp is at most its recorded time plus the forward window in force when it was written.
6. If a feed has answer bounds when a report arrives, the answer stored for that report is within those bounds.
7. A DF1 feed is written only for a round the store also stores, only with a newer timestamp than DF1's latest that fits u32, only by `submit` signed by the store's `df1_writer`, and only for the DF1 feed linked to the data ID.
8. A DF1 feed is linked from at most one data ID of a store.
9. Each store PDA signs one kind of thing: its own creation (feeds, link markers), DF1 `submit` (`df1_writer`), recovery transfers (store authority) or events (event authority).
10. Only the owner changes feed admins and ownership; only feed admins change feed configuration; nobody closes a feed.
11. The store keeps the DON's answers. Neither reads nor DF1 writes turn a non-zero answer into zero: such a read fails, and such a DF1 write is skipped with `RoundsToZero` while the exact store round lands. A timestamp above DF1's u32 range similarly skips only DF1 with `TimestampOverflow`.
12. A ring read returns round `r` only for `1 ≤ r ≤ latest` and `latest − r < ring size`.

## 9. Main risks

1. **A DF1 failure the store didn't foresee aborts the whole batch.** The pre-checks cover every way the current `submit` fails. Running out of compute still aborts, and so would a DF1 upgrade that adds a failure mode or a closed DF1 program. DF1 upgrades are under our control, and a feed admin can unblock a data ID by unlinking its DF1 feed or turning off its DF1 write.
2. **A compromised feed admin** can change who writes any feed, answer bounds, the forward window, ring sizes, freezes and DF1 links, and spend its own SOL creating feeds. Same power as on EVM; on mainnet the role is the MCMS timelock.
3. **An allowed sender can be any key,** as on EVM, where the store accepts any caller that matches a workflow entry. Listing an ordinary keypair would let its holder write without the DON. The changesets only derive senders from a forwarder's program and state; review workflow lists against known forwarders.
4. **Anyone can cause skip events.** Any signer can call `on_report` directly. The invalid-ID, not-initialized and write-freeze checks come before the permission check, so anyone can trigger those skips, and the not-initialized skip doesn't check the account's address. Nothing is written, and these events name the sender. Indexers should filter on our store state and senders, and not treat skip events as feed state.
5. **Transmitters choose writable flags.** A wrongly read-only account aborts the transaction (§4, steps 10 and 11), and the forwarder's record of delivery reverts with it, so another transmitter delivers the report. A skip instead would mark the report delivered and lose the round for good. This costs liveness, not data, and only a faulty or malicious transmitter causes it.
6. **Transaction limits cap batches.** The 64-account limit is the first one a batch reaches: a V1 transaction at F=5 holds at most 25 DF1-backed reports. A larger batch fails to send, without writing anything.
7. **Upgrades change behavior at the same address.** The upgrade authority is the MCMS timelock, and version checks stop a layout change from silently clobbering accounts.

## 10. Questions for auditors

1. **Is the split between skips and aborts sound?** A report is skipped when something about the report itself is wrong, and the whole batch aborts when an account is wrong. One case to check: reports take their accounts in order, and a report skipped because its data ID is invalid or its feed doesn't exist yet is assumed to use exactly one account (a feed that doesn't exist can't have a DF1 link). If that assumption were wrong, the later reports in the batch would read the wrong accounts. We believe the worst outcome is an aborted batch, because each account is checked before it is used. Is that right?
2. **Is the DF1 write-through the right approach, and is it correct?** The store keeps existing DF1 feeds written by checking in advance every way DF1's unchanged `submit` could fail, then calling it (design decision 3 compares this with alternatives). Is the approach sound, do the checks cover every failure, and are the decimal and timestamp conversions right?
3. Can a report land with a round or timestamp ordering that breaks invariants 1–5 (same batch, retries, two forwarders)?
4. Are the checks in the client crate's direct decode enough for a reader that passes the store state it trusts?
5. Anything to handle about CPI events and CPI depth, for indexers or callers wrapping the forwarder?

## 11. Tests, gaps and readiness

**Tested.**

- **31 Rust unit and property tests** check the pure pieces against simple models: the ring, ordering and answer bounds, ring growth, rescaling, DF1 layout, answer and timestamp checks, and that the DF1 layout check accepts exactly the accounts DF1's `submit` can write. The client crate's tests check each way a decoded feed is refused.
- **27 Anchor specs** run every instruction on a local validator, including permissions, both freezes, ring growth, read-only aborts, DF1 zero truncation and closed-feed disabling, DF1 cutover and rollback against the unchanged DF1 program, and 25 random batches checked event by event against a model.
- **The Go end-to-end test** sends reports through a real forwarder at F=2 and F=5, checks every transaction fits the account and size limits, and performs an in-place loader upgrade of the current binary mid-test.
- **Workflow tests:** 76 focused Solana tests and 773 full workflow tests pass. Report encoding uses u64 timestamps restricted to non-negative JavaScript safe integers, and feed decoding rejects timestamps above that safe range.
- **DF CRE staging on devnet:** both zones write the store and, through it, DF1 (§12).

**Gaps and compatibility boundaries.** We suggest closing the first two test gaps before the external audit, since auditors will ask about them. The third item is the current upgrade boundary, not a claim of old-layout support.

- **Ownership transfer, token and SOL recovery, and clearing answer bounds** aren't run end to end on a validator yet.
- **Refusing an account with the wrong version in program instructions.** The Anchor accounts are created by the program at the current version. The reader crate unit-tests version rejection, but the on-chain instruction paths do not exercise hand-made accounts with another version.
- **Old-layout migration.** The Go test's in-place loader upgrade uses the current binary and proves neither migration nor version 1 feed compatibility. Version 1 feeds are not migrated; the version 2 layout requires a new store and newly created version 2 feeds.

**Findings and exit.** One PR per finding. Severity runs:

- **Critical:** lost or corrupted feed data, or an unauthorized write.
- **High:** a broken invariant without a privileged role.
- **Medium:** liveness, or misuse needing a privileged role beyond its intent.
- **Low** and **Informational.**

The internal review exits when:

- each area has been reviewed by someone who didn't write it;
- every Critical and High finding is fixed;
- every gap is closed or accepted;
- this brief is re-checked against the commit that was reviewed.

## 12. Running on DF CRE staging

Since 2026-11-18, both DF CRE staging zones write the store on Solana devnet, and through it DF1, along the same path mainnet will use: each zone's DON (F=2) signs the report, that zone's forwarder delivers it, and the store keeps the round and calls DF1's `submit`. This staging evidence does not demonstrate version 1 feed compatibility with the version 2 layout; version 1 feeds are not migrated.

**What was deployed.**

- The store program [`J6Xc…`](https://explorer.solana.com/address/J6XcUrbL3pwKgv6Lryfi24QREkueGq7WDaniJJ2Ymsrj?cluster=devnet), upgraded in place to #1676 at `a4d8ef4f`.
- A store at [state `CQWU…`](https://explorer.solana.com/address/CQWU4ya5YaqDDoDwCVqyQMqyuJX1HwbrvrBcapSQwV99?cluster=devnet), whose DF1 program is Chainlink's devnet DF1 [`HEvS…`](https://explorer.solana.com/address/HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny?cluster=devnet) and whose default workflow list holds both zones' workflows.
- BTC/USD: store feed [`6YZ2…`](https://explorer.solana.com/address/6YZ2MCP4mTCiJmwtPGFE3n7fTKc2WyBwKLt3im931yER?cluster=devnet), linked to the DF1 feed [`CcKr…`](https://explorer.solana.com/address/CcKr6frHVWyFbN3bgxPFziUszEfreS2VHB3vWuehpjER?cluster=devnet) (8 decimals, one live round as on mainnet), whose writer is the store's [`df1_writer`](https://explorer.solana.com/address/3W8RwxvS1266dNcRYmUYeX3FXwZ8JUvwX5Pxo8qsA7LE?cluster=devnet).
- ETH/USD: store feed [`Dpz3…`](https://explorer.solana.com/address/Dpz38ZtuMdQVbPeJ5j1uq8zd46jX52hFaxCT7Fw4BBHH?cluster=devnet), linked to the DF1 feed [`468e…`](https://explorer.solana.com/address/468enw9SksE8Vyh3f8k9cC7dNKHqPkCcB1SBX4THyfma?cluster=devnet), with the same writer.
- The forwarders: zone-a [`DtLX…`](https://explorer.solana.com/address/DtLXqEtnSHebkaZJAMcT4viTg2crdXtKsDMqEUjfxWqf?cluster=devnet) and zone-b [`9muo…`](https://explorer.solana.com/address/9muoNeMxYaRVQTgEEpzfpuAo7AD73gvLaZF9QHr4skuN?cluster=devnet), each running the workflow from #290.

**What it showed.**

- Every round reached both copies: each store round was also a new DF1 round.
- DF1 customers' reader, `read_feed_v2` from the `chainlink_solana` crate, decodes both DF1 feeds, and its BTC/USD answer is the store's answer at 8 decimals.
- No transaction failed in either zone.
- A round stored and written to DF1 costs about 141–146k compute units, forwarder included. In zone-b's [BTC/USD round](https://explorer.solana.com/tx/5v4jx65qgwN4Rq2VyzmMQ9aDBPQHeCivMHQh4DCfXcyYdM5VoESi1kTL3oohdQWqVZcNu9AQSn8xUFTwVMD85Zkv?cluster=devnet), the trace shows the forwarder calling the store and the store calling DF1's `submit`.
- The other zone's copy of the same round is skipped as stale, as designed, at about 134–138k ([zone-a's copy](https://explorer.solana.com/tx/2vLrMDGkyvvTJcBaUQ6DumE2EjTwUE9ikwYTRGsw84wF3HVcDUE14rGxMitZogDrPBCodh5Vo5r8a6sh7ZseyDTg?cluster=devnet)).

**In progress:** the staging run continues, writing both feeds every round.

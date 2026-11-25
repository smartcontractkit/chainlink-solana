# Data Feeds Store on Solana: A guided tour of the code

This is a tour for someone who knows EVM contracts but not Solana. It explains why the code is shaped the way it is, then walks through it in the order worth reading. File paths are under `contracts/programs/data-feeds-store/src/` unless they say otherwise.

**Pull requests:**

- the program: chainlink-solana [#1676](https://github.com/smartcontractkit/chainlink-solana/pull/1676);
- the deployment changesets: chainlink-data-feeds [#291](https://github.com/smartcontractkit/chainlink-data-feeds/pull/291);
- the workflow's Solana adapter: chainlink-data-feeds [#290](https://github.com/smartcontractkit/chainlink-data-feeds/pull/290).

The program already runs on DF CRE staging against Solana devnet. The evidence, with explorer links, is in the [audit brief, §12](AUDIT.md#12-running-on-df-cre-staging).

## 1. Where the lines of code go

Program logic is about a seventh of the lines. Line counts from the repo at commit 24b9b3bf, blank lines left out (about 14,600 lines in all):

- Generated Go bindings: 8,818
- **Program logic: 2,179**
- Anchor spec (TypeScript): 1,057
- Comments in the program: 963
- Rust unit and property tests: 876
- Go end-to-end test: 598
- Client crate: 116

Most of the lines aren't program logic:

- **Generated Go bindings (8,818 lines)** are produced by a tool (anchor-go) from the program's interface description. Nobody writes or reviews them by hand; skip them.
- **Tests are about 2,500 lines:** Rust unit and property tests inside the program and the client crate, the Anchor spec in TypeScript, and the Go end-to-end test.
- **Comments are about 950 lines** inside the program itself, roughly one for every two lines of logic. They are written as full sentences for reviewers new to Solana, with a short glossary at the top of `lib.rs` and diagrams of the feed account, the report accounts and decisions, the DF1 link accounts and ring growth.

That leaves about **2,200 lines of program logic**, and a good share of that is declarations rather than behaviour:

- **Declarations (about 750 lines):** the account lists Anchor checks for each instruction (`context.rs`), the error and event types (`error.rs`, `event.rs`), the account layouts (`state.rs`) and the thin entry points (`lib.rs`).
- **Feed admin setters (about 500 lines, `instructions/feeds.rs`):** eleven setters that share one helper, plus the DF1 linking.
- **Shared helpers (about 350 lines, `utils.rs`):** creating accounts, loading and saving feeds, reading the ring, emitting events, permission checks.
- **The report path itself (about 500 lines):** `instructions/on_report.rs`, the ring and scaling in `logic.rs`, and the DF1 checks in `df1.rs`. This is the part that matters most, and it is small.
- **The rest:** owner actions (`instructions/admin.rs`) and reads (`instructions/reads.rs`).

Compared with the EVM store, the extra code comes from Solana making the program do work EVM does for you: listing and checking every account, allocating storage up front, and keeping memory and call counts within hard limits. Section 8 covers those.

## 2. The Solana basics you need, in EVM terms

Eight ideas explain almost every unfamiliar line in the program.

- **A program has no storage of its own.** An EVM contract is code plus its storage. A Solana _program_ is only code; its data lives in separate _accounts_, which are byte arrays, each owned by one program. Only the owning program can change an account's data. The store program owns one state account per store and one account per feed.
- **Every transaction lists every account up front.** An EVM contract can read any storage slot as it runs. On Solana the caller lists, before execution, every account the transaction will touch and whether it will write it. That's why each instruction has an account list, and why reports carry a variable list of feed accounts (the _remaining accounts_). The program's first job is to check that each account passed is the one it should be.
- **PDAs are addresses a program controls without a key.** A _program-derived address_ is computed from some seeds and the program ID, like a CREATE2 address. Nobody holds its private key, but the program can sign for it. Feed accounts live at `["feed", state, data ID]`, so given a data ID anyone can compute the address. The store also signs DF1 writes as a PDA.
- **Storage is paid for up front.** An account needs a deposit (_rent_) proportional to its size, paid by a signer when the account is created. There is no lazy SSTORE: a feed's whole history ring is allocated when the feed is created. The forwarder doesn't sign for the accounts it passes, so reports can't pay for new feeds; a feed admin creates them.
- **Calls to other programs can't be caught.** A call to another program (a _CPI_) that fails fails the whole transaction. There is no try/catch as in Solidity. That's why the store checks everything DF1's `submit` could fail on before calling it.
- **Programs are upgraded in place.** A program has an upgrade authority that can replace its code at the same address, much like an EVM proxy with upgrades built in. The accounts it owns stay where they are.
- **Anchor is the framework.** It's roughly Solidity plus OpenZeppelin-style helpers. `#[program]` marks the entry points, `#[derive(Accounts)]` structs declare each instruction's accounts and the checks on them (think modifiers), and `#[account]` types get an 8-byte type tag, the _discriminator_, at the start of their data. `zero_copy` accounts are read in place as fixed-layout structs.
- **Hard limits shape the code.** A transaction is at most 1,232 bytes (or 4,096 for the newer V1 format), locks at most 64 accounts, and runs at most 64 instructions, counting inner calls and events. A program gets a 32 KiB memory heap that is never freed, and can grow an account by at most 10 KiB per instruction. Section 8 shows where each of these appears in the code.

## 3. Map of the program, and the order to read it

Read in this order. The first four files give you the whole shape; the rest fill it in.

1. **`lib.rs`: the menu.** Every instruction the program offers, with a one-line doc each. Each entry just calls a handler in `instructions/`, so read it as a table of contents.
2. **`state.rs`: what's stored.** The `StoreState` account (one per store) and the `Feed` account (one per data ID), with their field layouts, plus the small `Df1Link` account and the argument types.
3. **`context.rs`: who may call what with which accounts.** One struct per instruction listing its accounts and Anchor's checks on them: who must sign, which account must be the owner, which must be writable.
4. **`instructions/on_report.rs`: the report path.** The one instruction the forwarder calls, and the only path that writes prices. Section 4 walks through it.
5. **`logic.rs`: the arithmetic.** Writing a round into the ring, finding a round in it, growing the ring, rescaling to the decimals a reader asks for. Plain functions with no Solana in them, so they're easy to test.
6. **`df1.rs`: talking to the old store.** The checks that DF1's `submit` won't fail, the decimal conversion, and the `submit` call itself.
7. **`instructions/feeds.rs`: feed admin settings.** Creating feeds, descriptions, workflow lists, answer bounds, the two freezes, minimum decimals, DF1 links and ring growth. Section 6.
8. **`instructions/reads.rs`: reads.** `latest_round`, `get_round` and the small getters. Section 7.
9. **`instructions/admin.rs`: the owner and store-wide settings.** Creating a store, ownership transfer, feed admins, the default workflow list, recovering tokens and SOL.
10. **`utils.rs`: shared helpers**, read as you meet them: creating an account at a PDA, loading and saving a feed, the ring accessors, the event emitter, permission checks.
11. **Reference files:** `error.rs` (error codes), `event.rs` (events), `common.rs` (constants, seeds and DF1's byte offsets), `data_id.rs` (the 16-byte data ID layout).

Outside the program:

- **`contracts/crates/chainlink-solana-data-feeds-store`:** the client crate other programs use to read feeds, either by calling the store or by decoding a feed account themselves.
- **`contracts/crates/arrayvec`:** the unchanged production macro that implements the store's admin and workflow lists. Review `src/lib.rs` with the program.
- **`contracts/programs/store`:** DF1, the existing store, unchanged. The store calls its `submit`.
- **`contracts/programs/keystone-forwarder`:** the forwarder that checks the DON's signatures and calls `on_report`.
- **`contracts/tests/data-feeds-store.spec.ts` and `contracts/gotests/data_feeds_store_test.go`:** the end-to-end tests (section 9).

## 4. Follow one report end to end

A report reaches the feed only after the forwarder and the store check it:

1. **The CRE DON signs a report:** a batch of (data ID, u64 Unix-second timestamp, answer), plus a hash of the accounts it will touch.
2. **A transmitter sends the transaction.** The DON's signature covers everything else, so the transmitter can only choose which accounts are writable, the compute budget and whether to send.
3. **The forwarder checks f+1 signatures**, then calls `on_report`, signing as its authority PDA (the sender the store checks).
4. **`on_report` checks each report in turn.** A bad report is skipped with an event; a bad account aborts the whole transaction.
5. **If the feed is linked, DF1's `submit` writes the round too**, but only after the store has checked every way `submit` could fail and checked the round's representation. A nonzero answer that rescales to zero is skipped with `RoundsToZero`; an observation timestamp above DF1's u32 range is skipped with `TimestampOverflow` before DF1 can wrap it internally. Every DF1 skip still stores the exact store round.
6. **The round goes into the feed's ring:** round ID + 1, the latest fields updated, then a `FeedUpdated` event.

The code for steps 4 to 6 is `instructions/on_report.rs`. `handler` reads the workflow name and owner from the forwarder's metadata, decodes the batch, loads the store state once, and then calls `Batch::apply` for each report. It converts the signed chain clock with `u64::try_from`; a negative clock timestamp aborts with `InvalidTimestamp`. `apply` checks the report in this order, and the order matters:

1. **Is the data ID valid?** Not zero and with no SVR path. If not, skip.
2. **Has the feed been created?** An empty account means a feed admin hasn't set it up yet. Skip.
3. **Is this really that data ID's feed?** Owned by the store, the right type tag, version, store and data ID. If not, abort: a wrong account can't be trusted.
4. **If the feed has a DF1 link, is the next account that DF1 feed?** If not, abort.
5. **Are writes frozen for this feed?** Skip.
6. **Is the workflow allowed?** The sender, workflow owner and workflow name must match the feed's own list, or the store's default list if the feed has none. Skip if not.
7. **Too far in the future, or not newer than the latest round, or outside the answer bounds?** Skip.
8. **Was the feed passed writable?** If not, abort. This comes after every skip, so a report that would be skipped anyway never aborts the batch for being read-only.
9. **DF1 write**, if linked: `write_df1` either calls DF1's `submit` or skips DF1 with a reason. The round is stored either way.
10. **Store the round:** `logic::store_round` writes it into the ring, `store_feed` saves the header, and a `FeedUpdated` event is emitted.

**Why this order.** The checks that an account really is the data ID's feed (3 and 4) come before any check that reads its settings, because a wrong account's settings can't be trusted. The writable check (8) comes after every skip, as noted above.

**Skip versus abort.** A skip emits an event and moves on, so the rest of the batch still lands, as on EVM. An abort fails the whole transaction when the payload, clock or accounts cannot be trusted, or when a required call fails. Reports also take their accounts from one list in order, so a skipped report still steps past exactly its own accounts; otherwise the next report would read the wrong feed.

**Why one feed buffer is reused.** `Batch` holds one `Box<Feed>` that every report loads into, rather than a fresh one per report. Solana's 32 KiB heap is never freed, so a fresh buffer per report would run a batch out of memory (section 8).

**Events are calls.** The store emits events by calling itself, signed by its event-authority PDA (_CPI events_), rather than by logging. They then appear in the transaction's inner instructions and can't be lost if the logs get truncated. Each event counts towards the 64-instruction limit.

## 5. What is stored where

There are three kinds of account. All are defined in `state.rs`.

**`StoreState`: one per store.** It holds the owner and a pending new owner, the feed admins (up to 64), the default workflow list (up to 16), the DF1 program the store may write to, the default ring size for new feeds, and the forward window (how far in the future a report's timestamp may be). `initialize`, which only the program's upgrade authority can call, creates it at an address derived from a store ID and fills it in, all in one instruction.

**`Feed`: one per data ID.** This is the account customers read. It's a fixed header followed by a ring of rounds:

```
| tag (8 B) | header (1,344 bytes)                  | slot 0  | slot 1  | slot 2  | slot 3  |
|           | latest round, description, settings   | round 8 | round 5 | round 6 | round 7 |
```

That's a ring of 4 after 8 rounds: round `r` is in slot `r mod 4`.

- **The header** holds the latest round and answer, the description, how the feed is read (minimum decimals, the read freeze) and how it's written (its own workflow list, answer bounds, the write freeze, the DF1 link). Timestamps are u64 Unix seconds.
- **The ring** keeps the last N rounds, 48 bytes each. Round `r` goes in slot `r mod N`, so once the ring is full, each new round overwrites the oldest. A read of an overwritten round fails with `NoDataPresent`.
- **The layout is public.** Readers may decode the account themselves, so fields never move without a version bump, and spare bytes are reserved for later fields. Store states, link markers and feeds all use version 1. This is the initial public layout; development-era accounts are abandoned and recreated rather than migrated.
- **It's created by a feed admin, who pays its rent:** about 0.01 SOL plus about 0.00033 SOL per round kept, so about 0.07 SOL for the largest default ring of 185 rounds.

**`Df1Link`: a small marker, one per linked DF1 feed.** It records which data ID links that DF1 feed, so no two data IDs can link the same one.

**The PDAs, and what each one signs for.** The program signs for these addresses itself:

- **Feed and link marker PDAs:** only their own creation.
- **Store authority** `["store_authority", state]`: holds any tokens or SOL sent to the store, and signs only when the owner recovers them.
- **DF1 writer** `["df1_writer", state]`: set as each linked DF1 feed's writer, and signs only DF1's `submit`.
- **Event authority:** signs the store's events.

This matters because a PDA's signature is the store vouching for something. If one PDA signed for several things, a bug in one path could let it authorise another, for example letting a DF1 write move recovered SOL. So each one is used for exactly one thing.

## 6. Feed admin settings: one helper, many setters

`instructions/feeds.rs` looks long, but most of it is one pattern repeated. Every per-feed setter takes a list of data IDs, with their feed accounts as the remaining accounts, and hands both to the shared helper `update_feeds` along with a small function saying what to change. The helper then, for each data ID:

1. **Checks the caller is a feed admin.**
2. **Creates the feed if it doesn't exist yet.** It checks the account passed is that data ID's PDA, then creates it with the feed admin paying the rent, filling in the defaults (version, 18 minimum decimals, the store's default ring size).
3. **Otherwise loads the feed** and checks it really belongs to this store and data ID.
4. **Runs the setter's change** on the header and saves it back.

It emits one `FeedsCreated` event first, listing every feed it created, so an indexer reading events in order sees a feed created before its settings. Then come each setter's own events.

So the setters themselves are short:

- **`set_feeds`:** creates feeds, sets descriptions and, optionally, their own workflow list. It's the main call for adding feeds.
- **`set_feed_descriptions`, `set_specific_feed_metadata`, `remove_specific_feed_metadata`, `set_min_max_answers`:** each changes one setting, named as on EVM.
- **`set_feed_writes_disabled` and `set_feed_frozen`:** the write freeze and the read freeze.
- **`set_min_decimals`:** the fewest decimals a read may ask for.

Two setters do more than change a field:

- **`set_df1_feeds` links a data ID to a DF1 feed.** Creating, changing or enabling a link checks that the account really is a DF1 feed that `submit` can write. Disabling an existing unchanged link skips that layout check, so it remains available if DF1's layout becomes unsupported. The link marker ensures no two data IDs link one DF1 feed. The marker is created on a DF1 feed's first link and cleared, not closed, on unlink. Closing an account means the program moves its SOL out by editing balances directly, and Solana rejects that if the same instruction later calls another program, which `set_df1_feeds` may do to create the next feed or marker.
- **`set_max_stored_reports` grows a feed's ring.** It only grows, and only before the ring has wrapped. For example, in a ring of 4, round 5 overwrites slot 1 (5 mod 4). If the ring then grew to 8, a read of round 5 would look in slot 5 and not find it. Before a wrap, every round sits in the slot equal to its round ID, so growing is safe.
  Solana lets an account grow by at most 10 KiB per instruction, counted from its size when the instruction started, so one call adds at most 213 rounds. The feed admin tops up the rent, then the account is resized.

The owner-only actions are in `instructions/admin.rs`: creating a store (`initialize`, upgrade authority only), the two-step ownership transfer, adding and removing feed admins, the store-wide default workflow list (on EVM it can only be replaced; here it can also be emptied) and the forward window, and recovering tokens or SOL sent to the store by mistake.

## 7. Reads and the client crate

A customer's program can read a feed in two ways, and both give the same answer.

**Call the store.** `instructions/reads.rs` has the read instructions:

- **`latest_round(data_id, decimals)` and `get_round(data_id, round_id, decimals)`** return the round with the answer scaled to the decimals the caller asks for. This matches Stellar.
- **`decimals`, `description` and `get_min_decimals`** are small getters.
- **`check_feed_permission`** says whether a workflow may write a data ID. It's mostly for tooling.

A round read checks, in this order (the same as Stellar):

1. **The decimals** are between the feed's minimum and 18, else `InvalidDecimals`.
2. **The read freeze** is off, else `FeedFrozen`.
3. **The round exists** and is still in the ring, else `NoDataPresent`.
4. **A non-zero answer doesn't come out as zero** at those decimals, else `RoundsToZero`. A reader should never mistake a tiny price for no price.

Answers are stored with 18 decimals and scaled down by dividing, which truncates toward zero. The scaling code is `logic::scale`.

`decimals` and `description` also check the freeze. `get_min_decimals` does not, but all three return `NoDataPresent` for an unset feed.

The result comes back as Solana _return data_, a small buffer a called program leaves for its caller (the equivalent of a Solidity return value).

**Decode the feed account directly.** Because a feed account's layout is public, a customer's program can read it in place with no call at all, which is cheaper. The client crate (`contracts/crates/chainlink-solana-data-feeds-store`) does this in `read_latest_round` and `read_round`, and in `read_decimals`, `read_description` and `read_min_decimals` for the other reads. For an uninitialized account it checks the feed PDA, returning `NoDataPresent` for the canonical missing feed and `AccountMismatch` for a wrong empty address. For an existing feed it checks that:

- the account is owned by the store program;
- it has the feed type tag and version;
- it belongs to the store and data ID the caller expects;
- its data is long enough for the ring its header claims.

Valid feeds can only be created at their canonical PDA, so existing-feed reads rely on that creation invariant rather than recomputing the address. These checks prevent a reader given another feed's account from getting its price or description without noticing. The round reads then run the same freeze, decimals and scaling checks as the program, using the same code. The crate also wraps the call route for each read (`latest_round`, `get_round`, `decimals`, `description` and `get_min_decimals`). These functions:

- take the store state the caller trusts and refuse any other;
- accept the answer only if the store program returned it.

Each deployment of the program is meant to hold one store, but the program doesn't prevent a second, which is why the store state is checked.

**No access control on reads.** On Solana any program can read any account passed to it, so reads can't be restricted. The EVM cache dropped read access control too.

## 8. How Solana's limits shape the code

Many of the program's checks and patterns, which a Solidity reader may not expect, come from one of Solana's limits. Each item below names a limit and the code that handles it.

- **The caller chooses every account.** Nothing stops a caller passing the wrong one, so every account is checked before it's trusted. That's what `context.rs` (Anchor's checks), `load_feed_into` and `decode_feed_into` in `utils.rs` are for: owner, type tag, version, store and data ID. New accounts are checked to be at the right PDA before they're created.
- **A failed call to another program fails everything.** There's no try/catch, so `df1::check_feed` and `df1::check_round` re-check, in advance, every way DF1's `submit` could fail. Each failure becomes a `Df1SkipReason` instead of a crash.
- **Writable flags aren't covered by the DON's signature.** The transmitter chooses which accounts are writable, so it could pass a needed account read-only. The store aborts in that case: a skip would mark the report delivered and lose the round, while an abort lets another transmitter deliver it.
- **The 32 KiB heap is never freed.** Each allocation is gone for the rest of the transaction. So:
  - one feed buffer is reused for every report (`Batch` in `on_report.rs`, `update_feeds` in `feeds.rs`);
  - one instruction buffer is reused for every event (`EventEmitter`) and every DF1 call (`df1::Submit`);
  - setters emit one workflow-list event per call rather than one per feed.
- **A transaction runs at most 64 instructions, events included.** So `FeedsCreated` is one event per call, a successful DF1 write emits no extra event, and `StoreInitialized` carries the whole admin list rather than an event per admin.
- **A transaction locks at most 64 accounts.** This is why a report passes just one account per feed (plus its DF1 feed), and why the feed's settings live in the feed account rather than a second account. It's also the main reason there is no separate proxy program.
- **An account grows by at most 10 KiB per instruction.** A new feed must fit in 10 KiB including its 1,344-byte header and 8-byte tag, which leaves room for 185 rounds; a later call can add a full 10 KiB, 213 rounds. That's why `set_max_stored_reports` records each account's starting size.
- **Storage needs a deposit paid by a signer.** The forwarder doesn't sign for the accounts it passes, so reports can't create feeds; a feed admin does.
- **Moving SOL out of an account directly fails if a call to another program follows in the same instruction.** That's why a DF1 link marker is cleared rather than closed (section 6).
- **The stack frame is 4 KiB.** The 3 KB store state and the 1.3 KB feed header are copied onto the heap (`Box`) rather than the stack.

## 9. Where the tests are and what each layer proves

There are four layers, from fastest to most realistic. Commands run from `contracts/`.

- **Rust unit tests** sit at the bottom of the files they test, in a `mod tests` block (`logic.rs`, `df1.rs`, `data_id.rs`, `event.rs`, and the client crate). There are 33 across the program and client crate. They check the pure pieces: the ring, ordering and bounds, scaling, DF1 layout, answer and timestamp checks, and that events encode the way indexers expect. Run with `cargo test`.
- **Property tests**, in `mod properties` inside `logic.rs` and `df1.rs`, generate thousands of random cases and compare the code with a simple model:

  - the ring serves exactly the last rounds, and growing before a wrap loses none;
  - scaling truncates and never turns a price into zero;
  - the DF1 layout check accepts exactly the accounts DF1's `submit` can write without crashing.

  They run with `cargo test` too.

- **The Anchor spec** (`tests/data-feeds-store.spec.ts`, 30 tests) deploys the program to a local Solana node and drives it from TypeScript:

  - a broad set of instructions and their callers;
  - skips and aborts, with the events decoded;
  - both freezes and decimals;
  - DF1 cutover and rollback against the real DF1 program;
  - 25 seeded random batches checked event by event against a model of the report path.

  `anchor test` runs it. `../scripts/test-data-feeds-store-wrong-versions.sh` runs three additional specs with preloaded wrong-version state, feed and link-marker accounts.

- **The Go end-to-end test** (`gotests/data_feeds_store_test.go`) runs a real keystone forwarder with real DON-style signatures at F=2 and F=5, as in staging and mainnet. It checks every transaction stays within the 64-account limit and V1's 4,096 bytes, measures transaction size and compute, and performs an in-place loader upgrade of the current binary. It does not test old-layout migration or compatibility with development-era accounts. It needs Docker.

The workflow has 76 focused Solana tests and 773 full workflow tests. Its report encoder uses u64 timestamps restricted to non-negative JavaScript safe integers, and feed decoding rejects timestamps above that safe range.

Beyond the tests, the program runs on DF CRE staging. Both staging zones' workflows write BTC/USD and ETH/USD to the store on Solana devnet, and every stored round is also written to a DF1 feed on Chainlink's deployed devnet DF1 program. This is the same path as mainnet: real DON signatures, the keystone forwarder, then the store's `submit` to DF1. The addresses and example transactions are in the [audit brief, §12](AUDIT.md#12-running-on-df-cre-staging).

If you read one test file, make it the Anchor spec: its test names read like a list of the program's behaviours.

## 10. Glossary

**Solana terms, with the nearest EVM idea**

- **Account:** a byte array with an owner program and a SOL balance. Roughly a contract's storage, but separate from the code.
- **Program:** deployed code with no storage of its own. Roughly a contract's code.
- **PDA (program-derived address):** an address computed from seeds and a program ID, with no private key; the program can sign for it. Like a CREATE2 address that the contract also controls. The _bump_ is one extra seed byte that makes the derivation work.
- **Rent:** the refundable deposit an account must hold for its size. Replaces paying gas per storage slot.
- **Lamports:** the smallest unit of SOL, like wei.
- **Discriminator:** the 8-byte type tag Anchor puts at the start of an account or instruction, like a function selector.
- **CPI (cross-program invocation):** a call to another program. Like an external call, but a failure can't be caught.
- **Remaining accounts:** the variable-length tail of an instruction's account list, used here for one feed per report.
- **Signer / writable:** flags on each account in a transaction. Signers have signed it; writable accounts may be changed.
- **Return data:** a small buffer a called program leaves for its caller, like a return value.
- **Zero-copy account:** an account read in place as a fixed-layout struct.
- **IDL:** the program's interface description, like an ABI. The Go bindings and the TypeScript types are generated from it.
- **Upgrade authority:** the key allowed to replace a program's code in place. Here, MCMS on mainnet.
- **Compute units:** Solana's gas, capped per transaction.
- **Legacy, V0 and V1 transactions:** transaction formats. Legacy and V0 are limited to 1,232 bytes; V1 allows 4,096 bytes. V0 can use _lookup tables_ to shorten account lists. Writes use legacy transactions today and will move to V1, without lookup tables, once CRE supports it.

**Project terms**

- **DON, F:** the decentralized oracle network that signs reports. F is how many faulty nodes it tolerates; a report needs F+1 signatures.
- **Forwarder:** the keystone program that checks the DON's signatures and calls the store's `on_report`.
- **Transmitter:** the node that sends a signed report to Solana.
- **Zone:** an independent CRE deployment with its own DON, forwarder and transmitters. Several zones write the same feeds.
- **Data ID:** the 16-byte ID of a feed, derived from its Data Streams feed ID, as on EVM.
- **DF1:** the existing Solana store program that OCR2 writes today and current customers read.
- **Cutover:** switching a DF1 feed from OCR2 to the store, by making the store's DF1 writer PDA its writer.
- **Link marker:** the small account recording which data ID links a DF1 feed.
- **Ring:** a feed's history, the last N rounds in a circular buffer.
- **Read freeze / write freeze:** per-feed switches that stop round, decimals and description reads, or stop writes, while the other carries on. `get_min_decimals` stays readable during a read freeze.
- **Skip event:** an event saying why one report in a batch wasn't stored, while the rest still land.
- **MCMS:** the multisig and timelock that holds every privileged role on mainnet.

For what was decided and why, see the design decisions ([design-alignment.md](design-alignment.md)). For the full detail of every instruction, error and event, see the audit reference ([AUDIT-REFERENCE.md](AUDIT-REFERENCE.md)).

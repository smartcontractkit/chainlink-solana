# Data Feeds Store on Solana: Design decisions

This doc lists the design decisions for the Solana Data Feeds program (the data-feeds-store), where we landed on each, and why. It compares each with the EVM `DataFeedsStore` and the Stellar proxy and cache. Comments are welcome on any of them.

**Pull requests:**

- the program: chainlink-solana [#1676](https://github.com/smartcontractkit/chainlink-solana/pull/1676);
- the deployment changesets: chainlink-data-feeds [#291](https://github.com/smartcontractkit/chainlink-data-feeds/pull/291);
- the workflow's Solana adapter: chainlink-data-feeds [#290](https://github.com/smartcontractkit/chainlink-data-feeds/pull/290).

The program already runs on DF CRE staging against Solana devnet. The evidence, with explorer links, is in the [audit brief, §12](AUDIT.md#12-running-on-df-cre-staging).

## What is different on Solana

Four properties of Solana drive most of the decisions below.

- **A program must be passed every account it touches.** It cannot look up another contract's storage itself. A customer's transaction therefore names the exact accounts a read uses. An EVM proxy can point customers at a new contract without them noticing; on Solana that would change the accounts customers pass, so it can't be done silently.
- **A failing call to another program fails the whole transaction.** There is no try/catch, so a program cannot contain a failing callee the way the EVM store contains a reverting callback.
- **Programs are upgraded in place.** The program ID and its accounts stay the same across upgrades. An upgrade applies to every account the program owns at once.
- **Accounts must be created, and rent paid, before data is written.** A feed's rent is about 0.01 SOL plus about 0.00033 SOL per round kept, so about 0.07 SOL at the largest default ring size (185 rounds). It stays locked because feeds are never closed.

DF1 on Solana already reflects these properties.

- **Its `store` program owns the feed accounts customers read.** Customers decode those accounts directly or call `store`'s `query`.
- **Its `ocr_2` program verifies reports** and writes them with `submit`.
- **A feed's writer is swapped with `set_writer`.** So DF1 puts the indirection on the write side, not on the read side.

## 1. Program layout: one program that owns the feeds

**Options.**

- One program that customers read directly.
- A read proxy that calls the store, as on EVM.
- A thin proxy that owns the feed data, as DF1 does.

**Landed.** One program, the data-feeds-store, owns every feed account customers read. A feed account holds the history of rounds, the description, the minimum read decimals, a read freeze, and the feed's write settings (its own workflow list, answer bounds, a write freeze and its DF1 link). Reads are instructions of the same program, or customers decode the account themselves. A report passes one account per feed, plus its DF1 feed if linked.

**Why.**

- **A read proxy doesn't work on Solana.** Customers would pass the proxy's accounts and the store's, so repointing the proxy would change the accounts they pass. It would also add a call and three accounts (the proxy program, its state and the store program) to every read.
- **A thin proxy that owns the feeds protects little.** Its purpose would be to keep the store's own settings out of the accounts customers read, so the store could change without changing them. But each feed's write settings (its workflow list, answer bounds, freezes and DF1 link) must sit in the feed account itself: a second account per feed would fit too few reports under the 64-account limit. So the store's settings end up in the proxy's accounts anyway, and the split still costs three accounts and a call per transaction.
- **Programs are upgraded in place.** Layout changes use independent account versions and reserved bytes, as DF1's have: store states are version 1, DF1 link markers version 1 and feeds version 2. As with DF1, upgrading the store upgrades the program customers read from; decision 12 covers seeing new code work before the upgrade.
- **Measured at F=5 (the mainnet DON), no lookup table:** a V1 transaction carries 40 store-only or 25 DF1-backed reports.

## 2. Phase IDs: none

**Landed.** No phase IDs, as on Stellar.

**Why.** EVM aggregator proxies encode a phase in round IDs so that IDs keep increasing when a proxy switches to an aggregator with its own counter. Here the program is upgraded in place, so a feed's round counter never restarts. Phase IDs could be added by upgrade if ever needed.

## 3. DF1 write-through instead of generic callbacks

**Why a write-through is needed.**

- **Existing customers read DF1 directly.** They read DF1 feed accounts through the `chainlink_solana` helpers, and no proxy layer exists to repoint.
- **Keep DF1 updated through `submit`.** So that customers change nothing, the store writes DF1 through its existing `submit`, with each DF1 feed's writer switched from OCR2 to the store.
- **DF1 is not upgraded.** Rollback is then `set_writer` back to OCR2, onto code that has run for years.

`submit` checks only that the writer signed. It trusts the writer to send rounds in order, which OCR2 does by epoch and round. So the store, as the new writer, must check the order itself before calling `submit`:

```rust
// programs/store (DF1)
pub struct Submit<'info> {
    #[account(mut)]
    pub feed: Account<'info, Transmissions>,
    #[account(address = feed.writer @ ErrorCode::Unauthorized)]
    pub authority: Signer<'info>,
}

// programs/ocr_2
require!((config.epoch, config.round) < (epoch, round), ErrorCode::StaleReport);
```

**Options.**

- **A generic report callback with an adapter program,** as on EVM.
- **A DF1-specific write in the store.**

**Landed.** No generic callbacks. The store calls DF1's `submit` itself, signing as a store PDA. Before calling, it checks everything that could make `submit` fail:

- the account's owner, layout and version;
- that the writer is the store;
- that the round counter can increase;
- that the observation timestamp is newer than DF1's latest round;
- that the observation timestamp fits DF1's u32 storage;
- the conversion from 18 decimals to the DF1 feed's decimals, including refusing to turn a nonzero answer into zero.

The store uses u64 Unix-second observation timestamps, but DF1's `submit` casts its u64 argument to u32 internally. A timestamp above `u32::MAX` therefore skips only DF1 with `TimestampOverflow`, preventing a wrapped legacy timestamp. A nonzero answer that rescales to zero skips only DF1 with `RoundsToZero`. Both paths store the exact store round. Any other failed precheck also skips the DF1 write with an event and still stores the round. A per-feed switch stops only the DF1 write. Creating, changing or enabling a link validates the DF1 layout; disabling an existing unchanged link does not, so an admin can still disable it if that layout becomes unsupported.

A linked DF1 feed passed read-only aborts the whole transaction, but only when DF1 would be written. The transmitter chooses which accounts are writable, and the DON's signature doesn't cover that choice, so only a faulty or malicious transmitter would pass the DF1 feed read-only. Aborting lets another transmitter deliver the report with the right flags, so the round reaches both the store and DF1; a skip would lose the DF1 write for good. When DF1 wouldn't be written (the switch is off, the store isn't the writer yet, or the round is stale), a read-only DF1 feed is fine, so a link that is disabled or not yet cut over doesn't take a write lock on the account OCR2 writes.

**Why.** The EVM store catches a reverting callback and lands every other report:

```solidity
(bool callSucceeded, bytes memory returnData,) = CallWithExactGas._callWithExactGasSafeReturnData(
  payload, address(rc.callback), rc.gasLimit, CALL_WITH_EXACT_GAS_CUSHION, REPORT_CALLBACK_MAX_RETURN_BYTES
);
```

On Solana a failing callee aborts every report in the transaction. With 20 or more reports per transaction, one bad callback would fail every feed in the batch on every retry, so all of them would go stale and miss their update SLOs. A DF1-specific write keeps any failure we can predict as a skip, avoids a second program to audit, and needs fewer accounts per report. Stellar and Aptos have no callbacks either. On EVM the only callback implementations are test mocks.

**One data ID per DF1 feed.** Two data IDs linked to the same DF1 feed would both write it, so the store refuses that. Linking records the data ID in a link marker for that DF1 feed, and a second data ID can't link it until the first unlinks. Only the feed admin call reads the link marker, so reports carry no extra account.

**Shown on staging.** DF CRE staging now writes the store on Solana devnet from both zones, and every stored round also reaches a DF1 feed on Chainlink's deployed devnet DF1 program through its unchanged `submit`. A round with its DF1 write costs about 141–146k compute units, forwarder included. The addresses and example transactions are in the [audit brief, §12](AUDIT.md#12-running-on-df-cre-staging).

**Callbacks later.** The store is upgradeable, so callbacks can still be added if a need appears, ideally for feeds in their own transactions so a failing callee can't block the rest.

## 4. Feeds are set by a feed admin

**Options.**

- Creation on the first report from a default-list workflow, as on EVM.
- The same, with rent paid from a vault the store holds.
- Creation by a feed admin call, as on Aptos and Stellar.

**Landed.** A feed admin creates each feed with `set_feeds`, which also sets its description and, optionally, its own workflow list (like Stellar's `set_feed_configs`). Other per-feed setters also create a feed that doesn't exist yet. The feed admin pays the rent. A report for a data ID whose feed hasn't been set is skipped with an event, even when the default workflow list would allow it.

**Why.**

- **Every feed needs a description,** which only a feed admin can give it.
- **A report can't pay for a new feed.** On EVM, a new feed's storage is paid in gas by the transmitter on its first report. On Solana a new account needs a rent deposit from a signer, and the forwarder passes every account on unsigned, so the store can't make the transmitter pay.
- **A vault would need its balance kept topped up.**

## 5. Workflow metadata: a default list plus per-feed lists

**Landed.** The EVM pattern. Write permission is a list of (forwarder, workflow owner, workflow name) entries. A feed's own list is used if it has one, otherwise the store-wide default list. `set_feeds` may leave a feed's own list empty to rely on the default.

**Why.**

- **On EVM the default list is the normal path:** a feed can be written with no setup. EVM's two default "workflows" are contracts: HandlerDF forwards DF1 aggregator rounds and HandlerDS forwards verified Data Streams reports.
- **On Solana every feed is set by a feed admin anyway** (decision 4), so the default list doesn't save a setup step. Its value is changing workflows across many feeds in one call, such as adding a zone's forwarder.
- **Other chains:** Aptos has one registry-wide list; Stellar has only per-feed lists, set with the feed.
- **The default list can be emptied,** unlike on EVM, where it can only be replaced. Then only feeds with their own list can be written, so a compromised default workflow can be revoked in one call without naming a replacement.

## 6. Who changes feed settings

**Landed.** The store's feed admins (MCMS on mainnet) change every per-feed setting: descriptions, ring size, the read freeze, the minimum read decimals, the feed's own workflow list, answer bounds, the DF1 link and the write freeze. On Stellar the proxy owner sets the minimum decimals; with no separate proxy here, it is a feed admin setting like the rest.

Instruction names follow EVM wherever EVM has the same function, for example `set_default_feed_metadata`, `set_min_max_answers` and `set_max_stored_reports`.

## 7. Decimals and reads

**Landed.** As on Stellar. Answers are stored with 18 decimals. Reads take the decimals the caller wants, between the feed's minimum and 18, and rescale. A non-zero answer that would round to zero is an error. Reports, feed headers, historical rounds, returned rounds and timestamp-carrying events all use u64 Unix seconds. The chain's signed clock is converted with `u64::try_from`, rejecting a negative value with `InvalidTimestamp`. Workflow encoding accepts non-negative JavaScript safe integers and rejects unsafe feed timestamps when decoding.

The reads are:

- `latest_round(data_id, decimals)`
- `get_round(data_id, round_id, decimals)`
- `decimals`
- `description`
- `get_min_decimals`
- `check_feed_permission`, whether a workflow may write a data ID, as on EVM.

A client library offers the same rescale for programs that decode the feed account directly, compiled into their program, so it adds no call.

## 8. Two freezes

**Landed.**

- **Freeze reads, not writes.** A flag in the feed makes reads and the client library refuse the feed, while rounds keep being written.
  - This matches Stellar's `set_feed_frozen` and disconnecting an EVM aggregator proxy from the store.
  - Programs that decode the account themselves are expected to check the flag.
- **Freeze writes, not reads.** A flag in the feed skips the feed's reports for every workflow, whether on the default list or the feed's own list. Reads keep serving the last round.
  - The EVM store has no write freeze.
  - Stellar stops writes by removing the feed's configuration; a flag keeps the feed's other settings.

The DF1 write has its own switch (decision 3).

## 9. Data IDs

**Landed.** The EVM 16-byte layout, with IDs derived from Data Streams feed IDs:

- the compact feed ID (bytes 0–7), which is the first 8 bytes of the Data Streams feed ID;
- the index of the value in the report (byte 8), for example benchmark, bid or ask;
- the SVR path (bytes 9–10);
- the instance (byte 11);
- 4 reserved bytes.

ETH/USD's benchmark is `0x000359843a543ee20000000000000000`.

**Why.**

There are two ID schemes in use. **Derived IDs**, used here, are built from the Data Streams feed ID as above. **Random IDs**, used on Aptos, Stellar and the EVM `DataFeedsCache`, start with a format byte followed by random bytes (ETH/USD is `0x01a9dde66f0003320000000000000000`).

- **The EVM handler for Data Streams needs no per-feed mapping.** HandlerDS, which forwards verified Data Streams reports to the EVM store (decision 5), works the data ID out from the signed report. SVR (Smart Value Recapture) also relies on derived IDs.
- **Solana can't easily change scheme later.** A feed account's address is derived from its data ID, so changing scheme would move customers to new accounts.
- **The two schemes never overlap.** Derived IDs start with the stream's schema version, and random IDs with a different format byte. Aptos and Stellar use the older scheme today, so for now the same asset has a different ID on Solana; moving them to derived IDs is agreed, as separate work.

## 10. Other decisions

- **MCMS holds every privileged role on mainnet:** the programs' upgrade authority, owners and feed admins. Only the upgrade authority can create a store.
- **`initialize` creates the store's state account itself,** at an address derived from a store ID, rather than filling in one created beforehand. The risk it removes: an empty state account created ahead of time sits waiting while the MCMS proposal to initialize it waits out its delay, and anyone could take it over through Anchor's built-in IDL instructions, which would block store creation. Building the program without those instructions (`no-idl`) would also close the gap, but explorers would then stop decoding the program's instructions. The upgrade authority pays the state's rent.
- **The client crate checks the store state on every read.** Each deployment of the program will hold one store: staging and production use separate programs. The program can still hold more than one, because `initialize` takes a store ID, so a reader passes the store state it trusts and every crate function refuses any other.
- **Feeds are never closed**, as on EVM, so their rent stays locked.
- **SVR is not implemented.** Data IDs with a non-zero SVR path are refused, and the feed account keeps spare bytes so SVR can be added by upgrade.
- **Descriptions live in each feed account and are keyed by data ID,** as on Stellar and DF1. EVM keeps them in a separate contract, shared by every instance of a feed; here each instance has its own.
- **Any key can be an allowed sender,** as on EVM, where the store accepts any caller that matches a workflow entry, contract or not. The forwarder is the normal sender; the store doesn't require one, and the deployment tooling only lists forwarder addresses.
- **There is no read access control.** Any program can decode an account passed to it, so Solana can't gate reads. This matches the EVM cache decision to drop read access control.

## 11. DF1 feed ownership and new feeds

**Landed.**

- **MCMS owns the DF1 feeds.** Cutover and rollback are `set_writer` on each DF1 feed, signed by its owner. Cutover goes through the timelock, like other changes; a rollback during an incident uses the MCMS bypasser, so it doesn't wait. DF1's upgrade authority is planned to move to MCMS too, as separate work outside this audit; until then any DF1 upgrade must be checked against the store's DF1 checks.
- **New feeds live in the store, with a DF1 feed on request.** A feed launched after the cutover is read from the store. For a customer who needs it at a DF1-style address, the deployment tooling creates a DF1 feed owned by MCMS, with the store as its writer, and links it. The store already accepts a DF1 feed that has no rounds yet.

**Why.** MCMS keeps every privileged role under one process, and the bypasser keeps a rollback fast. Creating DF1 feeds only on request avoids keeping two copies of every new feed.

## 12. Releasing new store code: a shadow deployment, then an upgrade

**The question.** On EVM a new store is deployed beside the live one and watched, then the aggregator proxy is repointed to it, with the old store as a fallback. Without a proxy, how do we see new store code working before customers read from it?

**Options.**

- A proxy that new store code is repointed behind, including a stateless store that writes rounds into a proxy that owns the feed accounts.
- A shadow deployment of the new build, then an in-place upgrade to it.

**Landed.** A shadow deployment, then an upgrade:

1. Deploy the candidate build under a second program ID on mainnet, with its own store and feeds.
2. Have a workflow, or a second zone, write to both, and compare the shadow feeds with the live ones for an agreed period.
3. Upgrade the main program to the exact build that ran as the shadow, checked by its verified-build hash, through an MCMS proposal.
4. To roll back, upgrade back to the previous verified build, through the MCMS bypasser in an incident.

Customers on DF1 feeds also keep their own switch: a DF1 feed's writer moves to the store only after the two have been compared, and moves back with `set_writer` (decision 11).

**Why.**

The proxy considered here is one that owns the feed accounts customers read, with the store writing into it, as DF1 does. A read proxy is ruled out by decision 1.

- **A proxy would be no faster or safer to switch.** Repointing the proxy and upgrading the program are each one MCMS action, with the same rollback path.
- **A proxy switches every feed at once too.** Switching feed by feed would need a writer per feed, which costs accounts in every report.
- **The settings would sit in the proxy.** Per-feed settings would have to live in the proxy's feed accounts, so a replacement store would either share their layout or need a migration, which undercuts the clean swap a proxy promises.
- **A proxy has standing costs:** a second program to audit and upgrade in step, one more call on every write, and about two fewer DF1-backed reports per V1 transaction (estimated). Reads gain no call, because readers would read the proxy's accounts directly.
- **The loader upgrade path is tested:** the end-to-end test upgrades the current binary in place and checks that current-layout feeds keep their history and keep taking rounds, DF1 writes included. It does not test old-layout migration. Version 1 feeds are not migrated: the version 2 layout requires a new store and newly created version 2 feeds. Staging's devnet program was also upgraded in place, keeping its program ID and the zones' forwarder addresses; that upgrade is not evidence of version 1 feed compatibility.

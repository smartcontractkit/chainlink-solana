# Data Feeds Store

`programs/data-feeds-store` is the Chainlink Data Feeds program on Solana for feeds written by CRE. It is a Solana port of the EVM `DataFeedsStore` (`svr-auction-don/contracts/src/v0.2-df-store-spf`) without SVR, and it is also the program readers integrate with: there is no separate proxy.

Reports arrive from the [keystone forwarder](../forwarder/README.md) through `on_report`. The store checks each one against the workflows allowed to write its data ID, its ordering and its answer bounds, and appends it to that data ID's feed account. A data ID can also be linked to an existing feed account of **DF1**, the current Solana store program that existing customers read. The store then also writes each round to that account, through DF1's unchanged `submit` instruction.

## Data IDs

A data ID is 16 bytes, laid out like `DataId.sol`: a compact feed ID (bytes 0–7, the first 8 bytes of the Data Streams feed ID), an index (byte 8, for example benchmark, bid or ask), an SVR path (bytes 9–10), an instance (byte 11) and 4 reserved bytes.

- **SVR paths are reserved.** The store serves data IDs whose SVR path is zero. Reports for other data IDs are skipped (`InvalidDataIdIgnored`), and feed admin calls reject them with `InvalidDataId`. The layout stays the same if SVR is added later.
- **Account addresses use the data ID padded with zeros to 32 bytes.** Reports and events use the 16-byte form. A longer data ID format later would therefore not change any address.
- **Instances are separate feeds.** Two data IDs that differ only in the instance or reserved bytes have separate histories and descriptions.
- **Answers are signed `i128` with 18 decimals**, the same answer type as DF1.

## Accounts

- **`StoreState`** (the PDA `["state", store ID]` that `initialize` creates: 3,240 bytes, a 3,232-byte struct after the discriminator): owner, feed admins, the default ring size, the forward window (`max_observation_forward_secs`, how far an observation timestamp may lead the clock), the DF1 program, and the default workflow list.
- **Feed** (PDA `["feed", state, data_id_32]`): the account readers pass. Its 1,328-byte header holds the latest round, the description, the minimum read decimals, the read freeze, the feed's own workflow list, its answer bounds, its write freeze and its DF1 link. A ring of `max_stored_reports` rounds follows (32 bytes each, round `r` at slot `r % max_stored_reports`). Its layout is public: fields move only with a version bump, and reserved bytes are kept for later fields, including SVR's.
- **Store authority** (PDA `["store_authority", state]`): holds tokens and SOL sent to the store, which the owner can recover. It signs nothing else.
- **DF1 writer** (PDA `["df1_writer", state]`): the writer the store signs DF1 `submit` calls with, set as each linked DF1 feed's writer.
- **Link marker** (PDA `["df1_link", state, df1_feed]`): which data ID links a DF1 feed, so no two do.

**Creating a store.** Only the program's upgrade authority can `initialize` a store. It passes a store ID (a `u64`), and `initialize` creates the state account itself at the PDA `["state", store ID as 8 little-endian bytes]`, so a timelock holding the authority can create a store by proposal. The authority becomes the store's owner and pays the state's rent (about 0.023 SOL), so a timelock signer needs that much SOL first. Creating the account in the same instruction leaves no window in which anyone could take over a half-created state account.

**Creating feeds.** A feed admin creates feeds with `set_feeds`, which also sets their descriptions and, optionally, their own workflow list. Every other per-feed setter but `remove_specific_feed_metadata` also creates a feed that doesn't exist yet. The feed admin pays the rent: about 0.01 SOL plus about 0.00022 SOL per round kept, so about 0.07 SOL at the largest default ring size (278 rounds). Reports never create feeds: the forwarder passes every account unsigned, so none could pay the rent. As on EVM, feeds are never closed.

## Reports

`on_report(metadata, report)` is called by the forwarder. `report` is Borsh `Vec<ReceivedReport { data_id: [u8; 16], timestamp: u32, answer: i128 }>`.

**Accounts.** After the forwarder's own two (`forwarder_state`, `forwarder_authority`) come the store state, the DF1 program, the DF1 writer, and the event authority and program. The DF1 program and writer are optional. Anchor marks an optional account as absent by passing the program's own ID in its place, so a transaction in which no report writes DF1 passes the store's program ID for both. Then, for each report in order, come the data ID's feed (writable) and its linked DF1 feed if the feed has one. Reports take these accounts in order, so even a skipped report steps past exactly its own accounts, and the next report reads the right feed.

**Each report is handled on its own**, as on EVM. A report is skipped with an event, and the rest of the batch still lands, when:

- its data ID is zero or has an SVR path (`InvalidDataIdIgnored`);
- its feed hasn't been set (`FeedNotInitializedIgnored`);
- the feed's write freeze is on (`FeedWritesDisabledIgnored`);
- its workflow isn't allowed (`InvalidUpdatePermission`);
- it is future-dated (`FutureReportIgnored`), stale (`StaleReport`) or outside the feed's answer bounds (`AnswerOutOfMinMaxRange`).

The first three skips (invalid data ID, feed not set, write freeze) come before the permission check, so anyone can cause them by calling the store directly. Their events carry the sender, so indexers can ignore those not sent by a known forwarder.

What still fails the whole transaction:

- a malformed payload or metadata;
- an account that isn't the data ID's feed or linked DF1 feed;
- a feed passed read-only;
- a linked DF1 feed passed read-only when the store would write DF1 (see "DF1 feeds").

Why a read-only account aborts rather than skips: the transmitter chooses which accounts are writable, and the DON's signature, which the forwarder checks, doesn't cover that choice. Only a faulty or malicious transmitter passes a needed account read-only. A skip would mark the report delivered and lose the round, while an abort lets another transmitter deliver it.

**Permissions.** A report is applied only when its sender, workflow owner and workflow name match an allowed workflow: the feed's own list if it has one, otherwise the store's default list. The sender is normally the forwarder authority PDA `["forwarder", forwarder_state, data_feeds_store_program]` under the forwarder program, which the forwarder signs with only for reports it verified. Each forwarder is just another entry; there is one per **zone**, an independent CRE deployment with its own DON and forwarder. The store doesn't check that a sender is a forwarder: as on EVM, any key can be an allowed sender. Listing an ordinary key would let its holder write without the DON, so the deployment tooling only lists forwarder PDAs.

**Events** are CPI events, a self-invoke signed by the program's event authority, carried in the transaction's inner instructions. Unlike log events, they can't be lost when a transaction with many reports exceeds the log limit.

## Reads

On EVM, a read returns the answer at the feed's fixed decimals. Here, as on Stellar, the caller passes the data ID and the decimals it wants.

- `latest_round(data_id, decimals)` and `get_round(data_id, round_id, decimals)` return `Round { round_id, answer, observations_timestamp, recorded_timestamp }`, with the answer rescaled to `decimals` (truncating toward zero). They take the store state and the feed.
- `decimals(data_id)` returns 18, `description(data_id)` the description, `get_min_decimals(data_id)` the fewest decimals a read may ask for.
- `check_feed_permission(data_id, workflow)` tells whether a workflow may write a data ID, from the feed's own list or else the default list. It takes the feed account and answers false for a feed that isn't set, since its reports are skipped. Like the EVM function it mirrors, it ignores the write freeze, which EVM doesn't have.

A round read fails with the first of these that applies:

1. `FeedFrozen` while the read freeze is on;
2. `InvalidDecimals` below the feed's minimum (18 until a feed admin lowers it) or above 18;
3. `NoDataPresent` for a round that was never written or has left the ring;
4. `RoundsToZero` when a non-zero answer would read as zero.

Round IDs keep increasing for the life of a feed: the program is upgraded in place, so there are no phase IDs.

**The `chainlink-solana-data-feeds-store` crate** (`contracts/crates/chainlink-solana-data-feeds-store`) offers every read the store gives readers, both ways. `read_latest_round`, `read_round`, `read_decimals`, `read_description` and `read_min_decimals` decode the feed account inside the customer's program, with no call. `latest_round`, `get_round`, `decimals`, `description` and `get_min_decimals` call the store. Every one of them checks that the account belongs to the store program, the store state the caller trusts and the data ID, whatever it returns, because another feed's account would otherwise give another feed's price or description. The call functions take that state as `expected_state` and refuse any other (`AccountMismatch`). The round reads also apply the program's freeze, decimals and rescaling checks, in the same order.

**No read access control.** Any program can decode an account passed to it, so reads can't be gated. EVM's `getDataIds` becomes a `getProgramAccounts` query on the feed discriminator and `state` (account offset 56).

## Feed settings

Feed admins (MCMS on mainnet) change every per-feed setting. Each setter takes the data IDs and their feeds as remaining accounts, and all but `remove_specific_feed_metadata` create any feed that doesn't exist yet:

- `set_feeds` and `set_feed_descriptions`: descriptions, up to 64 bytes of UTF-8;
- `set_specific_feed_metadata` and `remove_specific_feed_metadata`: the feed's own workflow list, which replaces the default;
- `set_min_max_answers`: answer bounds, cleared with `(i128::MIN, i128::MAX)`;
- `set_feed_writes_disabled`: the write freeze, which skips every workflow's reports while reads continue;
- `set_feed_frozen`: the read freeze, which refuses round reads (the description and decimals stay readable) while writes continue;
- `set_min_decimals`: the fewest decimals a read may ask for;
- `set_df1_feeds`: the DF1 link and its own switch (see below);
- `set_max_stored_reports`: the ring size. It only grows, and only before the ring has wrapped. Solana lets an account grow by at most 10 KiB per instruction, so one call adds at most 320 rounds; later calls can grow it further. Inside an MCMS timelock batch, the runtime applies that limit to the whole batch, so a batch should grow each feed once.

Store-wide settings are the default workflow list (`set_default_feed_metadata`) and the forward window (`set_max_observation_forward_duration`). On EVM the default list can only be replaced; here it can also be emptied, leaving only feeds with their own workflow list writable, which revokes a compromised default workflow at once. The owner manages feed admins and ownership (two steps) and can recover tokens and SOL sent to the store.

## DF1 feeds

Existing customers read DF1 prices from the DF1 store program's `Transmissions` accounts, so the store keeps those accounts written instead of moving customers.

**Linking.** `set_df1_feeds` links a data ID to a DF1 feed. It refuses an account that isn't a DF1 feed `submit` can write, and a DF1 feed another data ID already links (`Df1FeedAlreadyLinked`). Each linked DF1 feed has a link marker holding the data ID that links it: created on its first link, with the feed admin paying, and cleared on unlink. Reports never pass the link marker. Remaining accounts are the feeds, then for each data ID in order:

- the link marker of its current DF1 feed, if the link changes away from it;
- the new DF1 feed and its link marker, if one is linked.

So unlinking passes one account, linking or re-linking the same DF1 feed two, and switching three. To move a DF1 feed from one data ID to another in one call, list the unlink first: the link marker is then cleared before the second data ID claims it, which would otherwise fail with `Df1FeedAlreadyLinked`.

**Writing.** For each round of a linked feed, the store calls DF1's unchanged `submit`, signed by its `df1_writer` PDA, with the answer rescaled to the DF1 feed's decimals. Solana can't contain a failing call to another program, so the store first checks everything `submit` could fail on:

- the account's owner, discriminator, version and ring geometry;
- that its writer is the store's `df1_writer`;
- that the round is newer than DF1's latest (`submit` doesn't check order; OCR2 did that for it);
- that the rescaled answer fits;
- that DF1's round counter won't overflow.

If any check fails, the DF1 write is skipped with `Df1WriteSkipped { reason }` and the round is still stored. Each feed also has a switch, `df1_write_disabled`, that stops only its DF1 write. The DF1 feed has to be writable only when `submit` is called. So while a link is disabled, or before the cutover, the DF1 feed can be passed read-only, and store transactions don't compete with OCR2's for a write lock on the account OCR2 writes.

**Cutover, per feed:**

1. `set_feeds` for the feed's data ID; let the CRE workflow write it, and compare it with the DF1 feed.
2. `set_df1_feeds(data_id, df1_feed, write_disabled = false)`. The workflow chooses a report's accounts when it builds the report, and the DON signs them, so a report built before the link changed still carries the old accounts and aborts. Change links between rounds; at worst, one round is delivered by the next report instead.
3. The DF1 feed owner (MCMS on mainnet) calls DF1's `set_writer(df1_writer)`, then stops the feed's OCR2 job.
4. To roll back: DF1's `set_writer` back to the OCR2 store authority PDA, and restart OCR2.

## Releasing a new build

A new build is proven before customers read it, then installed by upgrading the program in place:

1. Deploy the candidate build under a second program ID on mainnet, with its own store and feeds.
2. Point a workflow, or a second zone, at both, and compare the shadow feeds with the live ones for an agreed period.
3. Upgrade the main program to the exact binary that ran as the shadow, checked by its verified-build hash, through an MCMS proposal.
4. To roll back, upgrade to the previous verified build (through the MCMS bypasser in an incident).

Feed accounts keep their addresses, history and settings across an upgrade; a layout change bumps the account version and migrates accounts explicitly.

Before the first mainnet release:
- the external audit is complete, and its findings are fixed or accepted;
- the audited commit has a verified build, and that build is the one deployed;
- the client crate `chainlink_solana_data_feeds_store` is published to crates.io from the audited commit, so customer programs can depend on it as they depend on `chainlink_solana` for DF1.

## Capacity

A CRE write is one forwarder report instruction with every account inline, plus the relayer's compute unit limit and price instructions. It is signed by F+1 nodes of the workflow DON (65 bytes each). Measured with `contracts/gotests/data_feeds_store_test.go`:

- **One report at F=2** (staging): 1,012 bytes store-only and 1,109 bytes with the DF1 write, both within a legacy or V0 transaction's 1,232 bytes.
- **One report at F=5** (the 16-node mainnet DON): 1,207 and 1,304 bytes. A store-only report fits a legacy transaction; a DF1-backed one needs a V1 transaction.
- **V1 transactions** (4,096 bytes, no lookup tables, at most 64 accounts): at F=5, 40 store-only reports take 3,898 bytes and 51 accounts; 25 DF1-backed reports take 3,752 bytes and 63 accounts.
- **Compute:** about 4.4k compute units per store-only report and 10.2k per DF1-backed report, on top of the forwarder (about 100k at F=2 and 188k at F=5). 20 DF1-backed reports in one `on_report` take about 211k.
- **Heap:** every batch above fits the default 32 KiB heap, which never frees, so `on_report` and the feed setters reuse one buffer for every feed.
- **Instruction limit:** a transaction carries at most 64 instructions, counting events and inner calls. A store-only report uses 1 (`FeedUpdated`), a DF1-backed report 2 (`submit` or the `Df1WriteSkipped` event, plus `FeedUpdated`).
- **Feed setters:** `set_feeds` creating 20 feeds takes about 238k compute units.

**How many reports fit in one transaction.** Three runtime limits apply: 4,096 bytes, 64 accounts and 64 instructions (inner calls included). Size and accounts run out first, at about 40 store-only or 25 DF1-backed reports at F=5. The instruction limit would allow about 57 and 28: each stored report adds one instruction (its event), and a report that writes DF1 adds two (`submit` and the event). Memory runs out at about the same point.

**Pre-funded accounts need extra headroom.** Anyone can send SOL to an address before the program creates its account there. Creating a pre-funded account takes `allocate` and `assign`, plus a `transfer` if more lamports are needed for rent, instead of a single `create_account`. Each pre-funded account therefore adds up to two instructions to the transaction's instruction count. This applies to every feed account `set_feeds` creates, every DF1 link marker `set_df1_feeds` creates, and the forwarder's per-report execution-state account, whose address anyone can derive from the report.

Allow two spare instructions for every account a call may create, rather than two for the whole transaction. Otherwise, a transaction at the 64-instruction limit can fail if someone pre-funds those addresses. A failed report transaction rolls back its account creation, so retrying the same report hits the same limit again; reduce the batch size if this happens.

## Adding SVR later

SVR is not supported, but nothing here blocks it: the data ID layout keeps the SVR path bytes, the feed account keeps spare bytes where EVM tracks SVR visibility, and the program upgrades in place at the same address.

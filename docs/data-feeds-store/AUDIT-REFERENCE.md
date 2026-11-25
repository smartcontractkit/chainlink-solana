# Data Feeds Store on Solana: Audit reference

The lookup detail behind the [audit brief](AUDIT.md): read the brief first for the architecture, trust model, invariants and risks. Any difference between this document and the code is a finding: report it, since either one may be wrong.

**Pull requests:**

- the program: chainlink-solana [#1676](https://github.com/smartcontractkit/chainlink-solana/pull/1676);
- the deployment changesets: chainlink-data-feeds [#291](https://github.com/smartcontractkit/chainlink-data-feeds/pull/291);
- the workflow's Solana adapter: chainlink-data-feeds [#290](https://github.com/smartcontractkit/chainlink-data-feeds/pull/290);
- the u64 and initial-v1 workflow/deployment follow-up: chainlink-data-feeds [#377](https://github.com/smartcontractkit/chainlink-data-feeds/pull/377).

The program already runs on DF CRE staging against Solana devnet. The evidence, with explorer links, is in the [audit brief, §12](AUDIT.md#12-running-on-df-cre-staging).

## 1. Scope and build

The program is `contracts/programs/data-feeds-store`, program ID `9qCsUGTdKCanybkroJuPzAhuRU8P2SZN4NCexevSZAwu`, Anchor 0.31. The client crate is `contracts/crates/chainlink-solana-data-feeds-store`. Also in production scope is `contracts/crates/arrayvec/src/lib.rs`: the unchanged macro that implements the store's feed-admin and workflow lists. DF1 is the unchanged `contracts/programs/store` program (declared ID `9kRNTZmoZSiTBuXC62dzK9E7gC7huYgcmRRhYv3i4osC`); only what the store assumes about it is in scope (section 5).

Source map, under `src/`:

- `lib.rs`: the entry points.
- `context.rs`: the account structs and their constraints.
- `state.rs`: account layouts with their const size and offset assertions, argument and return types, `Df1SkipReason`.
- `common.rs`: constants, seeds and DF1 offsets.
- `data_id.rs`: data ID validation and padding.
- `df1.rs`: the DF1 checks, `rescale` and the `Submit` call.
- `logic.rs`: the pure ring, resize and scaling functions.
- `utils.rs`: PDA creation, feed loading, ring accessors, the `EventEmitter`, permission helpers.
- `event.rs` and `error.rs`: events and errors.
- `instructions/admin.rs`, `feeds.rs`, `on_report.rs`, `reads.rs`: the handlers.
- `instructions/mod.rs`: the handler module wiring.

Build and test from `contracts`:

- `anchor build` builds every workspace program and the IDLs.
- `cargo test` runs the Rust unit and property tests of the program and the client crate.
- `anchor test` runs `tests/data-feeds-store.spec.ts` (30 tests, including 25 seeded random report batches checked against a model) against a local validator. `Anchor.toml` deploys workspace programs as upgradeable by the provider wallet (`[test] upgradeable = true`), as `initialize` requires.
- `../scripts/test-data-feeds-store-wrong-versions.sh` builds the program and runs three additional instruction tests with preloaded wrong-version state, feed and link-marker accounts.
- `go test ./gotests -run 'TestDataFeedsStore$' -v` runs the end-to-end suite through a real keystone forwarder at F=2 and F=5, and performs an in-place loader upgrade of the current binary mid-test. It does not test old-layout migration. It needs Docker for the local validator, and the validator must be able to upgrade a program it loaded at startup. On amd64 the test uses `anzaxyz/agave:v2.1.21`. The deployment framework's own Linux image, `solanalabs/solana:v1.18.26`, can't upgrade such a program, because it marks the program's data as executable. On arm64 the framework's default image works as it is.

The tracked `contracts/generated/data_feeds_store` bindings are customer-facing artifacts regenerated from the IDL. They are checked for synchronization with the program but are not independent production source files.

### Reference-only context

The following unchanged files provide context for the store's external assumptions without adding line-by-line production scope: `programs/store/src/lib.rs` and `state.rs` for DF1's layout and `submit`; `programs/keystone-forwarder/src/lib.rs`, `common.rs`, `context.rs`, `state.rs` and `utils.rs` for signatures, account hashing and execution state; and `programs/ocr_2/src/lib.rs`, `context.rs` and `sdk/chainlink-solana/src/lib.rs` for cutover and existing-customer reads.

## 2. Accounts and seeds

All program accounts start with an 8-byte Anchor discriminator. Offsets below are within the struct; add 8 for the account offset. Structs are `#[account(zero_copy)]` (`repr(C)`, `bytemuck::Pod`). Account layouts have independent versions, checked on load: `STORE_STATE_VERSION = 1`, `DF1_LINK_VERSION = 1` and `FEED_ACCOUNT_VERSION = 1`.

### StoreState

The PDA `["state", store_id as u64 little-endian]`, created by `initialize`, owned by the store, exactly `8 + 3,232 = 3,240` bytes. Only the total size is asserted; the offsets below follow from the layout:

- `version: u8` (0), `store_authority_bump: u8` (1), `df1_writer_bump: u8` (2), `_padding0: u8` (3). The bumps are computed at `initialize`.
- `max_observation_forward_secs: u32` (4): the forward window, how far an observation timestamp may lead the clock.
- `default_max_stored_reports: u64` (8): ring size of feeds this store creates. Set at `initialize`; there is no setter.
- `owner: Pubkey` (16), `proposed_owner: Pubkey` (48).
- `df1_program: Pubkey` (80): the DF1 program whose feeds the store may write. Set at `initialize`; there is no setter.
- `feed_admins: AccountList` (112): up to 64 keys, kept sorted for binary search, then `len: u64`. Freed slots are zero.
- `default_workflow_metadata: WorkflowMetadataList` (2168): up to 16 `WorkflowMetadata { allowed_sender: Pubkey, allowed_workflow_owner: [u8;20], allowed_workflow_name: [u8;10] }` of 62 bytes, then `len: u64`. Freed slots are zero.
- `_reserved: [u8; 64]` (3168).

### Feed

PDA `["feed", state, data_id_32]`, owned by the store. A 1,344-byte header (asserted) followed by the ring. The layout is public: readers decode it, and fields move only with a version bump. Offsets marked \* are const-asserted in `state.rs`. The current layout is the initial public version 1; development-era accounts are abandoned and recreated rather than migrated.

- **Flags and timestamps:**
  - `version: u8` (0): 2.
  - `frozen: u8` (1\*): the read freeze; round, decimals and description reads fail, `get_min_decimals` stays readable, and writes continue.
  - `min_decimals: u8` (2\*): fewest decimals a read may ask for, 18 at creation.
  - `writes_disabled: u8` (3): the write freeze; reports skipped, reads continue.
  - `_padding0: [u8; 4]` (4): zero bytes aligning the timestamps.
  - `latest_observations_timestamp: u64` (8\*), `latest_recorded_timestamp: u64` (16): Unix seconds.
  - `min_max_configured: u8` (24): whether the answer bounds apply.
  - `df1_write_disabled: u8` (25): skip only the DF1 write.
  - `_padding1: [u8; 6]` (26): zero bytes before `data_id`.
- **Identity:**
  - `data_id: [u8; 32]` (32\*): the data ID zero padded.
  - `state: Pubkey` (64\*): the owning store state. Account offset 72 is the `getProgramAccounts` filter.
- **Latest round and ring:**
  - `round_id: u64` (96\*): the latest round, 0 before the first.
  - `_reserved0: [u8; 8]` (104): kept for SVR's `visibleRoundId`.
  - `max_stored_reports: u64` (112\*): the ring size.
  - `latest_recorded_slot: u64` (120).
  - `latest_answer: i128` (128\*): 18 decimals.
- **Settings:**
  - `description: [u8; 64]` (144\*): UTF-8, zero padded.
  - `df1_feed: Pubkey` (208\*): the linked DF1 feed, default for none.
  - `min_answer: i128` (240), `max_answer: i128` (256): the answer bounds.
  - `specific_workflow_metadata` (272\*): a `WorkflowMetadataList` of 1,000 bytes; empty means the store default applies. Freed slots are zero.
- `_reserved: [u8; 72]` (1272).

The ring starts at account offset `FEED_RING_OFFSET = 8 + 1,344 = 1,352`. It holds `max_stored_reports` `StoredReport`s of 48 bytes (asserted): `answer: i128` (0), `observations_timestamp: u64` (16), `recorded_timestamp: u64` (24), `recorded_slot: u64` (32), `_padding0: [u8; 8]` (40). Round `r` lives at index `r % max_stored_reports`, at account offset `1,352 + 48 × index`. The account length is `feed_account_len(n) = 1,352 + 48n`, saturating so an absurd `n` fails bound checks instead of panicking. A loaded feed must be at least that long, so every ring index is in bounds. Because `initialize` caps the default at one 10 KiB allocation, the largest default ring size is 185 rounds.

### Link marker

PDA `["df1_link", state, df1_feed]`, owned by the store, 40-byte body (asserted), 48-byte account: `version: u8` (0), `_padding0: [u8; 7]` (1), `data_id: [u8; 32]` (8, account offset 16), the padded data ID that links the DF1 feed, all zero when unlinked. It is created on a DF1 feed's first link from this store, the feed admin paying rent. On unlink it is cleared rather than closed. Closing an account means the program moves its SOL out by editing balances directly, and the runtime rejects that if the same instruction later calls another program. `set_df1_feeds` can do exactly that after an unlink, when it creates a feed or a link marker through the system program, so markers are zeroed instead. `on_report` never reads link markers.

### PDAs and who signs with each

- **State** `["state", store_id_le8]`: signs only its own creation in `initialize`, through `create_pda_account` as for feeds. Its bump is not stored; nothing else signs as it.
- **Feed** `["feed", state, data_id_32]`: signs only its own creation in `create_pda_account` (`create_account`, or `allocate` and `assign` for a pre-funded address).
- **Store authority** `["store_authority", state]`, bump stored in the state: holds tokens and SOL sent to the store; signs only the `transfer_checked` of `recover_tokens` and the system `transfer` of `recover_native`.
- **DF1 writer** `["df1_writer", state]`, bump stored: the writer of linked DF1 feeds; signs only DF1 `submit`.
- **Link marker** `["df1_link", state, df1_feed]`: signs only its own creation.
- **Event authority** `["__event_authority"]`, program-wide (not per state): signs every CPI event self-invoke.
- `initialize` also reads the program's ProgramData, PDA `[program_id]` under the upgradeable loader.

## 3. Instructions

Every state-changing context uses `#[event_cpi]`, which appends the event authority and the program as the last named accounts. Every context but `Initialize` checks the state's `version` (`InvalidAccountVersion`); `on_report` makes this check in its handler rather than in its account list. Anchor's own account errors can also occur: a state passed as `AccountLoader<StoreState>` fails Anchor's owner and discriminator checks before any handler runs.

### Owner

**`initialize(store_id: u64, config: StoreConfig)`**

- Accounts: `owner` (writable signer, pays the rent); `state` (writable, the PDA `["state", store_id]`, checked by Anchor's `seeds` constraint); `program_data` (the program's ProgramData, whose `upgrade_authority_address` must be `owner`, else `Unauthorized`); `system_program`.
- Why the state is created here: an empty state account owned by the program, created beforehand and waiting for an MCMS proposal, could be taken over by anyone through Anchor's built-in IDL instructions (`IdlCreateBuffer`, then `IdlCloseAccount`), after which `initialize` would always fail. Creating the account inside `initialize` means no such account ever exists.
- `StoreConfig { feed_admins: Vec<Pubkey>, default_max_stored_reports: u64, max_observation_forward_secs: u32, df1_program: Pubkey }`.
- Checks, in order:
  - `default_max_stored_reports >= 2` and `feed_account_len(default) <= 10,240` (`InvalidMaxStoredReports`);
  - `df1_program` is neither the default key nor this program's ID (`InvalidAddress`). Anchor marks an optional account as absent by passing the program's own ID, so a DF1 program equal to the store's ID would read as "not passed" in every report;
  - at most 64 feed admins (`FeedAdminsFull`), strictly increasing, which also rejects the default key (`InvalidAddress`);
  - no account exists at the state address yet: system-owned with no data, though it may hold SOL (`StoreAlreadyInitialized`).
- Effects: creates the 3,240-byte state with `create_pda_account` (topping up a pre-funded address, then `allocate` and `assign`), then writes the discriminator and a zeroed `StoreState` with version, owner, defaults, DF1 program, both bumps and the feed admins. The forward window is not bounded.
- Event: one `StoreInitialized` with the owner and feed admins, not a `FeedAdminSet` each. A second `initialize` for the same store ID fails with `StoreAlreadyInitialized`.

**`transfer_ownership(proposed_owner)`**. The owner proposes a new owner, who must then accept. Accounts (`OwnerOnly`): the `owner` signer, which must be `state.owner` (`Unauthorized`), and the writable `state`. The proposed owner can't be the current one (`InvalidProposedOwner`). Proposing the default key cancels a pending transfer. Event `OwnershipTransferRequested`.

**`accept_ownership()`**. The proposed owner accepts and becomes the owner. Accounts (`AcceptOwnership`): the `new_owner` signer, which must be `state.proposed_owner` (`Unauthorized`). The proposal is cleared. Event `OwnershipTransferred`.

**`set_feed_admin(feed_admin, is_admin)`**. The owner adds or removes a feed admin (`OwnerOnly` accounts). The default key is rejected (`InvalidAddress`). Adding inserts at the sorted position (`FeedAdminsFull` at 64). Removing deletes and zeroes the freed slot, since the layout is public. A no-op when already in the requested state. Event `FeedAdminSet` in all cases.

**`recover_tokens(amount)`**. The owner recovers tokens sent to the store by mistake. Accounts: `owner`, `state`, `store_authority` (seeds and stored bump), `from` (token account with authority `store_authority` and mint `mint`), `to` (token account of `mint`), `mint`, `token_program` (Token or Token-2022 interface). `amount <= from.amount` else `InsufficientBalance`. `transfer_checked` signed by the store authority. Event `TokenRecovered`.

**`recover_native(amount)`**. The owner recovers SOL sent to the store by mistake. Accounts: `owner`, `state`, `store_authority` (writable), `to` (any writable account), `system_program`. The amount must be at most the balance, and what is left must be 0 or rent-exempt, since the runtime refuses anything between (`InsufficientBalance`). System `transfer` signed by the store authority. Event `TokenRecovered` with `mint` default.

### Feed admin, store-wide

Both use `FeedAdminState` (`feed_admin` signer, writable `state`); the signer must be in `feed_admins` (`Unauthorized`).

**`set_default_feed_metadata(workflow_metadata)`**. Replaces the store-wide default workflow list. At most 16 entries (`MaxEntriesExceeded`); this is checked before the feed admin check, so even a caller who isn't a feed admin gets this error for a list that is too long. Each entry needs a non-default sender and a non-zero owner (`InvalidAddress`) and a non-zero name (`InvalidWorkflowName`). Duplicates are not rejected, and the slots a shorter list frees are zeroed. The list may be empty. On EVM the default list can only be replaced; here an empty list leaves only feeds with their own list writable, which revokes a compromised default workflow at once. Event `DefaultFeedMetadataSet`.

**`set_max_observation_forward_duration(max_observation_forward_secs)`**. Sets the forward window to any `u32`. Event `MaxObservationForwardDurationSecondsUpdated`.

### Feed admin, per feed

All use `FeedAdminAccounts`: `feed_admin` (writable signer, pays rent), `state` (read-only), `system_program`. The data IDs are an argument; their feeds are the remaining accounts in the same order (for `set_df1_feeds`, the first N remaining accounts). Each handler first checks its own arguments, then calls the shared `update_feeds`:

1. Load the state, check the signer is a feed admin (`Unauthorized`), and require as many feeds as data IDs (`ArrayLengthMismatch`).
2. If any feed account is uninitialized, emit one `FeedsCreated` first, listing each such data ID once, so indexers see a feed created before its settings.
3. Per data ID:
   - validate it (`InvalidDataId`);
   - if the account is uninitialized (system-owned, empty), require it to be the feed PDA (`AccountMismatch`) and create it at `feed_account_len(default_max_stored_reports)` with the feed admin paying. The new header is zero except `version = 1`, `min_decimals = 18`, `data_id`, `state` and `max_stored_reports = default`;
   - otherwise load it with `load_feed_into`: owner, length and discriminator (`InvalidAccount`), version (`InvalidAccountVersion`), state, data ID and ring length (`AccountMismatch`).
4. Apply the setter to the header, then write it back (`AccountNotWritable` if read-only).

**`set_feeds(data_ids, descriptions, workflow_metadata)`**. Lengths equal (`ArrayLengthMismatch`); metadata at most 16 and valid. Sets each description (`DescriptionTooLong` over 64 bytes, `InvalidDescription` if it contains NUL), emitting `FeedDescriptionSet` per feed. A non-empty `workflow_metadata` becomes every feed's own list, with one `SpecificFeedMetadataSet` for the whole call after the per-feed events. An empty list leaves each feed's list unchanged.

**`set_feed_descriptions(data_ids, descriptions)`**. As the description part of `set_feeds`; an empty string clears.

**`set_specific_feed_metadata(data_ids, workflow_metadata)`**. Both non-empty (`EmptyConfig`), metadata valid and at most 16. Replaces each feed's list, zeroing the slots a shorter list frees. One `SpecificFeedMetadataSet` for the call; replacing a list emits no `SpecificFeedMetadataRemoved`.

**`remove_specific_feed_metadata(data_ids)`**. Each feed must have a list (`FeedNotConfigured`), so this setter can't create a feed. Clears the list. Event `SpecificFeedMetadataRemoved` per feed.

**`set_min_max_answers(data_ids, min_answers, max_answers)`**. Lengths equal. The pair `(i128::MIN, i128::MAX)` clears the answer bounds (`min_max_configured = 0`, both stored as 0); otherwise `min <= max` (`InvalidMinMaxRange`) and the bounds are set. Event `MinMaxAnswerSet` with the arguments.

**`set_feed_writes_disabled(data_ids, disabled)`**, **`set_feed_frozen(data_ids, frozen)`**. One flag for every listed feed. Events `FeedWritesDisabledSet`, `FeedFrozenSet`.

**`set_min_decimals(data_ids, min_decimals)`**. `min_decimals <= 18` (`InvalidDecimals`), one value for every listed feed. Event `MinDecimalsSet`.

**`set_df1_feeds(data_ids, df1_feeds, write_disabled)`**. Three equal lengths (`ArrayLengthMismatch`).

The remaining accounts are the N feeds, then the link accounts in data-ID order. How many link accounts a data ID needs depends on what changes: removing a link needs the old link marker, adding one needs the DF1 feed and its marker, and moving one needs both. For data ID `i`, `new = df1_feeds[i]`, and `old` is its current link, read from its feed as the call goes along. So if a data ID appears twice in one call, the second entry sees the first entry's change; that is what lets one call unlink a DF1 feed from data ID A and then link it to data ID B. The accounts are:

- unlink (`old` set, `new` default): 1 account, the old link marker;
- link or re-link (`new` set, `old` default or equal to `new`): 2 accounts, the DF1 feed and its link marker;
- switch (`old` and `new` set and different): 3 accounts, the old link marker, then the new DF1 feed and its link marker;
- neither link set: none.

Too few or leftover accounts: `InvalidAccountCount`.

**Unlink.** The old link marker must pass the shared checks below and hold this data ID (`AccountMismatch`). It is cleared.

**Link.** The DF1 feed's key must equal `new` (`AccountMismatch`). Creating, changing or enabling a link requires `df1::check_feed` against `state.df1_program` to pass (`InvalidDf1Feed`). Disabling an existing unchanged link skips that check, so it remains available if DF1's layout becomes unsupported. Its writer is not checked and it need not be writable. Then its link marker: an uninitialized one, at the right PDA, is created with the feed admin paying; an existing one must pass the shared checks and hold zero or this data ID (`Df1FeedAlreadyLinked`). The link marker is written with this data ID.

Shared link-marker checks:

- at the PDA of `(state, df1_feed)` (`AccountMismatch`);
- owned by the store (`InvalidAccount`);
- writable (`AccountNotWritable`);
- 48 bytes with the `Df1Link` discriminator (`InvalidAccount`);
- version 1 (`InvalidAccountVersion`).

The feed then gets `df1_feed = new` and `df1_write_disabled`; event `Df1FeedSet`.

**`set_max_stored_reports(data_ids, max_stored_reports)`**. Lengths equal. Per feed:

- `check_max_stored_reports`: the new ring size is at least 2 (`InvalidMaxStoredReports`), larger than the current one (`MaxStoredReportsNotIncreased`), and the ring has not wrapped, that is `round_id < current` (`MaxStoredReportsLocked`). The new size then always exceeds `round_id`.
- The feed must be writable (`AccountNotWritable`), checked before the rent top-up.
- Solana lets an account grow by at most 10,240 bytes per instruction, counted from its size when the instruction began. So the new length must be larger than the current one and at most 10,240 bytes (213 rounds) more than the account's size at the start of the instruction (`InvalidMaxStoredReports`). For a feed created in the same call, that starting size is 0, so the whole account (header and ring) must fit in 10,240 bytes, which allows 185 rounds. A data ID listed twice in one call is measured against the same starting size both times.
- The feed admin tops up rent, the account is resized (zeroing the new bytes) and the header updated. Event `MaxStoredReportsSet`.

Growing only before wrapping keeps every stored round `r` at index `r`, where the larger ring looks for it.

When `set_max_stored_reports` runs inside another program's instruction, such as an MCMS timelock batch, the runtime counts the 10 KiB growth limit from the start of that outer instruction. The program can't see that starting size, so two growths of one feed in the same batch could pass this check and still fail in the runtime. Grow each feed at most once per batch.

### Reports

**`on_report(metadata, report)`**. Accounts in order: `forwarder_state` (not read), `forwarder_authority` (signer, the sender), `state`, `df1_program` (optional; must equal `state.df1_program`, else `AccountMismatch`), `df1_writer` (optional; seeds `["df1_writer", state]`), event authority, program, then per report its feed and any linked DF1 feed. Anchor marks an optional account as absent by passing the program's own ID, so a transaction in which no report writes DF1 passes the store's program ID for `df1_program` and `df1_writer`. Section 4 gives the order.

### Reads

`FeedRead` takes `state` (version checked) and an unchecked `feed`. Handlers re-load the feed with `load_feed_into` against the state and the data ID argument, which checks the owner, discriminator, version, state, data ID and ring length. For an uninitialized account, the handler checks the feed PDA before any fallback decimals validation: a wrong empty address returns `AccountMismatch`; a canonical missing feed with invalid round decimals returns `InvalidDecimals`, and with otherwise valid arguments returns `NoDataPresent`. The IDL build retains the typed `Feed` account entry for generated account decoders. Results go through Anchor return data; reads change nothing and emit no events.

- **`latest_round(data_id, decimals) -> Round`** and **`get_round(data_id, round_id, decimals) -> Round`** check, through `to_round`, in the order below. When several checks fail, the first decides the error the caller sees; the order matches the Stellar proxy's.

  1. `InvalidDecimals`, below `min_decimals` or above 18;
  2. `FeedFrozen`;
  3. `NoDataPresent`: before the first round for `latest_round`; for `get_round`, unless `1 <= round_id <= latest` and `latest − round_id < max_stored_reports`;
  4. `RoundsToZero`.

  The answer is divided by `10^(18 − decimals)`, truncating toward zero.

- **`decimals(data_id) -> u8`**: 18 after loading the feed and checking it is not frozen.
- **`description(data_id) -> String`**: bytes up to the first NUL, after the same freeze check.
- **`get_min_decimals(data_id) -> u8`**: the feed's minimum. `NoDataPresent` for an unset feed; no freeze check.
- **`check_feed_permission(data_id, workflow_metadata) -> bool`** (`PermissionRead`: `state` and an unchecked `feed`, both required). `InvalidDataId` for an ID the store doesn't serve. An uninitialized `feed` must be the feed PDA (`AccountMismatch`) and answers false, as its reports would be skipped. Otherwise it is loaded as for other reads, and the answer uses its own list or else the default list. Like the EVM function it mirrors, it ignores the write freeze, which EVM doesn't have, so its answer is about permission only.

`Round` is `{ round_id: u64, answer: i128, observations_timestamp: u64, recorded_timestamp: u64 }`, with both timestamps in Unix seconds.

The client crate's decode functions (`read_latest_round`, `read_round`, `read_decimals`, `read_description`, `read_min_decimals`) check the PDA for an uninitialized account, returning `NoDataPresent` for the canonical missing feed and `AccountMismatch` for a wrong empty address. For an existing feed they check the store-program owner (`InvalidAccount`), then run the same `decode_feed_into` (discriminator, version, the store state the caller trusts, data ID, ring length) as the program. Valid feeds can only be created at their canonical PDA, so existing-feed reads rely on that creation invariant rather than recomputing the address. Round reads then run `to_round`, so they fail the same way. Its call functions (`latest_round`, `get_round`, `decimals`, `description`, `get_min_decimals`) check the program ID (`InvalidProgramId`) and that the store state passed is the one the caller trusts (`expected_state`, else `AccountMismatch`), and accept return data only from the store program.

## 4. on_report in detail

Before any report:

1. `workflow_metadata` reads the name from metadata bytes 32..42 and the owner from 42..62; shorter metadata is `MalformedReport`.
2. `report` must Borsh-decode exactly as `Vec<ReceivedReport { data_id: [u8;16], timestamp: u64, answer: i128 }>` (`MalformedReport`). Each entry is 40 bytes, and timestamps are Unix seconds.
3. The state, an `AccountLoader<StoreState>` whose owner and discriminator Anchor has checked, is copied into a heap box (`InvalidAccount` if shorter than `StoreState`), and its version checked (`InvalidAccountVersion`).
4. The clock is read once; `now.timestamp` is `u64::try_from(unix_timestamp)`. A negative clock value aborts with `InvalidTimestamp` (6031) rather than wrapping.
5. A `df1::Submit` is prepared only if both the DF1 program and writer were passed.

Then for each report, in order, `next_account` takes the feed account (`InvalidAccountCount` if none is left) and runs these steps. Reports take their accounts from one list in order, so every report, skipped or not, must step past exactly the accounts it uses; otherwise the next report would read the wrong feed. Event fields omit `state` (§8).

1. **Data ID** zero or with a non-zero SVR path: skip, `InvalidDataIdIgnored { data_id, sender }`. The report used one account, which isn't read.
2. **Uninitialized account** (owned by the system program, no data): skip, `FeedNotInitializedIgnored { data_id, sender }`. The report used one account. Its address isn't checked against the data ID's feed address, which is harmless because nothing is written (§10, "Wrong or fake accounts").
3. **Load the feed**: owner, discriminator, length, version, `state` and `data_id`. Any failure aborts (`InvalidAccount`, `InvalidAccountVersion`, `AccountMismatch`).
4. **DF1 account**: if `df1_feed` is set, take the next account (`InvalidAccountCount`) and require its key to equal `df1_feed` (`AccountMismatch`). From here on the report has used two accounts.
5. **Write freeze**: skip, `FeedWritesDisabledIgnored { data_id, sender }`.
6. **Permission**: `(sender, workflow_owner, workflow_name)` must match an entry of the feed's own list, or the default list when the feed's is empty; else skip, `InvalidUpdatePermission`.
7. **Future**: `timestamp > now + max_observation_forward_secs` (computed in `u64` with saturating addition): skip, `FutureReportIgnored`.
8. **Stale**: `timestamp <= latest_observations_timestamp`: skip, `StaleReport`.
9. **Answer bounds**: if `min_max_configured` and the answer is outside `[min_answer, max_answer]`: skip, `AnswerOutOfMinMaxRange`.
10. **Writable feed**: else abort with `AccountNotWritable`. This comes after every skip, so a skipped report never needs a writable feed, and before the DF1 write, so DF1 is never written for a round the store then fails to keep.
11. **DF1 write** if linked: the checks below, then either `submit` or `Df1WriteSkipped { data_id, df1_feed, reason }`. The program opens the DF1 account's data for reading once, before the checks, which then work on those bytes. If Solana refuses to open it (for example because the same account is already open for writing), the transaction aborts rather than skipping the DF1 write, so a report is never marked delivered without its DF1 write for that reason.
12. **Store**: `round_id += 1`, write `StoredReport { answer, observations_timestamp, recorded_timestamp: now.timestamp, recorded_slot: now.slot, _padding0: [0; 8] }` at index `round_id % max_stored_reports`, update the latest fields, write the header back, and emit `FeedUpdated { data_id, round_id, observations_timestamp, answer }`.

After the loop, any leftover account aborts with `InvalidAccountCount`.

Why steps 10 and 11 abort rather than skip: the transmitter chooses which accounts are writable, and the DON's signature, which the forwarder checks, doesn't cover that choice. The forwarder records a report as delivered whenever `on_report` succeeds, so a skip would lose the round for good, while an abort lets another transmitter deliver it (brief risk 5).

The DF1 write checks, in order, each giving a `Df1SkipReason`:

1. `df1_write_disabled` set: `Disabled`.
2. DF1 program or writer not passed: `NotPassed`.
3. Owner is not `state.df1_program`, data shorter than 200 bytes, or discriminator not `Transmissions`: `InvalidAccount`.
4. Version byte not 2: `InvalidVersion`.
5. Ring geometry `submit` would panic on: live length 0, live cursor not below live length, ring bytes shorter than the live ring or not a whole number of 48-byte transmissions, or a non-empty historical ring with granularity 0 or historical cursor out of range: `InvalidAccount`.
6. Writer not the store's `df1_writer`: `NotWriter`.
7. `latest_round_id == u32::MAX`: `RoundIdOverflow`.
8. The report's u64 observation timestamp exceeds `u32::MAX`: `TimestampOverflow`. DF1's `submit` would otherwise cast it to u32 and wrap it.
9. If DF1 has a round, the u32 timestamp stored at live slot `(live_cursor + live_length − 1) % live_length`, widened to u64, is at least the report's: `Stale`.
10. Rescaling to DF1's decimals overflows its scale factor or result: `AnswerOverflow`.
11. A nonzero answer rescales to zero at DF1's decimals: `RoundsToZero`.

If every check passes, the DF1 feed must be writable, else abort with `AccountNotWritable`; then `submit` is called. A DF1 feed that is skipped for any reason may be passed read-only. Every skip still stores the exact store round.

`on_report` aborts the whole transaction only in these cases; everything else is a skip of one report:

- a feed account that fails the checks in step 3, or a DF1 account that isn't the linked one (step 4);
- a feed passed read-only (step 10), or a DF1 feed passed read-only when `submit` would be called;
- malformed metadata or report payload;
- a bad state account or version, a negative chain-clock timestamp (`InvalidTimestamp`), or a wrong DF1 program or writer account;
- missing or leftover accounts;
- a failing `submit` or event call;
- running out of compute or heap.

## 5. DF1 interface

The store reads DF1 `Transmissions` accounts at fixed Borsh offsets (`common::df1`, discriminator included): version 8, writer 74..106, decimals 138, `latest_round_id` 143 (`u32`), granularity 147, live length 148, live cursor 152, historical cursor 156, rings from 200 (8 + 192-byte header). A transmission is 48 bytes with its `u32` timestamp at offset 8. The discriminators are `sha256("account:Transmissions")[..8]` and `sha256("global:submit")[..8]`.

The `submit` instruction built by `Submit::call` has program ID the DF1 program and accounts `[feed (writable), df1_writer (signer, read-only)]`. Its data is the submit discriminator, then `NewTransmission { timestamp: u64, answer: i128 }` in little-endian, with the report's u64 Unix-second observation timestamp. `check_round` has already required that timestamp to be at most `u32::MAX`, because DF1 stores it with an internal u32 cast. The call is signed with `["df1_writer", state, df1_writer_bump]`. No event is emitted on success: the `submit` inner instruction is the record.

Rescaling (`df1::rescale`): for DF1 decimals `d <= 18`, `answer / 10^(18 − d)`, truncating toward zero. `check_round` skips a nonzero answer that would become zero with `RoundsToZero`; a genuine zero remains valid at supported decimal settings. For `d > 18`, `answer × 10^(d − 18)` with checked power and multiplication. Scale-factor or result overflow gives `None` and the `AnswerOverflow` skip; the exact store round is still stored. At `d >= 57` the scale factor itself overflows, so every answer, including zero, is rejected as `AnswerOverflow`.

What the store assumes about DF1:

- The store's checks cover every way `submit` can fail, so a problem always becomes a skip instead of a failed transaction. `submit` fails only on its Anchor account checks (owner, discriminator, writable feed, signer equal to `writer`), a version other than 2, the split into live and historical rings, cursor indexing, the granularity modulo, and the `latest_round_id += 1` overflow (DF1 is built with overflow checks).
- `submit` doesn't check the order of rounds. OCR2, DF1's current writer, checks it before calling `submit`, so for a linked DF1 feed the store's `Stale` check is the only ordering check.
- The layout above is stable. DF1 is upgradeable by its own authority; a layout change or a new failure mode must be re-checked against `check_feed` and `check_round`.
- The checks were matched line by line against DF1's source in this repository. The deployed DF1 must be confirmed, by a verifiable build, to be at or after commit `c763bb71`: earlier builds could panic in `insert` on a feed with no historical ring.
- The flagging threshold logic in `submit` only sets the feed's `state` byte and cannot fail.

## 6. Data IDs

A data ID is 16 bytes, as in `DataId.sol`: compact feed ID (bytes 0–7), index (8), SVR path ID (9–10), instance (11), reserved (12–15). `is_valid_data_id` requires a non-zero ID with SVR path bytes `[0, 0]`. Feed admin setters and `check_feed_permission` reject other IDs with `InvalidDataId`; `on_report` skips them with `InvalidDataIdIgnored`. The instance and reserved bytes carry no invariant: IDs differing only there are separate feeds.

Accounts key the ID zero padded to 32 bytes (`to_seed`: the 16 bytes followed by 16 zeros). The feed PDA seed and the stored `Feed.data_id` and `Df1Link.data_id` all use the padded form; instruction arguments and events use 16 bytes. Every comparison between an argument and a stored ID pads the argument first, so the high 16 bytes are always zero in this version.

## 7. Errors

`StoreError` is one `#[error_code]` enum, numbered from 6000 in declaration order; new variants are appended.

- 6000 `ArrayLengthMismatch`: argument vectors, or feeds and data IDs, of different counts.
- 6001 `EmptyConfig`: empty data IDs or metadata in `set_specific_feed_metadata`.
- 6002 `InvalidDataId`: zero or SVR data ID in a feed admin setter or `check_feed_permission`.
- 6003 `InvalidAddress`: default sender, zero workflow owner; a `df1_program` that is the default key or this program's ID; unsorted or default feed admins in `initialize`; default key in `set_feed_admin`.
- 6004 `InvalidWorkflowName`: zero workflow name.
- 6005 `MaxEntriesExceeded`: more than 16 workflow entries.
- 6006 `AccountMismatch`: wrong PDA, key, state or data ID; a feed shorter than its ring; on unlink, a link marker holding another data ID; a wrong `df1_program` in `on_report`.
- 6007 `InvalidAccountCount`: too few or leftover remaining accounts in `on_report` or `set_df1_feeds`.
- 6008 `AccountNotWritable`: a read-only feed for a report that would be stored, or a read-only DF1 feed when `submit` would be called; a read-only feed in a setter; a read-only existing link marker.
- 6009 `InvalidAccount`: a feed or link marker with the wrong owner, length or discriminator; in `on_report`, a state shorter than `StoreState` (a wrong owner or discriminator fails Anchor's own `AccountLoader` check first); a state of the wrong size in `initialize`.
- 6010 `FeedNotConfigured`: removing a feed-specific list that doesn't exist.
- 6011 `InvalidMaxStoredReports`: ring size below 2, a default ring over 10 KiB, or growth over 10 KiB in one instruction.
- 6012 `MaxStoredReportsNotIncreased`: new ring size not larger.
- 6013 `MaxStoredReportsLocked`: the ring has wrapped.
- 6014 `MaxStoredReportsTooLow`: no longer returned; kept so later codes don't change.
- 6015 `InvalidMinMaxRange`: `min > max`.
- 6016 `NoDataPresent`: read of a correctly derived but uninitialized feed, a feed with no round, or a round outside the ring window.
- 6017 `DescriptionTooLong`: over 64 bytes.
- 6018 `InvalidDescription`: description containing NUL.
- 6019 `MalformedReport`: metadata under 62 bytes or a report that doesn't decode exactly.
- 6020 `FeedAdminsFull`: more than 64 feed admins.
- 6021 `InvalidProposedOwner`: proposing the current owner.
- 6022 `InsufficientBalance`: recovery amount above the balance, or leaving the store authority neither empty nor rent-exempt.
- 6023 `InvalidAccountVersion`: a state, feed or link marker whose version differs from its current account-type version (store state 1, link marker 1, feed 2).
- 6024 `Unauthorized`: signer not the owner, proposed owner, a feed admin, or the upgrade authority in `initialize`.
- 6025 `InvalidDf1Feed`: `set_df1_feeds` given an account `check_feed` rejects when the link operation requires that check.
- 6026 `InvalidDecimals`: read decimals below the minimum or above 18; `set_min_decimals` above 18.
- 6027 `RoundsToZero`: a non-zero answer would read as zero.
- 6028 `FeedFrozen`: round, decimals or description read of a frozen feed.
- 6029 `Df1FeedAlreadyLinked`: the DF1 feed's link marker holds another data ID.
- 6030 `StoreAlreadyInitialized`: `initialize` for a store ID whose state account exists.
- 6031 `InvalidTimestamp`: the chain's signed clock timestamp is negative and cannot be represented as u64 Unix seconds.

## 8. Events

Every event carries `state` first. The field lists below, and in §4, omit it.

All are CPI events. `EventEmitter` (and `emit_cpi!` in single-event handlers) invokes the store program with data `EVENT_IX_TAG_LE ‖ discriminator ‖ Borsh fields`, signed by the event authority `["__event_authority", bump]` as its only account. Anchor's `#[event_cpi]` handler accepts it only when that authority signed.

What follows from that:

- Events live in inner instructions, not logs, so log truncation can't lose them.
- Each event costs one instruction in the transaction trace and one level of CPI depth.
- Each event copies its instruction onto the 32 KiB heap, so where a call changes many feeds, the program emits one event for the whole call rather than one per feed (`StoreInitialized`, `FeedsCreated`, `SpecificFeedMetadataSet`).

`SpecificFeedMetadataSet` and `FeedDescriptionSet` are serialized from borrowed fields by `emit_with`; a unit test checks the bytes equal the struct encoding.

Owner and store-wide:

- `StoreInitialized { owner, default_max_stored_reports, max_observation_forward_secs, df1_program, feed_admins }`: `initialize`.
- `OwnershipTransferRequested { from, to }`, `OwnershipTransferred { from, to }`.
- `FeedAdminSet { feed_admin, is_admin }`: `set_feed_admin`.
- `DefaultFeedMetadataSet { workflow_metadata }`.
- `MaxObservationForwardDurationSecondsUpdated { new_duration }`.
- `TokenRecovered { mint, to, amount }`: both recoveries; `mint` default for SOL.

Per feed, from the setters:

- `FeedsCreated { data_ids, max_stored_reports }`: once per call that creates feeds, before any other event of the call; a data ID listed twice appears once.
- `FeedDescriptionSet { data_id, description }`.
- `SpecificFeedMetadataSet { data_ids, workflow_metadata }`: once per `set_feeds` (non-empty list) or `set_specific_feed_metadata` call, after the per-feed events.
- `SpecificFeedMetadataRemoved { data_id }`: only from `remove_specific_feed_metadata`.
- `MinMaxAnswerSet { data_id, min_answer, max_answer }`: the arguments, including the clearing pair.
- `FeedWritesDisabledSet { data_id, disabled }`, `FeedFrozenSet { data_id, frozen }`, `MinDecimalsSet { data_id, min_decimals }`.
- `Df1FeedSet { data_id, df1_feed, write_disabled }`.
- `MaxStoredReportsSet { data_id, max_stored_reports }`.

Reports (one per report; a DF1 skip adds `Df1WriteSkipped` before `FeedUpdated`). Every observation, latest-observation and recorded timestamp below is a u64 Unix-second value:

- `InvalidDataIdIgnored { data_id, sender }`, `FeedNotInitializedIgnored { data_id, sender }`, `FeedWritesDisabledIgnored { data_id, sender }`: emitted before the permission check, so any signer can cause them; they carry the sender for that reason.
- `InvalidUpdatePermission { data_id, sender, workflow_owner, workflow_name }`.
- `FutureReportIgnored { data_id, observations_timestamp, recorded_timestamp }`.
- `StaleReport { data_id, observations_timestamp, latest_observations_timestamp }`.
- `AnswerOutOfMinMaxRange { data_id, min_answer, max_answer, answer }`.
- `Df1WriteSkipped { data_id, df1_feed, reason }`. `Df1SkipReason`'s Borsh variants, in order, are `Disabled`, `NotPassed`, `InvalidAccount`, `InvalidVersion`, `NotWriter`, `Stale`, `AnswerOverflow`, `RoundIdOverflow`, `RoundsToZero`, `TimestampOverflow`.
- `FeedUpdated { data_id, round_id, observations_timestamp, answer }`.

## 9. Limits and measurements

Runtime limits the design works within:

- **Account locks:** at most 64 per transaction on mainnet; a store-only report adds one account, a DF1-backed report two.
- **Instruction trace:** at most 64 instructions per transaction, inner calls and CPI events included. A store-only report adds one (its event); a DF1-backed report two (`submit` or `Df1WriteSkipped`, plus `FeedUpdated`).
- **Heap:** 32 KiB, never freed. `on_report` and `update_feeds` reuse one feed buffer, one event instruction and one `submit` instruction, but each event and `submit` still copies its instruction onto the heap.
- **Account growth:** at most 10 KiB per account per instruction, hence the default ring cap at `initialize` and the growth cap in `set_max_stored_reports`.
- **CPI depth:** at most 5; the report path reaches 3 (forwarder, store, event or `submit`).
- **Transaction size:** 1,232 bytes for legacy and V0, 4,096 for V1.

**How many reports fit in one transaction.** Three runtime limits apply: 4,096 bytes, 64 accounts and 64 instructions (inner calls included). Size and accounts run out first, at about 40 store-only or 25 DF1-backed reports at F=5. The instruction limit would allow about 57 and 28: each stored report adds one instruction (its event), and a report that writes DF1 adds two (`submit` and the event). Memory runs out at about the same point.

**Leave two instructions spare.** Anyone can send SOL to an address before an account is created there. When a program then creates that account, it takes three instructions instead of one. This applies to feed and link-marker accounts created by `set_feeds`, and to the forwarder's per-report execution-state account, whose address anyone can work out from the report. A batch sized exactly to the instruction limit could then fail, and for a report, every retry would fail the same way. So size `set_feeds` calls and report batches with at least two instructions spare.

Measured with `contracts/gotests/data_feeds_store_test.go` (README "Capacity"):

- One report at F=2: 1,016 bytes store-only, 1,113 bytes DF1-backed. At F=5: 1,211 and 1,308 bytes, so a DF1-backed report at F=5 needs a V1 transaction.
- V1 at F=5, no lookup tables: 40 store-only reports in 4,058 bytes and 51 accounts; 25 DF1-backed reports in 3,852 bytes and 63 accounts.
- Compute: about 4.4k units per store-only report and 10.2k per DF1-backed report, on top of the forwarder (about 100k at F=2, 188k at F=5); 20 DF1-backed reports in one `on_report` about 212k.
- Every measured batch fits the default heap.
- `set_feeds` creating 20 feeds: about 243k compute units.

Measured on DF CRE staging (Solana devnet, workflow DON F=2, one report per transaction, 2026-11-18):

- A report whose round is stored and written to its DF1 feed: about 141–146k compute units for the whole forwarder transaction.
- The other zone's copy of the same round, skipped as stale: about 134–138k.
- The DF1 feeds are on Chainlink's deployed devnet DF1 program (`HEvSK…`). They accepted every `submit` the store made: the store and DF1 round counters stayed equal, and no transaction failed. The addresses and example transactions are in the [audit brief, §12](AUDIT.md#12-running-on-df-cre-staging).

## 10. Cases reviewed and considered safe

Each case below is a way someone might try to make the program misbehave, or an unusual situation it could meet. For each one, the first sentence says what the concern is, and the rest says what the program does and why the outcome is safe. Where a test covers the case, it is named.

### Wrong or fake accounts

On Solana the caller supplies every account, so the program must not trust an account just because it was passed in.

- **A feed account from another data ID or another store.** The concern is that a report for BTC/USD could be written into the ETH/USD feed, or into another store's feed. The program first checks that the account is owned by the store program and has the feed type tag (`InvalidAccount` otherwise). It then checks that the data ID and store state recorded in the account match the report's data ID and the store state passed. Any mismatch aborts the transaction (`AccountMismatch`). Spec: "refuses another feed's account".
- **An empty account at the wrong address, passed for a feed that hasn't been created.** The concern is that the program might create or write a feed at an address that isn't the data ID's real feed address. Reports never create feeds, so the program only sees an empty account and skips the report with a "not initialized" event; nothing is written. Through the forwarder this can't happen anyway, because the DON's signature covers the account addresses, so a transmitter can't swap one in. Someone calling the store directly only gets a skip event.
- **A feed or link-marker address that someone has sent SOL to in advance.** The concern is that this blocks the account from ever being created, since Solana's `create_account` fails on an address that already holds SOL. An address holding only SOL is still empty and owned by the system program, so reports skip it as not initialized. When a feed admin then creates the account, `create_pda_account` handles this case: it tops up the rent, allocates the space and assigns it to the store, instead of calling `create_account`.
- **`check_feed_permission` given another data ID's feed, or an empty account at the wrong address.** The concern is that the answer would come from the wrong feed's workflow list. An existing feed must match the store state and data ID asked about, and an empty account must be at the data ID's feed address (`AccountMismatch` otherwise), so the answer always comes from the list that applies.

### Order of rounds

- **The same feed twice in one batch.** The concern is that two reports for one feed in one transaction could be stored out of order, or both stored as the same round. Each report reloads the feed from the account, which the previous report has already written back, so the second report is compared with the first: it is skipped as stale if it isn't newer, and otherwise stored as the next round. Spec: "applies several reports of one feed in a batch in order".
- **Round ID overflow.** The concern is a round counter wrapping around to zero. The store's round ID is a `u64` and the program is built with overflow checks, so it can't wrap in practice. DF1's round counter is a `u32`; the store checks it before calling `submit` and skips the DF1 write (`RoundIdOverflow`) rather than letting `submit` fail.

### History ring

- **Reading a round that has been overwritten.** The concern is that once the ring is full, an old round number could return a newer round's answer from the same slot. Reads only serve the most recent rounds that fit in the ring (round IDs from `latest − size + 1` to `latest`) and return `NoDataPresent` for older ones.
- **Growing the ring after it has wrapped.** The concern is that after a resize, rounds would no longer be in the slot the bigger ring looks in. Growing is refused once the ring has wrapped (`MaxStoredReportsLocked`). Before a wrap, every round sits at the slot equal to its round ID, so growing keeps each round where the bigger ring will look for it. Rings never shrink.

### DF1 links

- **One DF1 feed written twice in one batch.** The concern is that the second write is checked against stale DF1 data. The checks before each `submit` read the DF1 account as it is at that moment, after the first `submit`, so the second write is checked against the new latest timestamp and round counter.
- **Moving a DF1 feed from data ID A to data ID B.** The concern is ending up with two data IDs linked to one DF1 feed, both writing it. Linking B while the DF1 feed's link marker still names A fails (`Df1FeedAlreadyLinked`). A must be unlinked first, either in an earlier call or in an earlier entry of the same call; both entries then use the same link-marker account, so B's entry sees it cleared. A can never be left linked to a DF1 feed whose marker names B.
- **A forged or misplaced link marker.** The concern is passing a fake marker, or one from another store, to get around the one-data-ID rule. Link markers must pass the checks in §3 (the right address for this store and DF1 feed, owned by the store, the right type and version), so a marker can't be faked or shared between stores.
- **One DF1 feed linked from two stores.** This is possible, because link markers are per store. It is harmless: each store writes DF1 as its own `df1_writer`, and a DF1 feed accepts only one writer, so the other store's writes are skipped with `NotWriter`.
- **A linked DF1 feed whose writer isn't the store yet.** This is the normal state during a cutover, so linking is allowed. Reports store the round and skip the DF1 write with `NotWriter`, and the DF1 feed may be passed read-only. Once the DF1 feed's owner makes the store its writer, DF1 writes start.
- **A round DF1 cannot represent safely.** A nonzero answer that rescales to zero is skipped with `RoundsToZero`, and an observation timestamp above `u32::MAX` is skipped with `TimestampOverflow` before DF1's internal u32 cast can wrap it. Both skips leave DF1 unchanged, may pass it read-only and still store the exact store round.
- **A closed DF1 feed, or an upgraded or closed DF1 program.** The concern is that a change on the DF1 side makes every report fail. A closed DF1 feed belongs to the system program again, so the DF1 write is skipped (`InvalidAccount`). A changed account layout is skipped too (`InvalidVersion`, or `InvalidAccount` for a broken layout). What can still fail is a new way for `submit` to fail, or a closed DF1 program (the store doesn't check that it is still executable): those abort the whole batch (brief risk 1). A feed admin unblocks the data ID by unlinking the DF1 feed or setting `df1_write_disabled`.

### Reads

- **A frozen feed read through the client crate.** The concern is that decoding the account directly bypasses the read freeze that the store's read instructions enforce. The crate uses the program's own `to_round`, so it refuses a frozen feed and checks decimals in the same order as the program (crate test `refuses_frozen_feeds`). A reader that decodes without the crate has to make these checks itself.

### Callers

- **Someone other than the forwarder calling `on_report`.** The concern is a write that bypasses the DON. A caller that isn't an `allowed_sender` of a permitted workflow can only cause skip events, and those events name the caller. Writes need a matching workflow entry, and the deployment tooling only creates entries for forwarders.

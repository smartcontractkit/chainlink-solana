# Data Feeds on CRE for Solana: rollout plan

**End goal:** every Solana Data Feed is written by the Data Feeds CRE instance (both zones), as on Aptos and Stellar, and OCR2 is retired for Solana. Customers keep reading the same DF1 feed accounts: the `data-feeds-store` program writes each round through to them, and new readers can read the store's own feed accounts.

**Phases:**

1. **Devnet.** DF CRE staging writes the store on Solana devnet, and through it test DF1 feeds, using today's CRE SDK and its legacy transactions.
2. **Mainnet shadow.** DF CRE prod writes the store on mainnet, store only, still on legacy transactions. OCR2 keeps writing the DF1 feeds customers read.
3. **After V1 transactions.** Once CRE supports Solana's larger V1 transactions, workflows send many reports per transaction, and each DF1 feed's writer is switched from OCR2 to the store, one feed at a time.

We are not using address lookup tables. Items marked _to confirm_ depend on other teams and need checking before the plan is shared.

## 1. Where we are

The pieces and their pull requests:

- **The `data-feeds-store` program** (chainlink-solana [#1676](https://github.com/smartcontractkit/chainlink-solana/pull/1676), draft). One program: it receives reports, keeps each data ID's rounds in its own feed account, serves reads, and keeps linked DF1 feeds written through DF1's unchanged `submit`. 31 Rust tests, 27 Anchor tests and the Go end-to-end test pass. The Go test's in-place loader upgrade uses the current binary and does not test old-layout migration. The program replaces the earlier store ([#1665](https://github.com/smartcontractkit/chainlink-solana/pull/1665)) and proxy ([#1668](https://github.com/smartcontractkit/chainlink-solana/pull/1668)) programs.
- **Deployment changesets** (chainlink-data-feeds [#291](https://github.com/smartcontractkit/chainlink-data-feeds/pull/291), merged). Deploy a store, set feeds and their settings, link DF1 feeds, cut over and roll back, directly or through MCMS. Unit tests pass. The integration harness currently stops during MCMS setup with `DeclaredProgramIdMismatch`, before any store operation.
- **The workflow's Solana adapter** (chainlink-data-feeds [#290](https://github.com/smartcontractkit/chainlink-data-feeds/pull/290), merged). Updated to the one-program store, DF1 links included; 76 focused Solana tests and 773 full workflow tests pass, and it runs on DF CRE staging.
- **Staging zone-a enablement** (chainlink-deployments [#20533](https://github.com/smartcontractkit/chainlink-deployments/pull/20533) and [#20573](https://github.com/smartcontractkit/chainlink-deployments/pull/20573), merged and run): the consensus job, the Solana capability in its own chain-capability DON, the zone-a forwarder, MCMS and the capability job. The Solana MCMS handover proposal is [#20609](https://github.com/smartcontractkit/chainlink-deployments/pull/20609).
- **Solana devnet chain config** on the zone-a chain-capability nodes (infra-k8s [#43837](https://github.com/smartcontractkit/infra-k8s/pull/43837), merged).

**Proven:**

- Reports signed by an F=2 and an F=5 DON, sent through the keystone forwarder exactly as CRE builds them, land in the store and the DF1 feeds (`TestDataFeedsStore`).
- Both DF CRE staging zones write the one-program store on devnet, and through it two DF1 feeds on Chainlink's devnet DF1 program, at about 141–146k compute units per DF1-backed report.

**Not yet proven:** the same on mainnet, and V1 transactions.

The program's design, its decisions and its audit brief are in [README.md](README.md), [design-alignment.md](design-alignment.md) and [AUDIT.md](AUDIT.md).

## 2. Why three phases

A CRE write is one forwarder instruction: the report, F+1 signatures (65 bytes each) and every account inline. Legacy transactions are capped at 1,232 bytes. Measured with the Go end-to-end test:

- **Staging, F=2:** one report fits a legacy transaction, store only (1,016 bytes) or with the DF1 write (1,113 bytes).
- **Prod, F=5:** one store-only report fits (1,211 bytes); one report with the DF1 write doesn't (1,308 bytes).

So the mainnet shadow can run on legacy transactions now, store only, but the DF1 cutover needs V1 transactions (4,096 bytes, at most 64 accounts). With V1, one transaction at F=5 carries about 40 store-only or 25 DF1-backed reports.

CRE also caps each write: 10 writes per workflow execution, a 265-byte report and 300k compute units per Solana write. Report entries are 40 bytes, so after fixed overhead at most two fit the raw-report limit. Each write carries one or two reports today (`maxReportsPerTransaction`, §4.4), so a workflow covers 10 to 20 feeds on staging (10 writes × 1 or 2) and 10 on the mainnet shadow (10 × 1). V1 transactions only help once CRE also raises the report-size and compute caps (ask 9).

## 3. Asks of PLEX

Node-side setup is with PLEX. Status of each ask is _to confirm_.

**Phase 1, staging (`domains/cre/data-feeds-cre-staging`), Solana devnet, zone-a first, then zone-b:**

1. **Staging enablement in chainlink-deployments:** sign and execute the MCMS handover proposal ([#20609](https://github.com/smartcontractkit/chainlink-deployments/pull/20609)), check the forwarder's final owner is the timelock, and accept the capability job on the chain-capability nodes.
2. **Node keys and JD (Job Distributor):** an OCR2 Solana key on each node, and Solana devnet added under JD's "Support Chains". On chain-capability nodes, the account address entered in Support Chains becomes the capability job's transmitter, so it must be the node's real Solana transmitter address.
3. **Node chain config (infra-k8s):** the nodes run the Solana devnet config before they accept the job. Zone-b needs its own PR.
4. **Funded wallets:** the CI deployer wallet (`ONCHAIN_SOLANA_WALLET_KEY`; Solana has no KMS option), and devnet SOL on the transmitter keys, with balance alerts.
5. **Forwarders:** one forwarder per zone, qualified by zone (`zone-a`, `zone-b`), never CRE mainline's. PLEX to confirm the per-zone layout, who holds upgrade authority, and to keep each forwarder in step with registry changes.
6. **CRESettings:** allow Solana devnet (chain selector `16423721717087811551`) in the zone's `ChainAllowed`, re-propose CRESettings, and check the accepted settings include it.

**Phase 2, prod (`domains/cre/data-feeds-cre-prod`), Solana mainnet:**

7. **Same as 1 to 6** for both prod zones, with a separate forwarder per zone.

**Phase 3:**

8. **V1 transactions** in the write capability, the relayer's transaction manager and an SDK release (date _to confirm_).
9. **Raise the DF limits:** the report size limit to about 1,100 bytes and the gas limit to about 500k compute units, to be measured, in both DF environments.

## 4. Phase 1: devnet on DF CRE staging

**Exit criterion:** both staging zones write a handful of devnet feeds every round, each round lands in the store and in the DF1 test feeds, and monitoring is green for a week.

**4.1 Deploy the one-program store (done).** The devnet program was upgraded in place to the one-program store, keeping its program ID. Each zone's forwarder authority, the sender the store allows, is derived from the receiving program's ID, so keeping the ID kept both zones' senders valid. Version 1 feeds are not migrated: the version 2 feed layout requires a new store and newly created version 2 feeds. The deployment therefore has:

- a new store (store ID 1, owned by the deployer, without MCMS on staging);
- both zones' workflows as default workflows;
- BTC/USD and ETH/USD feeds.

Each feed is linked to a new DF1 feed on Chainlink's devnet DF1 program, with the store's DF1 writer as its writer. The old store state and its feeds are no longer used and need no action; they only hold their rent. The old proxy program was closed and its rent reclaimed. Later stores and feeds can use the changesets (#291).

**4.2 Forwarders (ask 5).** As on EVM, the store trusts the caller through its allowed-workflow list: each zone's forwarder authority, with the DF workflow owner and name, is an entry. Adding or rotating a forwarder is a metadata update.

**4.3 Deployments and nodes (asks 1 to 4, 6).** The chainlink-deployments stages, CRESettings, node config and funded transmitters, then the capability job, accepted on each chain-capability node.

**4.4 Workflow (done for zone-a and zone-b on devnet).** The adapter (#290) now follows the one-program store:

- Report timestamps are encoded as u64 Unix seconds. Offchain timestamps must be non-negative JavaScript safe integers, and decoding rejects a feed timestamp above that safe range.
- Each report passes its data ID's feed account, then the linked DF1 feed when the feed links one.
- The fixed accounts are the store state, the DF1 program and the store's DF1 writer. When no report in the transaction writes DF1, the adapter passes the store's program ID in place of the DF1 program and writer, which is how Anchor marks an optional account as absent.
- A DF1 feed whose link is disabled, or not yet cut over, can be passed read-only, so OCR2's writes to it aren't blocked.

Each zone has its own config (the same store state, that zone's forwarder program and state), deployed with the CRE CLI. Both use `maxReportsPerTransaction: 1`, since two DF1-backed reports at F=2 exceed a legacy transaction; a store-only workflow can use 2. A workflow covers at most 20 feeds. Staging writes BTC/USD and ETH/USD, on the same streams as the EVM staging feeds, so prices can be compared.

**4.5 Monitoring.** Add the Solana capability dashboard and canaries on the CRE health-check dashboards, and reuse the chain-agnostic DF CRE alerts. Solana-specific signals:

- failed or dropped transactions;
- freshness: each feed's `latest_observations_timestamp` and the DF1 feed's latest round;
- `Df1WriteSkipped` events;
- transmitter balances.

## 5. Phase 2: mainnet shadow on DF CRE prod

**Starts** once phase 1 is green and ask 7 is done. **Exit criterion:** both prod zones write every planned feed into the mainnet store every round, prices track the DF1 feeds within deviation, and monitoring is green for two weeks.

1. **Same as 4.1 to 4.5** against the prod DONs (workflow DON F=5). On mainnet the MCMS timelock holds the program's upgrade authority and the store's owner and feed admin roles, so each change is a timelock proposal. The changesets fund the timelock signer with the rent of the accounts a proposal creates.
2. **Store only:** no DF1 links. The DF1 feeds stay on OCR2 and customers see no change.
3. **Workflow:** `maxReportsPerTransaction: 1`, at most 10 feeds per workflow.
4. **Compare:** alert when the store's rescaled answer and the DF1 answer diverge; `CutoverDF1Feeds` runs the same check per feed.
5. **Costs:** one transaction fee per feed per round until V1. Keep the shadow's feed count to what's needed.

## 6. Phase 3: V1 transactions, then DF1 cutover

**Exit criterion:** mainnet DF1 feeds written by CRE, OCR2 jobs stopped, rollback rehearsed.

**6.1 After V1 ships (ask 8).** Bump `@chainlink/cre-sdk` and raise the DF limits (ask 9). Raise `maxReportsPerTransaction` toward 25. That fits the 64-account limit at F=5 and leaves more than the two spare instructions every batch needs, because anyone can make the forwarder's per-report record cost two extra instructions (audit reference §9). Soak on devnet, then on the shadow, then merge mainnet workflows down to about one per 250 feeds (10 writes × 25 reports).

**6.2 DF1 cutover, per feed, with the DF1 feed's owner.**

1. `SetStoreDF1Feeds` links the data ID to its DF1 feed. The DF1 write is skipped until the cutover, and the round is still stored.
2. `CutoverDF1Feeds` checks the store's value is fresh and matches the DF1 feed's, then sets the DF1 feed's writer to the store's DF1 writer with DF1's `set_writer`.
3. Stop the feed's OCR2 job.

To roll back:

1. `RollbackDF1Feeds` sets the DF1 feed's writer back to OCR2's store authority.
2. Restart the feed's OCR2 job.
3. Disable the store's DF1 write for the feed. The store is no longer the DF1 feed's writer, so otherwise every round would emit a `Df1WriteSkipped` event.

## 7. Aside: fire drills for Solana

`chainlink-data-feeds/monitoring/fire-drills` runs the two-zone CRE stack locally. To add Solana:

- a `solana-test-validator` target with the forwarder, the store, DF1 and a few DF1 feeds, set up by the same changesets;
- one zone's workflow pointed at it (the local DON is F=1, so legacy transactions are fine);
- the existing transaction drills, and one new drill for a DF1 feed whose writer wasn't switched (`Df1WriteSkipped`).

It is a few days' work, off the critical path.

## 8. Risks and open questions

- **PLEX setup could delay phases 1 and 2.** Both need Solana enabled on the DF CRE instances. Raise asks 1 to 6 early so they don't queue behind V1.
- **V1 could slip.** The mainnet shadow keeps running regardless; only the DF1 cutover waits.
- **The DF1 cutover depends on DF1 feed ownership moving to MCMS.** Mainnet DF1 feeds move to MCMS ownership, with the bypasser for an urgent rollback, so the cutover and rollback become timelock proposals. Moving the DF1 program's upgrade authority to MCMS is separate work.
- **Event readers will notice the cutover.** OCR2's `NewTransmission` events stop after a feed's cutover, so event readers should switch to the store's `FeedUpdated` events. Account reads are unchanged.
- **Every admin change waits for the timelock.** On mainnet the MCMS timelock holds the store program's upgrade authority and the store's owner and feed admin roles. The upgrade authority is also the only key that can create stores, so revoking it would end that too.
- **No tooling creates DF1 feeds yet.** New feeds live in the store only, and a DF1 feed is created for one only on request, with the store's DF1 writer as its writer. No changeset creates DF1 feed accounts yet.
- **A new store build could misbehave on mainnet.** A candidate build runs first as a shadow deployment under a second program ID, then the main program is upgraded to the same verified build. Rolling back is upgrading back. The version 2 feed layout does not migrate version 1 feeds: using it requires a new store and newly created version 2 feeds, and the Go loader-upgrade test does not prove old-layout compatibility.
- **The audit must finish before the DF1 cutover.** No customer reads the shadow feeds, so the shadow can run the build from before the audit. Book the audit early.

# Beholder Metrics Assessment — chainlink-solana

**Scope:** observability (`beholder.GetMeter()` / OTel) for the following services:
- `pkg/solana/logpoller`
- `pkg/solana/client`
- `pkg/solana/txm` (not `pkg/txm`, as named in the request)
- `pkg/solana/chainreader`
- `pkg/solana/chainwriter`

**Goal:** For a production incident, how much metric data do we have to figure out "what's going on"?

**Method:** Grepped every `beholder.GetMeter()` call site in the Solana packages, mapped each metric to the type and attributes emitted, and verified record call sites in the hot paths. All metrics below are emitted to Beholder (OTel) unless explicitly flagged **Prometheus-only**.

---

## Summary Table

| Service | Beholder (OTel) metrics? | Prometheus-only metrics? | Overall incident visibility |
|---|---|---|---|
| `logpoller` | ✅ Yes (9 metrics, 2 sources) | ✅ parallel mirrors | **Good** |
| `txm` | ✅ Yes (11 counters/gauges) | ✅ parallel mirrors | **Good** |
| `client` | ❌ **No** | ⚠️ latency histogram only | **Weak** |
| `chainreader` | ❌ **None at all** | ❌ none | **None** |
| `chainwriter` | ❌ **None directly** | ❌ none | **Indirect only** (via txm/fees) |

**Bottom line:** Half the pipeline (logpoller ingestion + txm submission) is well instrumented for
Beholder. The other half (RPC client, chainreader, chainwriter) provides essentially **no OTel data** —
a production incident in those paths would be diagnosed primarily from logs, not metrics.

---

## 1. `pkg/solana/logpoller` — ✅ Good

Two independently-registered Beholder meter instances.

### a) Service-local metrics — `logpoller/metrics.go` (`NewSolLpMetrics`)
Registered against `beholder.GetMeter()`. Attributes: **`chainID`**.

| OTel metric | Type | Recorded in |
|---|---|---|
| `solana_log_poller_txs_truncated_succeeded` | Int64Counter | `job_get_block.go:208` |
| `solana_log_poller_txs_truncated_reverted` | Int64Counter | `job_get_block.go:208` |
| `solana_log_poller_txs_log_parsing_error_succeeded` | Int64Counter | `job_get_block.go:188` |
| `solana_log_poller_txs_log_parsing_error_reverted` | Int64Counter | `job_get_block.go:188` |
| `solana_log_poller_last_processed_slot` | Int64Gauge | `log_poller.go:561` |
| `solana_log_poller_blocks_skipped` | Int64Counter | `job_get_block.go:56` |

High-cardinality per-tx outcomes are collapsed into succeeded/reverted — lossy (raw errors absent)
but still directional.

### b) ORM/DB metrics — `chainlink-framework/metrics` (`NewGenericLogPollerMetrics`, `logpoller/observability.go`)
Registered against `beholder.GetMeter()`. Attributes: **`chainFamily`, `chainID`**, plus `query`/`type`.

| OTel metric | Type | Purpose |
|---|---|---|
| `log_poller_query_duration` | Float64Histogram | query latency |
| `log_poller_query_dataset_size` | Int64Gauge | rows returned |
| `log_poller_logs_inserted` | Int64Counter | insert throughput |
| `log_poller_blocks_inserted` | Int64Counter | block ingestion |
| `logpoller_log_discovery_latency` | Float64Histogram | discovery delay |

Recorded in `observability.go` for every DB exec/query (`withObservedExec`, `RecordQueryDatasetSize`, etc.).

Every metric is mirrored to Prometheus. **Strengths:** DB-level throughput/latency + slot progress +
per-tx outcome counts. **Gaps:** no per-consumer-filter transaction volume, no finality-lag distribution
beyond discovery latency (could add block-timestamp lag).

---

## 2. `pkg/solana/txm` — ✅ Good

Service-local `txm/metrics.go` (`newSolTxmMetrics`) against `beholder.GetMeter()`. Attributes: **`chainID`**.

| OTel metric | Type | Recorded in |
|---|---|---|
| `solana_txm_tx_success` | Int64Counter | `pendingtx.go:760` |
| `solana_txm_tx_finalized` | Int64Counter | `pendingtx.go:787` |
| `solana_txm_tx_pending` | Int64Gauge | `pendingtx.go:771` |
| `solana_txm_tx_error` | Int64Counter | `pendingtx.go:826` |
| `solana_txm_tx_error_revert` | Int64Counter | `pendingtx.go:816` |
| `solana_txm_tx_error_reject` | Int64Counter | `pendingtx.go:814` |
| `solana_txm_tx_error_drop` | Int64Counter | `pendingtx.go:818` |
| `solana_txm_tx_error_sim_revert` | Int64Counter | `pendingtx.go:820` |
| `solana_txm_tx_error_sim_other` | Int64Counter | `pendingtx.go:822` |
| `solana_txm_tx_error_dependency` | Int64Counter | `pendingtx.go:824` |
| `solana_txm_fee_bumps` | Int64Counter | `txm.go:364` |

All mirrored to Prometheus. **Strengths:** the best-outcome classification of any service — success /
finalized / pending / and 7 distinct failure modes (revert, reject, drop, sim-revert, sim-other,
dependency, generic). Straightforward "are my txs landing, and if not why" triage.
**Gaps:** counters-instrument a *transaction lifecycle*; individual per-tx diagnostic data (signature,
program_id, error code) is not carried as attributes (by design, to bound cardinality).

---

## 3. `pkg/solana/client` — ❌ Weak / No Beholder

**No `beholder.GetMeter()` anywhere in the client package.** There is no OTel export from the RPC client.

What exists is **Prometheus-only**:
- `monitor.SetClientLatency` (`monitor/prom.go:48`) → `metrics.RPCCallLatency` histogram and the
  deprecated `solana_client_latency_ms` gauge. Labels include `chainFamily`, `chainID`, `rpcUrl`,
  `isSendOnly`, `success`, `rpcCallName`. Recorded for every RPC via `Client.latency()` (`client.go:145`).
- `MultiNodeClient` (`multinode_client.go`) wires **no** `RPCClientMetrics` and no observer metrics.

**Key finding:** A comment-based, Beholder-backed RPC latency path already exists upstream —
`chainlink-framework/metrics.RPCClientMetrics` / `NewRPCClientMetrics` (`client.go`) emits
`rpc_call_latency` as a Beholder Float64Histogram with `success`, `callName`, `rpcDomain` attributes.
**It is defined but not wired into the Solana client.**

→ In an incident you can see RPC latency/error-rate by endpoint from Prometheus, but **nothing in OTel/Beholder**.

---

## 4. `pkg/solana/chainreader` — ❌ None

`chain_reader.go` contains **zero** metric code (no prometheus, no beholder, no meter). There is no
observability for read paths: contract read latency, batch fills (`batch.go`), bindings, filter
lookups, or account/event decoding errors.

→ **A degraded or wedged read path is invisible to metrics entirely.**

---

## 5. `pkg/solana/chainwriter` — ❌ None directly, indirect only

`chain_writer.go` has **zero** metric code of its own. Observability is inherited only from its
dependencies:
- Transactions it submits flow through `txm.TxManager` → the full txm metric set above (good signal
  for "are writes landing", but attributable to chainwriter only via timing/correlation, not labels).
- Fee estimation uses `fees.Estimator` whose block-history estimator emits a Beholder gauge
  `solana_bhe_compute_unit_price` (`fees/block_history.go:84`).

No direct metrics for: buffer management (`buffer_payload.go`), ATA creation (`ata_creation.go`),
write modelling/build/encode failures, per-recipient writes, submit/sign latency, or transform
registry (`transform_registry.go`).

→ **"Why is my write not working / stuck in buffer?" is not answerable from metrics.**

---

## Incident-Readiness Assessment

| Question you'd ask in prod | Answerable from Beholder today? |
|---|---|
| Is the logpoller keeping up / behind? | ✅ slots processed, blocks skipped, logs inserted, discovery latency |
| Are reads failing or slow? | ❌ no read-path metrics at all |
| Are my txms landing transactions? | ✅ success/finalized/pending |
| Why is a tx failing? | ✅ 7 failure-class counters |
| Is the RPC endpoint degraded? | ❌ Prometheus latency only, no OTel |
| Are chainwriter writes stuck/erroneous? | ❌ indirect via txm only |

### Recommended next steps (highest impact first)
1. **Wire `RPCClientMetrics` into `client`/`MultiNodeClient`** — a defined, tested, Beholder-backed
   histogram upstream is sitting unused. Instantiate with `NewRPCClientMetrics` and route
   `SetClientLatency` through `RecordRequest`. Deliverable: OTel RPC latency + error rate by call name.
2. **Add read-path observability to `chainreader`** — at minimum query latency + error counters
   (read / batch / decoding / binding) keyed by `chainID` (+ contract where safe); reuse the framework
   `metrics` package patterns.
3. **Add write-path observability to `chainwriter`** — counters for build/encode/submit failures,
   buffer size gauge (`buffer_payload.go`), ATA creation latency, and a per-write outcome counter that
   can be correlated to txm.
4. **Consider an EVM-style observer/metrics bridge** for `MultiNodeClient` so per-node health/selection
   is visible in OTel.

---

*Assessment generated from source inspection (commit-independent). MultiNode/account-balance (`node_balance`,
balance monitor) are OTel-instrumented but live in the monitor service, outside the 5 requested packages and
thus noted as context, not part of this scope.*

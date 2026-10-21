# Arcs — Events, Observability and Operations (M5)

This document covers the local event feed, metrics, logs, resource limits and
the operator runbook. For the data model and lifecycle see
[01-design.md](01-design.md); for the API contract see
[03-api-and-explorer-contract.md](03-api-and-explorer-contract.md).

## 1. Two kinds of "event"

| | Authoritative transitions | Observed events (`arc.events`) |
| --- | --- | --- |
| What | `CreateSubnetTx`, `CreateChainTx`, `AddSubnetValidatorTx`, `RemoveSubnetValidatorTx` accepted on the M-chain | Lifecycle, health and membership changes **as seen by one node** |
| Where | M-chain blocks; `/ext/index/M/block` when `--index-enabled` | `arc.events` on `/ext/bc/M/arc` |
| Persistence | M-chain state (consensus) | In-memory ring buffer (1024 events), lost on restart |
| `authoritative` | n/a | always `false` |
| Use | Rebuilding the registry, auditing creation and membership | Explorer freshness, alerting, dashboards |

An indexer that needs a durable, complete history of registrations and
membership changes must consume M-chain blocks (or simply re-read
`arc.list` / `arc.validators`, which are pure functions of M-chain
state). The observed feed is a convenience for liveness and health.

## 2. Observed event feed

Implemented in `arc/events.go` and `arc/observer.go`.

The observer polls every 5 s. Each poll samples engine health of every chain
it runs (`TryLock` on the chain's context lock; a busy chain is skipped and
counted in `arc_health_checks_skipped`) and re-derives the lifecycle of
every **tracked** Arc. Differences from the previous poll emit events:

| Type | Emitted when |
| --- | --- |
| `registered_observed` | first poll on this boot that sees a tracked registered chain |
| `initialisation_started` | lifecycle → `INITIALISING` |
| `activated` | lifecycle → `ACTIVE` (from anything other than `DEGRADED`) |
| `health_changed` | `ACTIVE` ↔ `DEGRADED` |
| `stopped` | lifecycle → `STOPPED` |
| `failed` | lifecycle → `FAILED` |
| `lifecycle_changed` | any other lifecycle change |
| `validator_membership_changed` | fingerprint of the current committee (node ID + weight) changed |

A chain first observed past `PENDING` (e.g. already `ACTIVE` after a restart,
or already `FAILED`) gets `registered_observed` followed by the event for its
current state, so consumers never miss a terminal state.

### Delivery semantics

- `id = "<bootID>-<seq>"` is globally unique; `seq` is strictly increasing
  within a `bootID`. Consumers de-duplicate by `id`.
- Poll with `after = nextCursor`. If `truncated` is `true`, events between
  the cursor and `oldestSeq` were evicted: resynchronise from
  `arc.list`, then continue from `nextCursor`.
- A changed `bootID` means the node restarted and its sequence restarted at 1:
  resynchronise from `arc.list`, then follow the new boot from
  `after = 0`.
- Events carry `schemaVersion` so consumers can reject unknown versions.
- Not emitted: metadata updates (descriptors have no mutable metadata — the
  only inputs are immutable `CreateChainTx` fields plus committee membership).

## 3. Metrics

All metrics are registered on the node's Prometheus registry (exposed at
`/ext/metrics`). **No metric is labelled by chain ID, chain name or any other
user-controlled value**; the only labels are bounded enums (`lifecycle` has 8
values, `method` is bounded by the service's 8 methods).

| Metric | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `arc_registered` | gauge | — | registered Arcs (M-chain), last poll |
| `arc_tracked_by_lifecycle` | gauge | `lifecycle` | tracked Arcs by local lifecycle |
| `arc_lifecycle_transitions` | counter | `lifecycle` | observed transitions, by target state |
| `arc_invalid_lifecycle_transitions` | counter | — | transitions not allowed by the state machine (should be 0) |
| `arc_initialisation_duration_seconds` | histogram | — | first `PENDING`/`INITIALISING` observation → operational, poll resolution (5 s) |
| `arc_initialisation_failures` | counter | — | tracked chains whose build failed (once per chain per boot) |
| `arc_accepted_txs` | counter | — | txs accepted across all Arcs run by this node |
| `arc_accepted_containers` | counter | — | vertices accepted across all Arcs run by this node |
| `arc_health_checks_skipped` | counter | — | health samples skipped because the chain lock was busy |
| `arc_health_check_panics` | counter | — | contained panics in engine health checks |
| `arc_poll_panics` | counter | — | contained panics in the poll loop |
| `arc_api_request_duration_count`, `_sum`, `arc_api_request_error_count` | counter / gauge (ns) / counter | `method` | request volume, cumulative latency and errors of `arc.*` (`utils/metric.APIInterceptor`); unknown methods are rejected before the interceptor runs, so `method` is bounded |

Per-chain detail (height, accepted counts, last acceptance) is available
through `arc.stats` and the chain's own namespaced metrics that the
chain manager already registers per chain alias.

Derived alerts (suggested):

- `arc_tracked_by_lifecycle{lifecycle="FAILED"} > 0`
- `arc_tracked_by_lifecycle{lifecycle="DEGRADED"} > 0 for 5m`
- `increase(arc_initialisation_failures[1h]) > 0`
- `arc_invalid_lifecycle_transitions > 0` (bug)
- `time() - <observerLastPoll>` via `arc.health.dependencies` > 30 s
  (stale observations)

## 4. Logs

Structured `zap` logs, via the M-chain logger (`main.log` / `M.log`):

- `arc API request failed` (warn) — method and internal error; the
  client only receives `internal_error`.
- `unexpected Arc lifecycle transition` (warn) — chain, from, to.
- `Arc engine health check panicked` (error) — chain, panic value.
- `Arc engine unhealthy` / `Arc evaluation failed` (debug).

Chain build failures are logged by the existing chain manager
(`error creating chain`) with the full error; the API only exposes
`initialisation_failed`.

## 5. Resource limits

| Limit | Value | Where |
| --- | --- | --- |
| Concurrent `arc.*` requests | 16 (`CodeBusy` after 1 s wait) | `service.go` |
| Request timeout | 10 s | `service.go` |
| `arc.list` page size | default 25, max 100 | `service.go` |
| `arc.events` page size | default 25, max 500 | `service.go` |
| `getCreationStatus` validator tx IDs | max 100 | `service.go` |
| Event buffer | 1024 events | `observer.go` |
| Concurrent chain initialisation | 1 (the chain manager builds chains sequentially from its queue) | `chains/manager.go` (existing) |

**Chain-creation abuse.** Every `CreateSubnetTx` and `CreateChainTx` pays the
M-chain's `CreateSubnetTxFee` / `CreateBlockchainTxFee`. A node only runs
Arcs of subnets its operator lists in `--whitelisted-subnets`, so
registrations by third parties consume M-chain state but **no execution
resources** on nodes that do not opt in. `arc.list` cost grows linearly
with registered chains (computed on demand from M-chain state under the
M-chain lock); the concurrency cap above bounds its impact.

**Isolation.** Each Arc has its own `prefixdb` keyspace in the node
database, its own chain data directory, its own handler goroutines and its own
consensus context lock. An Arc's failure is logged and reported as
`FAILED` without stopping the node (only V, M and U are critical chains).
Per-chain disk usage is not separately measurable because all chains share
the node's underlying database; this is a known gap.

## 6. Operator runbook

### Enabling a node to run Arcs

1. Staking must be enabled (`--staking-enabled=true`, default). With staking
   disabled the committee is not enforced and the API reports
   `enforcement: not_enforced_staking_disabled`.
2. Add the subnet ID to `--whitelisted-subnets` (comma separated) and restart
   the node. The list is static.
3. Optional: `<subnet-config-dir>/<subnetID>.json` with
   `{"validatorOnly": true}` restricts the chain's peer-to-peer messages to
   committee members (reduces exposure, does **not** encrypt state), and
   `consensusParameters` to tune `k`, `alpha`, `betaVirtuous`, `betaRogue`
   for small committees. Every committee member must use compatible values.

### Creating an Arc

Use the client-side workflow (`arc/workflow`), which signs ordinary
M-chain transactions with the subnet owner's keys:

```go
res, err := workflow.Create(ctx, pWallet, pClient, arc.NewClient(uri), workflow.Request{
    Name:            "alpha",
    Genesis:         genesisBytes, // workflow.BuildAVMGenesis(...)
    Owner:           &secp256k1fx.OutputOwners{Threshold: 1, Addrs: []ids.ShortID{owner}},
    Committee:       []workflow.Member{{NodeID: n1, Weight: 20}, {NodeID: n2, Weight: 20}, {NodeID: n3, Weight: 20}},
    ValidationStart: time.Now().Add(30 * time.Second),
    ValidationEnd:   time.Now().Add(30 * 24 * time.Hour),
})
```

Retrying with the same request, plus `SubnetID` from the first attempt, is
idempotent. Track progress with `arc.getCreationStatus`. Committee
members must whitelist the subnet (step 2 above) before the chain can make
progress.

### Diagnosing

| Symptom (`arc.health`) | Likely cause | Action |
| --- | --- | --- |
| `UNKNOWN`, `node_not_tracking_subnet` | node does not whitelist the subnet | ask a node that does, or whitelist |
| `PENDING`, `queued_for_creation` | M-chain still bootstrapping or queue busy | wait; check `dependencies.mChainBootstrapped` |
| `INITIALISING`, `bootstrapping` for long | < 50 % of committee weight reachable | check member nodes, `arc.validators` |
| `DEGRADED`, `connected_committee_weight_below_threshold` | members offline or not whitelisting | restore members; liveness needs ≈ 75 % |
| `DEGRADED`, `committee_below_minimum_size` | owner removed members / periods expired | add members via `AddSubnetValidatorTx` |
| `DEGRADED`, `engine_health_not_observed` | no fresh health sample (busy chain) | transient; check `arc_health_checks_skipped` |
| `FAILED`, `initialisation_failed` | invalid genesis or VM error | node log `error creating chain`; chain must be re-registered with a valid genesis |
| `FAILED`, `chain_handler_stopped_unexpectedly` | engine stopped outside shutdown | node log; restart node |

### Restart and recovery

No Arc-specific action is needed. On start, the M-chain re-queues every
registered chain of whitelisted subnets; each chain reopens its DB prefix and
re-bootstraps from its committee. Observed statistics and events restart from
zero (new `bootID`). Verified end-to-end in `tests/arc` phase 8.

### Per-chain stop

Not supported: the chain manager has no per-chain stop. To stop running a
Arc on a node, remove its subnet from `--whitelisted-subnets` and
restart; to stop it network-wide, the owner removes the committee
(`RemoveSubnetValidatorTx`), after which the chain cannot make progress.

### Local development network

`tests/arc/devnet_test.go` starts the same 5-node staking network as the
end-to-end test, creates Arcs `alpha` (committee node1–3) and `beta`
(node3–5) plus `gamma` (fails initialisation on purpose), sends a small
transfer on alpha and beta every 20 s, and keeps running:

```sh
./scripts/build_dijets.sh
ARC_DEVNET=1 ARC_DEVNET_HOLD=2h \
  go test -tags arc_e2e -timeout 0 -v -run TestDevNetwork ./tests/arc/
```

Nodes listen on `http://127.0.0.1:19650` … `19658` (node *n* on
`19650 + 2(n−1)`). Nodes 3 and 4 run every Arc and make good observers for
`arc-explorer/`. Ctrl-C stops all nodes; the network directory is kept under
`$TMPDIR/arc-e2e-*` for inspection.

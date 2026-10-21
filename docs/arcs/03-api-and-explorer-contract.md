# Arcs — API Contract (v1) and Explorer Integration Plan

The explorer is **not** part of this repository. This document is the
contract between the node (`dijetsnodesgo`) and the explorer, plus a plan for
integrating it. Every example below is a real response captured by the
end-to-end test (`tests/arc`, written to `<network dir>/api-examples/`),
trimmed only where marked `…`.

## 1. Transport and conventions

| Item | Value |
| --- | --- |
| Endpoint | `POST /ext/bc/M/arc` (also `/ext/bc/<M-chain ID>/arc`) |
| Protocol | JSON-RPC 2.0, gorilla `rpc/v2` — the same codec as `platform.*` and `avm.*` |
| Methods | `arc.list`, `get`, `validators`, `stats`, `health`, `capabilities`, `getCreationStatus`, `events` |
| Mutating methods | **none**. Creation and administration are signed M-chain transactions (`platform.issueTx`, wallet, `arc/workflow`). |
| Authentication | none needed (read-only, public data). Bind/filter with the node's existing `--http-host` / `--http-allowed-origins` / `--http-allowed-hosts`. |
| Integers ≥ 2^53 | strings (`"weight": "20"`), as in other node APIs |
| Times | RFC 3339, UTC |
| IDs | CB58 (chain, subnet, tx IDs); `NodeID-…`; owner addresses bech32 with `M-` prefix |
| Envelope | every reply has `apiVersion` (`"v1"`) and `schemaVersion` (`1`) |
| Limits | 16 concurrent requests, 10 s timeout, pages ≤ 100 (events ≤ 500) |

### Versioning

- `apiVersion` changes on any breaking change to method names, arguments or
  reply shapes; a new version would be served at a new path.
- `schemaVersion` changes on breaking changes to descriptor/event shapes.
- Adding fields, reason codes, lifecycle values, capabilities or profiles is
  **not** breaking. Clients must ignore unknown fields and treat unknown enum
  values as "unknown", never as healthy.

### Errors

```json
{"jsonrpc":"2.0","id":1,"error":{"code":-32004,"message":"Arc not found",
 "data":{"apiVersion":"v1","reason":"not_found"}}}
```

| Code | `data.reason` | Meaning |
| --- | --- | --- |
| -32602 | `missing_chain_id`, `invalid_chain_id`, `invalid_cursor`, `invalid_vm_id`, `invalid_subnet_id`, `invalid_lifecycle`, `invalid_profile`, `limit_too_large`, `missing_tx_id`, `invalid_tx_id`, `too_many_tx_ids`, `subnet_mismatch` | invalid request |
| -32004 | `not_found` | not a registered chain |
| -32005 | `not_an_arc` | V, M or U (primary network chain) |
| -32029 | `too_many_concurrent_requests` | retry with back-off |
| -32000 | `internal_error` | details only in the node log |

### Authoritative vs observed

Every field is one of:

- **Authoritative** — derived from committed M-chain state (`descriptor`,
  committee membership). Identical on every bootstrapped node at the same
  `mChainHeight`.
- **Local observation** — marked `source: "local-observation"` with
  `observerNode` and `observedAt` (lifecycle, health, connectivity, stats,
  events). Different nodes may legitimately disagree.

Unavailable data is `null` and named in an `unavailable` map with a reason:
`not_recorded_in_authoritative_state`, `not_configured`,
`chain_not_served_by_this_node`, `dag_chain_has_no_linear_height`,
`not_observed_since_node_start`, `node_not_tracking_subnet`.

## 2. Methods

### `arc.list`

Arguments (all optional): `lifecycle`, `owner` (address, with or without
`M-` prefix), `vmID`, `profile`, `subnetID`, `name`, `cursor`, `limit`.
Ordered by chain ID (bytes); `nextCursor` is `""` on the last page. Filters
apply to `chains` and `counts.matched`; all other counts cover every
registered chain.

```json
{"jsonrpc":"2.0","id":1,"method":"arc.list","params":{"limit":25}}
```
```json
{
  "apiVersion": "v1", "schemaVersion": 1,
  "chains": [ { "descriptor": {…}, "observation": {…} }, … ],
  "nextCursor": "",
  "counts": {
    "registered": 2, "tracked": 1, "operational": 1, "healthy": 1,
    "byLifecycle": {"ACTIVE":1,"DEGRADED":0,"FAILED":0,"INITIALISING":0,
                    "PENDING":0,"STOPPED":0,"STOPPING":0,"UNKNOWN":1},
    "matched": 2
  },
  "mChainHeight": 12,
  "observedAt": "2026-10-21T18:27:15.074632Z",
  "observerNode": "NodeID-7Xhw2mDxuDS44j42TCB6U5579esbSt3Lg"
}
```

| Count | Definition |
| --- | --- |
| `registered` | `CreateChainTx` committed on a non-primary subnet (authoritative) |
| `tracked` | registered chains whose subnet this node whitelists |
| `operational` | tracked chains in `ACTIVE` or `DEGRADED` on this node |
| `healthy` | tracked chains in `ACTIVE` on this node |
| `byLifecycle` | every registered chain by local lifecycle; untracked = `UNKNOWN` |

### `arc.get`

`{"chainID": "<ID or alias>"}` →

```json
{
  "apiVersion": "v1", "schemaVersion": 1,
  "descriptor": {
    "schemaVersion": 1, "protocolVersion": "arc/1", "metadataVersion": 1,
    "id": "28vcu7VaHmh4XMbagjioa1iWNaS82LfDmRGYuydeQYLYZ2MAwZ",
    "name": "alpha",
    "subnetID": "DUSqGPT9RCcowqVMMHzw6fkEH1YwvBgzNJcpzYtfRVfhPEasg",
    "networkID": 1337,
    "vmID": "jvYyfQTxGMJLuGWa55kdP2p2zSUYsQ5Raupu4TW34ZAUBAbtq", "vmName": "avm",
    "fxIDs": ["spdxUxVJQbX85MGxMHbKw1sHxMnSqJ3QBzDyDYEP3h6TLuxqQ"],
    "genesisHash": "sha256:cf110fbbec4c584257def1d3b603fd1eb36a7424fc55a24a105f16196c088b5b",
    "genesisSize": 150,
    "owner": {"threshold": 1, "locktime": 0, "addresses": ["M-custom18jma8ppw3nhx5r4ap8clazz0dps7rv5u9xde7p"]},
    "createdBy": {"chainTxID": "28vcu7Va…", "subnetTxID": "DUSqGPT9…"},
    "createdAt": null,
    "validatorPolicy": {
      "profile": "permissioned_committee", "enforcement": "consensus",
      "params": {"minCommitteeSize": 3, "minConnectedWeight": 0.8},
      "provides": ["registration_security", "consensus_security"]
    },
    "validators": {"currentCount": 3, "pendingCount": 0, "currentWeight": 60, "source": "M-chain"},
    "subnetChainCount": 1,
    "rpcEndpoints": ["/ext/bc/28vcu7VaHmh4XMbagjioa1iWNaS82LfDmRGYuydeQYLYZ2MAwZ"],
    "explorerURL": null,
    "privacyMode": "public",
    "capabilities": ["vm:avm","consensus:dag","state:utxo","assets:fungible","tx:base",
                     "tx:create_asset","tx:operation","api:avm","api:wallet","api:events",
                     "atomic:disabled_across_subnets","fx:secp256k1fx"],
    "registration": {"status": "REGISTERED", "authority": "M-chain", "observedAtMChainHeight": 12},
    "unavailable": {"createdAt": "not_recorded_in_authoritative_state", "explorerURL": "not_configured"}
  },
  "observation": {
    "lifecycle": "ACTIVE", "registered": true, "operational": true, "healthy": true,
    "reasons": [], "trackedByNode": true,
    "observedAt": "2026-10-21T18:27:15.077173Z",
    "observerNode": "NodeID-7Xhw2mDxuDS44j42TCB6U5579esbSt3Lg",
    "source": "local-observation"
  }
}
```

`rpcEndpoints` are paths relative to the answering node and are listed only
if that node serves the chain. For an untracked chain the observation is
`lifecycle: "UNKNOWN"`, `healthy: false`, `reasons: ["node_not_tracking_subnet"]`.

### `arc.validators`

```json
{
  "apiVersion": "v1", "schemaVersion": 1,
  "chainID": "28vcu7Va…", "subnetID": "DUSqGPT9…",
  "validatorPolicy": {…},
  "members": [
    {"nodeID": "NodeID-MFrZFVCXPv5iCn6M9K6XduxGTYp891xXZ",
     "txID": "XwoeHMv8svVrmEHK5hzsBBwnszBDnvP5DuM9voqPULZdpMLCq",
     "weight": "20", "startTime": "2026-10-21T18:26:45Z", "endTime": "2026-10-22T19:26:45Z",
     "membershipStatus": "current", "connected": true, "uptimePercent": 100},
    …
  ],
  "summary": {"currentCount": 3, "pendingCount": 0, "currentWeight": "60",
              "connectedCount": 3, "connectedWeight": "60",
              "connectedWeightFraction": 1, "connectivityObservable": true},
  "reasons": [],
  "membershipSource": "M-chain",
  "connectivitySource": "local-observation",
  "mChainHeight": 12, "observedAt": "…", "observerNode": "NodeID-7Xhw…"
}
```

`membershipStatus` (`current` / `pending`) is authoritative. `connected` and
`uptimePercent` are `null` unless the answering node tracks the subnet.

### `arc.stats`

Local counters since `observedSince` (when this node started running the
chain). Not persisted; reset on node restart.

```json
{
  "apiVersion": "v1", "schemaVersion": 1, "chainID": "28vcu7Va…",
  "source": "local-observation",
  "observedSince": "2026-10-21T18:26:46.020844Z",
  "observedAt": "2026-10-21T18:27:17.290061Z",
  "observerNode": "NodeID-7Xhw…",
  "acceptedTxs": "1", "acceptedContainers": "1",
  "lastAcceptedAt": "2026-10-21T18:27:16.090508Z",
  "lastAcceptedContainerID": "2Hf2LLGNvCPawjF84N2gqYVcxroHmgQLft99pHz48bEfv5fghk",
  "height": null, "createdAt": null,
  "unavailable": {"createdAt": "not_recorded_in_authoritative_state",
                  "height": "dag_chain_has_no_linear_height"}
}
```

On a node that does not track the chain every counter is `null` with reason
`node_not_tracking_subnet`.

### `arc.health`

```json
{
  "apiVersion": "v1", "schemaVersion": 1, "chainID": "28vcu7Va…",
  "observation": {"lifecycle": "DEGRADED", "registered": true, "operational": true,
                  "healthy": false,
                  "reasons": ["connected_committee_weight_below_threshold"], …},
  "engine": {"checked": true, "healthy": true, "checkedAt": "2026-10-21T18:27:13.791195Z"},
  "committee": {"currentCount": 3, "currentWeight": "60", "connectedCount": 2,
                "connectedWeight": "40", "connectedWeightFraction": 0.6666666666666666,
                "connectivityObservable": true, …},
  "params": {"minCommitteeSize": 3, "minConnectedWeight": 0.8},
  "dependencies": {"mChainBootstrapped": true, "observerLastPoll": "2026-10-21T18:27:13.791506Z"}
}
```

A chain that failed to build:
`"lifecycle": "FAILED", "reasons": ["initialisation_failed"], "engine": {"checked": false, "healthy": null, "checkedAt": null}`.
Error details are never returned; they are in the node log.

### `arc.capabilities`

`{"chainID": ""}` describes the service; with a chain ID it adds `chain`.
Returns `methods`, `profiles` (each with `operational`, `provides`,
`description`, `limitations`), `supportedVMs`, `privacyModes`, `lifecycles`,
`stakingEnabled`, and for a chain its `capabilities`, `privacyMode` and
`apis` (only if served locally).

### `arc.getCreationStatus`

Arguments: `subnetTxID` and/or `chainTxID`, optional `validatorTxIDs` (≤ 100).

```json
{"apiVersion": "v1", "schemaVersion": 1, "stage": "SUBNET_REGISTERED",
 "subnet": {"txID": "DUSqGPT9…", "status": "Committed"},
 "chain": null, "validators": [], "observation": null}
```

Stages: `UNKNOWN`, `SUBNET_PENDING_ACCEPTANCE`, `SUBNET_REJECTED`,
`SUBNET_REGISTERED`, `CHAIN_PENDING_ACCEPTANCE`, `CHAIN_REJECTED`,
`REGISTERED` (then `observation` holds the chain's local lifecycle).

### `arc.events`

`{"after": <seq>, "limit": <n>}` →

```json
{
  "apiVersion": "v1", "schemaVersion": 1,
  "bootID": "587405f55d00f8bc", "nextCursor": 10, "oldestSeq": 1, "truncated": false,
  "observerNode": "NodeID-NFBbbJ4qCmNaCzeW7sxErhvWqvEQMnYcN",
  "events": [
    {"schemaVersion": 1, "id": "587405f55d00f8bc-2", "seq": 2, "bootID": "587405f55d00f8bc",
     "type": "initialisation_started",
     "chainID": "28vcu7Va…", "subnetID": "DUSqGPT9…",
     "from": "UNKNOWN", "to": "INITIALISING", "reasons": ["bootstrapping"],
     "observedAt": "2026-10-21T18:26:48.836616Z",
     "observerNode": "NodeID-NFBb…", "source": "local-observation", "authoritative": false},
    …
  ]
}
```

Delivery semantics: see
[02-events-observability-operations.md §2](02-events-observability-operations.md).

## 3. Explorer integration plan

The integration in the main explorer is owned by the explorer team. This
section is the plan. A standalone reference view that implements it (overview
and detail pages, no build step) lives next to this repository in
`arc-explorer/`; it can be embedded in, or ported into, the main explorer.
Develop against a live local network with `TestDevNetwork` (see
[02-events-observability-operations.md](02-events-observability-operations.md) §6
and `tests/arc/devnet_test.go`).

### 3.1 Data sources

```
                    ┌──────────────────────────────┐
  any bootstrapped  │ arc.list / get /        │  authoritative registry
  node ───────────► │ validators (membership)       │  (same on every node)
                    └──────────────────────────────┘
                    ┌──────────────────────────────┐
  observer node(s)  │ arc.health / stats /    │  local observations
  that track the ─► │ validators (connectivity) /   │  (per observer)
  subnet            │ events                        │
                    └──────────────────────────────┘
                    ┌──────────────────────────────┐
  chain endpoint    │ /ext/bc/<chainID> (avm.*),    │  blocks/txs, only if the
  /ext/index ─────► │ /ext/index/<chainID>/…        │  explorer indexes them
                    └──────────────────────────────┘
```

Recommended deployment: one or more dedicated **observer nodes** that
whitelist the subnets of every public Arc. Tracking a subnet does not
make a node a committee member (verified in e2e phase 3), so observers add no
security risk, and their observations are labelled with `observerNode`.

### 3.2 Overview page

| UI element | Source | Rule |
| --- | --- | --- |
| Registered Arcs | `counts.registered` | label "registered on M-chain" |
| Operational / healthy | `counts.operational`, `counts.healthy` from an observer | show observer and `observedAt`; never derive from `registered` |
| Active / initialising / degraded / failed | `counts.byLifecycle` | `UNKNOWN` = "not observed" |
| Name, ID | `descriptor.name`, `descriptor.id` | names are **not unique**: always show the ID |
| Created | `descriptor.createdAt` (null) | show "—" (not recorded); optionally the block time of `createdBy.chainTxID` from an M-chain indexer, labelled as such |
| Owner | `descriptor.owner.addresses` | public M-chain data; apply the explorer's own privacy policy |
| VM | `descriptor.vmName` | |
| Validators / profile | `descriptor.validators.currentCount`, `validatorPolicy.profile` + `enforcement` | show `enforcement` whenever ≠ `consensus` |
| Activity | `arc.stats.lastAcceptedAt`, `acceptedTxs` | "since `observedSince`", per observer |
| Health | `observation.lifecycle` + `reasons` | grey/unknown when unavailable or stale |
| Links | `descriptor.rpcEndpoints` prefixed by the observer's public URL | only if listed |

Paging: follow `nextCursor` until empty; cache the full list for ≥ 10 s.

### 3.3 Detail page

- Metadata: full descriptor, including genesis hash and size, owner
  threshold, `createdBy` tx links to the M-chain explorer.
- Security panel: `validatorPolicy.provides` and the profile's
  `limitations` from `arc.capabilities` — display them verbatim.
  Never label an Arc "secured by the primary network".
- Committee: `arc.validators` — membership (authoritative) in one
  column, connectivity/uptime (observed, per observer) in another.
- Blocks/transactions: via the chain's `avm.*` API or `/ext/index/<chainID>/`
  on a node that runs the chain with `--index-enabled`. Label them "as
  indexed by <node>". DAG chains have no linear height: show vertices/txs, not
  a height.
- Health timeline: from `arc.events` (if the explorer stores them).

### 3.4 Freshness and failure handling

- Treat an observation as **stale** if `now − observedAt > 30 s` or
  `dependencies.observerLastPoll` is older than 30 s → render "unknown".
- If a node or endpoint is unreachable, times out, or returns an error,
  render that chain or panel as "unavailable" and keep the rest of the page.
  Never fall back to "healthy".
- Treat unknown lifecycle values and reason codes as "unknown".
- If observers disagree, show each observer's view or the worst one; do not
  average.

### 3.5 Index recovery

The explorer's Arc index is a cache: it can be dropped and rebuilt at
any time from `arc.list` (+ `validators`). For event-driven updates,
follow §2 delivery semantics and resynchronise from `arc.list` on
`truncated` or a new `bootID`. The explorer must never become a source of
truth for registration, ownership or membership.

### 3.6 Privacy

The API exposes only public M-chain data (owner addresses, tx IDs, genesis
hash/size, node IDs) and local health. It never returns genesis bytes, keys,
error details or anything from Arc application state. Future
private-assistant data must stay off-chain or encrypted and must not be added
to descriptors or explorer records.

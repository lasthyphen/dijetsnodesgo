# Arcs — Design

Documents in this directory:

| # | Document | Milestones |
| --- | --- | --- |
| 00 | [Architecture report](00-architecture-report.md) | M0 |
| 01 | Design (this document) | M1–M4 |
| 02 | [Events, observability and operations](02-events-observability-operations.md) | M5 |
| 03 | [API contract and explorer integration plan](03-api-and-explorer-contract.md) | M5–M6 |
| 04 | [Security review](04-security-review.md) | M7 |
| 05 | [Implementation report](05-implementation-report.md) | M7 |

**Naming.** The product name is **Arc** (plural **Arcs**): chains that people
own and manage to track portfolio-related records, submit authorised
transactions and enforce spending or transfer policies. The original RFC
calls them "mini-chains"; that was a placeholder and means Arc throughout.

## 1. What an Arc is

An Arc is a blockchain created by an M-chain `CreateChainTx` on a
**dedicated, permissioned subnet** (`CreateSubnetTx`, not transformed), running
the **AVM on the DAG consensus engine** (the V-chain's engine), with a
committee of primary-network validators assigned by `AddSubnetValidatorTx`.

Nothing in the primary network's consensus, transaction formats or execution
rules was changed. No fourth native chain was added.

| Concern | Mechanism | Authority |
| --- | --- | --- |
| Identity | chain ID = `CreateChainTx` ID (hash of the signed tx) | M-chain consensus |
| Ownership | `CreateSubnetTx.Owner` (secp256k1 threshold multisig) | M-chain consensus |
| Genesis | `CreateChainTx.GenesisData`; descriptor exposes `sha256` | M-chain consensus |
| Committee | current subnet validators (`AddSubnetValidatorTx`/`RemoveSubnetValidatorTx`) | M-chain consensus |
| Execution & state | AVM instance, own DB prefix and data dir | Arc committee consensus |
| Lifecycle / health | derived per node | local observation |

## 2. Descriptor (schema version 1)

Implemented in `arc/descriptor.go` (`Descriptor`, `BuildDescriptor`).

| RFC field | Descriptor field | Source |
| --- | --- | --- |
| `id` | `id` | chain ID |
| `name` | `name` | `CreateChainTx.ChainName` (not unique) |
| `owner` | `owner{threshold,locktime,addresses}` | subnet owner, addresses only |
| `status` | `observation.lifecycle` (in `ChainView`) | local observation |
| `vm_id` | `vmID`, `vmName` | `CreateChainTx.VMID` |
| `genesis_hash` | `genesisHash` (`sha256:<hex>`), `genesisSize` | genesis bytes |
| `created_at` | `createdAt` = `null`, `unavailable.createdAt` | **not recorded** in M-chain state (no tx→block-time index) |
| `created_by` | `createdBy{chainTxID, subnetTxID}` | tx IDs |
| `validator_policy` | `validatorPolicy{profile, enforcement, params, provides}` | derived |
| `validators` | `validators{currentCount,pendingCount,currentWeight,source}`; full list via `arc.validators` | M-chain |
| `network_id` | `networkID` | node |
| `rpc_endpoints` | `rpcEndpoints` (relative paths, only when this node serves the chain) | local |
| `explorer_url` | `explorerURL` = `null` (not configured) | — |
| `privacy_mode` | `privacyMode` = `"public"` | declared |
| `capabilities` | `capabilities` | derived from VM/fx |
| `protocol_version` | `protocolVersion` = `"arc/1"` | constant |
| `metadata_version` | `metadataVersion` = `1`, plus `schemaVersion` = `1` | constant |

Unavailable data is `null` and listed in `unavailable` with a reason code; it
is never fabricated.

### Schema evolution

- `schemaVersion` changes only on backwards-incompatible changes; additive
  fields keep the version.
- `metadataVersion` changes when the derivation rules change.
- Descriptors are computed on demand from M-chain state, so there is no stored
  metadata to migrate: a new node version re-derives every descriptor.

## 3. Lifecycle

Implemented in `arc/lifecycle.go` (`Derive`, `ValidTransition`).

```
             ┌────────────┐  retry / restart  ┌─────────┐
             │  PENDING   │◄──────────────────┤ FAILED  │
             └─────┬──────┘◄───────┐          └─────────┘
                   │               │               ▲
                   ▼               │ restart       │ init error / handler died
             ┌────────────┐   ┌────┴────┐          │
             │INITIALISING├──►│ STOPPED │◄─┐       │
             └─────┬──────┘   └─────────┘  │       │
                   ▼                       │       │
             ┌────────────┐  ┌──────────┐  │       │
             │   ACTIVE   │◄►│ DEGRADED │  │       │
             └─────┬──────┘  └────┬─────┘  │       │
                   └──────┬───────┘        │       │
                          ▼                │       │
                     ┌──────────┐──────────┘       │
                     │ STOPPING │──────────────────┘
                     └──────────┘
   UNKNOWN: registered, but this node does not run the chain (no observation).
```

Derivation (first match wins):

| Condition | Lifecycle | Reason |
| --- | --- | --- |
| subnet not tracked by this node (staking on) | `UNKNOWN` | `node_not_tracking_subnet` |
| chain manager does not know the chain | `PENDING` | `creation_not_yet_staged` |
| `buildChain` failed | `FAILED` | `initialisation_failed` |
| staged, not built | `PENDING` | `queued_for_creation` |
| handler stopped, node not shutting down | `FAILED` | `chain_handler_stopped_unexpectedly` |
| node shutting down | `STOPPING` / `STOPPED` | `node_shutting_down` |
| state syncing / bootstrapping | `INITIALISING` | `state_syncing` / `bootstrapping` |
| normal op, any health input unmet | `DEGRADED` | engine/committee reasons |
| normal op, all inputs met | `ACTIVE` | — |

- **Registered**: `CreateChainTx` committed on the M-chain (authoritative).
- **Operational**: `ACTIVE` or `DEGRADED` on the observing node.
- **Healthy**: `ACTIVE` only. Requires a *fresh* engine health sample (≤ 3 poll
  intervals), a current committee of at least `minCommitteeSize`, and observed
  connected committee weight ≥ `minConnectedWeight`. A registered chain is never
  reported healthy without these observations.
- **Persistence**: lifecycle is never persisted. It is recomputed on every
  request from M-chain state plus `chains.Manager.LocalStatus`. Interrupted
  transitions need no recovery: after a restart the M-chain re-queues every
  registered chain of tracked subnets and the derived lifecycle restarts at
  `PENDING`.
- **Externally visible**: every state, via `arc.get/list/health` and as
  local events.
- **Per-chain stop/restart is not implemented**: the chain manager has no
  per-chain stop; stopping a chain locally would not change authoritative
  state. `STOPPING`/`STOPPED` occur during node shutdown.

## 4. Registry and creation workflow

### Registry
The registry **is** M-chain state. `vms/platformvm/arc_registry.go`
is a read-only adapter (`arc.Registry`) that reads committed state under
the M-chain lock. The query view is a pure function of that state, sorted by
chain ID; there is no secondary index to rebuild or to diverge.

### Workflow (`arc/workflow`)
1. Client-side validation: naming policy (`ValidateName`), VM must be AVM,
   genesis parsed exactly as the AVM will parse it (`ValidateAVMGenesis`),
   committee size ≥ `minCommitteeSize`, no duplicate or zero-weight members.
2. Eligibility: each member must be a current primary validator whose primary
   period contains the requested subnet validation period (same rule the
   M-chain enforces).
3. `CreateSubnetTx` (unless `SubnetID` is supplied — resume/retry).
4. `AddSubnetValidatorTx` for members not already current/pending.
5. Wait until all members are **current** subnet validators.
6. Duplicate detection: a chain with the same name on the subnet and the same
   genesis hash → returned as-is (`CreatedChain=false`); same name with a
   different genesis/VM → `ErrConflictingRegistered`.
7. `CreateChainTx`.

Every step is an owner-signed M-chain transaction decided by consensus. No
API handler writes M-chain state. `arc.getCreationStatus` reports the
stage from M-chain tx statuses.

**Not protocol-enforced** (client policy only): name uniqueness, naming
policy, VM-aware genesis validation, minimum committee size. A client that
bypasses the workflow can register a malformed chain; nodes then report it as
`FAILED` (demonstrated in e2e phase 7) without affecting other chains.

## 5. Validator policy and security profiles

Implemented in `arc/policy.go`.

| Profile | Operational | Provides |
| --- | --- | --- |
| `permissioned_committee` | yes | registration security (M-chain), consensus security (committee) |
| `expanded_committee` | no | — (reserved) |
| `shared_primary_security` | no | — (needs a protocol change; the primary network does not validate or verify Arc state) |
| `permissionless_subnet` | no | registration security only; recognised for reporting |

`permissioned_committee`:
- **Eligibility**: current primary-network validator; validation period ⊆
  primary period; duration within `[MinStakeDuration, MaxStakeDuration]`
  (24h–365d on local params).
- **Selection**: explicit, owner-chosen, recorded on the M-chain;
  independently verifiable from M-chain state. No randomness.
- **Activation**: at the `StartTime` of each `AddSubnetValidatorTx`, by M-chain
  timestamp. **Removal**: `RemoveSubnetValidatorTx` (owner-signed) or expiry.
- **Enforcement**: the chain manager builds the chain with
  `Validators.Get(subnetID)` as both sampling set and bootstrap beacons. Only
  committee weight is sampled. Shown end-to-end: with one of three members
  down, a non-member tracking the subnet does not restore liveness.
- **Quorum and faults**: DAG Snowball with node-local consensus parameters
  (defaults `K=20, α=15, β_virtuous=15, β_rogue=20`). `K` is capped at the
  committee weight; votes are sampled by weight with replacement. Liveness
  needs roughly ≥ α/K (75 %) of committee weight responsive; bootstrapping needs
  > 50 % of beacon weight and startup waits for ~75 % connected. Safety is
  probabilistic and assumes Byzantine committee weight well below `(K−α)/K`.
- **Minimum committee size**: reporting policy, default 3
  (`DefaultMinCommitteeSize`); below it the chain is `DEGRADED`.
- **Liveness reporting**: `uptime.Manager.IsConnected(node, subnet)` – only
  observable on nodes that track the subnet; otherwise `null`.
- **Staking disabled**: every peer validates every chain; enforcement reported
  as `not_enforced_staking_disabled` and the chain is never `ACTIVE`.

### Finality
Primary-network finality covers registration and committee membership only.
Arc finality is decided solely by its committee; the M-chain neither
checkpoints nor verifies Arc state.

## 6. Observation, events and statistics (M5)

`arc/observer.go` is registered as a chain-manager registrant and as
decision/consensus acceptor. For every Arc this node runs it keeps
acceptance counters and samples engine health every 5 s; the poll loop
re-derives lifecycles and appends **observed** events to a bounded buffer.
Nothing is persisted; nothing is authoritative. Details, metrics and the
operator runbook: [02-events-observability-operations.md](02-events-observability-operations.md).
API contract: [03-api-and-explorer-contract.md](03-api-and-explorer-contract.md).

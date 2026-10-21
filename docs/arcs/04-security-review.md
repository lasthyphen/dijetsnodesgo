# Arcs — Security Review (prototype, schema v1)

Scope: everything added on branch `feature/arcs`: the `arc`
package, the M-chain registry adapter, the additive chain-manager status
accessor, the observer wiring in `node/node.go`, and the client-side workflow.
The review assumes the unmodified AvalancheGo v1.9.4-derived consensus,
networking and M-chain transaction rules of this fork.

## 1. Summary

| Area | Verdict |
| --- | --- |
| Registry authority | Sound: registration, ownership and membership are M-chain transactions verified by M-chain consensus. No API writes state. |
| Chain ID / genesis integrity | Sound: chain ID = hash of the signed `CreateChainTx`; genesis is immutable M-chain state; descriptor exposes its SHA-256. |
| Consensus security | **Committee-bounded**. An Arc is exactly as secure as its committee. |
| Primary-network security | **Not provided** for Arc state. Only registration and membership have primary-network finality. |
| API exposure | Read-only, public data only, bounded resources. |
| Database isolation | Per-chain prefix DB; no atomic shared memory across subnets. |
| Health reporting | Conservative: never healthy without fresh positive observations. |

## 2. Threats and controls

### 2.1 Registry authority and transaction authentication

- `CreateSubnetTx`, `CreateChainTx`, `AddSubnetValidatorTx`,
  `RemoveSubnetValidatorTx` are verified by the existing M-chain executor,
  including subnet-owner signature checks
  (`vms/platformvm/txs/executor/subnet_tx_verification.go`). Nothing in that
  path was modified.
- The `arc.*` service has no mutating method. A request to
  `arc.create` returns a JSON-RPC "method not found" error (e2e phase 1).
- A funded payer that signs the subnet authorisation with a wrong key is
  rejected by the M-chain (e2e phase 5, forged `CreateChainTx`).
- **Not protected**: compromise of the owner's key(s) gives full control of
  membership (add/remove committee members) and the ability to register
  further chains on the subnet. Mitigation: use a threshold multisig owner.

### 2.2 Chain ID and genesis integrity

- Chain ID = ID of the signed `CreateChainTx`: unique, deterministic,
  independently verifiable.
- Genesis bytes are immutable M-chain state; every node builds the chain from
  the same bytes. `genesisHash` lets clients verify what they expect.
- **Not protected**: the M-chain does not validate genesis semantics (only
  size). A malformed genesis registers successfully and fails on every node
  (`FAILED`, isolated — e2e phase 7). The workflow validates genesis
  client-side.
- **Not protected**: names are not unique at the protocol level. Clients
  must identify chains by ID. The workflow's duplicate detection is a client
  convenience, not a guarantee.

### 2.3 Validator membership and consensus assumptions

- With staking enabled, the chain manager builds each Arc with
  `Validators.Get(subnetID)` as sampling set and bootstrap beacons. Only
  committee members' votes are sampled; a node that tracks the subnet without
  being a member does not restore liveness (e2e phase 6).
- Safety is the DAG Snowball guarantee **within the committee**:
  probabilistic, assuming Byzantine committee weight well below `(K−α)/K`
  of the committee (≈ 25 % with defaults `K=20, α=15`).
- Liveness requires ≈ α/K (75 %) of committee weight responsive; with a
  3 × 20 committee, one member down stalls finality (e2e phase 6: stalled
  tx accepted after the member returned).
- **Staking disabled**: every peer is treated as a validator of every chain.
  The committee is not enforced; the API reports
  `enforcement: not_enforced_staking_disabled` and never `ACTIVE`.
- Membership changes take effect at the `StartTime`/`EndTime` of the
  M-chain staker transactions (M-chain timestamp), not at an Arc
  height. There is no Arc-level epoch.

### 2.4 Committee capture and collusion

- A colluding committee holding enough weight can finalise any state,
  including conflicting histories, censor transactions, or halt the chain.
  The primary network does not detect or punish this: subnet validators are
  not slashed and the M-chain does not verify Arc state.
- Small committees (3 members) are easily captured. Recommendation for
  production: ≥ 5 independent operators, with no operator controlling ≥ 20 %
  of committee weight.
- Committee members must be primary-network validators, which bounds Sybil
  creation of members by the primary-network stake requirement, but the
  **owner** chooses members, so the owner can choose colluding nodes.

### 2.5 Network partition and liveness failure

- A partition separating > 25 % of committee weight halts finality on both
  sides (no safety violation under the threshold assumptions).
- After recovery, the stalled transactions are finalised (e2e phase 6).
- Committee members that do not whitelist the subnet count as offline.
- Primary-network chains are unaffected by Arc outages (e2e
  regression checks during the outage and after the failed chain).

### 2.6 Administrative RPC exposure

- No admin methods were added. Existing `admin.*` is unchanged.
- `arc.*` errors return reason codes only; internal errors and chain
  build errors are logged, not returned.
- Recommendation: as for all node APIs, do not expose port 9650 publicly
  without a reverse proxy enforcing rate limits.

### 2.7 Resource exhaustion and chain-creation abuse

- Creating a subnet/chain costs M-chain fees. Registrations by others only
  consume execution resources on nodes that whitelist the subnet.
- API: 16 concurrent requests, 10 s timeout, page limits, bounded event
  buffer; per-request cost of `list` is linear in registered chains and holds
  the M-chain lock only while reading.
- Metrics have bounded label cardinality (no chain IDs or names as labels).
- **Not protected**: a node operator who whitelists many subnets runs every
  chain on them; the chain manager has no per-chain CPU/memory/disk quotas.
  Arc DBs share the node database; disk exhaustion by one chain
  affects the node.

### 2.8 Private metadata exposure

- The descriptor contains only public M-chain data. Genesis bytes are not
  returned (hash and size only). `privacyMode` is the declared value
  `public`; there is no confidential mode.
- AVM Arc state (UTXOs, transactions) is public to every node that
  runs the chain. `validatorOnly` subnet config restricts gossip to
  committee members but is **not encryption**; committee members see
  everything.
- **Not protected**: anything a user puts in genesis memos, chain names or
  transactions is public. Personal AI data must never be written in clear
  text to an Arc.

### 2.9 Database isolation

- Each chain: own `prefixdb` keyspace, own data directory. Verified that
  balances on one chain are absent on the other (e2e phase 4).
- Cross-chain atomic operations are rejected across subnets
  (`verify.SameSubnet`), so Arc assets cannot be imported into V/M/U.
- **Not protected**: a bug in the shared database layer would affect all
  chains; storage is logically, not physically, isolated.
- Fixed during review (DD-8): a chain that failed to build kept its whole
  subnet in "bootstrapping", so after a restart one malformed chain stalled
  every other chain on the same subnet. Only the subnet owner can register a
  chain on a subnet, so this was self-inflicted rather than a third-party
  attack, but it broke failure isolation. Covered by
  `TestFailedChainDoesNotBlockSubnetBootstrapping` and e2e phase 8 (gamma
  shares beta's subnet).

### 2.10 Upgrade and protocol-version compatibility

- No consensus, codec or transaction-format change was made, so there is no
  network upgrade and no activation height. Nodes without this branch
  validate the same M-chain transactions and can run Arcs; they only
  lack the `arc.*` API.
- `protocolVersion`, `schemaVersion`, `metadataVersion` are reported; the
  descriptor is recomputed from M-chain state, so there is nothing to migrate.

### 2.11 False or stale health indicators

- `registered` never implies `healthy`. `ACTIVE` requires: normal operation,
  a fresh engine health sample (≤ 3 poll intervals), current committee ≥
  `minCommitteeSize`, observed connected weight ≥ 80 %, and enforcement by
  consensus.
- Untracked chains are `UNKNOWN`, not healthy.
- Every observation carries `observerNode` and `observedAt`. A malicious or
  misconfigured node can still report false observations about itself; the
  explorer should use trusted observer nodes and treat stale data as unknown.

### 2.12 Anchoring vs consensus security

- There is **no anchoring/checkpointing** of Arc state to the M-chain.
- Even with checkpointing, a checkpoint would only make committee-signed
  history *detectable* (and prevent silent rewrites after the checkpoint); it
  would not make Arc state transitions verified by primary-network
  validators. `shared_primary_security` is therefore declared non-operational.

## 3. What the system protects against

- Unauthorised registration of chains on another owner's subnet.
- Unauthorised changes to committee membership.
- Non-members influencing Arc consensus (staking enabled).
- Arc failures (bad genesis, outage, quorum loss) affecting V, M, U or
  other Arcs.
- Leakage of Arc state into other chains via shared memory.
- The API being used to alter authoritative state.
- Explorer/consumers mistaking registration for health.

## 4. What it does not protect against

- A colluding or compromised committee (safety and liveness).
- Compromise of the subnet owner's key.
- Confidentiality of Arc data (none is provided).
- Resource exhaustion on nodes that opt in to many chains.
- Malformed chains registered by bypassing the client-side workflow
  (detected and isolated, not prevented).
- Networks running with staking disabled (no committee enforcement).

## 5. Smallest defensible next steps

1. Owner multisig and documented committee-size/independence policy.
2. Per-chain resource limits in the chain manager (bounded queue and
   per-subnet opt-in caps).
3. Optional M-chain checkpoint transaction (committee-signed state root) —
   a protocol change requiring a network upgrade; it would add
   *checkpointing*, not consensus security.
4. Encrypted application state and off-chain private memory for the
   personal-AI extension (never consensus-visible in clear text).

# Arcs — M0 Architecture Report

Status: M0 (reconnaissance + design decisions). This document describes what the
repository **actually** contains (verified by reading the code on branch
`feature/arcs`), and the extension strategy chosen as a result.

## A. Repository and version

| Item | Finding |
| --- | --- |
| Module | `github.com/lasthyphen/dijetsnodesgo` (single Go module, `go 1.18` in `go.mod`; built locally with go1.20.5) |
| Upstream lineage | AvalancheGo **v1.9.4** (Banff era). Evidence: `RELEASES.md` head is v1.9.4, `version.RPCChainVMProtocol = 20`, `CurrentDatabase = v1.4.5`, presence of `TransformSubnetTx`, `AddPermissionlessValidatorTx`, `RemoveSubnetValidatorTx`, BLS keys in `validators.Validator`. Node version string is fork-specific (`0.1.3`). |
| Fork renames | X→**V** (`avm`), P→**M** (`platform`), C→**U** (`evm`, coreth fork `github.com/lasthyphen/coreth v0.23.0`). Avalanche consensus package renamed `snow/consensus/dijets`, engine `snow/engine/dijets`. Asset `AVAX`→`DJTX`, Fuji→`lothar`. See `genesis/aliases.go`, `utils/constants/network_ids.go`. |
| Build | `go build ./...` or `./scripts/build.sh` (binary → `build/dijetsnodesgo`, plugins → `build/plugins`). |
| Tests | `go test ./...`; e2e (`scripts/tests.e2e.sh`) depends on an external avalanche-network-runner download and is not usable offline. |
| Lint | `scripts/lint.sh` (golangci-lint). |

### Pre-existing baseline test failures (before any Arc change)

These fail on the untouched baseline and are caused by fixture strings that
were not updated during the X/P→V/M rename and genesis edits. They are **not**
caused by Arc work and are tracked as baseline:

- `vms/platformvm`: `TestExportKey`, `TestImportKey`, `TestGetBalance`, `TestGetStake` (expect `P-` prefix).
- `vms/avm`: `TestServiceGetTxJSON_*` (9 tests), `TestVMFormat` (expect `X-` prefix / old IDs).
- `genesis`: `TestGenesisFromFile/custom`, `TestGenesisFromFlag/custom`, `TestGenesis/*`, `TestVMGenesis/*`, `TestDJTXAssetID/*` (hard-coded upstream genesis IDs).

All other packages under `chains`, `vms/platformvm/...`, `vms/avm/...`,
`snow/engine/dijets/...`, `snow/consensus/dijets/...`, `snow/networking/...`,
`api/...`, `node`, `indexer` pass.

## B. Chain lifecycle (how a chain becomes a running chain)

```
             (authoritative, consensus)                          (local, per node)
 M-chain block accepted ──► CreateChainTx executor
  vms/platformvm/txs/executor/standard_tx_executor.go
        │  state.AddChain(tx)  (persisted in M-chain state DB)
        ▼
 config.Config.CreateChain(chainID, tx)          vms/platformvm/config/config.go
        │  skipped unless staking disabled OR subnet ∈ --whitelisted-subnets
        ▼
 chains.Manager.QueueChainCreation(params)       chains/manager.go
        │  (queue drained only after the M-chain has bootstrapped)
        ▼
 manager.createChain → buildChain
        ├─ chain data dir  <chain-data-dir>/<chainID>
        ├─ DB: prefixdb(chainID) over the node DB (isolated key space)
        ├─ VM factory from vms.Manager (VMID)  ─ avm / evm / plugins
        ├─ validators = Validators.Get(subnetID)   (staking on)
        │             = primary set               (staking off!)
        ├─ DAGVM → createDijetsChain (Avalanche/DAG engine)
        │  ChainVM → createSnowmanChain (+ proposervm)
        ├─ health check registered under chain alias
        ▼
 registrants notified (API server registers /ext/bc/<chainID>, indexer)
 router.AddChain; handler.Start
```

- **Restart / recovery**: on start, `platformvm.VM.initBlockchains` re-reads
  every `CreateChainTx` for the primary network and each whitelisted subnet
  from M-chain state and re-queues them. Chain DBs are reopened by prefix; the
  engine bootstraps from the subnet's validators. There is no separate chain
  registry file.
- **Shutdown**: only node-wide (`chains.Manager.Shutdown` → router shutdown).
  There is **no per-chain stop/restart** in the chain manager.
- **Failure handling**: a non-critical chain that fails `buildChain` is logged
  and a permanently failing health check is registered under its alias; the
  node keeps running. (Baseline defect, fixed by DD-8: the failed chain also
  stayed in its subnet's bootstrapping set forever, blocking its siblings.) V, M and U are "critical chains": their failure shuts
  down the node.
- **RPC**: VM handlers are mounted by `api/server` at `/ext/bc/<chainID>[ext]`
  (aliases `/ext/bc/V`, `/ext/bc/M`, `/ext/bc/U`).
- **Discovery**: `platform.getBlockchains`, `platform.getSubnets`,
  `platform.validates`, `platform.validatedBy`, `platform.getBlockchainStatus`,
  `info.getBlockchainID`, `info.isBootstrapped`.

## C. Consensus and validator architecture

- **V-chain** runs AVM on the DAG engine (`snow/engine/dijets`,
  `snow/consensus/dijets` Topological + snowstorm). Untouched by this work.
- **M-chain** (platformvm, Snowman++) is the authority for validator sets of
  the primary network **and of every subnet** (`validators.Manager` keyed by
  subnet ID, maintained by the M-chain).
- **Subnets are fully present**: `CreateSubnetTx` (owner = secp256k1
  `OutputOwners` threshold multisig), `CreateChainTx`, `AddSubnetValidatorTx`,
  `RemoveSubnetValidatorTx`, `TransformSubnetTx` (permissionless). All are
  verified by M-chain consensus with owner signature checks
  (`txs/executor/subnet_tx_verification.go`).
- **Subnet validator eligibility** (`staker_tx_verification.go`): node must be
  a current/pending primary validator, the subnet validation period must be a
  subset of its primary validation period, duration within
  `[MinStakeDuration, MaxStakeDuration]`, no duplicate, start time after the
  current M-chain time, signed by the subnet owner.
- **Enforcement**: with staking enabled, each subnet chain is created with
  `Validators.Get(subnetID)` as both the sampling set and bootstrap beacons,
  so only committee members' votes count. **With staking disabled every peer
  validates every subnet** — committee membership is *not* enforced.
- **Participation is opt-in per node**: a committee member only runs the chain
  if its operator lists the subnet in `--whitelisted-subnets` (static flag,
  needs restart). A non-participating member still carries weight → liveness
  risk, not a safety bypass.
- **Liveness/health inputs available**: `uptime.Manager.IsConnected(node,
  subnet)` (peers connected *and* tracking the subnet, only for subnets this
  node tracks), handler health checks, `snow.State`.

## D. Explorer and API architecture

- No explorer front-end/back-end is present in this workspace — the explorer
  is an external system. Integration is therefore delivered as an API
  contract (see `03-api-and-explorer-contract.md`).
- Existing APIs: `info.*`, `admin.*` (aliases, profiling, log levels),
  `health.*`, `platform.*`, `avm.*`, `index` (optional, `--index-enabled`,
  ordered accepted containers per chain, persistent), `ipcs`.
- Consensus state lives in each chain's VM DB; the indexer is a derived,
  query-optimised view (rebuildable only from genesis if
  `--index-allow-incomplete=false`).

## Capability matrix (what already exists)

| # | Capability | Supported? | Where |
| --- | --- | --- | --- |
| 1 | Registering additional chains | **Yes**, consensus-enforced | `CreateSubnetTx`, `CreateChainTx` |
| 2 | Creating additional chain instances | **Yes** | `chains.Manager` queue/build |
| 3 | Associating chains with validator sets | **Yes** (staking enabled) | subnet validator sets, `AddSubnetValidatorTx` |
| 4 | Persisting chain metadata | **Yes** (ID, name, VM, fx, genesis, subnet, owner) | M-chain state; no creation timestamp index |
| 5 | Chain discovery via RPC | **Partial** | `platform.getBlockchains` etc.; no pagination, no local lifecycle/health per chain |
| 6 | Running multiple chains concurrently | **Yes** | per-chain handler, DB prefix, data dir |
| 7 | Restart and recovery | **Yes** (node-level) | `initBlockchains`; no per-chain stop/restart |

## Design decisions

- **DD-1 — No new native chain, no new M-chain tx type.** An Arc *is* a
  blockchain created by `CreateChainTx` on a dedicated, permissioned
  (non-transformed) subnet. The M-chain is the authoritative registry.
- **DD-2 — Execution: AVM on the DAG engine** (the same engine as the
  V-chain). No new VM. Each Arc's AVM genesis defines its own fee asset
  (first genesis asset, `vms/avm/vm.go:initGenesis`). Atomic import/export is
  rejected across subnets (`verify.SameSubnet`), so Arc state cannot
  leak into V/M/U through shared memory.
- **DD-3 — Security profile `permissioned_committee`**: committee = current
  subnet validators recorded on the M-chain; enforced by the consensus engine
  through the subnet validator set. Reported as `not_enforced` when staking
  is disabled. Other profiles are declared but marked non-operational.
- **DD-4 — Lifecycle is a local observation** derived on every query from
  authoritative registration + this node's chain manager/engine state. It is
  never written to consensus state, so there is no second source of truth and
  nothing to reconcile after restart.
- **DD-5 — Registry view is computed on demand from M-chain state**, sorted
  by chain ID for stable pagination. No separate index is introduced; the
  "rebuild" of the derived view is a pure function of M-chain state.
- **DD-6 — API**: new JSON-RPC service `arc` mounted at
  `/ext/bc/M/arc` (the M-chain owns the authoritative data), versioned
  (`apiVersion: "v1"`, `schemaVersion: 1`). Read-only. Creation goes through
  existing, signed M-chain transactions (wallet / `platform.*`); a client-side
  workflow helper adds validation and duplicate detection.
- **DD-7 — Minimal chain-manager change**: record queued/failed chains
  and expose a read-only local status snapshot. No engine/consensus code is
  modified.
- **DD-8 — Release failed chains from subnet bootstrapping** (added in M7
  after the restart test exposed it): the inherited chain manager adds every
  chain to its subnet's bootstrapping set before building it, and never
  removes a chain whose build failed. Because the DAG bootstrapper waits until
  *all* chains of its subnet have bootstrapped (`bootstrapper.go`,
  `Subnet.IsBootstrapped`), one malformed chain permanently stalled every
  sibling chain on that subnet after a node restart. Fix: on a non-critical
  build failure, `createChain` calls `sb.Bootstrapped(chainID)`. Local node
  behaviour only; no protocol, consensus or message change. Critical chains
  (V, M, U) are unaffected (their failure still shuts the node down).

## Recommended extension points (used)

- `chains/manager.go` — additive `LocalChainStatus(es)` accessors.
- `vms/platformvm/vm.go:CreateHandlers` — mount `/arc`.
- `vms/platformvm/config/config.go` — optional `ArcObserver`.
- `node/node.go` — construct observer, register as chain registrant.
- New package `arc/` — descriptor, lifecycle, policy, service, observer,
  workflow.

## Implementation risks

1. Chain-manager lock ordering: the M-chain API holds the M-chain lock; other
   chains take the M-chain lock through `validators.State`. The Arc API
   therefore never holds the M-chain lock while calling into other chains.
2. Staking-disabled networks silently remove committee enforcement → must be
   surfaced in every descriptor.
3. Committee members that do not whitelist the subnet → liveness loss.
4. Malformed genesis is not validated by the M-chain (only size) → chain
   fails at initialisation on every node; must be caught client-side and
   reported as `FAILED` locally.
5. Chain names are not unique at the protocol level.
6. `--whitelisted-subnets` is static; tracking a new Arc needs a node
   restart.
7. The shipped `local` genesis has validator periods that expired in 2023;
   local multi-node testing requires a custom genesis with a recent
   `startTime` (custom network ID).

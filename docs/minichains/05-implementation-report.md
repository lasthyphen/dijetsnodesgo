# Mini-Chains — Implementation Report (prototype)

Branch: `feature/minichains` (uncommitted working tree on top of `b19f695`).
Baseline for comparison: commit `b19f695` (master), unchanged.

## 1. What is operational

| Capability | Status | Evidence |
| --- | --- | --- |
| Authoritative registration (M-chain) | **Operational** — existing `CreateSubnetTx`/`CreateChainTx` | e2e phases 2–3 |
| Two independent mini-chains, own genesis/ID/state | **Operational** — AVM on the DAG engine | e2e phases 3–4 |
| Committee assignment enforced by consensus | **Operational** (staking enabled) | e2e phase 6 |
| Restart and recovery | **Operational** (node-level) | e2e phase 8 |
| Versioned read-only API `minichain.*` (8 methods) | **Operational** | unit + e2e |
| Truthful lifecycle/health, explicit unavailable data | **Operational** | unit + e2e phases 3, 6, 7 |
| Observed event feed | **Operational** (in-memory, per node) | unit + e2e phases 7–8 |
| Metrics, structured logs | **Operational** | unit |
| Client-side creation workflow (validation, idempotent retry, duplicate/conflict detection) | **Operational** | unit + e2e phases 3, 5 |
| Explorer integration | **Contract and plan only** — explorer is outside this repository (owned by the explorer team) | `03-api-and-explorer-contract.md` |

Not implemented (by design or blocked; see §5): administrative
`create/stop/restart/updateMetadata` RPCs, per-chain stop/restart, creation
timestamps, checkpointing, `expanded_committee` / `shared_primary_security`
profiles, persistent events.

## 2. Changed files

### Modified (no consensus, codec, tx-format or engine change)

| File | Change |
| --- | --- |
| `chains/manager.go` | `LocalStatus(chainID)` accessor; records staged and failed chains; **bug fix (DD-8)**: a chain that fails to build is released from its subnet's bootstrapping set, so sibling chains are not stalled after restart |
| `chains/mock_manager.go` | mock for `LocalStatus` |
| `node/node.go` | construct `minichain.Observer`, register it as chain registrant, mark shutdown |
| `vms/platformvm/config/config.go` | optional `MiniChainObserver` |
| `vms/platformvm/vm.go` | mount `/minichain` handler on the M-chain (`NoLock`; the registry adapter takes the M-chain context lock itself, only while reading state) |
| `build/dijetsnodesgo` | rebuilt binary (tracked in git upstream) |

### Added

| Path | Purpose |
| --- | --- |
| `minichain/descriptor.go` | `Descriptor` schema v1, `BuildDescriptor`, genesis hash, capabilities, unavailable reasons |
| `minichain/lifecycle.go` | lifecycle states, reasons, `Derive`, `ValidTransition` |
| `minichain/policy.go` | security profiles, `PolicyParams`, `EvaluateCommittee` |
| `minichain/service.go`, `handler.go`, `client.go` | JSON-RPC service, handler, Go client |
| `minichain/observer.go`, `events.go` | local observations, stats, health sampling, event ring buffer, metrics |
| `minichain/workflow/workflow.go` | client-side creation workflow and AVM genesis helpers |
| `vms/platformvm/minichain_registry.go` | read-only registry adapter over M-chain state |
| `*_test.go` (9 files) | unit tests (see §4) |
| `tests/minichain/` | 5-node end-to-end test (build tag `minichain_e2e`), 8 phases |
| `docs/minichains/00–05` | architecture, design, operations, API contract, security review, this report |

## 3. Configuration

No new flags. Relevant existing configuration:

- `--staking-enabled=true` (default) — required for committee enforcement.
- `--whitelisted-subnets=<subnetID,…>` — opt in to run a mini-chain (restart required).
- `<subnet-config-dir>/<subnetID>.json` — optional `validatorOnly`, `consensusParameters`.
- `--index-enabled` — optional, for explorer block/tx indexing of mini-chains.

The API is always mounted at `/ext/bc/M/minichain` (read-only).

## 4. Tests

### Commands

```sh
go build ./...
go test ./minichain/... ./chains/... ./node/... ./snow/engine/dijets/...
golangci-lint run ./minichain/... ./chains/ ./tests/minichain/... --build-tags minichain_e2e   # v1.49.0, repo defaults
./scripts/build_dijets.sh
go test -tags minichain_e2e -timeout 60m -v ./tests/minichain/        # 5-node local network, ~7 min
```

The e2e test needs ≥ 1 GB free disk: the node shuts itself down below 512 MB
(`--system-tracker-disk-required-available-space`).

### Unit tests (all pass)

| Package | Covers |
| --- | --- |
| `minichain` | descriptor build/serialisation and genesis hash; lifecycle derivation and the transition table; policy/profile evaluation; service request validation, error codes over HTTP, pagination and filters; observer health sampling (no check before normal op, panic containment, busy chains), shutdown, initialisation metrics; event ring buffer, truncation |
| `minichain/workflow` | request validation (name, committee size, duplicates, zero weight, periods, owner), AVM genesis build/validation |
| `vms/platformvm` (`minichain_registry_test.go`) | registry adapter over real M-chain state: primary chains rejected, subnet chains listed with owner/committee |
| `chains` | `LocalStatus`; failed chain does not block subnet bootstrapping (fails without the DD-8 fix) |

### Regression (baseline `b19f695` vs branch, same packages)

`chains/...`, `vms/platformvm/...`, `vms/avm/...`, `snow/engine/dijets/...`,
`snow/consensus/dijets/...`, `snow/networking/...`, `api/...`, `node/...`,
`indexer/...`, `genesis/...`, `vms/components/...`: 48 packages pass on both.
The **same failures** occur on the baseline and on the branch (no new
failures, none fixed): 19 top-level tests (34 entries including subtests),
in `vms/platformvm` (4), `vms/avm` (10) and `genesis` (5): fixture strings and genesis IDs not updated during the fork's
X/P→V/M rename. They are unrelated to mini-chains.

### End-to-end (`tests/minichain`, real 5-node staking network)

**Result: PASS** (2026-10-21, all 8 phases, 117 s), with the branch binary
built from this tree.

Earlier runs during this milestone failed, and those failures led to fixes:

- Phase 8 (restart): beta stayed `INITIALISING` because the failed chain
  gamma on the same subnet blocked subnet bootstrapping. This was a real
  defect, fixed by DD-8 in `chains/manager.go`.
- Phase 8: the descriptor comparison used a phase-3 snapshot, but beta's
  `subnetChainCount` legitimately changed when gamma was registered. This
  was a test bug: the snapshot is now taken just before the restart.
- Phase 7: node3's `failed` event was asserted before its observer had
  polled. This was a test race: the test now waits up to 1 min.
- One run aborted because the host disk dropped below the node's 512 MB
  safety threshold while other builds ran concurrently. This was an
  environment problem, not a code problem.

| Phase | Demonstrates |
| --- | --- |
| 1 | V transfer accepted (DAG), M height, U `eth_blockNumber`; empty registry; no `minichain.create` method |
| 2 | subnets registered by M-chain tx; `getCreationStatus` = `SUBNET_REGISTERED` |
| 3 | two mini-chains, committees {1,2,3} and {3,4,5}, `ACTIVE` on members; untracked chain reported `UNKNOWN`, not healthy; counts; non-member tracker is not a member |
| 4 | distinct genesis → distinct assets; independent transfers; no state leakage; stats |
| 5 | idempotent retry; conflicting genesis rejected; forged owner signature rejected by M-chain |
| 6 | member down → `DEGRADED`, alpha tx stalls (non-member cannot help); beta, V, M, U unaffected; recovery finalises the stalled tx |
| 7 | malformed genesis registers but is `FAILED` and isolated; events observed, all non-authoritative |
| 8 | full restart: alpha, beta `ACTIVE` again (beta shares its subnet with failed gamma), balances persisted, descriptors identical, new `bootID`; regression checks |

## 5. Remaining gaps and proposed next steps

| Gap | Why | Smallest next step |
| --- | --- | --- |
| No admin RPCs (`create`, `stop`, `restart`, `updateMetadata`) | Creation must go through owner-signed M-chain txs; an RPC holding owner keys would be a custodial signer. Chain manager has no per-chain stop; descriptors have no mutable metadata. | Keep creation client-side (workflow). Per-chain stop would need a chain-manager change and is local-only. |
| `createdAt` unavailable | M-chain state has no tx → block-time index | Read it from an M-chain block indexer (`/ext/index/M/block`) in the explorer, labelled as indexer data |
| DAG mini-chains have no height | AVM/DAG has no linear height | Report vertex counts (already: `acceptedContainers`) |
| Events not persistent | in-memory by design; authoritative history is the M-chain | Explorer consumes M-chain blocks for durable history |
| Name uniqueness, genesis validation, min committee size not protocol-enforced | would need M-chain rule changes (network upgrade) | Keep client-side; malformed chains are isolated as `FAILED` |
| Whitelisting is static | existing node behaviour | Dynamic subnet tracking would be a node change |
| No per-chain resource quotas / disk usage | shared node DB, sequential chain builder | Chain-manager quotas; per-prefix size estimates |
| Interrupted-creation tests mid-transaction, DB-open failure, restart during INITIALISING | not covered by automated tests | Add fault-injection tests in `chains` (DB factory error) and e2e restart during bootstrap |
| Checkpointing / `shared_primary_security` | needs new M-chain tx type (protocol change) | Specify a committee-signed checkpoint tx; it adds detectability, not consensus security |
| Explorer UI | outside this repository | Implement per `03-api-and-explorer-contract.md` §3 |

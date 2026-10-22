# Native Naming, Name-Based Payments and Bob — Implementation Report

Branch `feature/names` (based on `feature/arcs` at `f10ae00`). Demo: `../bob-demo` (its own repository). Nothing has been pushed.

## 1. Milestones

| Milestone | Status | Evidence |
|---|---|---|
| **M0** Architecture report | Done | `00-architecture-report.md`: the fork mapped; six naming architectures compared; Approach A recommended over B on determinism grounds (V-chain vertices carry no M-chain height or timestamp) |
| **M1** Native naming prototype | Done | Two M-chain transaction types, state, mempool claims, the `names.*` API and wallet builders. Executor, state, mempool, codec and service unit tests. e2e phases 1–6, 13 and 14 on 5 nodes |
| **M2** Name-based payments | Done | `txservice` pays `alice` by name through conventional V-chain transactions; accepted on all 5 nodes (e2e phase 7) |
| **M3** Explorer integration | **Partly done** | The attribution logic and data contract (`payments.describeTx`) are implemented and tested, and rendered by the demo's transaction record page. **Main explorer integration is pending**: its source isn't on this machine, and Ortelius needs the new codec types (architecture report §8) |
| **M4** Bob's transaction service | Done | Prepare, approve (digest-bound), submit, receipt; policy, allowance, audit chain, idempotency, rate limits. 19 unit tests (race detector clean); e2e phases 7–15 |
| **M5** End-to-end demonstration | Done, with the rule-based parser | Try me → instruction → intent → approval → V-chain acceptance → receipt → transaction record, driven in headless Chrome against a live 5-node network (§3). **The Claude intent path is implemented but was not run live**: this machine has no Anthropic credentials |
| **M6** Security and regression | Done | `03-security-and-testing.md`; no new test failures compared with the base (§4); Arc e2e 8/8 phases |

## 2. Definition of done

| # | Step | Status |
|---|---|---|
| 1 | Alice has a registered human-readable identity | ✅ `RegisterNameTx`, resolved identically by all nodes |
| 2 | The user instructs Bob to transfer 10 DJT to Alice | ✅ typed, via the demo; voice input wired but not exercised with a microphone |
| 3 | Bob produces a structured intent | ✅ `{action: "transfer", recipient: "alice", amount: "10", asset: "DJT"}`, validated server-side |
| 4 | Alice's identity is resolved through native naming | ✅ `names.resolve` at accepted M-chain state, record pinned |
| 5 | Authorisation and signing requirements are enforced | ✅ agent can't approve; approval bound to the digest; signed by the allowance co-owner key in `txservice` |
| 6 | The V-chain processes it through its DAG architecture | ✅ an ordinary `BaseTx`; accepted on every node |
| 7 | The actual result is retrieved and verified | ✅ the receipt is built from the accepted transaction; the memo claim is checked against registry history |
| 8 | The explorer displays the payment in human-readable form | ⚠️ the demo's transaction record does; **the main explorer does not yet** |
| 9 | The user receives a receipt linking to the transaction | ✅ links to the record page, or to the explorer when `explorerTxURL` is set |

## 3. Demonstration run

The run was driven with `TestDevNetwork` followed by `bob-demo`, through Chrome DevTools in headless Chrome, on 2026-10-22. It covered the landing page, Try me, the approval and receipt for "Transfer 10 DJT to Alice", balance, the simulated civic payment, a name-not-found error, a policy refusal, history and the transaction record. Each screen was captured at 900 px and 390 px with no horizontal overflow.

Two live payments were accepted:
- `2RFa7ca3pJXkxYoRGGYUUeo14dTeYv3qzKzs11Q2L3oCEe3T9B`
- `addyensAyg4TnwnA8fyht3EYw944z5pgTk4zgXp9h9tMQqoiC`

Both receipts showed:

| Field | Value |
|---|---|
| From | `1-of-2 {bob, demo-user}` |
| To | `alice` |
| Amount | 10 DJT |
| Fee | 0.001 DJT |
| Change | 89.999 DJT, back to the allowance, labelled *change* with its evidence |
| Name check | verified |

## 4. Tests executed

| Suite | Result |
|---|---|
| `go test ./vms/platformvm/names` | pass |
| `go test ./vms/platformvm/txs/executor -run Name` (8 tests, 8 subtests) | pass |
| `go test ./vms/platformvm/state -run Names`, `./txs/mempool -run Name`, `./txs -run Naming`, `./blocks -run Name`, `./vms/platformvm -run TestNamesService` | pass |
| `go test -race ./txservice` (19 tests) | pass |
| `go test -tags names_e2e ./tests/names` (16 phases, 157 s) | pass |
| `go test -tags arc_e2e ./tests/arc` (8 phases, 117 s, on the naming binary and the shared launcher) | pass |
| `npm test` in `bob-demo` (9 tests) | pass |
| `golangci-lint` v1.49.0 on all new and changed packages, including the e2e build tags | clean |
| Regression comparison against `f10ae00`, covering chains, platformvm, avm, components, secp256k1fx, the DAG engine, consensus, networking, api, node, indexer, genesis, wallet, version and arc | Every package that passes on the base passes on the branch; 21 top-level tests (36 including subtests) fail **identically on both sides** |

The 21 failures predate this work:
- `genesis` 5;
- `vms/avm` 10, the `TestServiceGetTxJSON_*` fixtures;
- `vms/platformvm` 4;
- `version` 2: the application prefix is still "avalanche".

`txs/executor` failed to link once during the regression, while the disk was full (`errno=28`). Rerun on its own, it passes.

## 5. Architectural decisions

| Decision | Reason |
|---|---|
| Registry in M-chain state, not on the V-chain, an Arc or a contract | A linear order makes uniqueness trivial. The M-chain already has owners, permission checks, fees, versioned state and fork gating. An Arc would give only committee security. A contract is excluded by the RFC |
| Approach A payments (resolve, then a conventional transaction) | V-chain validation has no deterministic view of M-chain state, so name references inside V-chain transactions would resolve inconsistently |
| Target consent and one live name per address | Prevents pointing names at others' addresses and makes reverse attribution unambiguous |
| Compare-and-swap versions; generations and record IDs | Stale updates fail instead of overwriting; reassignment after release is detectable |
| New codec types registered after the Banff blocks | Upstream later used the same layout; existing type IDs are unchanged (`TestNamingTxTypeIDs`) |
| Mempool name and target claims | The block builder doesn't re-verify mempool transactions against each other |
| Activation unscheduled on mainnet and Lothar | Ortelius and old nodes must understand the types first |
| A 1-of-2 on-chain allowance for Bob | The protocol caps exposure and the user can revoke unilaterally. Bob's authority comes from V-chain UTXO ownership, not from any Arc |
| `txservice` as a separate process | The only key holder. The web server and the LLM never see keys or approval |
| Shared `tests/localnet` launcher | The Arc and naming suites share one implementation; they use different ports |

## 6. Changed files (since `f10ae00`)

**Protocol (M-chain)**
- `vms/platformvm/names/` (new): naming rules, record status, parameters, claims, memo format
- `vms/platformvm/txs/register_name_tx.go`, `update_name_tx.go` (new)
- `vms/platformvm/txs/{codec,visitor}.go` and `vms/platformvm/blocks/codec.go`
- `vms/platformvm/txs/executor/name_tx_executor.go` (new), plus stubs in the `atomic`, `proposal` and `mempool_verifier` executors
- `vms/platformvm/txs/mempool/{mempool,issuer,remover}.go`
- `vms/platformvm/state/names.go` (new), plus `state.go`, `diff.go` and three mocks
- `vms/platformvm/metrics/tx_metrics.go`
- `vms/platformvm/config/config.go`, `version/constants.go`, `node/node.go`

**API and wallet**
- `vms/platformvm/names_service.go`, `names_client.go` (new), `vm.go`
- `wallet/chain/p/builder_names.go` (new), plus `builder.go`, `builder_with_options.go`, `signer_visitor.go`, `backend_visitor.go`
- `wallet/subnet/primary/names.go` (new), `wallet.go`

**Transaction service:** `txservice/` (new), including `cmd/txservice`

**Tests:**
- unit tests alongside each package;
- `tests/names/{e2e,devnet}_test.go` (new);
- `tests/localnet/localnet.go`, moved from `tests/arc/network.go`;
- `tests/arc/*_test.go` updated to use it.

**Docs:** `docs/naming/00`–`04`

**Binary:** `build/dijetsnodesgo` rebuilt

**Demo:** `../bob-demo/` (new repository)

## 7. Known limitations

- **Main explorer integration.** Not done; the source is unavailable. Ortelius decodes M-chain blocks with upstream codecs, so it will fail on blocks containing name transactions until its codec is updated. That must happen before naming is activated on any network it indexes.
- **Claude intent engine.** Implemented against the documented SDK and unit-tested with an injected client: request shape, `fallbacks: "default"`, refusal handling, output validation. It has **not been called live**. The demo ran on the rule-based parser, which the UI labels.
- **Voice.** Uses the browser's Web Speech API. Present in Chrome; not exercised with a real microphone.
- **Attribution is current-state.** V-chain transactions carry no timestamps, so a historical "who held this name at acceptance" cannot be anchored; only memo claims can be verified against history.
- **Registration interfaces.** Registration and updates are available through the Go wallet SDK and `platform.issueTx`. There is no end-user registration UI or CLI yet.
- **Arc anchoring.** Anchoring Bob's audit chain to an Arc is designed (`01-design.md` §6.7), not implemented.
- **Agent key custody.** File-based; production needs an HSM or KMS behind `keychain.Signer`.
- **Service storage.** The payment store is a single JSON file, adequate for a demo but not high volume. History lists payments made through the service, not every transaction of the user's addresses.
- **Demo identity.** The demo uses one shared identity (`demo-user`) for all visitors, bounded by the allowance and limits.
- **Disputes.** No dispute-resolution or administrative override process exists in v1, by design.

## 8. Recommended next steps

1. Port the attribution view to the main explorer and add the naming types to Ortelius's codec (and an M-chain names index), then schedule `NamingTimes` for Lothar.
2. Run the Claude intent engine with credentials and build a small evaluation set of instructions, including injection attempts.
3. Add a registration and management UI (wallet), with name-change notifications for saved contacts.
4. Replace file-based agent custody with KMS or HSM signing; consider WebAuthn for user approval.
5. Anchor `txservice`'s audit-chain head to Bob's Arc periodically.
6. Decide mainnet economics (registration fee and terms) and governance for disputes.

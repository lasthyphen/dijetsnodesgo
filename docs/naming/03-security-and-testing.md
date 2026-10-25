# Native Naming, Payments and Bob — Security Review and Test Matrix

## 1. Assets and trust boundaries

| Component | Trusted for | Holds |
|---|---|---|
| M-chain validators | Registry state, enforced by consensus | — |
| V-chain validators | Payment execution, enforced by DAG consensus | — |
| `txservice` | Policy, approval binding, signing within the allowance | The agent key (allowance co-owner) |
| Bob demo server | Session handling; relays the user's click to the approver credential | Agent and approver bearer tokens |
| Intent engine (Claude or rule-based) | **Nothing.** Its output is validated, policy-checked and user-approved | — |
| Browser and speech recognition | **Nothing.** It produces text only | — |

## 2. Threats and mitigations

| Threat | Mitigation | Evidence |
|---|---|---|
| Name squatting by pointing a name at someone else's address | Every binding needs a signature from the target's key (`TargetAuth`) | `TestRegisterNameRejections/target_did_not_consent`; e2e phase 3 |
| Two names for one address making attribution ambiguous | At most one live name per target | `…/target_already_bound`; e2e phase 13 |
| Duplicate registration | M-chain total order plus a status check; mempool name claims | `…/duplicate_name`; `TestMempoolNameConflicts`; e2e phase 3 |
| Homoglyph or confusable names | v1 names are ASCII `[a-z0-9-]` only; non-ASCII is rejected, never folded | `TestCanonicalise`; `TestRecipientErrors/аlice` |
| Names that look like addresses | Second character `-` forbidden; addresses longer than any name | `TestValidate/v-local1abc` |
| Hijacking a name | Updates need the current owner's signature (k-of-n) | `TestUpdateNameAuthorization`; e2e phase 4 |
| Lost-update races between owners | Compare-and-swap on `Version` | `TestUpdateNameTarget`; e2e phase 5 |
| Silent reassignment after expiry or revocation | Grace period, then a **new generation and record ID**; payments pin the record | `TestNameExpiryAndRelease`; e2e phase 13 |
| Name changed between the user's instruction and execution | Payment pins `(recordID, generation, version, target)`; submission refuses on any change; the signed transaction holds the explicit address | `TestNameChangedBeforeSubmission`; e2e phase 8 |
| Paying a name about to lapse | Wall-clock margin (`expiryMargin`) besides chain-time status | `TestRecipientErrors/soon` |
| An AI agent authorising itself | The agent credential can't approve; approval must quote the digest the user saw; the LLM has no tools or keys | `TestPayByName`; `TestAuthorization`; e2e phase 7; bob-demo `server.test.mjs` |
| Prompt injection widening Bob's reach | Server-side `validateIntent` fixes service recipients and validates amounts, asset and recipient syntax; policy and approval still apply | `intent.test.mjs` |
| Overspending | On-chain cap (the allowance) plus per-payment and 24-hour policy, re-checked at submission | `TestPolicyLimits`; `TestInsufficientFunds`; e2e phases 9 and 10 |
| Double spend by concurrent submissions | Per-delegation lock, UTXO reservation, wait for decision; snowstorm as final authority | `TestConcurrentSpending`; `TestUndecidedPaymentReservesItsInputs`; e2e phases 10 and 11 |
| Replay | Payment IDs are single-use and `submit` is idempotent; V-chain inputs are consumed; `NetworkID` and `BlockchainID` are signed | `TestIdempotencyAndRepeatedSubmission`; e2e phase 11 |
| Tampering with audit history | Hash-chained, append-only log, verified at start-up | `TestAuditTamperDetection` |
| Revoking Bob | `revokeDelegation` stops payments immediately; the user can sweep the 1-of-2 allowance with their own key | `TestRevocation`; e2e phase 15 |
| Abuse of the demo | Per-IP and per-session rate limits, CSRF, `SameSite=Strict` `HttpOnly` cookies, size limits, strict CSP, `textContent`-only rendering | `server.test.mjs` |
| Exposure of private keys | No API accepts or returns keys; the agent key file is 0600; the demo server holds no key | Code review: `txservice/service.go` `LoadOrCreateKey` |
| Misattribution in the explorer view | A name is shown only for an ACTIVE reverse binding; memo claims are verified against history; outputs classified with stated evidence; attribution caveat attached | `TestPayByName` (receipt); e2e phases 7, 8 and 12 |
| Activation breaking indexers and old nodes | Naming active on mainnet only from its new genesis, so no existing node or index has to change; unscheduled on Lothar; new type IDs appended after the Banff blocks | `TestNamingNotActivated`; `TestMainnetNaming`; `TestNamingTxTypeIDs`; `TestBanffBlockWithNameTx` |

### Residual risks

- **Window after submission.** A name changed *after* submission doesn't redirect the payment: the approved address is paid. This is intended (`01-design.md` §4.1).
- **Chain time.** M-chain time advances only with blocks. Status reported on a quiet chain can lag wall-clock expiry; `txservice` adds a wall-clock margin.
- **Agent key custody.** The agent key is a file. Production should use an HSM or KMS behind the `keychain.Signer` interface.
- **Shared demo identity.** Bob demo visitors share one demo identity. Its exposure is bounded by the allowance and the policy limits.
- **Disputes.** There is no administrative override. Recovery relies on k-of-n owners.
- **Voice.** The Web Speech API sends audio to the browser vendor's speech service. The UI says so.

## 3. RFC Phase 7 test matrix

Unit tests: `go test` in the packages named. End-to-end: `go test -tags names_e2e ./tests/names/`, on a real 5-node network.

| Required test | Unit | End-to-end |
|---|---|---|
| Duplicate name registration | `executor.TestRegisterNameRejections/duplicate_name`; `mempool.TestMempoolNameConflicts` | phase 3 |
| Invalid and malformed names | `names.TestValidate`; `names.TestCanonicalise`; `…/non-canonical`, `…/reserved`, `…/malformed` | phase 3 (`Alice`, `al ice`, `admin`, `a`, `v-alice`, `xn--alice`) |
| Unauthorised registration updates | `executor.TestUpdateNameAuthorization` | phase 4 (redirection) |
| Unauthorised ownership transfers | `executor.TestUpdateNameAuthorization` | phase 4 (owner change) |
| Expired or revoked registrations | `names.TestStatusAt`; `executor.TestNameExpiryAndRelease`; `executor.TestRevokeName`; `txservice.TestRecipientErrors` | phases 6, 9, 13 |
| Resolution consistency | `platformvm.TestNamesService` | every resolution checked on all 5 nodes at the same block |
| Name changes between preparation and submission | `txservice.TestNameChangedBeforeSubmission` | phase 8 |
| Invalid recipients | `txservice.TestRecipientErrors` | phase 9 |
| Insufficient balances | `txservice.TestInsufficientFunds` | phase 10 |
| Incorrect asset or amount handling | `txservice.TestAmountAndAssetErrors`; `txservice.TestAmounts`; `intent.test.mjs` | phase 9 (`61` exceeds the limit) |
| Transaction replay | `txservice.TestIdempotencyAndRepeatedSubmission` | phase 11 (re-issuing an accepted transaction pays nothing) |
| Repeated submission | `txservice.TestIdempotencyAndRepeatedSubmission` | phase 14 (idempotency key survives restart) |
| Concurrent spending attempts | `txservice.TestConcurrentSpending`; `txservice.TestUndecidedPaymentReservesItsInputs` | phases 10 and 11 (snowstorm double spend: every node accepts one and rejects the other) |
| Signing and permission failures | `txservice.TestAuthorization`; `TestPayByName` (agent can't approve or submit) | phases 3, 4 and 7 |
| API rate limiting | `txservice.TestRateLimit`; `server.test.mjs` | — |
| Explorer attribution errors | `TestPayByName` (roles, labels, memo claim) | phases 8 and 12 (old address no longer labelled; unnamed shown as address) |
| Restart and recovery of naming state | `state.TestNamesPersistAcrossRestart`; `txservice.TestRestartKeepsPaymentsAndLimits` | phase 14 (all nodes restarted; record and history identical) |
| Backward compatibility with conventional V-chain transactions | `txs.TestNamingTxTypeIDs`; `blocks.TestBanffBlockWithNameTx` | phases 11, 12 and 16 |
| No regression of the V-chain DAG, M-chain, U-chain or Arcs | Regression comparison (`04-implementation-report.md` §4) | phase 16 (M, V and U bootstrapped, U-chain EVM answering on every node); Arc e2e, all 8 phases |

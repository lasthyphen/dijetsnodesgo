# Native Naming, Name-Based Payments and Bob — Design

Companion documents in this directory:

| Document | Contents |
|---|---|
| `00-architecture-report.md` | Phase 0 reconnaissance; the recommendation of an M-chain registry with Approach A payments |
| `01-design.md` | This document (Phases 1–6) |
| `02-api.md` | API reference and examples |
| `03-security-and-testing.md` | Threat model and test matrix (Phase 7) |
| `04-implementation-report.md` | What was built, evidence, limitations and next steps |

## 1. Name format and canonicalisation

A name is a single label of ASCII `[a-z0-9-]`, for example `alice` or `dijets-cafe`.

| Rule | Value | Reason |
|---|---|---|
| Length | 3–32 bytes | Short names are scarce and squattable; an upper bound keeps records small |
| Characters | `a-z`, `0-9`, `-` | No Unicode in v1, so no homoglyphs, mixed scripts, bidi controls or normalisation ambiguity |
| First character | a letter | Avoids names that look like amounts or IDs |
| Last character | a letter or digit | Avoids `alice-` versus `alice` |
| `--` | forbidden | Reserved for a future encoded-Unicode scheme, like IDNA `xn--` |
| Second character `-` | forbidden | `v-…`, `m-…` and `u-…` could be mistaken for chain-prefixed addresses (`V-local1…`) |
| `.` | forbidden | Hierarchical names (`alice.team`) are **reserved for a future version** |
| Reserved | `dijets`, `djt`, `djtx`, `admin`, `administrator`, `root`, `system`, `validator`, `validators`, `treasury`, `foundation`, `official`, `support`, `security`, `wallet`, `explorer`, `bridge`, `null`, `undefined`, `none`, `unknown`, `anonymous`, `everyone`, `self`, `me`, `names`, `registry` | Impersonation of the network or ambiguous meaning in user interfaces |

Canonicalisation is applied by **clients and APIs only**:

1. Trim surrounding whitespace.
2. Strip one leading `@`.
3. Lowercase ASCII.

Any non-ASCII input is rejected with the reason `non_ascii`; it is never transliterated.
**On-chain transactions must carry the canonical form exactly.** Validators
reject non-canonical bytes, so every name has exactly one encoding. The rule
set lives in one package (`vms/platformvm/names`) used by the node, the
wallet and the transaction service, so every component applies identical rules.

## 2. Registry: records and lifecycle

The registry is part of M-chain state. Each record holds:

| Field | Meaning |
|---|---|
| `name` | Canonical name |
| `target` | The 20-byte identity (secp256k1 ShortID) that payments go to, displayed as `V-<hrp>1…` |
| `owner` | `secp256k1fx.OutputOwners`, a k-of-n set that controls updates |
| `expiry` | Unix seconds, M-chain time |
| `revoked` | Tombstone flag |
| `generation` | 1 for the first registration of a name, +1 each time a released name is registered again |
| `version` | 1 at registration, +1 for every accepted update |
| `recordID` | ID of the `RegisterNameTx` that started this generation |
| `lastTxID` | ID of the latest transaction that changed the record |
| `registeredAt` | Chain timestamp |
| `updatedAt` | Chain timestamp |

Every version is retained in a history index (`name ‖ generation ‖ version`).
A reverse index maps `target` to `name`.

### 2.1 Status

Status is derived, never stored, from the record and the M-chain timestamp `T` of the accepted state being read:

| Status | Condition | Resolvable for payment | Who may act |
|---|---|---|---|
| `ACTIVE` | not revoked, `T < expiry` | yes | owner: update |
| `EXPIRED` | not revoked, `expiry ≤ T < expiry + grace` | **no** | owner: renew or update |
| `REVOKED` | revoked, `T < updatedAt + grace` | **no** | nobody |
| `RELEASED` | after the grace period | **no** | anyone: register (new generation) |
| `NOT_FOUND` | no record | no | anyone: register |

The grace period ensures that a lapsed or revoked name cannot be taken over
immediately. When a released name is registered again, `generation` and
`recordID` change, and clients detect this (§5).

### 2.2 Transactions

Both transactions pay fees from M-chain UTXOs, like every other M-chain transaction.

**`RegisterNameTx{BaseTx, Name, Target, Owner, Expiry, TargetAuth}`**

* Valid only after the naming activation time (§2.4).
* `Name` must be canonical and not reserved.
* The name's status must be `NOT_FOUND` or `RELEASED`.
* `Target` must not be the target of any record that is not `RELEASED` (§2.3).
* `T + minTerm ≤ Expiry ≤ T + maxTerm`, where `T` is the block timestamp.
* `TargetAuth` plus the last credential must prove control of `Target`:
  `Fx.VerifyPermission` against `OutputOwners{Threshold: 1, Addrs: [Target]}`.
  **Nobody can point a name at an address without that address's signature.**
* The fee is `RegisterNameTxFee`.

**`UpdateNameTx{BaseTx, Name, Version, Target, Owner, Expiry, Revoke, NameAuth, TargetAuth}`**

This is a full replacement of the record, with compare-and-swap on `Version`.

* `Version` must equal the current version, so a concurrent or stale update fails instead of overwriting.
* The status must be `ACTIVE` or `EXPIRED`. Revoked and released names cannot be updated.
* `NameAuth` plus the second-to-last credential must satisfy the current `Owner`.
* If `Target` changes, `TargetAuth` plus the last credential must prove control of
  the new target, and the new target must be free. If `Target` is
  unchanged, `TargetAuth` must be an empty input and the last credential empty,
  so unverified bytes cannot alter the transaction ID.
* If `Revoke` is set, `Target`, `Owner` and `Expiry` must be unchanged.
* Otherwise `T < Expiry ≤ T + maxTerm`. Renewal is an update that raises `Expiry`.
* Ownership transfer is an update that changes `Owner`. A k-of-n owner provides
  **recovery** without any privileged key: for example, 1-of-2 of the user's key
  and a recovery key.
* The fee is `TxFee`.

Disputes and abandoned names: v1 has no administrator override, because a
privileged key would become the most valuable attack target in the system.
Abandoned names lapse through expiry and the grace period. Dispute resolution
is a governance question and is left for a future version.

### 2.3 Uniqueness and conflicts

* **Names.** M-chain blocks are totally ordered, and each transaction is verified against
  the state produced by its predecessors in the same block. A duplicate registration fails verification.
* **Targets.** One live name per target makes reverse resolution unambiguous, which the
  explorer needs in order to attribute "From" and "To".
* **Pending transactions.** The block builder includes mempool transactions without
  re-verifying them one by one. Two pending transactions touching the same name
  or target would make every block built from them invalid. The mempool
  therefore records **claims** (`hash("name:" + name)` and `hash("target:" + target)`)
  in the same way it already records consumed UTXOs, and rejects a second
  transaction that touches the same claim. This is the M-chain equivalent of a snowstorm conflict set.

### 2.4 Activation

`version.GetNamingTime(networkID)` follows the `GetBanffTime` pattern:

* **Mainnet and Lothar:** not scheduled (year 10000), because Ortelius and old nodes
  must be upgraded first (architecture report §8).
* **Local and custom networks:** active, provided Banff is also active.

Before activation, both transactions are rejected by the standard executor and
therefore also by the mempool verifier.

### 2.5 Parameters

Parameters are per network and consensus-critical. They are **not** node flags,
because validators must agree on them.

| | Local and custom | Mainnet and Lothar (when scheduled) |
|---|---|---|
| `minTerm` | 1 min | 30 days |
| `maxTerm` | 5 years | 5 years |
| `grace` | 1 min | 30 days |
| `RegisterNameTxFee` | 0.01 DJT | 1 DJT |

The short terms on local networks exist so that expiry, grace and release can be exercised by the end-to-end tests.

## 3. Resolution

`names.resolve(name)` on `/ext/bc/M/names` reads the **last accepted**
M-chain state under the VM lock, never preferred or processing blocks. The
result is a pure function of the accepted block, so all honest nodes return the
same answer for the same `blockID`. It returns:

* the input and the canonical name;
* `status` and `resolvable`;
* `target`, as both a `V-` address and a ShortID;
* `owner`;
* `version`, `generation` and `recordID`;
* `expiry`, `registeredAt`, `updatedAt` and `lastTxID`;
* the evaluation point: `chainTime`, `height` and `blockID`.

| Case | Behaviour |
|---|---|
| Malformed input | Error `-32602`, reason `invalid_name:<rule>` |
| Unknown or never registered | Result with `status: "NOT_FOUND"`, `resolvable: false` |
| Expired, revoked or released | Record returned, `resolvable: false` |
| Ambiguous | Impossible by construction: one canonical form (§1) and one live record per name |

`names.reverse(address)` returns the name whose live record (status `ACTIVE`)
targets that address, or none. `names.history(name)` returns all versions.
`names.params` returns the parameters and activation time.

**Chain-time caveat.** M-chain time advances only when blocks are produced, so
a quiet chain can report `ACTIVE` slightly after the expiry has passed in wall-clock time. Clients that move funds must also check
`expiry` against their own clock with a safety margin. The transaction service does this (§5).

## 4. Integration with V-chain payments (Approach A)

The V-chain is not changed. A payment is a conventional `BaseTx` that pays the
resolved address. The transaction service:

1. resolves the name at accepted M-chain state;
2. **pins** `(recordID, generation, version, target)` into the prepared payment;
3. shows the user the name **and** the resolved address;
4. on submission, re-resolves and refuses (`recipient_changed`) if any pinned field differs, or if the record is no longer resolvable;
5. builds, signs and issues the transaction through `avm.issueTx`, using the Go wallet SDK;
6. waits for the DAG decision.

The memo carries `djtname/1 to=<name> g=<generation> v=<version>`. This is a
**claim, not proof**: an explorer may display it as "verified" only after
checking that the stated version of the record targeted the paid address.

### 4.1 Name changes between instruction and acceptance

| Window | Outcome |
|---|---|
| Change before preparation | The user sees the new target |
| Change between preparation and submission | Submission refused with `recipient_changed`; the user must prepare again and approve the new address |
| Change between submission and acceptance | The transaction still pays the address the user approved. It can never be redirected, because the address is inside the signed transaction |

The consequence of the last row: if Alice moves her name because a key was
compromised, a payment already submitted in that window still goes to the old
address. This is inherent to any design where the user approves a concrete
recipient. It is documented and preferable to a recipient chosen at acceptance time.

### 4.2 Concurrency

* The transaction service serialises spends per spending identity and tracks
  UTXOs consumed by its own in-flight transactions.
* The V-chain's snowstorm conflict rules remain the final authority: two transactions spending the same UTXO cannot both be accepted.

## 5. Transaction service (`txservice`)

`txservice` is a separate Go process, the only component that holds
signing keys. It uses the same JSON-RPC 2.0 conventions as the node (gorilla
`rpc/v2`, `service.method`, envelope `{apiVersion, schemaVersion}`, stable
error codes with `data.reason`). The service and method list is in `02-api.md`.

* **Preparation and submission are separate.** Accepting a name never implies authorisation, signing or execution.
* **Prepared payments expire** after 5 minutes.
* **Recipient checks.** Names expiring within 10 minutes of wall-clock time are refused (`name_expiring`).
* **Approval** is bound to the payment's digest: SHA-256 over the canonical JSON of the payment.
  The approving client must send the digest it displayed.
* **Idempotency.** A `prepare` with a reused idempotency key returns the original payment
  if the parameters match, and otherwise fails with `idempotency_conflict`.
  A repeated `submit` returns the same transaction ID.
* **Errors distinguish:**
  * `name_not_found`, `name_not_resolvable`, `name_expiring`, `invalid_recipient`
  * `invalid_amount`, `unsupported_asset`, `insufficient_funds`
  * `policy_denied`, `approval_required`, `approval_mismatch`, `unauthorized`, `delegation_revoked`
  * `recipient_changed`, `payment_expired`
  * `tx_rejected`, `network_unavailable`, `rate_limited`, `idempotency_conflict`

## 6. Bob: the AI-agent execution model

### 6.1 Principles

* **The LLM is untrusted.** It turns text into a structured intent and has no tools, keys or tokens.
* **Policy is enforced by `txservice`**, never by the agent.
* **Authority over V-chain assets comes only from V-chain UTXO ownership.**

### 6.2 Identities

| Identity | Key | Role |
|---|---|---|
| User (`demo-user`) | User key, not held by the agent | Owns funds and creates and revokes the delegation |
| Agent (`bob`) | Agent key, held by `txservice` and never by the LLM process | Co-owner of the allowance |
| Alice and the demo services | Their own keys | Recipients with registered names |

### 6.3 Delegation as an on-chain allowance

The user transfers an allowance, for example 100 DJT, to outputs owned
`1-of-2 {user, agent}`.

* The agent's key can spend **only** these outputs. The protocol caps total exposure at the allowance.
* The user can sweep them back at any time with their own key. This is an on-chain revocation that needs nobody's cooperation.
* Change returns to the same 1-of-2 owner set.

### 6.4 Policy (off-chain, enforced by `txservice`)

The delegation record holds:

* the allowed assets (DJT);
* the per-transaction maximum and rolling 24-hour maximum;
* the approval threshold (default 0: **every** payment needs user approval);
* whether only named recipients are allowed;
* rate limits;
* expiry and a `revoked` flag.

`delegations.revoke` takes effect immediately for all further payments.

### 6.5 Approval

* The agent credential may call `prepare`, `status`, `receipt`, `balance`, `history` and `describeTx`.
* Only the approver credential may call `approve`, `submit` and `delegations.revoke`.
* In the demo, the web back end uses the approver credential only in the handler for the user's explicit click. That handler is protected by the session cookie and a CSRF header.

### 6.6 Audit, replay and abuse

* **Audit log.** Every prepare, approve, submit, decision, refusal and revocation is
  appended to an audit log. Each record hashes its predecessor
  (`prevHash`), so the log is tamper-evident.
* **Replay protection.** Payment IDs are single-use and approval is bound to the digest. On-chain, a
  signed V-chain transaction cannot be replayed, because its inputs are consumed, and
  its `NetworkID` and `BlockchainID` bind it to one chain.
* **Rate limits** apply per principal and per client.

### 6.7 Arcs and Bob

**What an Arc does not grant.** Bob's Arc, an AVM chain on a subnet that Bob's
operator controls, gives **no** authority over V-chain assets.

**What an Arc offers Bob.** It provides a place where Bob's own records live under a committee the user chooses:

* policy versions;
* portfolio notes;
* anchors of the audit-log hash chain.

**Prototype status.** The integration is designed but not implemented. `txservice`
keeps the audit chain locally, and anchoring the head hash to an Arc
periodically is the recommended next step (`04-implementation-report.md`).

## 7. Explorer contract (Phase 4)

The explorer UI is deferred until its source is available.

`payments.describeTx(txID)` implements the attribution rules so they can be
ported without being reinvented. It returns:

* **Status**, from `avm.getTxStatus`.
* **Fee**: Σ inputs − Σ outputs of DJT, computed from the transaction itself.
* **Inputs**: each with its owner set, from the producing transaction's output, and the owner's name via `names.reverse`.
* **Outputs**, each classified as one of:

| Class | Evidence |
|---|---|
| `change` | The output's owner set equals the owner set of the inputs being spent |
| `payment` | Every other output that transfers value |
| `unknown` | The evidence is insufficient, for example the inputs' owners cannot be fetched |

* **From and To:**
  * a name only when the address has a live record with status `ACTIVE`;
  * otherwise the raw address;
  * a multisig owner set is shown as "1-of-2 {demo-user, bob}".
* **Memo claims:** reported as `verified` only if the named record version targeted the paid address. Otherwise they are shown as `unverified`.

Attribution uses the **current** registry state, because V-chain transactions
carry no timestamp that could anchor a historical lookup. The view states this.

Any local index is a cache of node data and can be rebuilt from it.

## 8. Demonstration (Phase 6)

The main web application's source is not available, so the demo ships as
`bob-demo/`, a standalone Node 22 back end and static front end, ready to be ported.

* **Try me** starts a session with an HttpOnly cookie and a CSRF token.
* Rate limits apply per session and per IP.
* The demo offers suggested tasks.
* Typed input is the primary method. Voice uses the browser's Web Speech API
  where it exists, and its transcript is only ever text input. The four stages
  run in separate places:

| Stage | Where it runs |
|---|---|
| Speech recognition | Browser |
| Reasoning | LLM, or a deterministic parser when no API key is configured; labelled in the UI |
| Policy | `txservice` |
| Execution | V-chain |

* Receipts show resolved names, the address, asset, amount, actual fee, status and
  transaction ID, with a link to the transaction record. The explorer URL is
  configurable, with a local `describeTx` view as the default.
* The demo service (`dijets-cafe`) and the civic payment (`civic-demo`) are **real
  test-network transfers** to demo recipients. They are clearly labelled as simulated
  services: no real-world obligation is settled.

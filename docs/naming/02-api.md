# Native Naming and Payments — API Reference

There are two APIs, and both follow the node's existing conventions:
- gorilla `rpc/v2` JSON-RPC 2.0, with methods named `service.method`;
- numbers as JSON strings, through `utils/json`;
- every reply wrapped in a versioned envelope (`apiVersion: "v1"`, `schemaVersion: 1`);
- every error carrying `data.reason` (stable and machine-readable) and, where useful, `data.detail`.

| API | Where | Authority | Holds keys |
|---|---|---|---|
| `names.*` | Node, `/ext/bc/M/names` | Read-only view of accepted M-chain state | No |
| `payments.*` | `txservice`, `/ext/payments` | Prepares, approves and submits payments under delegation policy | Agent key only |

Registering and updating names are ordinary signed M-chain transactions, `RegisterNameTx` and `UpdateNameTx`:
- **Build** them with the Go wallet SDK: `p.Builder.NewRegisterNameTx` and `NewUpdateNameTx`.
- **Sign** them client-side.
- **Submit** them through the existing `platform.issueTx`.

No API accepts a private key. Every example below was captured from the 5-node e2e network (`tests/names`).

## 1. `names.*` (node)

Every answer is computed from the **last accepted** M-chain block under the VM lock, and reports that block in `at`. Nodes at the same `at.blockID` return identical answers; the e2e test checks this on all five nodes.

| Method | Params | Returns |
|---|---|---|
| `names.resolve` | `{name}` (any case; a leading `@` is allowed) | `{input, name, status, resolvable, record?, at}` |
| `names.reverse` | `{address}` (`V-…`, `M-…` or bare bech32) | `{address, name?, record?, at}`. A name appears only when an ACTIVE record targets the address |
| `names.history` | `{name}` | `{name, versions[], at}`, oldest first; superseded versions have status `SUPERSEDED` |
| `names.validate` | `{name}` | `{input, canonical}`, or the `invalid_name:<rule>` error |
| `names.params` | `{}` | `{active, activationTime, minTermSeconds, maxTermSeconds, gracePeriodSeconds, registerFee, updateFee, minLength, maxLength, at}` |

**Example: `names.resolve {"name": "Alice"}`**

```json
{
  "apiVersion": "v1", "schemaVersion": 1,
  "input": "Alice", "name": "alice", "status": "ACTIVE", "resolvable": true,
  "record": {
    "name": "alice", "status": "ACTIVE",
    "target": "V-custom18t3nxs6gy4rqgwa9kp58sppylpqs2yazvpw4uf",
    "targetID": "6NNNH1LqtAhdvUxRtXhEqvuPNY31sdUue",
    "owner": {"threshold": "1", "locktime": "0", "addresses": ["M-custom18t3nxs6gy4rqgwa9kp58sppylpqs2yazvpw4uf"]},
    "expiry": "1792637060", "revoked": false, "generation": "1", "version": "1",
    "recordID": "PXQ4T5oFAwezECibgN8CEsCDQMh1PuxrU6XBbzN81mmWxok4i",
    "lastTxID": "PXQ4T5oFAwezECibgN8CEsCDQMh1PuxrU6XBbzN81mmWxok4i",
    "registeredAt": "1792633460", "updatedAt": "1792633460"
  },
  "at": {"blockID": "JX8NQvn6WzVf2SckyVqJmipPPH5oaCarUbyg7B86PGK1DTFWa", "height": "1", "chainTime": "1792633460"}
}
```

An unknown name is not an error: it returns `status: "NOT_FOUND"`, `resolvable: false` and no `record`.

**Errors**

| Code | Reason |
|---|---|
| -32602 | `invalid_name:<rule>`. Rules: `empty`, `non_ascii`, `too_short`, `too_long`, `character`, `first_character`, `last_character`, `double_hyphen`, `chain_prefix`, `hierarchical`, `reserved`, `not_canonical` |
| -32602 | `invalid_address` |
| -32029 | `too_many_concurrent_requests` |
| -32000 | `internal_error` |

**Transaction rejections** from `platform.issueTx` carry the executor's message:

| Message | Meaning |
|---|---|
| `name is not available for registration` | The name exists and is not RELEASED |
| `target is already bound to another name` | The target address already carries a name |
| `target did not authorize the name` | The target's consent signature is missing or wrong |
| `unauthorized name update` | The signer is not the record's owner |
| `name record version mismatch` | A stale or concurrent update |
| `name can no longer be updated` | The name is REVOKED or RELEASED |
| `name expiry out of range` | The expiry is outside the allowed term |
| `a revocation can't change the target, owner or expiry` | A revocation tried to change other fields |
| `native naming is not activated` | Naming is not active on this network |

## 2. `payments.*` (`txservice`)

Every request needs `Authorization: Bearer <token>`. A client's role decides what it may call:

| Method | Agent | Approver | Notes |
|---|---|---|---|
| `payments.info` | ✓ | ✓ | The caller, network, asset, and its delegations and policies |
| `payments.resolve` | ✓ | ✓ | Whether a name can be paid now, and why not if it can't |
| `payments.prepare` | ✓ | ✓ | `{delegation, recipient, asset, amount, idempotencyKey?, note?}` → `PREPARED` payment with `digest`. Nothing is signed or sent |
| `payments.get` / `payments.status` | ✓ | ✓ | `status` also polls the V-chain |
| `payments.approve` | ✗ | ✓ | `{paymentID, digest}`: the digest must be the one the user saw |
| `payments.submit` | only policy-approved | ✓ | Re-resolves and re-checks policy, then builds, signs and issues, and waits up to `decisionTimeout`. Idempotent |
| `payments.cancel` | ✓ | ✓ | Only before submission |
| `payments.receipt` | ✓ | ✓ | Only for `ACCEPTED`: the payment plus its `TxView`, read back from the chain |
| `payments.balance` | ✓ | ✓ | Allowance available, reserved, spent in the last 24 hours, policy |
| `payments.history` | ✓ | ✓ | Newest first |
| `payments.describeTx` | ✓ | ✓ | `TxView` of **any** V-chain transaction (the explorer contract) |
| `payments.revokeDelegation` | ✗ | ✓ | Stops further payments and cancels pending ones |

A payment moves through these statuses:

```
PREPARED → APPROVED → SUBMITTED → ACCEPTED | REJECTED
```

At any point before submission it can end as `FAILED`, `CANCELLED` or `EXPIRED`.

**Example: the payment and receipt from e2e phase 7 (abridged)**

```json
{
  "payment": {
    "id": "pay_04519f34073a853b38551605624923ad", "status": "ACCEPTED",
    "to": {"name": "alice", "address": "V-custom1j5uam2f74qq6gyp4ljj0e3j7wj0al4hjyj32kx",
           "record": {"recordID": "PXQ4T5oF…", "generation": "1", "version": "2", "expiry": "1792637060",
                      "blockID": "6QasHGBt…", "height": "8"}},
    "amount": {"units": "10000000000", "display": "10"},
    "fee": {"units": "1000000", "display": "0.001"},
    "memo": "djtname/1 to=alice g=1 v=2",
    "approvalRequired": true, "approvedBy": "user",
    "txID": "wRtcbb4Kk6JtSQQoZ9BuoUzfd2x3cUWrFtf7G5C1MUmEKogSW"
  },
  "transaction": {
    "status": "Accepted",
    "from": {"label": "1-of-2 {demo-user, bob}", "threshold": 1, "addresses": [
      {"address": "V-custom1n333q50efvgtvhpu9t92s3a8ulkka2sl6yr6az", "name": "demo-user"},
      {"address": "V-custom1hvjr3jzdrl3t5crz3srvw4gam0mlaunh6q7xcg", "name": "bob"}]},
    "to": [{"label": "alice", "threshold": 1, "addresses": [{"address": "V-custom1j5uam2f7…", "name": "alice"}]}],
    "outputs": [
      {"index": 0, "amount": {"display": "10"}, "owner": {"label": "alice"}, "role": "payment",
       "evidence": "owned by someone other than the inputs' owner"},
      {"index": 1, "amount": {"display": "89.999"}, "owner": {"label": "1-of-2 {demo-user, bob}"}, "role": "change",
       "evidence": "returns to the owner of the inputs"}],
    "fees": [{"asset": {"symbol": "DJT", "denomination": 9}, "amount": {"units": "1000000", "display": "0.001"}}],
    "memoClaim": {"name": "alice", "generation": 1, "version": 2, "verified": true,
                  "detail": "version 2 of \"alice\" targeted the paid address"},
    "attribution": "Names are attributed from the naming registry's current state. …"
  }
}
```

**Errors.** Clients should branch on `data.reason`; the codes group reasons:

| Code | Reasons |
|---|---|
| -32602 | `invalid_params`, `invalid_name` (`detail` = rule), `invalid_recipient`, `invalid_amount`, `unsupported_asset`, `self_payment` |
| -32030 | `name_not_found`, `name_not_resolvable` (`detail` = status), `name_expiring`, `recipient_changed` |
| -32031 | `payment_expired` |
| -32032 | `idempotency_conflict`, `invalid_payment_state`, `not_final` |
| -32040 | `policy_denied` (`detail`: `per_payment_max`, `daily_max` or `named_recipients_only`), `approval_required`, `approval_mismatch`, `delegation_revoked` |
| -32041 | `insufficient_funds` |
| -32001 | `unauthorized`, `forbidden` |
| -32004 | `delegation_not_found`, `payment_not_found`, `tx_not_found` |
| -32010 | `network_unavailable` |
| -32029 | `rate_limited` |
| -32050 | `tx_rejected` |
| -32000 | `internal_error` |

## 3. Configuration (`txservice`)

`txservice -config txservice.json`:

```json
{
  "nodeURI": "http://127.0.0.1:9650",
  "listen": "127.0.0.1:19800",
  "dataDir": "/var/lib/txservice",
  "clients": [
    {"name": "bob-agent", "tokenSHA256": "<hex sha256 of token>", "role": "agent", "delegations": ["demo"]},
    {"name": "user-approver", "tokenSHA256": "<hex sha256 of token>", "role": "approver", "delegations": ["demo"]}
  ],
  "delegations": [{
    "id": "demo", "principal": "demo-user", "principalAddress": "V-local1…",
    "agentName": "bob", "agentKeyFile": "/var/lib/txservice/keys/bob.key",
    "assets": ["DJT"], "perPaymentMax": "25", "dailyMax": "60", "autoApproveMax": "0",
    "namedRecipientsOnly": false
  }],
  "preparedTTL": "5m", "expiryMargin": "10m", "decisionTimeout": "15s",
  "explorerTxURL": "https://explorer.example/tx/{txID}",
  "rateLimits": {"read": {"perMinute": 120, "burst": 30}, "prepare": {"perMinute": 10, "burst": 5}, "submit": {"perMinute": 5, "burst": 3}}
}
```

- **Agent key:** created with mode 0600 on first start. The service logs the agent's address so that the allowance can be funded: `1-of-2 {principalAddress, agent}`.
- **Data directory:** holds `payments.json`, written atomically, and `audit.jsonl`, hash-chained. The chain is verified at start-up, and the service refuses to start if it is broken.

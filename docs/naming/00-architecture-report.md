# Native Naming — Phase 0 Architecture Report (M0)

Status: complete. This report maps the fork as it actually is and recommends
the design that `01-design.md` specifies. Every path is relative to
`dijetsnodesgo/` unless stated otherwise. Line numbers refer to branch
`feature/arcs` at `f10ae00`, the base of `feature/names`.

## 1. Lineage, version and existing modifications

| Item | Finding |
|---|---|
| Upstream | AvalancheGo v1.9.4, module renamed to `github.com/lasthyphen/dijetsnodesgo` |
| Go | 1.20.5 |
| Chain renames | X → **V** (`V`, `avm`), P → **M** (`M`, `platform`), C → **U** (`U`, `evm`), set in `genesis/aliases.go:17-66` |
| Package renames | `vms/components/avax` → `vms/components/djtx`; `snow/engine/avalanche` → `snow/engine/dijets`; `snow/consensus/avalanche` → `snow/consensus/dijets` |
| Native asset | Name "Dijets", symbol **DJT**, denomination 9 (`genesis/genesis.go:264-266`); the asset alias `"DJT"` is registered at `vms/avm/vm.go:450` |
| Networks | mainnet 1 (`dijets`), lothar 5 (`lothar`), local 12345 (`local`), any other ID uses HRP `custom` (`utils/constants/network_ids.go`) |
| Upgrades | Banff is **not** active on Lothar (scheduled for year 10000, `version/constants.go`); it is active on local and custom networks, and on mainnet from its 2026-10-25 genesis (1.0.0) |
| Fork additions already present | Arcs prototype (`arc/`, `vms/platformvm/arc_registry.go`, `chains/manager.go` local status and DD-8 fix, `tests/arc/`); see `docs/arcs/` |

## 2. V-chain transaction format and validation pipeline

* **Format.** `txs.Tx{Unsigned, Creds}` (`vms/avm/txs/tx.go:53`). `BaseTx` wraps
  `djtx.BaseTx{NetworkID, BlockchainID, Outs, Ins, Memo}`
  (`vms/components/djtx/base_tx.go:25-31`). Codec type IDs:
  * 0–4: the transactions (`vms/avm/txs/parser.go:48-99`)
  * 5–9: secp256k1fx
  * 10–14: nftfx
  * 15–19: propertyfx

  The tx ID is SHA-256 of the signed bytes, and the memo is limited to 256 bytes.
* **Syntactic checks.**
  * The number of credentials must equal the number of inputs (`txs/tx.go:87-116`).
  * Network and chain IDs must be correct, and the memo must fit (`djtx/base_tx.go:63-77`).
  * The flow check requires produced ≤ consumed, with the fee burned (`djtx/transferables.go:203-241`).
  * Inputs and outputs must be sorted, and inputs must be unique.
* **Semantic checks.** `tx_semantic_verify.go:19` → `vm.verifyTransfer`
  (`vm.go:620`) → `secp256k1fx.Fx.VerifyCredentials` (`vms/secp256k1fx/fx.go:176-215`).
  This step recovers public keys from signatures over SHA-256 of the unsigned bytes, and checks threshold and locktime.

## 3. DAG consensus and conflict handling

* `UniqueTx` is the snowstorm transaction. Its conflict set is `InputIDs()`, the
  consumed UTXO IDs (`vms/avm/unique_tx.go:276`); dependencies are the parent
  transactions (`:240`).
* The engine never places overlapping inputs in one vertex
  (`snow/engine/dijets/transitive.go:569-617`).
* `snowstorm.Directed` adds conflict edges between spenders of the same UTXO
  (`snow/consensus/snowstorm/directed.go:339-360`) and rejects the losers on
  acceptance (`:518-560`).
* **Key finding:** vertices and transactions carry **no timestamp and no
  M-chain height**. Nothing in `vms/avm` or `snow/engine/dijets/vertex`
  references M-chain state. V-chain validation therefore has no deterministic
  view of any state outside the V-chain.

## 4. UTXO model, addresses and transfers

* UTXOs are `djtx.UTXO`. A UTXO ID is the tx ID plus the output index
  (`djtx/utxo_id.go:44`). Ownership is `secp256k1fx.OutputOwners{Locktime,
  Threshold, Addrs}`, which allows multisig and timelocks.
* State is held in `vms/avm/states`, with a per-address UTXO index in `djtx/utxo_state.go:192`.
  Acceptance (`unique_tx.go:125-199`) deletes the inputs, puts the outputs and indexes addresses.
* An address is `<chainAlias>-<bech32(hrp, 20-byte ShortID)>`
  (`utils/formatting/address`). The same secp256k1 key yields the **same
  ShortID on every chain**: `V-local1…`, `M-local1…`.
* **Change outputs are indistinguishable on-chain.** Change is an ordinary
  `TransferOutput`, and all outputs are sorted (`SortTransferableOutputs`), so
  neither type nor position marks change.
  * The node wallet API sends change to `changeAddr`, or else to the first keystore key (`vms/avm/vm.go:1028`).
  * The Go SDK sends change to `WithChangeOwner`, or else to an arbitrary key in the keychain (`wallet/chain/x/builder.go:548`).
* **Fees.** `TxFee` is a minimum burn. The actual fee is Σ inputs − Σ outputs of
  the fee asset, and it can be higher than `TxFee`. `TransferInput` includes
  `Amt`, so the actual fee can be computed from the transaction bytes alone.

## 5. Wallet and RPC APIs

| Surface | Path | Signing |
|---|---|---|
| `avm.send`, `avm.sendMultiple` | `vms/avm/service.go:964-1112` | Node keystore (username/password) |
| `wallet.send` | `vms/avm/wallet_service.go:108` | Node keystore; chains unconfirmed UTXOs |
| `avm.issueTx`, `getTx`, `getTxStatus`, `getUTXOs`, `getBalance`, `getAllBalances`, `getAddressTxs` | `vms/avm/service.go` | Externally signed transactions |
| `platform.issueTx` and getters | `vms/platformvm/service.go` | Externally signed transactions |
| Go wallet SDK | `wallet/subnet/primary`, `wallet/chain/{p,x}` | Client-side keychain (`utils/crypto/keychain`), including external signers |
| Node indexer | `indexer/`, `/ext/index/{V,M,U}/…` | — |
| Auth | `api/auth` (JWT, `--api-auth-required`) | — |

`getAddressTxs` requires the AVM chain config `index-transactions`. The Go SDK
is already used by `tests/arc`, so payments can be built and signed **without
the node keystore**.

## 6. Existing identity, aliasing, naming and metadata

There is **no name-to-address mapping anywhere** in the fork. What exists:

* `ids.Aliaser` (`ids/aliases.go`) and `admin.aliasChain`
  (`api/admin/service.go:109-175`) are in-memory, local and lost on restart.
  They are unsuitable: not consensus-governed.
* AVM asset aliases exist only for genesis assets (`vm.go:450`).
  `CreateAssetTx` names and symbols are **not unique**
  (`vms/avm/txs/create_asset_tx.go`).
* `CreateChainTx.ChainName` is not unique.
* Keystore user names are local accounts.
* Memos are arbitrary signed bytes (256 bytes).
* nftfx and propertyfx can represent ownership of a token, but cannot enforce
  **uniqueness** of a name, because any account can create an asset with any name.

## 7. Relationship of V, M, U and Arcs

* **V-chain:** AVM on the DAG engine (snowstorm). It holds DJT payments.
* **M-chain:** platformvm on Snowman (linear). It holds validators, subnets and
  chain registrations. Its state has versioned diffs per block (`state.Diff`) and
  is written by `state.write()`.
* **U-chain:** coreth (EVM). It is unaffected by this work.
* **Arcs:** AVM chains created by M-chain `CreateChainTx` on permissioned
  subnets. The registry is a read-only adapter over M-chain state, served at
  `/ext/bc/M/arc`.
  * An Arc's committee has authority only over that Arc. It has **no authority over V-chain UTXOs**: a V-chain spend requires signatures satisfying the UTXO's `OutputOwners`, and nothing else.

## 8. Explorer data sources

* The explorer source is not on this machine.
* The compiled bundle (`~/lothar-explorer`, Vue/Vuetify) reads an Ortelius-style
  API (`/v2/transactions`, `/v2/addresses`, `/v2/assets`, `/v2/ctxdata`, …) from
  per-network hosts.
* `~/ortelius` is upstream `github.com/ava-labs/ortelius`. It streams containers
  from the node indexer (`/ext/index/<chain>/<tx|vtx|block>`,
  `stream/producer_chain.go:281`), decodes them with **its own copy of the
  codecs**, stores them in MySQL and serves `/v2`.

Consequences for naming:

* The explorer needs either Ortelius to understand the two new M-chain tx types,
  or to query the node's `names` API directly.
* Because Ortelius decodes M-chain blocks with upstream codecs, it will **fail to
  decode** M-chain blocks containing name transactions until its codec is
  updated. This must be handled before naming is activated on a network
  Ortelius indexes. The activation time described in §11 makes this a
  deliberate, scheduled step.
* Explorer UI work is deferred until its source is available. The
  attribution logic and data contract are implemented outside the explorer (see the design, §9).

## 9. Test network, assets and signing

* **Local genesis:** `genesis/genesis_local.json` (12345). The pre-funded **ewoq**
  key is `genesis.EWOQKey` (`genesis/genesis_local.go:18-31`), at
  `local18jma8ppw3nhx5r4ap8clazz0dps7rv5u00z96u`.
* `tests/arc/network.go` starts 5 nodes on network 1337 (HRP `custom`) with
  `staking/local` certificates on ports 19650+2i, and is reused here.
* Local fees: `TxFee` = 1 mDJT and `CreateSubnetTxFee` = 100 mDJT
  (`genesis/genesis_local.go:37-47`).

## 10. Trace: an ordinary V-chain transfer

1. **Construct.** In the SDK, `x.Builder.NewBaseTx` selects UTXOs (`builder.go:528`), adds the
   fee and a change output to the change owner, and sorts.
   The node API path is `avm.send` → `vm.Spend` → `SignSECP256K1Fx`.
2. **Sign.** `x.Signer` signs SHA-256 of the unsigned bytes for each input's
   signature indices (`signer.go:60-258`).
3. **Submit.** `avm.issueTx` (`service.go:66`) → `VM.IssueTx` → `parseTx`
   (parse, syntactic check, store as Processing) → `SemanticVerify` → batched
   into `PendingTxs` (`vm.go:373-524`).
4. **Consensus.** The engine batches the transaction into a vertex and snowstorm
   adds conflict edges by input UTXO. Polling then decides the transaction.
5. **Accept.** `UniqueTx.Accept` updates the address index, deletes the input UTXOs,
   adds the output UTXOs, sets the status to Accepted and commits. It then calls
   `DecisionAcceptor` → node indexer `/ext/index/V/tx`.
6. **Index.** Ortelius consumes `/ext/index/V/tx`, decodes the transaction, and
   serves `/v2/transactions` to the explorer.

## 11. Naming architectures compared

| Option | Authority | Uniqueness | Fit |
|---|---|---|---|
| **N1. M-chain registry (new M-chain tx types)** | Primary-network validators, Snowman total order | Trivial: state check in a linear order; pending conflicts tracked in the mempool | ✅ M-chain already has owners (`OutputOwners`), signature-based permission (`Fx.VerifyPermission`, used for subnet auth), fees, versioned state and fork gating |
| N2. New V-chain tx type with a synthetic conflict ID (hash of the name) in `InputIDs` | Primary network, DAG | Possible via snowstorm conflicts | ❌ Changes the V-chain tx format and codec, adds non-UTXO state to the AVM, and makes registry reads depend on processing DAG state. Disruptive, and the RFC asks to preserve the V-chain |
| N3. Registry on a dedicated Arc | Arc committee only | Arc consensus | ❌ Security equals a permissioned committee, not the primary network, which the Arcs RFC forbids claiming |
| N4. NFT or propertyfx "name tokens" | V-chain | **None**: anyone can mint a duplicate name | ❌ |
| N5. U-chain contract (ENS-like) | — | — | ❌ Excluded by the RFC |
| N6. Explorer or indexer database | — | — | ❌ Not consensus-governed |

**Recommendation: N1.** The registry lives in M-chain state, maintained by two
new M-chain transaction types (`RegisterNameTx`, `UpdateNameTx`). Each
validator verifies them inside the existing Snowman block executor. Their
activation is gated per network, following the pattern of `version.GetBanffTime`.

## 12. Payments: Approach A versus Approach B

| | A. Resolve, then build a conventional transaction | B. Name references inside V-chain transactions |
|---|---|---|
| V-chain change | None | New tx or output type, codec and verifier |
| Determinism | Resolution is an M-chain read at a stated accepted block; the V-chain transaction pays an explicit address | **Not achievable today:** V-chain validators have no M-chain height or timestamp to resolve against, so two validators could resolve the same name differently (§3) |
| Conflict handling | Unchanged snowstorm conflicts on UTXOs | A name update would have to conflict with in-flight payments across chains, which has no mechanism |
| Backward compatibility | Complete; old wallets and explorers keep working | Old explorers and indexers cannot decode the new types |
| Safety against a name change in flight | The user approves the explicit address and the record version; submission re-checks the version; the accepted transaction cannot be redirected | Recipient is decided at acceptance, so it can silently differ from what the user approved |

**Recommendation: A.** Names are native (M-chain consensus) and resolution is
deterministic. Payments stay conventional V-chain transactions, so the DAG
architecture is untouched. Approach B is rejected on determinism grounds, not
only on complexity.

## 13. Components to change and to leave untouched

| Component | Change |
|---|---|
| `vms/platformvm/txs` | Add `RegisterNameTx` and `UpdateNameTx`, codec registration appended at the end, and visitor methods |
| `vms/platformvm/txs/executor` | Verification and execution, plus wrong-type stubs |
| `vms/platformvm/txs/mempool` | Name and target claims, so two pending transactions for one name cannot share a block |
| `vms/platformvm/state` | Name records, history and reverse index; diff and persistence |
| `vms/platformvm/names` (new) | Canonicalisation, record status rules, parameters |
| `vms/platformvm` service | Read-only `names` JSON-RPC at `/ext/bc/M/names` |
| `vms/platformvm/metrics` | Transaction counters |
| `wallet/chain/p` | Builders and signers for the new transactions |
| `version`, `node`, `api/info` | Naming activation time and fee wiring |
| `txservice/` (new, separate process) | Payment preparation, policy, approval, signing, submission and receipts |
| V-chain (`vms/avm`), DAG engine, snowstorm, U-chain, Arcs | **Untouched** |

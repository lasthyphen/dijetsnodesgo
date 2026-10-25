# Running the mainnet genesis validators

Mainnet (network ID 1) was relaunched with a new genesis on 2026-10-25
(`genesis/genesis_mainnet.json`, node 1.0.0). Its five genesis validators are
the only validators, so the network runs when they do.
`scripts/run_mainnet_cluster.sh` runs all five on one host.

| | |
|---|---|
| Genesis start | 2026-10-25 08:27:36 UTC (`startTime` 1792916856) |
| Validators | `NodeID-FU6TByNdgJhbQF8WeFwfKuD3QnMcQJMjP`, `NodeID-MxGY1zYcaP9CM1LvW9RKBDZfbZa5dBSK3`, `NodeID-HTdNxjm2cKe1hUzdHBiEvA3zr4wYj3Vg9`, `NodeID-5nJLrvj92ztUAGQrfcPHzuHdm5X9ykeRd`, `NodeID-71qPRtPrLBcaHrAMKF39tNrExJWhfaKpr` |
| Stake ends | 2027-10-25 08:27 UTC for the first, then every 15 hours. Add validators before then, or the network stops. |
| M-Chain genesis block | `2AFGBPNE29LGEXy2rDhtQ7T6bRD9yVAJto5z6F529ia7kTBW3p` |
| V-Chain | `2abk8oCqF74M1L2UhsPGe1MMXoEkRVubGZxV1Nfn7aF5s5T8fM` |
| U-Chain | `vWaU1HuvgqAQ9KkKSh5ThrxXvSCZ2ZDK1k8zeg2innRv2SdrN`, EVM chain ID 98200 |
| DJT asset | `2U4xzjHGnxXKWzFKqVZVMEnnepKFRNcBC75346X129Nj7uzyP1` |
| Upgrades | Every upgrade, including Banff and native naming, is active from genesis |

The allocations, the genesis message and the U-Chain genesis are unchanged
from the previous mainnet genesis, so the V-Chain, U-Chain and asset IDs are
the same as before and the genesis UTXOs are identical. A transaction signed
on the previous network can therefore be replayed on this one. Don't run the
two side by side.

## Keys

The validators' staking keys are not in this repository. Each node needs its
own directory holding `staker.crt` and `staker.key`:

```
$KEY_DIR/node1/staker.crt  staker.key
...
$KEY_DIR/node5/staker.crt  staker.key
```

Copy them to the host over SSH, readable only by the service user:

```sh
scp -r staking/ <user>@<host>:/tmp/staking
sudo mv /tmp/staking /data/dijets/staking
sudo chown -R dijets:dijets /data/dijets/staking
sudo chmod -R go-rwx /data/dijets/staking
```

Losing a key means losing that validator: its stake can't be moved to a new
key before it ends.

## Build

```sh
git clone -b feature/explorer https://github.com/lasthyphen/dijetsnodesgo
cd dijetsnodesgo
./scripts/build_dijets.sh   # build/dijetsnodesgo; the U-Chain EVM is built in
```

## Start

```sh
KEY_DIR=/data/dijets/staking DATA_DIR=/data/dijets/mainnet \
PUBLIC_IP=<the host's static public IP> \
./scripts/run_mainnet_cluster.sh
```

The script stays in the foreground and stops the nodes on SIGINT or SIGTERM.
Node *i* listens for HTTP on `127.0.0.1:BASE_PORT+2(i-1)` and for staking on
`BASE_PORT+2(i-1)+1`, so with the default `BASE_PORT=9650` node 1 is
9650/9651 and node 5 is 9658/9659. Each node logs to
`$DATA_DIR/node<i>/stdout.log`.

The flags it passes:

| Flag | Nodes | Why |
|---|---|---|
| `--network-id=mainnet` | all | The built-in genesis; mainnet refuses `--genesis` |
| `--staking-tls-cert-file`, `--staking-tls-key-file` | all | The validator's identity |
| `--http-host=127.0.0.1`, `--http-port`, `--staking-port`, `--data-dir` | all | One set per node; the APIs stay on the host |
| `--api-keystore-enabled=false` | all | The keystore API stores keys on the node; validators don't need it |
| `--public-ip=$PUBLIC_IP` | all | The address peers on other hosts dial. With `127.0.0.1` only this host can join |
| `--bootstrap-ips=127.0.0.1:<node 1 staking port>`, `--bootstrap-ids=<node 1>` | 2-5 | The genesis has no beacon addresses yet |
| `--index-enabled`, `--index-subnet-chains` | 1 | For the explorer. Must be on from node 1's **first** start; otherwise delete its database and resync |
| `--whitelisted-subnets=$WHITELISTED_SUBNETS` | all, if set | The subnet of every Arc the nodes validate or index |
| `--chain-config-dir=$CHAIN_CONFIG_DIR` | all, if set | For example `U/config.json` with `"debug-tracer"` in `eth-apis`, for internal calls in the explorer |

As a systemd service:

```ini
[Unit]
Description=Dijets mainnet genesis validators
After=network-online.target
Wants=network-online.target

[Service]
User=dijets
Environment=KEY_DIR=/data/dijets/staking
Environment=DATA_DIR=/data/dijets/mainnet
Environment=PUBLIC_IP=<static public IP>
Environment=PATH=/usr/local/go/bin:/usr/bin:/bin
WorkingDirectory=/data/dijets/src/dijetsnodesgo
ExecStart=/data/dijets/src/dijetsnodesgo/scripts/run_mainnet_cluster.sh
KillSignal=SIGTERM
TimeoutStopSec=60
Restart=on-failure
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

Firewall: open the staking ports (9651, 9653, 9655, 9657, 9659) to any
source, so that nodes on other hosts can join. Never open the HTTP ports.

The genesis lists these five as the mainnet beacons, at `20.108.178.171`
(`genesis/beacons.go`), so the cluster must run there with
`PUBLIC_IP=20.108.178.171` and the default `BASE_PORT`.

## Joining from another host

Any other node joins with no bootstrap flags; it syncs from the beacons:

```sh
./build/dijetsnodesgo --network-id=mainnet --http-host=127.0.0.1
```

It doesn't validate unless it is added as a validator (`AddValidatorTx`,
at least 2,000 DJT).

## A public API

Wallets, scripts and the browser extension need an HTTPS endpoint. Serve
one from a node that is not a validator (for example the explorer's node,
below), started with:

```sh
--api-keystore-enabled=false   # the keystore API stores users' keys on the node; never expose it
```

The admin and IPC APIs are off by default; leave them off. Then publish only
the chain, info and health routes through Caddy, next to the explorer API:

```
<api domain> {
	@node path /ext/info /ext/health /ext/bc/V /ext/bc/V/* /ext/bc/M /ext/bc/M/* /ext/bc/U/rpc /ext/bc/U/ws /ext/bc/U/djtx
	reverse_proxy @node 127.0.0.1:9650
	reverse_proxy 127.0.0.1:8090
}
```

`/ext/bc/M/*` includes the name registry (`/ext/bc/M/names`) and Arcs
(`/ext/bc/M/arc`). Add `/ext/bc/<Arc chain ID>` for each Arc chain the
node tracks.

## Explorer

`explorerd` reads a node's index API, which is served only on localhost, so
it needs a node on its own host. On the cluster host that is node 1. On a
separate explorer host, run a non-validating node that joins as above, with
the index flags from its first start:

```sh
./build/dijetsnodesgo --network-id=mainnet --http-host=127.0.0.1 \
  --index-enabled --index-subnet-chains --api-keystore-enabled=false \
  [--whitelisted-subnets=<Arc subnet IDs>]
```

Give `explorerd` a new database. The previous database holds a different
network, and `explorerd` refuses to mix them.

```json
{
  "nodeURI": "http://127.0.0.1:9650",
  "databaseURL": "postgres:///explorer_mainnet?sslmode=disable",
  "listen": "127.0.0.1:8090",
  "genesisFile": "",
  "indexArcs": true,
  "roles": ["indexer", "api"]
}
```

`genesisFile` stays empty: the mainnet genesis is built in. The explorer
reports names, Arcs and validators from node 1's APIs. Keep Caddy in front
of it as before; the web app needs no change while the API keeps its domain.

## Names

The registry is part of the M-Chain and active from genesis, with the public
rules: terms from 30 days to 5 years, a 30-day grace period and a 1 DJT
registration fee (`vms/platformvm/names/params.go`). Nothing needs to be
enabled. The optional transaction service (`txservice`) is a separate process;
its configuration is in `docs/naming/02-api.md` §3.

## Arcs

`arcctl create -spec arc.json -key-file owner.key -node http://127.0.0.1:9650`
creates the subnet, adds the committee and creates the chain. The committee
must be mainnet validators, which at first means the five above. Then add the
subnet ID to `WHITELISTED_SUBNETS` and restart the service, so that the
committee runs the Arc's chain and node 1 indexes it.

#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

print_usage() {
  printf "Usage: run_mainnet_cluster.sh

  Runs the five mainnet genesis validators on one host, in the foreground,
  until it receives SIGINT or SIGTERM (for example from systemd).

  Node i (1-5) uses:
    keys     \$KEY_DIR/node<i>/staker.crt and staker.key
    data     \$DATA_DIR/node<i>
    ports    HTTP \$BASE_PORT+2(i-1), staking \$BASE_PORT+2(i-1)+1

  Nodes 2-5 bootstrap from node 1. Node 1 is the one the explorer reads: it
  runs with the index flags, which must be on from its first start.

  Environment:
    KEY_DIR              required; the staking keys of the genesis validators
    DATA_DIR             default ./mainnet-cluster
    BASE_PORT            default 9650
    PUBLIC_IP            the address other hosts reach this one at
                         (default 127.0.0.1: reachable from this host only)
    WHITELISTED_SUBNETS  comma-separated subnet IDs (Arcs) that every node
                         tracks; restart after changing it
    CHAIN_CONFIG_DIR     optional --chain-config-dir (for example U/config.json)
    NODE_BINARY          default build/dijetsnodesgo
"
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  print_usage
  exit 0
fi

REPO_ROOT=$( cd "$( dirname "${BASH_SOURCE[0]}" )"; cd .. && pwd )
KEY_DIR=${KEY_DIR:?set KEY_DIR to the directory holding node1..node5 staking keys}
DATA_DIR=${DATA_DIR:-$PWD/mainnet-cluster}
BASE_PORT=${BASE_PORT:-9650}
PUBLIC_IP=${PUBLIC_IP:-127.0.0.1}
WHITELISTED_SUBNETS=${WHITELISTED_SUBNETS:-}
CHAIN_CONFIG_DIR=${CHAIN_CONFIG_DIR:-}
NODE_BINARY=${NODE_BINARY:-$REPO_ROOT/build/dijetsnodesgo}

for i in 1 2 3 4 5; do
  for f in staker.crt staker.key; do
    if [[ ! -f "$KEY_DIR/node$i/$f" ]]; then
      echo "missing $KEY_DIR/node$i/$f" >&2
      exit 1
    fi
  done
done

# Node 1's ID, from its certificate, for the other nodes' --bootstrap-ids.
NODE1_ID=$(cd "$REPO_ROOT" && go run ./scripts/nodeid "$KEY_DIR/node1/staker.crt")

pids=()
stop() {
  echo "stopping ${#pids[@]} nodes"
  kill "${pids[@]}" 2>/dev/null || true
  wait "${pids[@]}" 2>/dev/null || true
}
trap stop INT TERM

for i in 1 2 3 4 5; do
  http_port=$((BASE_PORT + 2 * (i - 1)))
  staking_port=$((http_port + 1))
  mkdir -p "$DATA_DIR/node$i"
  args=(
    --network-id=mainnet
    --data-dir="$DATA_DIR/node$i"
    --http-host=127.0.0.1
    --http-port="$http_port"
    --staking-port="$staking_port"
    --public-ip="$PUBLIC_IP"
    --staking-tls-cert-file="$KEY_DIR/node$i/staker.crt"
    --staking-tls-key-file="$KEY_DIR/node$i/staker.key"
    --log-display-level=warn
  )
  if [[ $i == 1 ]]; then
    args+=(--index-enabled --index-subnet-chains)
  else
    args+=(--bootstrap-ips="127.0.0.1:$((BASE_PORT + 1))" --bootstrap-ids="$NODE1_ID")
  fi
  if [[ -n "$WHITELISTED_SUBNETS" ]]; then
    args+=(--whitelisted-subnets="$WHITELISTED_SUBNETS")
  fi
  if [[ -n "$CHAIN_CONFIG_DIR" ]]; then
    args+=(--chain-config-dir="$CHAIN_CONFIG_DIR")
  fi
  "$NODE_BINARY" "${args[@]}" >>"$DATA_DIR/node$i/stdout.log" 2>&1 &
  pids+=($!)
  echo "node$i: pid $! http 127.0.0.1:$http_port staking $PUBLIC_IP:$staking_port"
done

wait "${pids[@]}"

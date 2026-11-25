#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "$0")/../contracts"

work=$(mktemp -d "${TMPDIR:-/tmp}/df-store-wrong-versions.XXXXXX")
pid=""
cleanup() {
  if [[ -n "$pid" ]]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT

NO_DNA=1 anchor build -p data_feeds_store

fixtures="$work/fixtures"
ledger="$work/ledger"
log="$work/validator.log"
rpc_port="${RPC_PORT:-18999}"
faucet_port="${FAUCET_PORT:-19001}"
gossip_port="${GOSSIP_PORT:-19002}"
account_args=()
while IFS=' ' read -r address file; do
  account_args+=(--account "$address" "$file")
done < <(node tests/data-feeds-store-wrong-versions.cjs "$fixtures")

solana-test-validator \
  --reset \
  --ledger "$ledger" \
  --rpc-port "$rpc_port" \
  --faucet-port "$faucet_port" \
  --gossip-port "$gossip_port" \
  --dynamic-port-range 19100-19200 \
  --mint "$(solana-keygen pubkey id.json)" \
  --upgradeable-program 9qCsUGTdKCanybkroJuPzAhuRU8P2SZN4NCexevSZAwu \
    target/deploy/data_feeds_store.so id.json \
  "${account_args[@]}" \
  > "$log" 2>&1 &
pid=$!

for _ in {1..60}; do
  if ! kill -0 "$pid" 2>/dev/null; then
    cat "$log"
    exit 1
  fi
  if curl -sS --max-time 2 "http://127.0.0.1:${rpc_port}" \
    -H 'Content-Type: application/json' \
    --data '{"jsonrpc":"2.0","id":1,"method":"getHealth"}' \
    2>/dev/null | grep -q '"result":"ok"'; then
    break
  fi
  sleep 1
done

if ! kill -0 "$pid" 2>/dev/null; then
  cat "$log"
  exit 1
fi
if ! curl -sS --max-time 2 "http://127.0.0.1:${rpc_port}" \
  -H 'Content-Type: application/json' \
  --data '{"jsonrpc":"2.0","id":1,"method":"getHealth"}' \
  2>/dev/null | grep -q '"result":"ok"'; then
  cat "$log"
  exit 1
fi

ANCHOR_PROVIDER_URL="http://127.0.0.1:${rpc_port}" \
  ANCHOR_WALLET=id.json \
  pnpm exec ts-mocha -t 1000000 tests/data-feeds-store-wrong-versions.validator.ts

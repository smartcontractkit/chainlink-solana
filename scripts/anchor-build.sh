#!/usr/bin/env bash

# get this scripts directory
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)

CONTRACTS="${SCRIPT_DIR}"/../contracts

cd "${SCRIPT_DIR}"/../ || exit 1
./scripts/programs-keys-gen.sh

cd "${CONTRACTS}" || exit 1
anchor build
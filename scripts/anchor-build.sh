#!/usr/bin/env bash

# get this scripts directory
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

CONTRACTS=${SCRIPT_DIR}/../contracts

cd ${SCRIPT_DIR}/../
./scripts/programs-keys-gen.sh

cd ${CONTRACTS}
anchor build
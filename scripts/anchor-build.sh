#!/bin/bash
set -e

# get this scripts directory
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

CONTRACTS=${SCRIPT_DIR}/../contracts

cd ${SCRIPT_DIR}/../
# ./scripts/programs-keys-gen.sh
cd ${CONTRACTS}

# Skip IDL build entirely: keystone-forwarder's Accounts (seeds referencing state/don_id/etc.)
# fail in IDL context with "cannot find value" (Anchor 0.31 + Rust 1.79). Use checked-in IDLs.
mkdir -p target/idl
anchor build --no-idl
# Populate target/idl from checked-in idl/ so cp_gauntlet_idl and other steps have IDLs.
cp -f idl/*.json target/idl/ 2>/dev/null || true

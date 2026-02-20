#!/bin/bash
set -e

# get this scripts directory
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

CONTRACTS=${SCRIPT_DIR}/../contracts

cd ${SCRIPT_DIR}/../
# ./scripts/programs-keys-gen.sh
cd ${CONTRACTS}

# Build with IDL so target/idl gets 0.31 format (writable/signer). resolution = false in
# Anchor.toml lets keystone-forwarder's IDL build succeed despite seeds from instruction data.
mkdir -p target/idl
# Pin serde_json to 1.0.140 — last version using ryu+itoa.
# 1.0.141+ pulls in zmij whose build.rs can't detect Solana's non-standard rustc
# version string (falls back to assuming latest rustc and emits select_unpredictable
# which requires Rust 1.88+, unavailable in the Solana build image).
cargo update -p serde_json --precise 1.0.140

anchor build

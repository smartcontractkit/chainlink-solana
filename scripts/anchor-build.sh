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
mkdir -p "${SCRIPT_DIR}/../.build-tmp"
# Remove blake3 from crates.io index cache; image has truncated entry missing 1.8.2.
find ~/.cargo/registry/index -name blake3 -path '*/bl/ak/*' -delete 2>/dev/null || true
# Pin serde_json to 1.0.140 — last version using ryu+itoa.
# 1.0.141+ pulls in zmij whose build.rs can't detect Solana's non-standard rustc
# version string (falls back to assuming latest rustc and emits select_unpredictable
# which requires Rust 1.88+, unavailable in the Solana build image).
cargo update -p serde_json --precise 1.0.140
# Force fresh blake3 metadata fetch; Docker image's crates.io index may be stale.
cargo update -p blake3 --precise 1.8.2

anchor build

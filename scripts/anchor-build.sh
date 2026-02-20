#!/bin/bash
set -e

# get this scripts directory
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

CONTRACTS=${SCRIPT_DIR}/../contracts

cd ${SCRIPT_DIR}/../
# ./scripts/programs-keys-gen.sh
cd ${CONTRACTS}

# Use image default SBF toolchain. Force Cargo.lock to avoid edition2024 deps (constant_time_eq, blake3).
awk '
  /^\[\[patch\.unused\]\]$/ { skip=1; next }
  skip && /^name = |^version = / { next }
  skip && /^$/ { skip=0; next }
  skip { next }
  /^name = "constant_time_eq"$/ { in_cte=1; print; next }
  in_cte && /^version = / { print "version = \"0.4.2\""; next }
  in_cte && /^source = "registry/ { print "source = \"path+file:///workdir/contracts/vendor/constant_time_eq\""; next }
  in_cte && /^checksum = / { next }
  in_cte && /^$/ { in_cte=0; print; next }
  /^name = "zmij"$/ { in_zmij=1; print; next }
  in_zmij && /^source = "registry/ { print "source = \"path+file:///workdir/contracts/vendor/zmij\""; next }
  in_zmij && /^checksum = / { next }
  in_zmij && /^$/ { in_zmij=0; print; next }
  /^name = "blake3"$/ { if (in_blake3) next; in_blake3=1; print; next }
  in_blake3 && /^version = "1.8.3"$/ { print "version = \"1.8.2\""; next }
  in_blake3 && /^checksum = "2468ef7d57b3fb7e16b576e8377cdbde2320c60e1491e961d11da40fc4f02a2d"$/ { print "checksum = \"3888aaa89e4b2a40fca9848e400f6a658a5a3978de7be858e209cafa8be9a4a0\""; next }
  in_blake3 && /^$/ { in_blake3=0; print; next }
  { print }
' Cargo.lock > Cargo.lock.tmp && mv Cargo.lock.tmp Cargo.lock
CARGO_REGISTRY="${CARGO_HOME:-$HOME/.cargo}/registry/src"
for dir in "${CARGO_REGISTRY}"/*/; do
  rm -rf "${dir}constant_time_eq-0.4.2" "${dir}blake3-1.8.3" "${dir}zmij-1.0.7"
done 2>/dev/null || true

# Skip IDL build entirely: keystone-forwarder's Accounts (seeds referencing state/don_id/etc.)
# fail in IDL context with "cannot find value" (Anchor 0.31 + Rust 1.79). Use checked-in IDLs.
mkdir -p target/idl
anchor build --no-idl
# Populate target/idl from checked-in idl/ so cp_gauntlet_idl and other steps have IDLs.
cp -f idl/*.json target/idl/ 2>/dev/null || true

#!/bin/bash

# Solana Nix Package SHA256 Hash Updater
#
# This script updates the SHA256 hashes for Solana binaries in the solana.nix file.
#
# Usage: ./update_nix_hashes.sh <version>

set -e

if [ $# -eq 0 ]; then
  echo "Error: No version provided"
  echo "Usage: $0 <version>"
  exit 1
fi

VERSION=$1

echo "Updating SHA256 hashes for Nix packages (Solana version: $VERSION)"

get_nix_hash() {
  local url=$1
  nix hash convert --hash-algo sha256 $(nix-prefetch-url --unpack $url)
}

linux_hash=$(get_nix_hash https://github.com/anza-xyz/agave/releases/download/${VERSION}/solana-release-x86_64-unknown-linux-gnu.tar.bz2)
darwin_hash=$(get_nix_hash https://github.com/anza-xyz/agave/releases/download/${VERSION}/solana-release-aarch64-apple-darwin.tar.bz2)

echo "Linux Hash: $linux_hash"
echo "Darwin Hash: $darwin_hash"

LINUX_START_MARKER="### BEGIN_LINUX_SHA256 ###"
LINUX_END_MARKER="### END_LINUX_SHA256 ###"
DARWIN_START_MARKER="### BEGIN_DARWIN_SHA256 ###"
DARWIN_END_MARKER="### END_DARWIN_SHA256 ###"

if [ "$(uname -s)" = "Darwin" ]; then
  sed_in_place=(-i '')
else
  sed_in_place=(-i)
fi

sed "${sed_in_place[@]}" \
  -e "/$LINUX_START_MARKER/,/$LINUX_END_MARKER/ s|sha256 = \"[^\"]*\"|sha256 = \"$linux_hash\"|" \
  solana.nix

sed "${sed_in_place[@]}" \
  -e "/$DARWIN_START_MARKER/,/$DARWIN_END_MARKER/ s|sha256 = \"[^\"]*\"|sha256 = \"$darwin_hash\"|" \
  solana.nix

echo "Updated hashes in solana.nix"
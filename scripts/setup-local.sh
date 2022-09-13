#!/usr/bin/env bash

set -euxo pipefail
source "${BASH_SOURCE%/*}/lib.sh"

ACCESS_CONTROLLER_PROGRAM_ID=$(<"${BASH_SOURCE%/*}/../contracts/artifacts/localnet/access_controller-keypair.pub")
OCR2_PROGRAM_ID=$(<"${BASH_SOURCE%/*}/../contracts/artifacts/localnet/ocr2-keypair.pub")
STORE_PROGRAM_ID=$(<"${BASH_SOURCE%/*}/../contracts/artifacts/localnet/store-keypair.pub")

modify_program "$ACCESS_CONTROLLER_PROGRAM_ID" "$OCR2_PROGRAM_ID" "$STORE_PROGRAM_ID"

# build artifacts
build "${PWD%/}"

# copy build artifacts
mkdir -p "${BASH_SOURCE%/*}/../gauntlet/packages/gauntlet-solana-contracts/artifacts/bin"
echo "$PWD"
cp "${BASH_SOURCE%/*}/../contracts/target/deploy/*.so" "${BASH_SOURCE%/*}/../gauntlet/packages/gauntlet-solana-contracts/artifacts/bin"

# copy keypairs
mkdir -p "${BASH_SOURCE%/*}/../gauntlet/packages/gauntlet-solana-contracts/artifacts/programId"
programs=("access_controller" "store" "ocr2")
for t in "${programs[@]}"; do
    cp "${BASH_SOURCE%/*}/../contracts/artifacts/localnet/$t-keypair.json" "${BASH_SOURCE%/*}/../gauntlet/packages/gauntlet-solana-contracts/artifacts/programId/$t.json"
done

#!/usr/bin/env bash

set -e
source ./scripts/lib.sh

ACCESS_CONTROLLER_PROGRAM_ID=$(<./contracts/artifacts/localnet/access_controller-keypair.pub)
OCR2_PROGRAM_ID=$(<./contracts/artifacts/localnet/ocr2-keypair.pub)
STORE_PROGRAM_ID=$(<./contracts/artifacts/localnet/store-keypair.pub)

modify_program $ACCESS_CONTROLLER_PROGRAM_ID $OCR2_PROGRAM_ID $STORE_PROGRAM_ID

# build artifacts
build

# copy build artifacts
mkdir -p ./gauntlet/packages/gauntlet-solana-contracts/artifacts/bin
cp ./contracts/target/deploy/*.so ./gauntlet/packages/gauntlet-solana-contracts/artifacts/bin

# copy keypairs
mkdir -p ./gauntlet/packages/gauntlet-solana-contracts/artifacts/programId
programs=("access_controller" "store" "ocr2")
for t in ${programs[@]}; do
  cp "./contracts/artifacts/localnet/$t-keypair.json" ./gauntlet/packages/gauntlet-solana-contracts/artifacts/programId/$t.json
done

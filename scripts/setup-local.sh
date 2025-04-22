#!/usr/bin/env bash

set -euxo pipefail
source "${BASH_SOURCE%/*}/lib.sh"

ACCESS_CONTROLLER_PROGRAM_ID=$(<"${BASH_SOURCE%/*}/../contracts/artifacts/localnet/access_controller-keypair.pub")
OCR2_PROGRAM_ID=$(<"${BASH_SOURCE%/*}/../contracts/artifacts/localnet/ocr_2-keypair.pub")
STORE_PROGRAM_ID=$(<"${BASH_SOURCE%/*}/../contracts/artifacts/localnet/store-keypair.pub")
KEYSTONE_FORWARDER_PROGRAM_ID=$(<"${BASH_SOURCE%/*}/../contracts/artifacts/localnet/keystone_forwarder-keypair.pub")
DUMMY_RECEIVER_PROGRAM_ID=$(<"${BASH_SOURCE%/*}/../contracts/artifacts/localnet/dummy_receiver-keypair.pub")

modify_program $ACCESS_CONTROLLER_PROGRAM_ID $OCR2_PROGRAM_ID $STORE_PROGRAM_ID $KEYSTONE_FORWARDER_PROGRAM_ID $DUMMY_RECEIVER_PROGRAM_ID

# build artifacts
build ${PWD%/}

# copy build artifacts
mkdir -p "${BASH_SOURCE%/*}/../gauntlet/packages/gauntlet-solana-contracts/artifacts/bin"
echo $PWD
cp ${BASH_SOURCE%/*}/../contracts/target/deploy/*.so "${BASH_SOURCE%/*}/../gauntlet/packages/gauntlet-solana-contracts/artifacts/bin"

# copy keypairs
mkdir -p "${BASH_SOURCE%/*}/../gauntlet/packages/gauntlet-solana-contracts/artifacts/programId"
programs=("access_controller" "store" "ocr_2" "keystone_forwarder" "dummy_receiver")
for t in ${programs[@]}; do
  cp "${BASH_SOURCE%/*}/../contracts/artifacts/localnet/$t-keypair.json" "${BASH_SOURCE%/*}/../gauntlet/packages/gauntlet-solana-contracts/artifacts/programId/$t.json"
done

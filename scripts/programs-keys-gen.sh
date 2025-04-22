#!/usr/bin/env bash

# Generate a new set of keys to use for testing. Primarily used for e2e testing on CI 
# assumes that test program ids and keypairs have been generated via solana-keygen and placed in the 
# associated network folder

set -euxo pipefail
source "${BASH_SOURCE%/*}/lib.sh"

network=${1:-localnet}

# solana-keygen new -o ./contracts/artifacts/$network/access_controller-keypair.json
ACCESS_CONTROLLER_PROGRAM_ID=$(solana-keygen pubkey ./contracts/artifacts/$network/access_controller-keypair.json)
echo $ACCESS_CONTROLLER_PROGRAM_ID > ./contracts/artifacts/$network/access_controller-keypair.pub

# solana-keygen new -o ./contracts/artifacts/$network/store-keypair.json
STORE_PROGRAM_ID=$(solana-keygen pubkey ./contracts/artifacts/$network/store-keypair.json)
echo $STORE_PROGRAM_ID > ./contracts/artifacts/$network/store-keypair.pub

# solana-keygen new -o ./contracts/artifacts/$network/ocr2-keypair.json
OCR2_PROGRAM_ID=$(solana-keygen pubkey ./contracts/artifacts/$network/ocr_2-keypair.json)
echo $OCR2_PROGRAM_ID > ./contracts/artifacts/$network/ocr_2-keypair.pub

# solana-keygen new -o ./contracts/artifacts/$network/keystone_forwarder-keypair.json
KEYSTONE_FORWARDER_PROGRAM_ID=$(solana-keygen pubkey ./contracts/artifacts/$network/keystone_forwarder-keypair.json)
echo $KEYSTONE_FORWARDER_PROGRAM_ID > ./contracts/artifacts/$network/keystone_forwarder-keypair.pub

# solana-keygen new -o ./contracts/artifacts/$network/dummy_receiver-keypair.json
DUMMY_RECEIVER_PROGRAM_ID=$(solana-keygen pubkey ./contracts/artifacts/$network/dummy_receiver-keypair.json)
echo $DUMMY_RECEIVER_PROGRAM_ID > ./contracts/artifacts/$network/dummy_receiver-keypair.pub

mkdir -p ./contracts/target/deploy
cp ./contracts/artifacts/$network/*.json ./contracts/target/deploy

# Replace existing declare_id!()
modify_program $ACCESS_CONTROLLER_PROGRAM_ID $OCR2_PROGRAM_ID $STORE_PROGRAM_ID $KEYSTONE_FORWARDER_PROGRAM_ID $DUMMY_RECEIVER_PROGRAM_ID

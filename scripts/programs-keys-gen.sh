#!/usr/bin/env bash

# Generate a new set of keys to use for testing. Primarily used for e2e testing on CI

set -euxo pipefail

network=${1:-localnet}

# solana-keygen new -o ./contracts/artifacts/$network/access_controller-keypair.json
ACCESS_CONTROLLER_PROGRAM_ID=$(solana-keygen pubkey ./contracts/artifacts/$network/access_controller-keypair.json)
echo $ACCESS_CONTROLLER_PROGRAM_ID > ./contracts/artifacts/$network/access_controller-keypair.pub

# solana-keygen new -o ./contracts/artifacts/$network/store-keypair.json
STORE_PROGRAM_ID=$(solana-keygen pubkey ./contracts/artifacts/$network/store-keypair.json)
echo $STORE_PROGRAM_ID > ./contracts/artifacts/$network/store-keypair.pub

# solana-keygen new -o ./contracts/artifacts/$network/ocr2-keypair.json
OCR2_PROGRAM_ID=$(solana-keygen pubkey ./contracts/artifacts/$network/ocr2-keypair.json)
echo $OCR2_PROGRAM_ID > ./contracts/artifacts/$network/ocr2-keypair.pub

mkdir -p ./contracts/target/deploy
cp ./contracts/artifacts/$network/*.json ./contracts/target/deploy

# Replace existing declare_id!()
sed -i.bak "s/9xi644bRR8birboDGdTiwBq3C7VEeR7VuamRYYXCubUW/$ACCESS_CONTROLLER_PROGRAM_ID/" contracts/programs/access-controller/src/lib.rs
sed -i.bak "s/HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny/$STORE_PROGRAM_ID/" contracts/programs/store/src/lib.rs
sed -i.bak "s/cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ/$OCR2_PROGRAM_ID/" contracts/programs/ocr2/src/lib.rs
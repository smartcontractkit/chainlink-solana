#!/usr/bin/env bash

set -e

ACCESS_CONTROLLER_PROGRAM_ID=$(<./contracts/artifacts/localnet/access_controller-keypair.pub)
STORE_PROGRAM_ID=$(<./contracts/artifacts/localnet/store-keypair.pub)
OCR2_PROGRAM_ID=$(<./contracts/artifacts/localnet/ocr2-keypair.pub)

# Replace existing declare_id!()
sed -i "s/DzzjdPWNfwHZmzPVxnmqkkMJraYQQRCpgFZajqkqmU6G/$ACCESS_CONTROLLER_PROGRAM_ID/" contracts/programs/access-controller/src/lib.rs
sed -i "s/CaH12fwNTKJAG8PxEvo9R96Zc2j8qNHZaFj8ZW49yZNT/$STORE_PROGRAM_ID/" contracts/programs/store/src/lib.rs
sed -i "s/HW3ipKzeeduJq6f1NqRCw4doknMeWkfrM4WxobtG3o5v/$OCR2_PROGRAM_ID/" contracts/programs/ocr2/src/lib.rs

# build artifacts
cd contracts
anchor build
cd ..

# copy build artifacts
mkdir -p ./gauntlet/packages/gauntlet-solana-contracts/artifacts/bin
cp ./contracts/target/deploy/*.so ./gauntlet/packages/gauntlet-solana-contracts/artifacts/bin

# copy keypairs
mkdir -p ./gauntlet/packages/gauntlet-solana-contracts/artifacts/programId
programs=("access_controller" "store" "ocr2")
for t in ${programs[@]}; do
  cp "./contracts/artifacts/localnet/$t-keypair.json" ./gauntlet/packages/gauntlet-solana-contracts/artifacts/programId/$t.json
done

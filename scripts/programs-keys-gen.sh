#!/usr/bin/env bash

set -e

network=${1:-localnet}

solana-keygen new -o ./contracts/artifacts/$network/access_controller-keypair.json
solana-keygen pubkey ./contracts/artifacts/$network/access_controller-keypair.json > ./contracts/artifacts/$network/access_controller-keypair.pub

solana-keygen new -o ./contracts/artifacts/$network/deviation_flagging_validator-keypair.json
solana-keygen pubkey ./contracts/artifacts/$network/deviation_flagging_validator-keypair.json > ./contracts/artifacts/$network/deviation_flagging_validator-keypair.pub

solana-keygen new -o ./contracts/artifacts/$network/ocr2-keypair.json
solana-keygen pubkey ./contracts/artifacts/$network/ocr2-keypair.json > ./contracts/artifacts/$network/ocr2-keypair.pub

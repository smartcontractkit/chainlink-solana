#!/usr/bin/env bash
set -euxo pipefail

CLUSTER=localnet
# KEYPAIR_FILE=$HOME/.config/solana/id.json
CLUSTER_URL=http://127.0.0.1:8899
# PUBKEY=$(solana-keygen KEYPAIR_FILE)
PUBKEY=E3j24rx12SyVsG6quKuZPbQqZPkhAUCh8Uek4XrKYD2x
# export PRIVATE_KEY=[9,218,36,113,218,176,180,196,27,75,171,187,105,81,84,58,52,79,85,169,125,13,0,102,214,246,82,252,133,222,160,252,193,218,154,28,253,34,136,185,53,68,165,141,248,188,247,143,17,100,91,130,75,49,212,131,37,18,151,175,201,153,131,185]

solana-test-validator --bpf-program HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny contracts/target/deploy/store.so --bpf-program 9xi644bRR8birboDGdTiwBq3C7VEeR7VuamRYYXCubUW contracts/target/deploy/access_controller.so --bpf-program cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ contracts/target/deploy/ocr2.so &
VALIDATOR_PID=$!

sleep 3

solana airdrop 100 $PUBKEY -u localhost

wait # block on solana-test-validator
#!/usr/bin/env bash
set -euxo pipefail

CLUSTER=localnet
KEYPAIR_FILE=$HOME/.config/solana/id.json
# KEYPAIR_FILE=migrate/id.json
#CLUSTER_URL=http://localhost:8899
CLUSTER_URL=http://127.0.0.1:8899
PROGRAM_ID=$(solana-keygen pubkey migrate/store-keypair.json)

# Prerequisites:
# anchor build inside contracts/examples
# generate a new store-keypair.json and anchor build on latest develop with that program_id inserted. Copy to migrate/store.so
# anchor build on this branch with this keypair's program_id inserted

#
# Assumes the current working directory is contracts/ dir.
#
main() {
    set +e
    #
    # Build the program.
    #
    # anchor build
    #
    # Start the local validator. Use the old store.so built from e3a643e31b90947d482a8cb457d97f1e5f427780
    #
    solana-test-validator -l migrate/test-ledger --bpf-program $PROGRAM_ID migrate/store.so --bpf-program 2F5NEkMnCRkmahEAcQfTQcZv1xtGgrWFfjENtTwHLuKg target/deploy/access_controller.so --bpf-program Fg6PaFpoGXkYsidMpWTK6W2BeZ7FEfcYkg476zPFsLnS examples/hello-world/target/deploy/hello_world.so > migrate/validator.log &
    #
    # Wait for the validator to start.
    #
    sleep 5
    #
    # Create a keypair for the tests.
    #
    yes | solana-keygen new --outfile $KEYPAIR_FILE
    #
    # Fund the keypair.
    #
    yes | solana airdrop --url $CLUSTER_URL 100
    set -e
    #
    # Run the migration script.
    #
    solana program deploy --url $CLUSTER_URL --program-id migrate/store-keypair.json migrate/store.so
    # Upgrade authority already defaults to the keypair
    # solana program set-upgrade-authority --new-upgrade-authority $PROT $OTHER_PROGRAM
    BUFFER=$(solana program write-buffer target/deploy/store.so --url $CLUSTER_URL | cut -d ' ' -f2)
    # solana program set-buffer-authority --new-buffer-authority $PROT $BUFFER

    CHAINLINK_PROGRAM_ID="$PROGRAM_ID" ANCHOR_PROVIDER_URL="$CLUSTER_URL" ANCHOR_WALLET="$KEYPAIR_FILE" BUFFER="$BUFFER" node migrate/deploy.js
}

main
#!/usr/bin/env bash

set -euxo pipefail
source "${BASH_SOURCE%/*}/lib.sh"
source "${BASH_SOURCE%/*}/../gauntlet/packages/gauntlet-solana-contracts/networks/.env.staging"

modify_program $PROGRAM_ID_ACCESS_CONTROLLER $PROGRAM_ID_OCR2 $PROGRAM_ID_STORE

# build artifacts
build ${PWD%/}
